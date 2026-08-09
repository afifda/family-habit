package habits

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("resource has activity or conflicts")
	ErrIdempotency     = errors.New("idempotency key conflict")
	ErrVersionConflict = errors.New("resource version conflict")
	ErrParentAuthority = errors.New("parent authority is no longer active")
)

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

type Habit struct {
	ID, Title, Description, Icon, Color string
	Active                              bool
	CreatedAt, UpdatedAt                time.Time
	Version                             int64
	Assignments                         []Assignment
}

type Schedule struct {
	Kind     string
	Weekdays []int16
}

type Assignment struct {
	ID, HabitID, ChildID string
	Points               int32
	Schedule             Schedule
	EffectiveStartDate   time.Time
	EffectiveUntil       *time.Time
	Active               bool
	Version              int64
}

type Task struct {
	ID, ChildID, Title, Description string
	DueDate                         time.Time
	Points                          int32
	Status                          string
	CreatedAt, UpdatedAt            time.Time
	Version                         int64
}

type Occurrence struct {
	ID, ChildID, Type, Title, Description, Icon, Color, Status string
	LocalDate                                                  time.Time
	Points                                                     int32
}

type HabitInput struct {
	Title, Description, Icon, Color   string
	DescriptionSet, IconSet, ColorSet bool
}
type AssignmentInput struct {
	ChildID, Kind string
	Points        int32
	Weekdays      []int16
	EffectiveDate time.Time
}
type TaskInput struct {
	ChildID, Title, Description string
	DueDate                     time.Time
	Points                      int32
	DescriptionSet              bool
}

