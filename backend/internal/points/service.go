package points

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrForbidden             = errors.New("forbidden")
	ErrNotFound              = errors.New("not found")
	ErrInvalidState          = errors.New("invalid state transition")
	ErrVersionConflict       = errors.New("version conflict")
	ErrIdempotency           = errors.New("idempotency conflict")
	ErrValidation            = errors.New("validation")
	ErrInsufficientAvailable = errors.New("insufficient available points")
)

type TransitionError struct {
	Kind    error
	Status  string
	Version int64
}

func (e *TransitionError) Error() string { return e.Kind.Error() }
func (e *TransitionError) Unwrap() error { return e.Kind }

type Service struct {
	pool  *pgxpool.Pool
	now   func() time.Time
	fault func(string) error
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool, now: time.Now} }

type Completion struct {
	ID, OccurrenceID, ChildID, ChildName, Title, ItemType string
	LedgerEntryID                                         string
	ChildAvatar, ChildColor, LocalDate, DueDate           string
	AttemptStatus, OccurrenceStatus, Reason               string
	Points, Version, AttemptNumber                        int64
	LedgerAmount                                          int64
	SubmittedAt                                           time.Time
	DecidedAt                                             *time.Time
}
type LedgerEntry struct {
	ID, ChildID, Kind, Reason, OccurrenceID, Title, OriginalEntryID string
	DisplayLabel                                                    string
	Amount                                                          int64
	CreatedAt                                                       time.Time
}
type HistoryEntry struct {
	ID            string           `json:"id"`
	Type          string           `json:"type"`
	Date          string           `json:"localDate"`
	Title         string           `json:"title"`
	Status        string           `json:"status"`
	Points        int64            `json:"points"`
	Version       int64            `json:"version"`
	SubmittedAt   *time.Time       `json:"submittedAt"`
	DecidedAt     *time.Time       `json:"decidedAt"`
	Decision      string           `json:"decision"`
	DecisionNote  string           `json:"decisionNote"`
	Attempts      []AttemptSummary `json:"attempts"`
	DueDate       string           `json:"dueDate,omitempty"`
	Description   string           `json:"description,omitempty"`
	Icon          string           `json:"icon,omitempty"`
	Color         string           `json:"color,omitempty"`
	AwardDelta    int64            `json:"awardDelta"`
	ReversalDelta int64            `json:"reversalDelta"`
}

type cursorValue struct {
	Kind, Binding, Time, Date, ID string
}

func cursorBinding(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func encodeCursor(v cursorValue) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(raw, kind, binding string) (cursorValue, error) {
	if raw == "" {
		return cursorValue{Kind: kind, Binding: binding}, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return cursorValue{}, ErrValidation
	}
	var v cursorValue
	if json.Unmarshal(b, &v) != nil || v.Kind != kind || v.Binding != binding || v.ID == "" {
		return cursorValue{}, ErrValidation
	}
	return v, nil
}

type AttemptSummary struct {
	ID            string     `json:"id"`
	AttemptNumber int64      `json:"attemptNumber"`
	Status        string     `json:"status"`
	SubmittedAt   time.Time  `json:"submittedAt"`
	DecidedAt     *time.Time `json:"decidedAt"`
	Reason        string     `json:"reason"`
}
type Report struct {
	ChildID           string `json:"childId"`
	Period            string `json:"period"`
	StartDate         string `json:"startDate"`
	EndDate           string `json:"endDate"`
	Timezone          string `json:"timezone"`
	WeekStartsOn      int    `json:"weekStartsOn"`
	Assigned          int64  `json:"assigned"`
	Pending           int64  `json:"pending"`
	Submitted         int64  `json:"submitted"`
	Approved          int64  `json:"approved"`
	Reversed          int64  `json:"reversed"`
	Rejected          int64  `json:"rejected"`
	Incomplete        int64  `json:"incomplete"`
	Cancelled         int64  `json:"cancelled"`
	PointsEarned      int64  `json:"pointsEarned"`
	ManualCorrections int64  `json:"manualCorrections"`
	PointsRedeemed    int64  `json:"pointsRedeemed"`
	PointsRefunded    int64  `json:"pointsRefunded"`
	NetPointsChange   int64  `json:"netPointsChange"`
}

func lockParent(ctx context.Context, tx pgx.Tx, sessionID, familyID string) (string, error) {
	var userID string
	err := tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE id=$1 AND family_id=$2 AND mode='parent' AND revoked_at IS NULL AND expires_at>now() FOR UPDATE`, sessionID, familyID).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	return userID, err
}

func reserve(ctx context.Context, tx pgx.Tx, familyID, sessionID, route, key string, hash []byte) ([]byte, bool, error) {
	tag, err := tx.Exec(ctx, `INSERT INTO idempotency_records(family_id,session_id,route_family,idempotency_key,request_hash,expires_at)
		VALUES($1,$2,$3,$4,$5,now()+interval '24 hours') ON CONFLICT DO NOTHING`, familyID, sessionID, route, key, hash)
	if err != nil {
		return nil, false, err
	}
	var oldHash, body []byte
	if err = tx.QueryRow(ctx, `SELECT request_hash,response_body FROM idempotency_records WHERE family_id=$1 AND session_id=$2 AND route_family=$3 AND idempotency_key=$4 FOR UPDATE`, familyID, sessionID, route, key).Scan(&oldHash, &body); err != nil {
		return nil, false, err
	}
	if !bytes.Equal(oldHash, hash) {
		return nil, false, ErrIdempotency
	}
	return body, tag.RowsAffected() == 0 && len(body) > 0, nil
}
func finish(ctx context.Context, tx pgx.Tx, familyID, sessionID, route, key string, status int, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE idempotency_records SET response_status=$5,response_body=$6 WHERE family_id=$1 AND session_id=$2 AND route_family=$3 AND idempotency_key=$4`, familyID, sessionID, route, key, status, body)
	return err
}

