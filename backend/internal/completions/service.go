package completions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/family-habit/family-habit/backend/internal/habits"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrForbidden       = errors.New("forbidden")
	ErrNotFound        = errors.New("not found")
	ErrInvalidState    = errors.New("invalid state transition")
	ErrVersionConflict = errors.New("version conflict")
	ErrIdempotency     = errors.New("idempotency key conflict")
	ErrFuture          = errors.New("future occurrence is not actionable")
)

type TransitionError struct {
	Kind    error
	Status  string
	Version int64
}

func (e *TransitionError) Error() string { return e.Kind.Error() }
func (e *TransitionError) Unwrap() error { return e.Kind }

type Service struct {
	pool   *pgxpool.Pool
	habits *habits.Service
	now    func() time.Time
	fault  func(stage string) error
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, habits: habits.NewService(pool), now: time.Now}
}

type Today struct {
	ChildID, Date, Timezone string
	Occurrences             []Occurrence
}

type Occurrence struct {
	ID, ChildID, Type, LocalDate, DueDate, Title, Description, Icon, Color string
	Points, Version                                                        int64
	Status, Group, DueState, CompletionID                                  string
	AvailableActions                                                       []string
	RoutineGroupID, RoutineGroupName, RoutineGroupIcon, RoutineGroupColor  string
	RoutineGroupSortOrder                                                  *int
	ItemSortOrder                                                          int
}

type Completion struct {
	ID, OccurrenceID, ChildID, AttemptStatus, OccurrenceStatus string
	SubmittedAt                                                time.Time
	DecidedAt                                                  *time.Time
	Version, AttemptNumber                                     int64
}

// HouseholdToday returns the current calendar date in the family's IANA zone.
func (s *Service) HouseholdToday(ctx context.Context, familyID string) (string, string, error) {
	var timezone string
	if err := s.pool.QueryRow(ctx, `SELECT timezone FROM families WHERE id=$1`, familyID).Scan(&timezone); err != nil {
		return "", "", err
	}
	date, err := localDateAt(s.now(), timezone)
	return date, timezone, err
}

func localDateAt(now time.Time, timezone string) (string, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "", err
	}
	return now.In(location).Format("2006-01-02"), nil
}