func requireParent(ctx context.Context, tx pgx.Tx, sessionID, userID, familyID string) error {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions s JOIN family_memberships m ON (m.family_id,m.user_id)=(s.family_id,s.user_id) JOIN families f ON f.id=s.family_id JOIN users u ON u.id=s.user_id WHERE s.id=$1 AND s.user_id=$2 AND s.family_id=$3 AND s.mode='parent' AND s.revoked_at IS NULL AND s.expires_at>now() AND u.active AND m.role='owner' AND now()<s.last_activity_at+make_interval(mins=>f.parent_idle_minutes))`, sessionID, userID, familyID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrParentAuthority
	}
	return nil
}

func (s *Service) ListHabits(ctx context.Context, familyID string, active *bool) ([]Habit, error) {
	rows, err := s.pool.Query(ctx, `SELECT h.id,v.title,coalesce(v.description,''),coalesce(v.icon,''),coalesce(v.color,''),h.inactive_from IS NULL OR h.inactive_from>(now() AT TIME ZONE f.timezone)::date,h.created_at,h.updated_at,h.version FROM habits h JOIN families f ON f.id=h.family_id JOIN LATERAL (SELECT * FROM habit_versions x WHERE x.habit_id=h.id AND x.effective_from<=(now() AT TIME ZONE f.timezone)::date AND (x.effective_until IS NULL OR x.effective_until>=(now() AT TIME ZONE f.timezone)::date) ORDER BY x.effective_from DESC LIMIT 1) v ON true WHERE h.family_id=$1 AND ($2::boolean IS NULL OR (h.inactive_from IS NULL OR h.inactive_from>(now() AT TIME ZONE f.timezone)::date)=$2) ORDER BY lower(v.title),h.id`, familyID, active)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Habit{}
	for rows.Next() {
		var h Habit
		if err = rows.Scan(&h.ID, &h.Title, &h.Description, &h.Icon, &h.Color, &h.Active, &h.CreatedAt, &h.UpdatedAt, &h.Version); err != nil {
			return nil, err
		}
		h.Assignments, err = s.listAssignments(ctx, familyID, h.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Service) listAssignments(ctx context.Context, familyID, habitID string) ([]Assignment, error) {
	rows, err := s.pool.Query(ctx, `SELECT a.id,a.habit_id,a.child_id,a.points,sc.kind::text,sc.weekdays,a.effective_from,a.effective_until,a.effective_until IS NULL OR a.effective_until>=(now() AT TIME ZONE f.timezone)::date,a.version FROM habit_assignments a JOIN families f ON f.id=a.family_id JOIN habit_schedules sc ON sc.assignment_id=a.id WHERE a.family_id=$1 AND a.habit_id=$2 ORDER BY a.child_id,a.effective_from`, familyID, habitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Assignment{}
	for rows.Next() {
		var a Assignment
		if err = rows.Scan(&a.ID, &a.HabitID, &a.ChildID, &a.Points, &a.Schedule.Kind, &a.Schedule.Weekdays, &a.EffectiveStartDate, &a.EffectiveUntil, &a.Active, &a.Version); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func reserve(ctx context.Context, tx pgx.Tx, familyID, sessionID, route, key string, hash []byte) ([]byte, bool, error) {
	if key == "" {
		return nil, false, nil
	}
	tag, err := tx.Exec(ctx, `INSERT INTO idempotency_records(family_id,session_id,route_family,idempotency_key,request_hash,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '24 hours') ON CONFLICT (family_id,session_id,route_family,idempotency_key) DO NOTHING`, familyID, sessionID, route, key, hash)
	if err != nil {
		return nil, false, err
	}
	if tag.RowsAffected() == 1 {
		return nil, false, nil
	}
	var oldHash, body []byte
	err = tx.QueryRow(ctx, `SELECT request_hash,response_body FROM idempotency_records WHERE family_id=$1 AND session_id=$2 AND route_family=$3 AND idempotency_key=$4 FOR UPDATE`, familyID, sessionID, route, key).Scan(&oldHash, &body)
	if err == nil {
		if sha256.Sum256(oldHash) != sha256.Sum256(hash) {
			return nil, false, ErrIdempotency
		}
		return body, true, nil
	}
	return nil, false, err
}

func finish(ctx context.Context, tx pgx.Tx, familyID, sessionID, route, key string, v any, status int) error {
	if key == "" {
		return nil
	}
	b, _ := json.Marshal(v)
	_, err := tx.Exec(ctx, `UPDATE idempotency_records SET response_status=$5,response_body=$6 WHERE family_id=$1 AND session_id=$2 AND route_family=$3 AND idempotency_key=$4`, familyID, sessionID, route, key, status, b)
	return err
}

func versionMatches(current int64, expected *int64) bool {
	return expected == nil || *expected == current
}

func (s *Service) CreateHabit(ctx context.Context, sessionID, userID, familyID, key string, hash []byte, in HabitInput) (Habit, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Habit{}, false, err
	}
	defer tx.Rollback(ctx)
	if err = requireParent(ctx, tx, sessionID, userID, familyID); err != nil {
		return Habit{}, false, err
	}
	body, replay, err := reserve(ctx, tx, familyID, sessionID, "habits.create", key, hash)
	if err != nil {
		return Habit{}, false, err
	}
	if replay {
		var h Habit
		err = json.Unmarshal(body, &h)
		return h, true, err
	}
	var h Habit
	err = tx.QueryRow(ctx, `INSERT INTO habits(family_id,title,description,icon,color) VALUES($1,$2,nullif($3,''),nullif($4,''),nullif($5,'')) RETURNING id,title,coalesce(description,''),coalesce(icon,''),coalesce(color,''),true,created_at,updated_at,version`, familyID, in.Title, in.Description, in.Icon, in.Color).Scan(&h.ID, &h.Title, &h.Description, &h.Icon, &h.Color, &h.Active, &h.CreatedAt, &h.UpdatedAt, &h.Version)
	if err != nil {
		return Habit{}, false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO habit_versions(family_id,habit_id,title,description,icon,color,effective_from) VALUES($1,$2,$3,nullif($4,''),nullif($5,''),nullif($6,''),'0001-01-01')`, familyID, h.ID, in.Title, in.Description, in.Icon, in.Color)
	if err == nil {
		err = finish(ctx, tx, familyID, sessionID, "habits.create", key, h, 201)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,after_status,idempotency_key) VALUES($1,$2,$3,'habit.created','habit',$4,'active',$5)`, familyID, userID, sessionID, h.ID, key)
	}
	if err != nil {
		return Habit{}, false, err
	}
	return h, false, tx.Commit(ctx)
}

func (s *Service) UpdateHabit(ctx context.Context, sessionID, userID, familyID, habitID string, effective time.Time, in HabitInput) (Habit, error) {
	h, _, err := s.UpdateHabitConditional(ctx, sessionID, userID, familyID, habitID, "", nil, nil, effective, in)
	return h, err
}

func (s *Service) UpdateHabitConditional(ctx context.Context, sessionID, userID, familyID, habitID, key string, hash []byte, expected *int64, effective time.Time, in HabitInput) (Habit, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Habit{}, false, err
	}
	defer tx.Rollback(ctx)
	if err = requireParent(ctx, tx, sessionID, userID, familyID); err != nil {
		return Habit{}, false, err
	}
	body, replay, err := reserve(ctx, tx, familyID, sessionID, "habits.update", key, hash)
	if err != nil {
		return Habit{}, false, err
	}
	if replay {
		var h Habit
		err = json.Unmarshal(body, &h)
		return h, true, err
	}
	var old Habit
	var versionID string
	var from time.Time
	var until *time.Time
	err = tx.QueryRow(ctx, `SELECT h.id,v.id,v.title,coalesce(v.description,''),coalesce(v.icon,''),coalesce(v.color,''),v.effective_from,v.effective_until,h.created_at,h.updated_at,h.version FROM habits h JOIN habit_versions v ON v.habit_id=h.id WHERE h.id=$1 AND h.family_id=$2 AND v.effective_from<=$3 AND (v.effective_until IS NULL OR v.effective_until>=$3) FOR UPDATE OF h,v`, habitID, familyID, effective).Scan(&old.ID, &versionID, &old.Title, &old.Description, &old.Icon, &old.Color, &from, &until, &old.CreatedAt, &old.UpdatedAt, &old.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Habit{}, false, ErrNotFound
	}
	if err != nil {
		return Habit{}, false, err
	}
	if !versionMatches(old.Version, expected) {
		return Habit{}, false, ErrVersionConflict
	}
	if in.Title == "" {
		in.Title = old.Title
	}
	if !in.DescriptionSet && in.Description == "" {
		in.Description = old.Description
	}
	if !in.IconSet && in.Icon == "" {
		in.Icon = old.Icon
	}
	if !in.ColorSet && in.Color == "" {
		in.Color = old.Color
	}
	if effective.Equal(from) {
		_, err = tx.Exec(ctx, `UPDATE habit_versions SET title=$2,description=nullif($3,''),icon=nullif($4,''),color=nullif($5,'') WHERE id=$1`, versionID, in.Title, in.Description, in.Icon, in.Color)
	} else {
		_, err = tx.Exec(ctx, `UPDATE habit_versions SET effective_until=$2::date-1 WHERE id=$1`, versionID, effective)
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO habit_versions(family_id,habit_id,title,description,icon,color,effective_from,effective_until) VALUES($1,$2,$3,nullif($4,''),nullif($5,''),nullif($6,''),$7,$8)`, familyID, habitID, in.Title, in.Description, in.Icon, in.Color, effective, until)
		}
	}
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM occurrences o USING habit_assignments a WHERE o.assignment_id=a.id AND a.habit_id=$1 AND a.family_id=$2 AND o.local_date>=$3 AND o.state='not_started' AND NOT EXISTS(SELECT 1 FROM completion_attempts ca WHERE ca.occurrence_id=o.id)`, habitID, familyID, effective)
	}
	if err == nil {
		err = tx.QueryRow(ctx, `UPDATE habits SET title=$2,description=nullif($3,''),icon=nullif($4,''),color=nullif($5,''),updated_at=now(),version=version+1 WHERE id=$1 RETURNING updated_at,version`, habitID, in.Title, in.Description, in.Icon, in.Color).Scan(&old.UpdatedAt, &old.Version)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status,idempotency_key) VALUES($1,$2,$3,'habit.updated','habit',$4,'active','active',nullif($5,''))`, familyID, userID, sessionID, habitID, key)
	}
	old.Title, old.Description, old.Icon, old.Color, old.Active = in.Title, in.Description, in.Icon, in.Color, true
	if err == nil {
		err = finish(ctx, tx, familyID, sessionID, "habits.update", key, old, 200)
	}
	if err != nil {
		return Habit{}, false, mapDB(err)
	}
	return old, false, tx.Commit(ctx)
}

