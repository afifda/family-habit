package routines

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrForbidden       = errors.New("forbidden")
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("conflict")
	ErrVersionConflict = errors.New("version conflict")
	ErrIdempotency     = errors.New("idempotency conflict")
	ErrValidation      = errors.New("validation")
)
var allowedIcons = map[string]bool{"": true, "🌅": true, "☀️": true, "🏫": true, "🌆": true, "🌙": true, "⭐": true, "🎁": true, "🍦": true, "🎮": true, "🎬": true, "📚": true, "🚲": true, "🍕": true, "🎨": true, "⚽": true}

type Service struct{ pool *pgxpool.Pool }

func NewService(p *pgxpool.Pool) *Service { return &Service{p} }

type Group struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Icon          string     `json:"icon"`
	Color         string     `json:"color"`
	StartsAtLocal string     `json:"startsAtLocal"`
	EndsAtLocal   string     `json:"endsAtLocal"`
	SortOrder     int        `json:"sortOrder"`
	ArchivedAt    *time.Time `json:"archivedAt"`
	Version       int64      `json:"version"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}
type Input struct {
	Name, Icon, Color, StartsAtLocal, EndsAtLocal string
	SortOrder                                     int
}
type UpdateInput struct {
	Name                                      *string
	Icon, Color, StartsAtLocal, EndsAtLocal   *string
	IconSet, ColorSet, StartsAtSet, EndsAtSet bool
	SortOrder                                 *int
}

func parent(c context.Context, t pgx.Tx, s, f string) (string, error) {
	var u string
	e := t.QueryRow(c, `SELECT s.user_id FROM sessions s JOIN family_memberships m ON (m.family_id,m.user_id)=(s.family_id,s.user_id) JOIN families f ON f.id=s.family_id WHERE s.id=$1 AND s.family_id=$2 AND s.mode='parent' AND s.revoked_at IS NULL AND s.expires_at>now() AND m.role='owner' AND now()<s.last_activity_at+make_interval(mins=>f.parent_idle_minutes)`, s, f).Scan(&u)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrForbidden
	}
	return u, e
}
func reserve(c context.Context, t pgx.Tx, f, s, r, k string, h []byte) ([]byte, bool, error) {
	tag, e := t.Exec(c, `INSERT INTO idempotency_records(family_id,session_id,route_family,idempotency_key,request_hash,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '24 hours') ON CONFLICT DO NOTHING`, f, s, r, k, h)
	if e != nil {
		return nil, false, e
	}
	var old, b []byte
	e = t.QueryRow(c, `SELECT request_hash,response_body FROM idempotency_records WHERE family_id=$1 AND session_id=$2 AND route_family=$3 AND idempotency_key=$4 FOR UPDATE`, f, s, r, k).Scan(&old, &b)
	if e != nil {
		return nil, false, e
	}
	if !bytes.Equal(old, h) {
		return nil, false, ErrIdempotency
	}
	return b, tag.RowsAffected() == 0 && len(b) > 0, nil
}
func finish(c context.Context, t pgx.Tx, f, s, r, k string, status int, v any) error {
	b, e := json.Marshal(v)
	if e == nil {
		_, e = t.Exec(c, `UPDATE idempotency_records SET response_status=$5,response_body=$6 WHERE family_id=$1 AND session_id=$2 AND route_family=$3 AND idempotency_key=$4`, f, s, r, k, status, b)
	}
	return e
}

const cols = `id,name,coalesce(icon,''),coalesce(color,''),starts_at_local,ends_at_local,sort_order,archived_at,version,created_at,updated_at`

func scan(r pgx.Row) (Group, error) {
	var g Group
	var a, b *time.Time
	e := r.Scan(&g.ID, &g.Name, &g.Icon, &g.Color, &a, &b, &g.SortOrder, &g.ArchivedAt, &g.Version, &g.CreatedAt, &g.UpdatedAt)
	if a != nil {
		g.StartsAtLocal = a.Format("15:04")
	}
	if b != nil {
		g.EndsAtLocal = b.Format("15:04")
	}
	return g, e
}
func (s *Service) List(c context.Context, f string, arch bool) ([]Group, error) {
	rows, e := s.pool.Query(c, `SELECT `+cols+` FROM routine_groups WHERE family_id=$1 AND ($2 OR archived_at IS NULL) ORDER BY archived_at NULLS FIRST,sort_order,id`, f, arch)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		g, x := scan(rows)
		if x != nil {
			return nil, x
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
func (s *Service) Create(c context.Context, sid, f, k string, h []byte, in Input) (Group, bool, error) {
	if !allowedIcons[in.Icon] {
		return Group{}, false, ErrValidation
	}
	tx, e := s.pool.Begin(c)
	if e != nil {
		return Group{}, false, e
	}
	defer tx.Rollback(c)
	actor, e := parent(c, tx, sid, f)
	if e != nil {
		return Group{}, false, e
	}
	b, re, e := reserve(c, tx, f, sid, "routine.create", k, h)
	if e != nil {
		return Group{}, false, e
	}
	if re {
		var g Group
		e = json.Unmarshal(b, &g)
		return g, true, e
	}
	g, e := scan(tx.QueryRow(c, `INSERT INTO routine_groups(family_id,name,icon,color,starts_at_local,ends_at_local,sort_order) VALUES($1,$2,nullif($3,''),nullif($4,''),nullif($5,'')::time,nullif($6,'')::time,(SELECT count(*) FROM routine_groups WHERE family_id=$1 AND archived_at IS NULL)) RETURNING `+cols, f, in.Name, in.Icon, in.Color, in.StartsAtLocal, in.EndsAtLocal))
	if e == nil {
		e = finish(c, tx, f, sid, "routine.create", k, 201, g)
	}
	if e == nil {
		_, e = tx.Exec(c, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,after_status,idempotency_key) VALUES($1,$2,$3,'routine_group.created','routine_group',$4,'active',$5)`, f, actor, sid, g.ID, k)
	}
	if e != nil {
		return Group{}, false, e
	}
	return g, false, tx.Commit(c)
}
func (s *Service) Update(c context.Context, sid, f, id, k string, h []byte, v int64, in UpdateInput) (Group, bool, error) {
	if in.IconSet && !allowedIcons[stringValue(in.Icon)] {
		return Group{}, false, ErrValidation
	}
	tx, e := s.pool.Begin(c)
	if e != nil {
		return Group{}, false, e
	}
	defer tx.Rollback(c)
	actor, e := parent(c, tx, sid, f)
	if e != nil {
		return Group{}, false, e
	}
	b, re, e := reserve(c, tx, f, sid, "routine.update", k, h)
	if e != nil {
		return Group{}, false, e
	}
	if re {
		var g Group
		e = json.Unmarshal(b, &g)
		return g, true, e
	}
	var cur int64
	e = tx.QueryRow(c, `SELECT version FROM routine_groups WHERE id=$1 AND family_id=$2 AND archived_at IS NULL FOR UPDATE`, id, f).Scan(&cur)
	if errors.Is(e, pgx.ErrNoRows) {
		return Group{}, false, ErrNotFound
	}
	if e != nil {
		return Group{}, false, e
	}
	if cur != v {
		return Group{}, false, ErrVersionConflict
	}
	g, e := scan(tx.QueryRow(c, `UPDATE routine_groups SET
		name=coalesce($3,name),
		icon=CASE WHEN $4 THEN nullif($5,'') ELSE icon END,
		color=CASE WHEN $6 THEN nullif($7,'') ELSE color END,
		starts_at_local=CASE WHEN $8 THEN nullif($9,'')::time ELSE starts_at_local END,
		ends_at_local=CASE WHEN $10 THEN nullif($11,'')::time ELSE ends_at_local END,
		sort_order=coalesce($12,sort_order),version=version+1,updated_at=now()
		WHERE id=$1 AND family_id=$2 RETURNING `+cols, id, f, in.Name, in.IconSet, stringValue(in.Icon), in.ColorSet, stringValue(in.Color), in.StartsAtSet, stringValue(in.StartsAtLocal), in.EndsAtSet, stringValue(in.EndsAtLocal), in.SortOrder))
	if e == nil {
		e = finish(c, tx, f, sid, "routine.update", k, 200, g)
	}
	if e == nil {
		_, e = tx.Exec(c, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status,idempotency_key) VALUES($1,$2,$3,'routine_group.updated','routine_group',$4,'active','active',$5)`, f, actor, sid, id, k)
	}
	if e != nil {
		return Group{}, false, e
	}
	return g, false, tx.Commit(c)
}
func (s *Service) Reorder(c context.Context, sid, f, k string, h []byte, ids []string, versions map[string]int64) ([]Group, bool, error) {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return nil, false, e
	}
	defer tx.Rollback(c)
	actor, e := parent(c, tx, sid, f)
	if e != nil {
		return nil, false, e
	}
	b, re, e := reserve(c, tx, f, sid, "routine.order", k, h)
	if e != nil {
		return nil, false, e
	}
	if re {
		var out []Group
		e = json.Unmarshal(b, &out)
		return out, true, e
	}
	var count int
	e = tx.QueryRow(c, `SELECT count(*) FROM routine_groups WHERE family_id=$1 AND archived_at IS NULL`, f).Scan(&count)
	if e != nil {
		return nil, false, e
	}
	if count != len(ids) {
		return nil, false, ErrConflict
	}
	// Move the complete set out of the destination range before assigning dense
	// positions; this is compatible with the active `(family,sort_order)` guard.
	if _, e = tx.Exec(c, `UPDATE routine_groups SET sort_order=sort_order+$2 WHERE family_id=$1 AND archived_at IS NULL`, f, count); e != nil {
		return nil, false, e
	}
	for i, id := range ids {
		tag, x := tx.Exec(c, `UPDATE routine_groups SET sort_order=$3,version=version+1,updated_at=now() WHERE id=$1 AND family_id=$2 AND archived_at IS NULL AND version=$4`, id, f, i, versions[id])
		if x != nil {
			return nil, false, x
		}
		if tag.RowsAffected() != 1 {
			return nil, false, ErrVersionConflict
		}
	}
	rows, e := tx.Query(c, `SELECT `+cols+` FROM routine_groups WHERE family_id=$1 AND archived_at IS NULL ORDER BY sort_order,id`, f)
	if e != nil {
		return nil, false, e
	}
	out := []Group{}
	for rows.Next() {
		g, x := scan(rows)
		if x != nil {
			rows.Close()
			return nil, false, x
		}
		out = append(out, g)
	}
	rows.Close()
	if e = rows.Err(); e == nil {
		e = finish(c, tx, f, sid, "routine.order", k, 200, out)
	}
	if e == nil {
		_, e = tx.Exec(c, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,after_status,idempotency_key) VALUES($1,$2,$3,'routine_groups.reordered','routine_group','active',$4)`, f, actor, sid, k)
	}
	if e != nil {
		return nil, false, e
	}
	return out, false, tx.Commit(c)
}
func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (s *Service) Archive(c context.Context, sid, f, id, k string, h []byte, v int64, effective time.Time, dest *string) (Group, bool, error) {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return Group{}, false, e
	}
	defer tx.Rollback(c)
	actor, e := parent(c, tx, sid, f)
	if e != nil {
		return Group{}, false, e
	}
	b, replay, e := reserve(c, tx, f, sid, "routine.archive", k, h)
	if e != nil {
		return Group{}, false, e
	}
	if replay {
		var g Group
		e = json.Unmarshal(b, &g)
		return g, true, e
	}
	var current int64
	e = tx.QueryRow(c, `SELECT version FROM routine_groups WHERE id=$1 AND family_id=$2 AND archived_at IS NULL FOR UPDATE`, id, f).Scan(&current)
	if errors.Is(e, pgx.ErrNoRows) {
		return Group{}, false, ErrNotFound
	}
	if e != nil {
		return Group{}, false, e
	}
	if current != v {
		return Group{}, false, ErrVersionConflict
	}
	if dest != nil {
		var ok bool
		e = tx.QueryRow(c, `SELECT EXISTS(SELECT 1 FROM routine_groups WHERE id=$1 AND family_id=$2 AND archived_at IS NULL AND id<>$3)`, *dest, f, id).Scan(&ok)
		if e != nil {
			return Group{}, false, e
		}
		if !ok {
			return Group{}, false, ErrNotFound
		}
	}
	var blocked int
	e = tx.QueryRow(c, `SELECT count(*) FROM one_off_tasks t JOIN occurrences o ON o.task_id=t.id WHERE t.family_id=$1 AND t.routine_group_id=$2 AND (o.state<>'not_started' OR EXISTS(SELECT 1 FROM completion_attempts a WHERE a.occurrence_id=o.id))`, f, id).Scan(&blocked)
	if e != nil {
		return Group{}, false, e
	}
	if blocked > 0 {
		return Group{}, false, ErrConflict
	}
	// Snapshot changes must be limited to editable tasks that actually belonged to
	// this group. Update their occurrences before moving the source rows.
	_, e = tx.Exec(c, `UPDATE occurrences o SET routine_group_id_snapshot=g.id,routine_group_name_snapshot=g.name,routine_group_icon_snapshot=g.icon,routine_group_color_snapshot=g.color,routine_group_sort_order_snapshot=g.sort_order,version=o.version+1,updated_at=now()
		FROM one_off_tasks t LEFT JOIN routine_groups g ON g.id=$3 AND g.family_id=$1
		WHERE o.task_id=t.id AND t.family_id=$1 AND t.routine_group_id=$2 AND o.state='not_started' AND NOT EXISTS(SELECT 1 FROM completion_attempts a WHERE a.occurrence_id=o.id)`, f, id, dest)
	if e == nil {
		_, e = tx.Exec(c, `UPDATE one_off_tasks SET routine_group_id=$3,updated_at=now(),version=version+1 WHERE family_id=$1 AND routine_group_id=$2`, f, id, dest)
	}
	if e != nil {
		return Group{}, false, e
	}
	// Recurring membership is effective-dated. Preserve every assignment before
	// effectiveFrom and replace only the portion on/after that local date.
	rows, e := tx.Query(c, `SELECT a.id,a.habit_id,a.child_id,a.points,a.sort_order,a.effective_from,a.effective_until,s.kind::text,s.weekdays
		FROM habit_assignments a JOIN habit_schedules s ON s.assignment_id=a.id
		WHERE a.family_id=$1 AND a.routine_group_id=$2 AND (a.effective_until IS NULL OR a.effective_until >= $3::date)
		ORDER BY a.effective_from,a.id FOR UPDATE OF a,s`, f, id, effective)
	if e != nil {
		return Group{}, false, e
	}
	type assignment struct {
		id, habit, child, kind string
		points, order          int32
		from                   time.Time
		until                  *time.Time
		weekdays               []int16
	}
	assignments := []assignment{}
	for rows.Next() {
		var a assignment
		if e = rows.Scan(&a.id, &a.habit, &a.child, &a.points, &a.order, &a.from, &a.until, &a.kind, &a.weekdays); e != nil {
			rows.Close()
			return Group{}, false, e
		}
		assignments = append(assignments, a)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return Group{}, false, e
	}
	for _, a := range assignments {
		if !a.from.Before(effective) {
			_, e = tx.Exec(c, `UPDATE habit_assignments SET routine_group_id=$2,version=version+1 WHERE id=$1`, a.id, dest)
		} else {
			_, e = tx.Exec(c, `DELETE FROM occurrences WHERE assignment_id=$1 AND local_date >= $2::date AND state='not_started' AND NOT EXISTS(SELECT 1 FROM completion_attempts ca WHERE ca.occurrence_id=occurrences.id)`, a.id, effective)
			if e == nil {
				_, e = tx.Exec(c, `UPDATE habit_assignments SET effective_until=$2::date-1,version=version+1 WHERE id=$1`, a.id, effective)
			}
			var replacement string
			if e == nil {
				e = tx.QueryRow(c, `INSERT INTO habit_assignments(family_id,habit_id,child_id,points,effective_from,effective_until,supersedes_assignment_id,routine_group_id,sort_order) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, f, a.habit, a.child, a.points, effective, a.until, a.id, dest, a.order).Scan(&replacement)
			}
			if e == nil {
				_, e = tx.Exec(c, `INSERT INTO habit_schedules(assignment_id,kind,weekdays) VALUES($1,$2,$3)`, replacement, a.kind, a.weekdays)
			}
		}
		if e != nil {
			return Group{}, false, e
		}
	}
	g, e := scan(tx.QueryRow(c, `UPDATE routine_groups SET archived_at=now(),version=version+1,updated_at=now() WHERE id=$1 RETURNING `+cols, id))
	if e != nil {
		return Group{}, false, e
	}
	e = finish(c, tx, f, sid, "routine.archive", k, 200, g)
	if e == nil {
		_, e = tx.Exec(c, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status,idempotency_key) VALUES($1,$2,$3,'routine_group.archived','routine_group',$4,'active','archived',$5)`, f, actor, sid, id, k)
	}
	if e != nil {
		return Group{}, false, e
	}
	var activeCount int
	if e = tx.QueryRow(c, `SELECT count(*) FROM routine_groups WHERE family_id=$1 AND archived_at IS NULL`, f).Scan(&activeCount); e != nil {
		return Group{}, false, e
	}
	if activeCount > 0 {
		_, e = tx.Exec(c, `UPDATE routine_groups SET sort_order=sort_order+$2 WHERE family_id=$1 AND archived_at IS NULL`, f, activeCount)
	}
	if e == nil {
		_, e = tx.Exec(c, `WITH ranked AS (SELECT id,row_number() OVER(ORDER BY sort_order,id)-1 AS n FROM routine_groups WHERE family_id=$1 AND archived_at IS NULL) UPDATE routine_groups g SET sort_order=ranked.n,version=g.version+1,updated_at=now() FROM ranked WHERE g.id=ranked.id`, f)
	}
	if e != nil {
		return Group{}, false, e
	}
	return g, false, tx.Commit(c)
}