// Today rechecks the persisted session and child in the same household before
// materializing. Parents may inspect household children; child mode is exact-child.
func (s *Service) Today(ctx context.Context, sessionID, familyID, childID, date string) (Today, error) {
	var mode, activeChild, timezone string
	err := s.pool.QueryRow(ctx, `SELECT s.mode::text,coalesce(s.active_child_id::text,''),f.timezone
		FROM sessions s JOIN families f ON f.id=s.family_id JOIN children c ON c.id=$3 AND c.family_id=s.family_id AND c.archived_at IS NULL
		WHERE s.id=$1 AND s.family_id=$2 AND s.revoked_at IS NULL AND s.expires_at>now()`, sessionID, familyID, childID).Scan(&mode, &activeChild, &timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return Today{}, ErrNotFound
	}
	if err != nil {
		return Today{}, err
	}
	if mode != "parent" && (mode != "child" || activeChild != childID) {
		return Today{}, ErrNotFound
	}
	childActor := mode == "child"
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return Today{}, err
	}
	if err = s.habits.EnsureDate(ctx, familyID, parsed); err != nil {
		return Today{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT o.id,o.child_id,o.item_type_snapshot,o.local_date,o.due_date,o.title_snapshot,
		o.description_snapshot,o.icon_snapshot,o.color_snapshot,o.points_snapshot,o.state::text,o.version,
		coalesce(ca.id::text,''),coalesce(o.routine_group_id_snapshot::text,''),coalesce(o.routine_group_name_snapshot,''),coalesce(o.routine_group_icon_snapshot,''),coalesce(o.routine_group_color_snapshot,''),o.routine_group_sort_order_snapshot,o.item_sort_order_snapshot
		FROM occurrences o LEFT JOIN completion_attempts ca ON ca.occurrence_id=o.id AND ca.decision='pending'
		WHERE o.family_id=$1 AND o.child_id=$2 AND (
			(o.local_date=$3) OR
			(o.source_type='task' AND o.local_date<$3 AND o.state IN ('not_started','pending_approval')))
		AND o.state NOT IN ('approval_reversed','cancelled')`, familyID, childID, parsed)
	if err != nil {
		return Today{}, err
	}
	defer rows.Close()
	out := Today{ChildID: childID, Date: date, Timezone: timezone, Occurrences: []Occurrence{}}
	for rows.Next() {
		var o Occurrence
		var localDate time.Time
		var dueDate *time.Time
		if err = rows.Scan(&o.ID, &o.ChildID, &o.Type, &localDate, &dueDate, &o.Title, &o.Description, &o.Icon, &o.Color, &o.Points, &o.Status, &o.Version, &o.CompletionID, &o.RoutineGroupID, &o.RoutineGroupName, &o.RoutineGroupIcon, &o.RoutineGroupColor, &o.RoutineGroupSortOrder, &o.ItemSortOrder); err != nil {
			return Today{}, err
		}
		o.LocalDate = localDate.Format("2006-01-02")
		if dueDate != nil {
			o.DueDate = dueDate.Format("2006-01-02")
		}
		o.DueState = "scheduled_today"
		if localDate.Before(parsed) {
			o.DueState = "overdue"
		}
		switch o.Status {
		case "not_started":
			o.Group = "to_do"
			o.AvailableActions = []string{}
			if childActor {
				o.AvailableActions = []string{"submit"}
			}
		case "pending_approval":
			o.Group = "waiting_for_parent"
			o.AvailableActions = []string{}
			if childActor {
				o.AvailableActions = []string{"withdraw"}
			} else {
				o.CompletionID = ""
			}
		case "approved":
			o.Group = "done"
			o.AvailableActions = []string{}
		default:
			o.AvailableActions = []string{}
		}
		out.Occurrences = append(out.Occurrences, o)
	}
	if err = rows.Err(); err != nil {
		return Today{}, err
	}
	groupRank := map[string]int{"to_do": 0, "waiting_for_parent": 1, "done": 2}
	sort.Slice(out.Occurrences, func(i, j int) bool {
		a, b := out.Occurrences[i], out.Occurrences[j]
		if (a.RoutineGroupSortOrder == nil) != (b.RoutineGroupSortOrder == nil) {
			return a.RoutineGroupSortOrder != nil
		}
		if a.RoutineGroupSortOrder != nil && *a.RoutineGroupSortOrder != *b.RoutineGroupSortOrder {
			return *a.RoutineGroupSortOrder < *b.RoutineGroupSortOrder
		}
		if a.ItemSortOrder != b.ItemSortOrder {
			return a.ItemSortOrder < b.ItemSortOrder
		}
		if groupRank[a.Group] != groupRank[b.Group] {
			return groupRank[a.Group] < groupRank[b.Group]
		}
		if (a.DueState == "overdue") != (b.DueState == "overdue") {
			return a.DueState == "overdue"
		}
		if a.Type != b.Type {
			return a.Type == "habit"
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.ID < b.ID
	})
	return out, nil
}

func reserve(ctx context.Context, tx pgx.Tx, familyID, sessionID, route, key string, hash []byte) ([]byte, bool, error) {
	tag, err := tx.Exec(ctx, `INSERT INTO idempotency_records(family_id,session_id,route_family,idempotency_key,request_hash,expires_at)
		VALUES($1,$2,$3,$4,$5,now()+interval '24 hours') ON CONFLICT DO NOTHING`, familyID, sessionID, route, key, hash)
	if err != nil {
		return nil, false, err
	}
	var oldHash, body []byte
	if err = tx.QueryRow(ctx, `SELECT request_hash,response_body FROM idempotency_records
		WHERE family_id=$1 AND session_id=$2 AND route_family=$3 AND idempotency_key=$4 FOR UPDATE`, familyID, sessionID, route, key).Scan(&oldHash, &body); err != nil {
		return nil, false, err
	}
	if !bytes.Equal(oldHash, hash) {
		return nil, false, ErrIdempotency
	}
	return body, tag.RowsAffected() == 0 && len(body) > 0, nil
}

func finalize(ctx context.Context, tx pgx.Tx, familyID, sessionID, route, key string, status int, value Completion) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE idempotency_records SET response_status=$5,response_body=$6
		WHERE family_id=$1 AND session_id=$2 AND route_family=$3 AND idempotency_key=$4`, familyID, sessionID, route, key, status, body)
	return err
}

func decodedCompletion(body []byte) (Completion, error) {
	var c Completion
	err := json.Unmarshal(body, &c)
	return c, err
}