func (s *Service) DeactivateHabit(ctx context.Context, sessionID, userID, familyID, habitID string, effective time.Time) error {
	_, err := s.DeactivateHabitConditional(ctx, sessionID, userID, familyID, habitID, "", nil, nil, effective)
	return err
}

func (s *Service) DeactivateHabitConditional(ctx context.Context, sessionID, userID, familyID, habitID, key string, hash []byte, expected *int64, effective time.Time) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if err = requireParent(ctx, tx, sessionID, userID, familyID); err != nil {
		return false, err
	}
	_, replay, err := reserve(ctx, tx, familyID, sessionID, "habits.deactivate", key, hash)
	if err != nil || replay {
		return replay, err
	}
	var current int64
	err = tx.QueryRow(ctx, `SELECT version FROM habits WHERE id=$1 AND family_id=$2 FOR UPDATE`, habitID, familyID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if !versionMatches(current, expected) {
		return false, ErrVersionConflict
	}
	tag, err := tx.Exec(ctx, `UPDATE habits SET inactive_from=LEAST(coalesce(inactive_from,$3),$3),updated_at=now(),version=version+1 WHERE id=$1 AND family_id=$2`, habitID, familyID, effective)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, ErrNotFound
	}
	_, err = tx.Exec(ctx, `UPDATE habit_assignments SET effective_until=$3::date-1 WHERE habit_id=$1 AND family_id=$2 AND effective_from<$3 AND (effective_until IS NULL OR effective_until>=$3)`, habitID, familyID, effective)
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM habit_assignments WHERE habit_id=$1 AND family_id=$2 AND effective_from>=$3 AND NOT EXISTS(SELECT 1 FROM occurrences o WHERE o.assignment_id=habit_assignments.id)`, habitID, familyID, effective)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM occurrences o USING habit_assignments a WHERE o.assignment_id=a.id AND a.habit_id=$2 AND o.family_id=$1 AND o.local_date>=$3 AND o.state='not_started' AND NOT EXISTS(SELECT 1 FROM completion_attempts ca WHERE ca.occurrence_id=o.id)`, familyID, habitID, effective)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status,idempotency_key) VALUES($1,$2,$3,'habit.deactivated','habit',$4,'active','inactive',nullif($5,''))`, familyID, userID, sessionID, habitID, key)
	}
	if err == nil {
		err = finish(ctx, tx, familyID, sessionID, "habits.deactivate", key, nil, 204)
	}
	if err != nil {
		return false, err
	}
	return false, tx.Commit(ctx)
}

func (s *Service) CreateAssignment(ctx context.Context, sessionID, userID, familyID, habitID, key string, hash []byte, in AssignmentInput) (Assignment, bool, error) {
	if in.Weekdays == nil {
		in.Weekdays = []int16{}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Assignment{}, false, err
	}
	defer tx.Rollback(ctx)
	if err = requireParent(ctx, tx, sessionID, userID, familyID); err != nil {
		return Assignment{}, false, err
	}
	body, replay, err := reserve(ctx, tx, familyID, sessionID, "assignments.create", key, hash)
	if err != nil {
		return Assignment{}, false, err
	}
	if replay {
		var a Assignment
		err = json.Unmarshal(body, &a)
		return a, true, err
	}
	var eligible bool
	err = tx.QueryRow(ctx, `SELECT h.inactive_from IS NULL OR h.inactive_from>$3 FROM habits h JOIN children c ON c.family_id=h.family_id WHERE h.id=$2 AND h.family_id=$1 AND c.id=$4 AND c.archived_at IS NULL FOR UPDATE OF h,c`, familyID, habitID, in.EffectiveDate, in.ChildID).Scan(&eligible)
	if errors.Is(err, pgx.ErrNoRows) || !eligible {
		return Assignment{}, false, ErrNotFound
	}
	if err != nil {
		return Assignment{}, false, err
	}
	var a Assignment
	err = tx.QueryRow(ctx, `INSERT INTO habit_assignments(family_id,habit_id,child_id,points,effective_from) VALUES($1,$2,$3,$4,$5) RETURNING id,habit_id,child_id,points,effective_from,effective_until,true,version`, familyID, habitID, in.ChildID, in.Points, in.EffectiveDate).Scan(&a.ID, &a.HabitID, &a.ChildID, &a.Points, &a.EffectiveStartDate, &a.EffectiveUntil, &a.Active, &a.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, false, ErrNotFound
	}
	if err != nil {
		return Assignment{}, false, mapDB(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO habit_schedules(assignment_id,kind,weekdays) VALUES($1,$2,$3)`, a.ID, in.Kind, in.Weekdays)
	a.Schedule = Schedule{Kind: in.Kind, Weekdays: in.Weekdays}
	if err == nil {
		err = finish(ctx, tx, familyID, sessionID, "assignments.create", key, a, 201)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,after_status,idempotency_key) VALUES($1,$2,$3,'assignment.created','assignment',$4,'active',$5)`, familyID, userID, sessionID, a.ID, key)
	}
	if err != nil {
		return Assignment{}, false, mapDB(err)
	}
	return a, false, tx.Commit(ctx)
}

func (s *Service) CreateAssignments(ctx context.Context, sessionID, userID, familyID, habitID, key string, hash []byte, inputs []AssignmentInput) ([]Assignment, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	if err = requireParent(ctx, tx, sessionID, userID, familyID); err != nil {
		return nil, false, err
	}
	body, replay, err := reserve(ctx, tx, familyID, sessionID, "assignments.batch.create", key, hash)
	if err != nil {
		return nil, false, err
	}
	if replay {
		var out []Assignment
		err = json.Unmarshal(body, &out)
		return out, true, err
	}
	var inactiveFrom *time.Time
	err = tx.QueryRow(ctx, `SELECT inactive_from FROM habits WHERE id=$2 AND family_id=$1 FOR UPDATE`, familyID, habitID).Scan(&inactiveFrom)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, ErrNotFound
	}
	if err != nil {
		return nil, false, err
	}
	for _, in := range inputs {
		if inactiveFrom != nil && !inactiveFrom.After(in.EffectiveDate) {
			return nil, false, ErrNotFound
		}
	}
	ids := uniqueChildIDs(inputs)
	rows, err := tx.Query(ctx, `SELECT id FROM children WHERE family_id=$1 AND id=ANY($2::uuid[]) AND archived_at IS NULL ORDER BY id FOR UPDATE`, familyID, ids)
	if err != nil {
		return nil, false, err
	}
	locked := 0
	for rows.Next() {
		locked++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	if locked != len(ids) {
		return nil, false, ErrNotFound
	}
	out := make([]Assignment, 0, len(inputs))
	for _, in := range inputs {
		if in.Weekdays == nil {
			in.Weekdays = []int16{}
		}
		var a Assignment
		err = tx.QueryRow(ctx, `INSERT INTO habit_assignments(family_id,habit_id,child_id,points,effective_from) VALUES($1,$2,$3,$4,$5) RETURNING id,habit_id,child_id,points,effective_from,effective_until,true,version`, familyID, habitID, in.ChildID, in.Points, in.EffectiveDate).Scan(&a.ID, &a.HabitID, &a.ChildID, &a.Points, &a.EffectiveStartDate, &a.EffectiveUntil, &a.Active, &a.Version)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, ErrNotFound
		}
		if err != nil {
			return nil, false, mapDB(err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO habit_schedules(assignment_id,kind,weekdays) VALUES($1,$2,$3)`, a.ID, in.Kind, in.Weekdays)
		if err != nil {
			return nil, false, mapDB(err)
		}
		a.Schedule = Schedule{Kind: in.Kind, Weekdays: in.Weekdays}
		out = append(out, a)
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,after_status,idempotency_key) VALUES($1,$2,$3,'assignment.created','assignment',$4,'active',$5)`, familyID, userID, sessionID, a.ID, key)
		if err != nil {
			return nil, false, err
		}
	}
	if err = finish(ctx, tx, familyID, sessionID, "assignments.batch.create", key, out, 201); err != nil {
		return nil, false, err
	}
	return out, false, tx.Commit(ctx)
}

func (s *Service) UpdateAssignment(ctx context.Context, sessionID, userID, familyID, id string, in AssignmentInput) (Assignment, error) {
	a, _, err := s.UpdateAssignmentConditional(ctx, sessionID, userID, familyID, id, "", nil, nil, in)
	return a, err
}

func (s *Service) UpdateAssignmentConditional(ctx context.Context, sessionID, userID, familyID, id, key string, hash []byte, expected *int64, in AssignmentInput) (Assignment, bool, error) {
	if in.Kind == "daily" && in.Weekdays == nil {
		in.Weekdays = []int16{}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Assignment{}, false, err
	}
	defer tx.Rollback(ctx)
	if err = requireParent(ctx, tx, sessionID, userID, familyID); err != nil {
		return Assignment{}, false, err
	}
	body, replay, err := reserve(ctx, tx, familyID, sessionID, "assignments.update", key, hash)
	if err != nil {
		return Assignment{}, false, err
	}
	if replay {
		var a Assignment
		err = json.Unmarshal(body, &a)
		return a, true, err
	}
	var old Assignment
	err = tx.QueryRow(ctx, `SELECT a.id,a.habit_id,a.child_id,a.points,s.kind::text,s.weekdays,a.effective_from,a.effective_until,a.version FROM habit_assignments a JOIN habit_schedules s ON s.assignment_id=a.id WHERE a.id=$1 AND a.family_id=$2 AND a.effective_from<=$3 AND (a.effective_until IS NULL OR a.effective_until>=$3) FOR UPDATE OF a,s`, id, familyID, in.EffectiveDate).Scan(&old.ID, &old.HabitID, &old.ChildID, &old.Points, &old.Schedule.Kind, &old.Schedule.Weekdays, &old.EffectiveStartDate, &old.EffectiveUntil, &old.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, false, ErrNotFound
	}
	if err != nil {
		return Assignment{}, false, err
	}
	if !versionMatches(old.Version, expected) {
		return Assignment{}, false, ErrVersionConflict
	}
	if in.Points == 0 {
		in.Points = old.Points
	}
	if in.Kind == "" {
		in.Kind = old.Schedule.Kind
		in.Weekdays = old.Schedule.Weekdays
	}
	_, err = tx.Exec(ctx, `DELETE FROM occurrences WHERE assignment_id=$1 AND local_date>=$2 AND state='not_started' AND NOT EXISTS(SELECT 1 FROM completion_attempts ca WHERE ca.occurrence_id=occurrences.id)`, id, in.EffectiveDate)
	if err != nil {
		return Assignment{}, false, err
	}
	var out Assignment
	if in.EffectiveDate.Equal(old.EffectiveStartDate) {
		err = tx.QueryRow(ctx, `UPDATE habit_assignments SET points=$2,version=version+1 WHERE id=$1 RETURNING version`, id, in.Points).Scan(&old.Version)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE habit_schedules SET kind=$2,weekdays=$3 WHERE assignment_id=$1`, id, in.Kind, in.Weekdays)
		}
		out = old
		out.Points = in.Points
		out.Schedule = Schedule{in.Kind, in.Weekdays}
	} else {
		_, err = tx.Exec(ctx, `UPDATE habit_assignments SET effective_until=$2::date-1,version=version+1 WHERE id=$1`, id, in.EffectiveDate)
		if err == nil {
			err = tx.QueryRow(ctx, `INSERT INTO habit_assignments(family_id,habit_id,child_id,points,effective_from,effective_until,supersedes_assignment_id) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,habit_id,child_id,points,effective_from,effective_until,true,version`, familyID, old.HabitID, old.ChildID, in.Points, in.EffectiveDate, old.EffectiveUntil, id).Scan(&out.ID, &out.HabitID, &out.ChildID, &out.Points, &out.EffectiveStartDate, &out.EffectiveUntil, &out.Active, &out.Version)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO habit_schedules(assignment_id,kind,weekdays) VALUES($1,$2,$3)`, out.ID, in.Kind, in.Weekdays)
		}
		out.Schedule = Schedule{in.Kind, in.Weekdays}
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status,idempotency_key) VALUES($1,$2,$3,'assignment.replaced','assignment',$4,'active','active',nullif($5,''))`, familyID, userID, sessionID, out.ID, key)
	}
	if err == nil {
		err = finish(ctx, tx, familyID, sessionID, "assignments.update", key, out, 200)
	}
	if err != nil {
		return Assignment{}, false, mapDB(err)
	}
	return out, false, tx.Commit(ctx)
}