func (s *Service) Pending(ctx context.Context, sessionID, familyID, childID, cursor string, limit int) ([]Completion, string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback(ctx)
	if _, err = lockParent(ctx, tx, sessionID, familyID); err != nil {
		return nil, "", err
	}
	if childID != "" {
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT true FROM children WHERE id=$1 AND family_id=$2`, childID, familyID).Scan(&exists); errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrNotFound
		}
		if err != nil {
			return nil, "", err
		}
	}
	binding := cursorBinding(familyID, childID)
	cur, err := decodeCursor(cursor, "pending", binding)
	if err != nil {
		return nil, "", err
	}
	var cursorTime time.Time
	if cur.Time != "" {
		cursorTime, err = time.Parse(time.RFC3339Nano, cur.Time)
		if err != nil {
			return nil, "", ErrValidation
		}
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, err := tx.Query(ctx, `SELECT ca.id,o.id,o.child_id,c.nickname,c.avatar,c.color,o.title_snapshot,o.item_type_snapshot,o.local_date,o.due_date,o.points_snapshot,ca.decision::text,o.state::text,ca.submitted_at,o.version,ca.attempt_number
		FROM completion_attempts ca JOIN occurrences o ON o.id=ca.occurrence_id AND o.family_id=ca.family_id JOIN children c ON c.id=o.child_id AND c.family_id=o.family_id
		WHERE ca.family_id=$1 AND ca.decision='pending' AND o.state='pending_approval' AND ($2='' OR o.child_id::text=$2)
		AND (NULLIF($4,'') IS NULL OR (ca.submitted_at,ca.id)>($5,NULLIF($4,'')::uuid))
		ORDER BY ca.submitted_at,ca.id LIMIT $3`, familyID, childID, limit+1, cur.ID, cursorTime)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []Completion{}
	for rows.Next() {
		var v Completion
		var localDate time.Time
		var dueDate *time.Time
		if err = rows.Scan(&v.ID, &v.OccurrenceID, &v.ChildID, &v.ChildName, &v.ChildAvatar, &v.ChildColor, &v.Title, &v.ItemType, &localDate, &dueDate, &v.Points, &v.AttemptStatus, &v.OccurrenceStatus, &v.SubmittedAt, &v.Version, &v.AttemptNumber); err != nil {
			return nil, "", err
		}
		v.LocalDate = localDate.Format("2006-01-02")
		if dueDate != nil {
			v.DueDate = dueDate.Format("2006-01-02")
		}
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > limit {
		last := out[limit-1]
		next = encodeCursor(cursorValue{Kind: "pending", Binding: binding, Time: last.SubmittedAt.UTC().Format(time.RFC3339Nano), ID: last.ID})
		out = out[:limit]
	}
	return out, next, tx.Commit(ctx)
}

func (s *Service) decide(ctx context.Context, sessionID, familyID, completionID, decision, reason, key string, hash []byte, expected int64) (Completion, bool, error) {
	if len(strings.TrimSpace(reason)) > 500 {
		return Completion{}, false, ErrValidation
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Completion{}, false, err
	}
	defer tx.Rollback(ctx)
	userID, err := lockParent(ctx, tx, sessionID, familyID)
	if err != nil {
		return Completion{}, false, err
	}
	route := "completion." + decision
	body, replay, err := reserve(ctx, tx, familyID, sessionID, route, key, hash)
	if err != nil {
		return Completion{}, false, err
	}
	if replay {
		var out Completion
		if err = json.Unmarshal(body, &out); err != nil {
			return out, false, err
		}
		return out, true, tx.Commit(ctx)
	}
	var out Completion
	err = tx.QueryRow(ctx, `SELECT ca.id,ca.occurrence_id,ca.child_id,ca.decision::text,ca.submitted_at,o.state::text,o.version,ca.attempt_number,o.points_snapshot,o.title_snapshot
		FROM completion_attempts ca JOIN occurrences o ON o.id=ca.occurrence_id AND o.family_id=ca.family_id AND o.child_id=ca.child_id
		WHERE ca.id=$1 AND ca.family_id=$2 FOR UPDATE OF o,ca`, completionID, familyID).Scan(&out.ID, &out.OccurrenceID, &out.ChildID, &out.AttemptStatus, &out.SubmittedAt, &out.OccurrenceStatus, &out.Version, &out.AttemptNumber, &out.Points, &out.Title)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, false, ErrNotFound
	}
	if err != nil {
		return out, false, err
	}
	if out.Version != expected {
		return out, false, &TransitionError{ErrVersionConflict, out.OccurrenceStatus, out.Version}
	}
	if out.AttemptStatus != "pending" || out.OccurrenceStatus != "pending_approval" {
		return out, false, &TransitionError{ErrInvalidState, out.OccurrenceStatus, out.Version}
	}
	if decision != "approved" && decision != "rejected" {
		return out, false, ErrValidation
	}
	// The child row is the serialization boundary for every balance-affecting
	// operation, including awards and reward reservations.
	if decision == "approved" {
		if _, err = tx.Exec(ctx, `SELECT 1 FROM children WHERE id=$1 AND family_id=$2 FOR UPDATE`, out.ChildID, familyID); err != nil {
			return out, false, err
		}
	}
	err = tx.QueryRow(ctx, `UPDATE completion_attempts SET decision=$2,decided_at=now(),decided_by=$3,decision_note=nullif($4,'') WHERE id=$1 RETURNING decision::text,decided_at,coalesce(decision_note,'')`, completionID, decision, userID, strings.TrimSpace(reason)).Scan(&out.AttemptStatus, &out.DecidedAt, &out.Reason)
	if err != nil {
		return out, false, err
	}
	if s.fault != nil {
		if err = s.fault("attempt_decision"); err != nil {
			return out, false, err
		}
	}
	next := "not_started"
	action := "completion.rejected"
	if decision == "approved" {
		next = "approved"
		action = "completion.approved"
	}
	if err = tx.QueryRow(ctx, `UPDATE occurrences SET state=$2,version=version+1,updated_at=now() WHERE id=$1 RETURNING version`, out.OccurrenceID, next).Scan(&out.Version); err != nil {
		return out, false, err
	}
	out.OccurrenceStatus = next
	if s.fault != nil {
		if err = s.fault("occurrence_update"); err != nil {
			return out, false, err
		}
	}
	if decision == "approved" {
		err = tx.QueryRow(ctx, `INSERT INTO point_ledger(family_id,child_id,occurrence_id,completion_attempt_id,kind,amount,reason,actor_user_id) VALUES($1,$2,$3,$4,'award',$5,'Approved completion',$6) RETURNING id,amount`, familyID, out.ChildID, out.OccurrenceID, out.ID, out.Points, userID).Scan(&out.LedgerEntryID, &out.LedgerAmount)
		if err != nil {
			return out, false, err
		}
		if s.fault != nil {
			if err = s.fault("ledger_insert"); err != nil {
				return out, false, err
			}
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status,idempotency_key,metadata) VALUES($1,$2,$3,$4,'occurrence',$5,'pending_approval',$6,$7,jsonb_build_object('completionId',$8::text,'attemptNumber',$9::bigint,'reason',$10::text))`, familyID, userID, sessionID, action, out.OccurrenceID, next, key, out.ID, out.AttemptNumber, out.Reason)
	if err != nil {
		return out, false, err
	}
	if s.fault != nil {
		if err = s.fault("audit_insert"); err != nil {
			return out, false, err
		}
	}
	if err = finish(ctx, tx, familyID, sessionID, route, key, 200, out); err != nil {
		return out, false, err
	}
	if s.fault != nil {
		if err = s.fault("idempotency_finish"); err != nil {
			return out, false, err
		}
	}
	return out, false, tx.Commit(ctx)
}
func (s *Service) Approve(ctx context.Context, sessionID, familyID, id, key string, hash []byte, expected int64) (Completion, bool, error) {
	return s.decide(ctx, sessionID, familyID, id, "approved", "", key, hash, expected)
}
func (s *Service) Reject(ctx context.Context, sessionID, familyID, id, reason, key string, hash []byte, expected int64) (Completion, bool, error) {
	return s.decide(ctx, sessionID, familyID, id, "rejected", reason, key, hash, expected)
}