// lockChildActor follows the child-before-session lock order used by archival
// and profile entry, preventing submit/archive deadlocks.
func lockChildActor(ctx context.Context, tx pgx.Tx, sessionID, familyID, childID string) error {
	var lockedChild string
	err := tx.QueryRow(ctx, `SELECT id::text FROM children WHERE id=$1 AND family_id=$2 AND archived_at IS NULL FOR UPDATE`, childID, familyID).Scan(&lockedChild)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	var active string
	err = tx.QueryRow(ctx, `SELECT active_child_id::text FROM sessions WHERE id=$1 AND family_id=$2 AND mode='child' AND active_child_id=$3 AND revoked_at IS NULL AND expires_at>now() FOR UPDATE`, sessionID, familyID, childID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrForbidden
	}
	return err
}

// Submit atomically creates the next numbered attempt and moves the occurrence
// to pending approval. Persisted session authority is rechecked under row lock.
func (s *Service) Submit(ctx context.Context, sessionID, familyID, childID, occurrenceID, key string, hash []byte, expectedVersion int64) (Completion, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Completion{}, false, err
	}
	defer tx.Rollback(ctx)
	if err = lockChildActor(ctx, tx, sessionID, familyID, childID); err != nil {
		return Completion{}, false, err
	}
	body, replay, err := reserve(ctx, tx, familyID, sessionID, "completion.submit", key, hash)
	if err != nil {
		return Completion{}, false, err
	}
	if replay {
		out, decodeErr := decodedCompletion(body)
		if decodeErr != nil {
			return Completion{}, false, decodeErr
		}
		if out.ChildID != childID {
			return Completion{}, false, ErrNotFound
		}
		return out, true, tx.Commit(ctx)
	}
	var state, occurrenceChild, timezone, sourceType string
	var sourceActive bool
	var localDate time.Time
	var version int64
	err = tx.QueryRow(ctx, `SELECT o.child_id::text,o.local_date,o.state::text,o.version,f.timezone,o.source_type,
		CASE WHEN o.source_type='task' THEN EXISTS(SELECT 1 FROM one_off_tasks t WHERE t.id=o.task_id AND t.family_id=o.family_id AND t.child_id=o.child_id AND t.state='active')
		ELSE EXISTS(SELECT 1 FROM habit_assignments a JOIN habits h ON h.id=a.habit_id AND h.family_id=a.family_id
			WHERE a.id=o.assignment_id AND a.family_id=o.family_id AND a.child_id=o.child_id
			AND a.effective_from<=o.local_date AND (a.effective_until IS NULL OR a.effective_until>=o.local_date)
			AND (h.inactive_from IS NULL OR h.inactive_from>o.local_date)) END
		FROM occurrences o JOIN families f ON f.id=o.family_id
		WHERE o.id=$1 AND o.family_id=$2 FOR UPDATE OF o`, occurrenceID, familyID).Scan(&occurrenceChild, &localDate, &state, &version, &timezone, &sourceType, &sourceActive)
	if errors.Is(err, pgx.ErrNoRows) || occurrenceChild != childID {
		return Completion{}, false, ErrNotFound
	}
	if err != nil {
		return Completion{}, false, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return Completion{}, false, err
	}
	now := s.now().In(location)
	currentDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if !sourceActive || localDate.After(currentDate) || (sourceType == "habit" && !localDate.Equal(currentDate)) {
		return Completion{}, false, ErrFuture
	}
	if version != expectedVersion {
		return Completion{}, false, &TransitionError{Kind: ErrVersionConflict, Status: state, Version: version}
	}
	if state != "not_started" {
		return Completion{}, false, &TransitionError{Kind: ErrInvalidState, Status: state, Version: version}
	}
	var out Completion
	err = tx.QueryRow(ctx, `INSERT INTO completion_attempts(family_id,occurrence_id,child_id,attempt_number)
		SELECT $1,$2,$3,coalesce(max(attempt_number),0)+1 FROM completion_attempts WHERE occurrence_id=$2
		RETURNING id,occurrence_id,child_id,decision::text,submitted_at,attempt_number`, familyID, occurrenceID, childID).Scan(&out.ID, &out.OccurrenceID, &out.ChildID, &out.AttemptStatus, &out.SubmittedAt, &out.AttemptNumber)
	if err != nil {
		return Completion{}, false, err
	}
	if s.fault != nil {
		if err = s.fault("attempt_inserted"); err != nil {
			return Completion{}, false, err
		}
	}
	out.OccurrenceStatus = "pending_approval"
	err = tx.QueryRow(ctx, `UPDATE occurrences SET state='pending_approval',version=version+1,updated_at=now() WHERE id=$1 RETURNING version`, occurrenceID).Scan(&out.Version)
	if err == nil {
		if s.fault != nil {
			err = s.fault("occurrence_updated")
		}
	}
	if err == nil {
		if s.fault != nil {
			err = s.fault("before_audit")
		}
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_child_id,session_id,action,subject_type,subject_id,before_status,after_status,idempotency_key,metadata)
			VALUES($1,$2,$3,'completion.submitted','occurrence',$4,'not_started','pending_approval',$5,jsonb_build_object('completionId',$6::text,'attemptNumber',$7::bigint))`, familyID, childID, sessionID, out.OccurrenceID, key, out.ID, out.AttemptNumber)
	}
	if err == nil {
		if s.fault != nil {
			err = s.fault("before_idempotency_finalize")
		}
	}
	if err == nil {
		err = finalize(ctx, tx, familyID, sessionID, "completion.submit", key, 201, out)
	}
	if err != nil {
		return Completion{}, false, err
	}
	return out, false, tx.Commit(ctx)
}

// Withdraw closes only the currently pending attempt and returns its occurrence
// to not_started. It never deletes attempts or affects the point ledger.
func (s *Service) Withdraw(ctx context.Context, sessionID, familyID, childID, completionID, key string, hash []byte, expectedVersion int64) (Completion, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Completion{}, false, err
	}
	defer tx.Rollback(ctx)
	if err = lockChildActor(ctx, tx, sessionID, familyID, childID); err != nil {
		return Completion{}, false, err
	}
	body, replay, err := reserve(ctx, tx, familyID, sessionID, "completion.withdraw", key, hash)
	if err != nil {
		return Completion{}, false, err
	}
	if replay {
		out, decodeErr := decodedCompletion(body)
		if decodeErr != nil {
			return Completion{}, false, decodeErr
		}
		if out.ChildID != childID {
			return Completion{}, false, ErrNotFound
		}
		return out, true, tx.Commit(ctx)
	}
	var out Completion
	var occurrenceState string
	err = tx.QueryRow(ctx, `SELECT ca.id,ca.occurrence_id,ca.child_id,ca.decision::text,ca.submitted_at,o.state::text,o.version,ca.attempt_number
		FROM completion_attempts ca JOIN occurrences o ON o.id=ca.occurrence_id AND o.family_id=ca.family_id AND o.child_id=ca.child_id
		WHERE ca.id=$1 AND ca.family_id=$2 FOR UPDATE OF o,ca`, completionID, familyID).Scan(&out.ID, &out.OccurrenceID, &out.ChildID, &out.AttemptStatus, &out.SubmittedAt, &occurrenceState, &out.Version, &out.AttemptNumber)
	if errors.Is(err, pgx.ErrNoRows) || out.ChildID != childID {
		return Completion{}, false, ErrNotFound
	}
	if err != nil {
		return Completion{}, false, err
	}
	if out.Version != expectedVersion {
		return Completion{}, false, &TransitionError{Kind: ErrVersionConflict, Status: occurrenceState, Version: out.Version}
	}
	if out.AttemptStatus != "pending" || occurrenceState != "pending_approval" {
		return Completion{}, false, &TransitionError{Kind: ErrInvalidState, Status: occurrenceState, Version: out.Version}
	}
	err = tx.QueryRow(ctx, `UPDATE completion_attempts SET decision='withdrawn',decided_at=now() WHERE id=$1 RETURNING decision::text,decided_at`, completionID).Scan(&out.AttemptStatus, &out.DecidedAt)
	if err == nil && s.fault != nil {
		err = s.fault("attempt_finalized")
	}
	if err == nil {
		err = tx.QueryRow(ctx, `UPDATE occurrences SET state='not_started',version=version+1,updated_at=now() WHERE id=$1 RETURNING version`, out.OccurrenceID).Scan(&out.Version)
	}
	out.OccurrenceStatus = "not_started"
	if err == nil {
		if s.fault != nil {
			err = s.fault("occurrence_updated")
		}
	}
	if err == nil {
		if s.fault != nil {
			err = s.fault("before_audit")
		}
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_child_id,session_id,action,subject_type,subject_id,before_status,after_status,idempotency_key,metadata)
			VALUES($1,$2,$3,'completion.withdrawn','occurrence',$4,'pending_approval','not_started',$5,jsonb_build_object('completionId',$6::text,'attemptNumber',$7::bigint))`, familyID, childID, sessionID, out.OccurrenceID, key, out.ID, out.AttemptNumber)
	}
	if err == nil {
		if s.fault != nil {
			err = s.fault("before_idempotency_finalize")
		}
	}
	if err == nil {
		err = finalize(ctx, tx, familyID, sessionID, "completion.withdraw", key, 200, out)
	}
	if err != nil {
		return Completion{}, false, err
	}
	return out, false, tx.Commit(ctx)
}