func (s *Service) DeactivateAssignment(ctx context.Context, sessionID, userID, familyID, id string, effective time.Time) error {
	_, err := s.DeactivateAssignmentConditional(ctx, sessionID, userID, familyID, id, "", nil, nil, effective)
	return err
}

func (s *Service) DeactivateAssignmentConditional(ctx context.Context, sessionID, userID, familyID, id, key string, hash []byte, expected *int64, effective time.Time) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if err = requireParent(ctx, tx, sessionID, userID, familyID); err != nil {
		return false, err
	}
	_, replay, err := reserve(ctx, tx, familyID, sessionID, "assignments.deactivate", key, hash)
	if err != nil || replay {
		return replay, err
	}
	var from time.Time
	var current int64
	err = tx.QueryRow(ctx, `SELECT effective_from,version FROM habit_assignments WHERE id=$1 AND family_id=$2 FOR UPDATE`, id, familyID).Scan(&from, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if !versionMatches(current, expected) {
		return false, ErrVersionConflict
	}
	_, err = tx.Exec(ctx, `DELETE FROM occurrences WHERE assignment_id=$1 AND local_date>=$2 AND state='not_started' AND NOT EXISTS(SELECT 1 FROM completion_attempts ca WHERE ca.occurrence_id=occurrences.id)`, id, effective)
	if err != nil {
		return false, err
	}
	if !effective.After(from) {
		tag, e := tx.Exec(ctx, `DELETE FROM habit_assignments WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM occurrences WHERE assignment_id=$1)`, id)
		if e != nil {
			return false, e
		}
		if tag.RowsAffected() == 0 {
			return false, ErrConflict
		}
	} else {
		_, err = tx.Exec(ctx, `UPDATE habit_assignments SET effective_until=$2::date-1,version=version+1 WHERE id=$1`, id, effective)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status,idempotency_key) VALUES($1,$2,$3,'assignment.deactivated','assignment',$4,'active','inactive',nullif($5,''))`, familyID, userID, sessionID, id, key)
	}
	if err == nil {
		err = finish(ctx, tx, familyID, sessionID, "assignments.deactivate", key, nil, 204)
	}
	if err != nil {
		return false, err
	}
	return false, tx.Commit(ctx)
}

func (s *Service) ListTasks(ctx context.Context, familyID string, childID, status string) ([]Task, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,child_id,title,coalesce(description,''),due_date,points,state::text,created_at,updated_at,version FROM one_off_tasks WHERE family_id=$1 AND ($2='' OR child_id::text=$2) AND ($3='' OR state::text=$3) ORDER BY due_date DESC,id`, familyID, childID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var t Task
		if err = rows.Scan(&t.ID, &t.ChildID, &t.Title, &t.Description, &t.DueDate, &t.Points, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.Version); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Service) CreateTask(ctx context.Context, sessionID, userID, familyID, key string, hash []byte, in TaskInput) (Task, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Task{}, false, err
	}
	defer tx.Rollback(ctx)
	if err = requireParent(ctx, tx, sessionID, userID, familyID); err != nil {
		return Task{}, false, err
	}
	body, replay, err := reserve(ctx, tx, familyID, sessionID, "tasks.create", key, hash)
	if err != nil {
		return Task{}, false, err
	}
	if replay {
		var t Task
		err = json.Unmarshal(body, &t)
		return t, true, err
	}
	var t Task
	err = tx.QueryRow(ctx, `INSERT INTO one_off_tasks(family_id,child_id,title,description,points,due_date) SELECT $1,$2,$3,nullif($4,''),$5,$6 WHERE EXISTS(SELECT 1 FROM children WHERE id=$2 AND family_id=$1 AND archived_at IS NULL) RETURNING id,child_id,title,coalesce(description,''),due_date,points,state::text,created_at,updated_at,version`, familyID, in.ChildID, in.Title, in.Description, in.Points, in.DueDate).Scan(&t.ID, &t.ChildID, &t.Title, &t.Description, &t.DueDate, &t.Points, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, false, ErrNotFound
	}
	if err != nil {
		return Task{}, false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO occurrences(family_id,task_id,child_id,local_date,title_snapshot,points_snapshot,source_type,due_date,item_type_snapshot) VALUES($1,$2,$3,$4,$5,$6,'task',$4,'task')`, familyID, t.ID, t.ChildID, t.DueDate, t.Title, t.Points)
	if err == nil {
		err = finish(ctx, tx, familyID, sessionID, "tasks.create", key, t, 201)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,after_status,idempotency_key) VALUES($1,$2,$3,'task.created','task',$4,'active',$5)`, familyID, userID, sessionID, t.ID, key)
	}
	if err != nil {
		return Task{}, false, err
	}
	return t, false, tx.Commit(ctx)
}