func (s *Service) Reverse(ctx context.Context, sessionID, familyID, completionID, reason, key string, hash []byte, expected int64) (Completion, bool, error) {
	if strings.TrimSpace(reason) == "" || len(strings.TrimSpace(reason)) > 500 {
		return Completion{}, false, ErrValidation
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Completion{}, false, err
	}
	defer tx.Rollback(ctx)
	userID, err := lockParent(ctx, tx, sessionID, familyID)
	if err != nil {
		return Completion{}, false, err
	}
	body, replay, err := reserve(ctx, tx, familyID, sessionID, "completion.reverse", key, hash)
	if err != nil {
		return Completion{}, false, err
	}
	if replay {
		var o Completion
		err = json.Unmarshal(body, &o)
		if err != nil {
			return o, false, err
		}
		return o, true, tx.Commit(ctx)
	}
	var out Completion
	var awardID string
	err = tx.QueryRow(ctx, `SELECT ca.id,ca.occurrence_id,ca.child_id,ca.decision::text,ca.submitted_at,ca.decided_at,o.state::text,o.version,ca.attempt_number,o.points_snapshot,o.title_snapshot,pl.id
		FROM completion_attempts ca JOIN occurrences o ON o.id=ca.occurrence_id AND o.family_id=ca.family_id JOIN point_ledger pl ON pl.completion_attempt_id=ca.id AND pl.kind='award' WHERE ca.id=$1 AND ca.family_id=$2 FOR UPDATE OF o,ca,pl`, completionID, familyID).Scan(&out.ID, &out.OccurrenceID, &out.ChildID, &out.AttemptStatus, &out.SubmittedAt, &out.DecidedAt, &out.OccurrenceStatus, &out.Version, &out.AttemptNumber, &out.Points, &out.Title, &awardID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, false, ErrNotFound
	}
	if err != nil {
		return out, false, err
	}
	if out.Version != expected {
		return out, false, &TransitionError{ErrVersionConflict, out.OccurrenceStatus, out.Version}
	}
	if out.AttemptStatus != "approved" || out.OccurrenceStatus != "approved" {
		return out, false, &TransitionError{ErrInvalidState, out.OccurrenceStatus, out.Version}
	}
	// Serialize against child redemption and correction transactions. An award
	// that has already been reserved/spent cannot be reversed below zero.
	if _, err = tx.Exec(ctx, `SELECT 1 FROM children WHERE id=$1 AND family_id=$2 FOR UPDATE`, out.ChildID, familyID); err != nil {
		return out, false, err
	}
	var balance int64
	if err = tx.QueryRow(ctx, `SELECT coalesce(sum(amount),0) FROM point_ledger WHERE family_id=$1 AND child_id=$2`, familyID, out.ChildID).Scan(&balance); err != nil {
		return out, false, err
	}
	if balance < out.Points {
		return out, false, ErrInsufficientAvailable
	}
	if err = tx.QueryRow(ctx, `UPDATE occurrences SET state='approval_reversed',version=version+1,updated_at=now() WHERE id=$1 RETURNING version`, out.OccurrenceID).Scan(&out.Version); err != nil {
		return out, false, err
	}
	out.OccurrenceStatus = "approval_reversed"
	if s.fault != nil {
		if err = s.fault("reverse_occurrence_update"); err != nil {
			return out, false, err
		}
	}
	out.Reason = strings.TrimSpace(reason)
	err = tx.QueryRow(ctx, `INSERT INTO point_ledger(family_id,child_id,occurrence_id,kind,amount,reason,actor_user_id,reverses_entry_id) VALUES($1,$2,$3,'approval_reversal',$4,$5,$6,$7) RETURNING id,amount`, familyID, out.ChildID, out.OccurrenceID, -out.Points, out.Reason, userID, awardID).Scan(&out.LedgerEntryID, &out.LedgerAmount)
	if err != nil {
		return out, false, err
	}
	if s.fault != nil {
		if err = s.fault("reverse_ledger_insert"); err != nil {
			return out, false, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status,idempotency_key,metadata) VALUES($1,$2,$3,'completion.approval_reversed','occurrence',$4,'approved','approval_reversed',$5,jsonb_build_object('completionId',$6::text,'reason',$7::text))`, familyID, userID, sessionID, out.OccurrenceID, key, out.ID, out.Reason)
	if err != nil {
		return out, false, err
	}
	if s.fault != nil {
		if err = s.fault("reverse_audit_insert"); err != nil {
			return out, false, err
		}
	}
	if err = finish(ctx, tx, familyID, sessionID, "completion.reverse", key, 200, out); err != nil {
		return out, false, err
	}
	if s.fault != nil {
		if err = s.fault("reverse_idempotency_finish"); err != nil {
			return out, false, err
		}
	}
	return out, false, tx.Commit(ctx)
}

func (s *Service) Correct(ctx context.Context, sessionID, familyID, childID string, amount int64, reason, key string, hash []byte) (LedgerEntry, bool, error) {
	if amount <= 0 || amount > 10000 || strings.TrimSpace(reason) == "" || len(strings.TrimSpace(reason)) > 500 {
		return LedgerEntry{}, false, ErrValidation
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return LedgerEntry{}, false, err
	}
	defer tx.Rollback(ctx)
	userID, err := lockParent(ctx, tx, sessionID, familyID)
	if err != nil {
		return LedgerEntry{}, false, err
	}
	body, replay, err := reserve(ctx, tx, familyID, sessionID, "points.correction", key, hash)
	if err != nil {
		return LedgerEntry{}, false, err
	}
	if replay {
		var o LedgerEntry
		err = json.Unmarshal(body, &o)
		if err != nil {
			return o, false, err
		}
		return o, true, tx.Commit(ctx)
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT true FROM children WHERE id=$1 AND family_id=$2 FOR UPDATE`, childID, familyID).Scan(&exists); errors.Is(err, pgx.ErrNoRows) {
		return LedgerEntry{}, false, ErrNotFound
	}
	if err != nil {
		return LedgerEntry{}, false, err
	}
	o := LedgerEntry{ChildID: childID, Kind: "manual_correction", Amount: amount, Reason: strings.TrimSpace(reason)}
	err = tx.QueryRow(ctx, `INSERT INTO point_ledger(family_id,child_id,kind,amount,reason,actor_user_id) VALUES($1,$2,'manual_correction',$3,$4,$5) RETURNING id,created_at`, familyID, childID, amount, o.Reason, userID).Scan(&o.ID, &o.CreatedAt)
	if err != nil {
		return o, false, err
	}
	if s.fault != nil {
		if err = s.fault("correction_ledger_insert"); err != nil {
			return o, false, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,idempotency_key,metadata) VALUES($1,$2,$3,'points.corrected','child',$4,$5,jsonb_build_object('ledgerEntryId',$6::text,'amount',$7::bigint,'reason',$8::text))`, familyID, userID, sessionID, childID, key, o.ID, amount, o.Reason)
	if err != nil {
		return o, false, err
	}
	if s.fault != nil {
		if err = s.fault("correction_audit_insert"); err != nil {
			return o, false, err
		}
	}
	if err = finish(ctx, tx, familyID, sessionID, "points.correction", key, 201, o); err != nil {
		return o, false, err
	}
	if s.fault != nil {
		if err = s.fault("correction_idempotency_finish"); err != nil {
			return o, false, err
		}
	}
	return o, false, tx.Commit(ctx)
}

func (s *Service) authorizeChild(ctx context.Context, sessionID, familyID, childID string, parentOnly bool) error {
	var mode, active string
	var archived bool
	err := s.pool.QueryRow(ctx, `SELECT s.mode::text,coalesce(s.active_child_id::text,''),c.archived_at IS NOT NULL FROM sessions s JOIN children c ON c.id=$3 AND c.family_id=s.family_id WHERE s.id=$1 AND s.family_id=$2 AND s.revoked_at IS NULL AND s.expires_at>now()`, sessionID, familyID, childID).Scan(&mode, &active, &archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if mode == "parent" {
		return nil
	}
	if !parentOnly && mode == "child" && active == childID && !archived {
		return nil
	}
	return ErrNotFound
}
func (s *Service) Balance(ctx context.Context, sessionID, familyID, childID string) (int64, error) {
	if err := s.authorizeChild(ctx, sessionID, familyID, childID, false); err != nil {
		return 0, err
	}
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT coalesce(sum(amount),0) FROM point_ledger WHERE family_id=$1 AND child_id=$2`, familyID, childID).Scan(&n)
	return n, err
}
func (s *Service) Ledger(ctx context.Context, sessionID, familyID, childID, cursor string, limit int) ([]LedgerEntry, string, error) {
	if err := s.authorizeChild(ctx, sessionID, familyID, childID, false); err != nil {
		return nil, "", err
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}
	var parent bool
	if err := s.pool.QueryRow(ctx, `SELECT mode='parent' FROM sessions WHERE id=$1 AND family_id=$2`, sessionID, familyID).Scan(&parent); err != nil {
		return nil, "", err
	}
	binding := cursorBinding(familyID, childID)
	cur, err := decodeCursor(cursor, "ledger", binding)
	if err != nil {
		return nil, "", err
	}
	var cursorTime time.Time
	if cur.Time != "" {
		cursorTime, err = time.Parse(time.RFC3339Nano, cur.Time)
		if err != nil {
			return nil, "", ErrValidation
		}
	}
	rows, err := s.pool.Query(ctx, `SELECT pl.id,pl.child_id,pl.kind::text,pl.amount,CASE WHEN $6 THEN coalesce(pl.reason,'') ELSE '' END,coalesce(pl.occurrence_id::text,''),coalesce(o.title_snapshot,''),pl.created_at,CASE WHEN $6 THEN coalesce(pl.reverses_entry_id::text,'') ELSE '' END FROM point_ledger pl LEFT JOIN occurrences o ON o.id=pl.occurrence_id WHERE pl.family_id=$1 AND pl.child_id=$2 AND (NULLIF($4,'') IS NULL OR (pl.created_at,pl.id)<($5,NULLIF($4,'')::uuid)) ORDER BY pl.created_at DESC,pl.id DESC LIMIT $3`, familyID, childID, limit+1, cur.ID, cursorTime, parent)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []LedgerEntry{}
	for rows.Next() {
		var v LedgerEntry
		if err = rows.Scan(&v.ID, &v.ChildID, &v.Kind, &v.Amount, &v.Reason, &v.OccurrenceID, &v.Title, &v.CreatedAt, &v.OriginalEntryID); err != nil {
			return nil, "", err
		}
		switch v.Kind {
		case "award":
			v.DisplayLabel = "Points earned"
		case "approval_reversal":
			v.DisplayLabel = "Approval corrected"
		case "manual_correction":
			v.DisplayLabel = "Points added"
		}
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > limit {
		last := out[limit-1]
		next = encodeCursor(cursorValue{Kind: "ledger", Binding: binding, Time: last.CreatedAt.UTC().Format(time.RFC3339Nano), ID: last.ID})
		out = out[:limit]
	}
	return out, next, nil
}
func (s *Service) History(ctx context.Context, sessionID, familyID, childID, from, to, cursor string, limit int) ([]HistoryEntry, string, error) {
	if err := s.authorizeChild(ctx, sessionID, familyID, childID, true); err != nil {
		return nil, "", err
	}
	var timezone string
	if err := s.pool.QueryRow(ctx, `SELECT timezone FROM families WHERE id=$1`, familyID).Scan(&timezone); err != nil {
		return nil, "", err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, "", err
	}
	today := s.now().In(location)
	if to == "" {
		to = today.Format("2006-01-02")
	}
	if from == "" {
		from = today.AddDate(0, 0, -29).Format("2006-01-02")
	}
	fromDate, e1 := time.Parse("2006-01-02", from)
	toDate, e2 := time.Parse("2006-01-02", to)
	if e1 != nil || e2 != nil || fromDate.After(toDate) || toDate.Sub(fromDate) > 365*24*time.Hour {
		return nil, "", ErrValidation
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback(ctx)
	binding := cursorBinding(familyID, childID, from, to)
	cur, err := decodeCursor(cursor, "history", binding)
	if err != nil {
		return nil, "", err
	}
	rows, err := tx.Query(ctx, `SELECT o.id,o.item_type_snapshot,o.local_date,o.title_snapshot,o.state::text,o.points_snapshot,o.version,ca.submitted_at,ca.decided_at,coalesce(ca.decision::text,''),coalesce(ca.decision_note,''),coalesce(o.due_date::text,''),coalesce(o.description_snapshot,''),coalesce(o.icon_snapshot,''),coalesce(o.color_snapshot,''),coalesce(sum(pl.amount) FILTER(WHERE pl.kind='award'),0),coalesce(sum(pl.amount) FILTER(WHERE pl.kind='approval_reversal'),0) FROM occurrences o LEFT JOIN LATERAL(SELECT * FROM completion_attempts x WHERE x.occurrence_id=o.id ORDER BY x.attempt_number DESC LIMIT 1)ca ON true LEFT JOIN point_ledger pl ON pl.family_id=o.family_id AND pl.occurrence_id=o.id WHERE o.family_id=$1 AND o.child_id=$2 AND o.local_date BETWEEN $3::date AND $4::date AND (NULLIF($6,'') IS NULL OR (o.local_date,o.id)<(NULLIF($6,'')::date,NULLIF($7,'')::uuid)) GROUP BY o.id,ca.submitted_at,ca.decided_at,ca.decision,ca.decision_note ORDER BY o.local_date DESC,o.id DESC LIMIT $5`, familyID, childID, from, to, limit+1, cur.Date, cur.ID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []HistoryEntry{}
	for rows.Next() {
		var v HistoryEntry
		var d time.Time
		if err = rows.Scan(&v.ID, &v.Type, &d, &v.Title, &v.Status, &v.Points, &v.Version, &v.SubmittedAt, &v.DecidedAt, &v.Decision, &v.DecisionNote, &v.DueDate, &v.Description, &v.Icon, &v.Color, &v.AwardDelta, &v.ReversalDelta); err != nil {
			return nil, "", err
		}
		v.Date = d.Format("2006-01-02")
		v.Attempts = []AttemptSummary{}
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		return nil, "", err
	}
	rows.Close()
	for i := range out {
		attemptRows, attemptErr := tx.Query(ctx, `SELECT id,attempt_number,decision::text,submitted_at,decided_at,coalesce(decision_note,'') FROM completion_attempts WHERE family_id=$1 AND occurrence_id=$2 ORDER BY attempt_number`, familyID, out[i].ID)
		if attemptErr != nil {
			return nil, "", attemptErr
		}
		for attemptRows.Next() {
			var a AttemptSummary
			if attemptErr = attemptRows.Scan(&a.ID, &a.AttemptNumber, &a.Status, &a.SubmittedAt, &a.DecidedAt, &a.Reason); attemptErr != nil {
				attemptRows.Close()
				return nil, "", attemptErr
			}
			out[i].Attempts = append(out[i].Attempts, a)
		}
		attemptRows.Close()
		if attemptErr = attemptRows.Err(); attemptErr != nil {
			return nil, "", attemptErr
		}
	}
	next := ""
	if len(out) > limit {
		last := out[limit-1]
		next = encodeCursor(cursorValue{Kind: "history", Binding: binding, Date: last.Date, ID: last.ID})
		out = out[:limit]
	}
	return out, next, tx.Commit(ctx)
}

func (s *Service) Report(ctx context.Context, sessionID, familyID, childID, period, anchor string) (Report, error) {
	if err := s.authorizeChild(ctx, sessionID, familyID, childID, true); err != nil {
		return Report{}, err
	}
	a, err := time.Parse("2006-01-02", anchor)
	if err != nil {
		return Report{}, ErrValidation
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback(ctx)
	var tz string
	var week int
	err = tx.QueryRow(ctx, `SELECT timezone,week_starts_on FROM families WHERE id=$1`, familyID).Scan(&tz, &week)
	if err != nil {
		return Report{}, err
	}
	start, end, err := reportPeriod(period, a, week)
	if err != nil {
		return Report{}, err
	}
	out := Report{ChildID: childID, Period: period, StartDate: start.Format("2006-01-02"), EndDate: end.Format("2006-01-02"), Timezone: tz, WeekStartsOn: week}
	err = tx.QueryRow(ctx, `SELECT count(*)FILTER(WHERE o.state<>'cancelled'),count(*)FILTER(WHERE o.state='pending_approval'),count(*)FILTER(WHERE o.state<>'cancelled' AND EXISTS(SELECT 1 FROM completion_attempts ca WHERE ca.occurrence_id=o.id)),count(*)FILTER(WHERE o.state='approved'),count(*)FILTER(WHERE o.state='approval_reversed'),count(*)FILTER(WHERE o.state<>'cancelled' AND EXISTS(SELECT 1 FROM completion_attempts ca WHERE ca.occurrence_id=o.id AND ca.decision='rejected')),count(*)FILTER(WHERE o.state='not_started'),count(*)FILTER(WHERE o.state='cancelled'),coalesce((SELECT sum(pl.amount) FROM point_ledger pl JOIN occurrences po ON po.id=pl.occurrence_id WHERE pl.family_id=$1 AND pl.child_id=$2 AND po.local_date BETWEEN $3 AND $4),0),coalesce((SELECT sum(pl.amount) FROM point_ledger pl JOIN families f ON f.id=pl.family_id WHERE pl.family_id=$1 AND pl.child_id=$2 AND pl.kind='manual_correction' AND (pl.created_at AT TIME ZONE f.timezone)::date BETWEEN $3 AND $4),0) FROM occurrences o WHERE o.family_id=$1 AND o.child_id=$2 AND o.local_date BETWEEN $3 AND $4`, familyID, childID, start, end).Scan(&out.Assigned, &out.Pending, &out.Submitted, &out.Approved, &out.Reversed, &out.Rejected, &out.Incomplete, &out.Cancelled, &out.PointsEarned, &out.ManualCorrections)
	if err != nil {
		return out, fmt.Errorf("report: %w", err)
	}
	err = tx.QueryRow(ctx, `SELECT coalesce(-sum(amount) FILTER(WHERE kind='reward_redemption'),0),coalesce(sum(amount) FILTER(WHERE kind='reward_refund'),0) FROM point_ledger pl JOIN families f ON f.id=pl.family_id WHERE pl.family_id=$1 AND pl.child_id=$2 AND (pl.created_at AT TIME ZONE f.timezone)::date BETWEEN $3 AND $4`, familyID, childID, start, end).Scan(&out.PointsRedeemed, &out.PointsRefunded)
	if err != nil {
		return out, fmt.Errorf("report rewards: %w", err)
	}
	out.NetPointsChange = out.PointsEarned + out.ManualCorrections - out.PointsRedeemed + out.PointsRefunded
	return out, tx.Commit(ctx)
}

func reportPeriod(period string, anchor time.Time, weekStart int) (time.Time, time.Time, error) {
	start := anchor
	switch period {
	case "day":
	case "week":
		start = anchor.AddDate(0, 0, -(int(anchor.Weekday())-weekStart+7)%7)
	case "month":
		start = time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return time.Time{}, time.Time{}, ErrValidation
	}
	end := start
	if period == "week" {
		end = start.AddDate(0, 0, 6)
	}
	if period == "month" {
		end = start.AddDate(0, 1, -1)
	}
	return start, end, nil
}