func (s *Service) UpdateTask(ctx context.Context, sessionID, userID, familyID, id string, in TaskInput) (Task, error) {
	t, _, err := s.UpdateTaskConditional(ctx, sessionID, userID, familyID, id, "", nil, nil, in)
	return t, err
}

func (s *Service) UpdateTaskConditional(ctx context.Context, sessionID, userID, familyID, id, key string, hash []byte, expected *int64, in TaskInput) (Task, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Task{}, false, err
	}
	defer tx.Rollback(ctx)
	if err = requireParent(ctx, tx, sessionID, userID, familyID); err != nil {
		return Task{}, false, err
	}
	body, replay, err := reserve(ctx, tx, familyID, sessionID, "tasks.update", key, hash)
	if err != nil {
		return Task{}, false, err
	}
	if replay {
		var t Task
		err = json.Unmarshal(body, &t)
		return t, true, err
	}
	var t Task
	err = tx.QueryRow(ctx, `SELECT t.id,t.child_id,t.title,coalesce(t.description,''),t.due_date,t.points,t.state::text,t.created_at,t.updated_at,t.version FROM one_off_tasks t JOIN occurrences o ON o.task_id=t.id WHERE t.id=$1 AND t.family_id=$2 AND t.state='active' AND o.state='not_started' AND NOT EXISTS(SELECT 1 FROM completion_attempts WHERE occurrence_id=o.id) FOR UPDATE OF t,o`, id, familyID).Scan(&t.ID, &t.ChildID, &t.Title, &t.Description, &t.DueDate, &t.Points, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM one_off_tasks WHERE id=$1 AND family_id=$2)`, id, familyID).Scan(&exists)
		if exists {
			return Task{}, false, ErrConflict
		}
		return Task{}, false, ErrNotFound
	}
	if err != nil {
		return Task{}, false, err
	}
	if !versionMatches(t.Version, expected) {
		return Task{}, false, ErrVersionConflict
	}
	if in.Title == "" {
		in.Title = t.Title
	}
	if !in.DescriptionSet && in.Description == "" {
		in.Description = t.Description
	}
	if in.DueDate.IsZero() {
		in.DueDate = t.DueDate
	}
	if in.Points == 0 {
		in.Points = t.Points
	}
	err = tx.QueryRow(ctx, `UPDATE one_off_tasks SET title=$3,description=nullif($4,''),due_date=$5,points=$6,updated_at=now(),version=version+1 WHERE id=$1 AND family_id=$2 RETURNING id,child_id,title,coalesce(description,''),due_date,points,state::text,created_at,updated_at,version`, id, familyID, in.Title, in.Description, in.DueDate, in.Points).Scan(&t.ID, &t.ChildID, &t.Title, &t.Description, &t.DueDate, &t.Points, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.Version)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE occurrences SET local_date=$2,due_date=$2,title_snapshot=$3,points_snapshot=$4,updated_at=now(),version=version+1 WHERE task_id=$1`, id, in.DueDate, in.Title, in.Points)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status,idempotency_key) VALUES($1,$2,$3,'task.updated','task',$4,'active','active',nullif($5,''))`, familyID, userID, sessionID, id, key)
	}
	if err == nil {
		err = finish(ctx, tx, familyID, sessionID, "tasks.update", key, t, 200)
	}
	if err != nil {
		return Task{}, false, err
	}
	return t, false, tx.Commit(ctx)
}

func (s *Service) CancelTask(ctx context.Context, sessionID, userID, familyID, id string) error {
	_, err := s.CancelTaskConditional(ctx, sessionID, userID, familyID, id, "", nil, nil, "Cancelled by parent")
	return err
}

func (s *Service) CancelTaskConditional(ctx context.Context, sessionID, userID, familyID, id, key string, hash []byte, expected *int64, reason string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if err = requireParent(ctx, tx, sessionID, userID, familyID); err != nil {
		return false, err
	}
	_, replay, err := reserve(ctx, tx, familyID, sessionID, "tasks.cancel", key, hash)
	if err != nil {
		return false, err
	}
	if replay {
		return true, nil
	}
	var current int64
	err = tx.QueryRow(ctx, `SELECT t.version FROM one_off_tasks t JOIN occurrences o ON o.task_id=t.id WHERE t.id=$1 AND t.family_id=$2 AND t.state='active' AND o.state IN ('not_started','pending_approval') FOR UPDATE OF t,o`, id, familyID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM one_off_tasks WHERE id=$1 AND family_id=$2)`, id, familyID).Scan(&exists)
		if exists {
			return false, ErrConflict
		}
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if !versionMatches(current, expected) {
		return false, ErrVersionConflict
	}
	tag, err := tx.Exec(ctx, `UPDATE one_off_tasks SET state='cancelled',cancellation_reason=$3,updated_at=now(),version=version+1 WHERE id=$1 AND family_id=$2`, id, familyID, reason)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM one_off_tasks WHERE id=$1 AND family_id=$2)`, id, familyID).Scan(&exists)
		if exists {
			return false, ErrConflict
		}
		return false, ErrNotFound
	}
	_, err = tx.Exec(ctx, `UPDATE completion_attempts SET decision='cancelled',decided_at=now() WHERE occurrence_id=(SELECT id FROM occurrences WHERE task_id=$1) AND decision='pending'`, id)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE occurrences SET state='cancelled',updated_at=now(),version=version+1 WHERE task_id=$1`, id)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status,idempotency_key,reason) VALUES($1,$2,$3,'task.cancelled','task',$4,'active','cancelled',nullif($5,''),$6)`, familyID, userID, sessionID, id, key, reason)
	}
	if err == nil {
		err = finish(ctx, tx, familyID, sessionID, "tasks.cancel", key, nil, 204)
	}
	if err != nil {
		return false, err
	}
	return false, tx.Commit(ctx)
}

// Materialize lazily creates immutable recurring occurrences for one household-local date.
// The unique assignment/child/date identity and ON CONFLICT make concurrent calls deterministic.
func (s *Service) Materialize(ctx context.Context, familyID, childID string, date time.Time) ([]Occurrence, error) {
	if err := s.ensureDate(ctx, familyID, childID, date); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id,child_id,item_type_snapshot,local_date,title_snapshot,description_snapshot,icon_snapshot,color_snapshot,points_snapshot,state::text FROM occurrences WHERE family_id=$1 AND child_id=$2 AND ((source_type='habit' AND local_date=$3) OR (source_type='task' AND local_date<=$3 AND state NOT IN('approved','approval_reversed','cancelled'))) ORDER BY local_date,id`, familyID, childID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Occurrence{}
	for rows.Next() {
		var o Occurrence
		if err = rows.Scan(&o.ID, &o.ChildID, &o.Type, &o.LocalDate, &o.Title, &o.Description, &o.Icon, &o.Color, &o.Points, &o.Status); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// EnsureDate materializes recurring occurrences for every active child on a
// household-local calendar date. It is safe to call concurrently.
func (s *Service) EnsureDate(ctx context.Context, familyID string, date time.Time) error {
	return s.ensureDate(ctx, familyID, "", date)
}

func (s *Service) ensureDate(ctx context.Context, familyID, childID string, date time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id FROM children WHERE family_id=$1 AND archived_at IS NULL AND ($2='' OR id::text=$2) ORDER BY id FOR SHARE`, familyID, childID)
	if err != nil {
		return err
	}
	for rows.Next() {
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = tx.Query(ctx, `SELECT id FROM habits WHERE family_id=$1 AND (inactive_from IS NULL OR inactive_from>$2) ORDER BY id FOR SHARE`, familyID, date)
	if err != nil {
		return err
	}
	for rows.Next() {
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO occurrences(family_id,assignment_id,child_id,local_date,title_snapshot,description_snapshot,icon_snapshot,color_snapshot,points_snapshot,source_type,item_type_snapshot) SELECT a.family_id,a.id,a.child_id,$3,v.title,coalesce(v.description,''),coalesce(v.icon,''),coalesce(v.color,''),a.points,'habit','habit' FROM habit_assignments a JOIN children c ON c.id=a.child_id AND c.family_id=a.family_id JOIN habits h ON h.id=a.habit_id AND h.family_id=a.family_id JOIN habit_schedules sc ON sc.assignment_id=a.id JOIN habit_versions v ON v.habit_id=h.id AND v.effective_from<=$3 AND (v.effective_until IS NULL OR v.effective_until>=$3) WHERE a.family_id=$1 AND ($2='' OR a.child_id::text=$2) AND c.archived_at IS NULL AND a.effective_from<=$3 AND (a.effective_until IS NULL OR a.effective_until>=$3) AND (h.inactive_from IS NULL OR h.inactive_from>$3) AND (sc.kind='daily' OR (sc.kind='weekdays' AND extract(dow from $3::date)::smallint=ANY(sc.weekdays))) ON CONFLICT (assignment_id,child_id,local_date) DO NOTHING`, familyID, childID, date)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func uniqueChildIDs(inputs []AssignmentInput) []string {
	seen := make(map[string]struct{}, len(inputs))
	out := make([]string, 0, len(inputs))
	for _, in := range inputs {
		if _, ok := seen[in.ChildID]; ok {
			continue
		}
		seen[in.ChildID] = struct{}{}
		out = append(out, in.ChildID)
	}
	return out
}

func mapDB(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && (pe.Code == "23P01" || pe.Code == "23505") {
		return ErrConflict
	}
	return err
}
