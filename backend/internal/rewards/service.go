package rewards

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

var (
	ErrForbidden           = errors.New("forbidden")
	ErrNotFound            = errors.New("not found")
	ErrDisabled            = errors.New("rewards disabled")
	ErrUnavailable         = errors.New("reward unavailable")
	ErrInsufficient        = errors.New("insufficient points")
	ErrInvalidState        = errors.New("invalid state")
	ErrVersionConflict     = errors.New("version conflict")
	ErrIdempotency         = errors.New("idempotency conflict")
	ErrCursor              = errors.New("invalid cursor")
	ErrValidation          = errors.New("validation")
	ErrEligibility         = errors.New("reward eligibility required")
	ErrEligibilityNotFinal = errors.New("reward eligibility not final")
	ErrEligibilityNotMet   = errors.New("reward eligibility not met")
	ErrEligibilityExpired  = errors.New("reward eligibility expired")
	ErrRedemptionLimit     = errors.New("reward redemption limit reached")
)
var allowedIcons = map[string]bool{"": true, "🌅": true, "☀️": true, "🏫": true, "🌆": true, "🌙": true, "⭐": true, "🎁": true, "🍦": true, "🎮": true, "🎬": true, "📚": true, "🚲": true, "🍕": true, "🎨": true, "⚽": true}

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewService(p *pgxpool.Pool) *Service { return &Service{pool: p, now: time.Now} }

type Reward struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	Description       string    `json:"description"`
	Icon              string    `json:"icon"`
	CostPoints        int64     `json:"costPoints"`
	AvailabilityScope string    `json:"availabilityScope"`
	EligibleChildIDs  []string  `json:"eligibleChildIds"`
	Active            bool      `json:"active"`
	Version           int64     `json:"version"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	CanRedeem         bool      `json:"canRedeem,omitempty"`
	ShortfallPoints   int64     `json:"shortfallPoints,omitempty"`
}
type ChildReward struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Icon            string `json:"icon"`
	CostPoints      int64  `json:"costPoints"`
	Version         int64  `json:"version"`
	CanRedeem       bool   `json:"canRedeem"`
	ShortfallPoints int64  `json:"shortfallPoints"`
}
type RewardInput struct {
	Title, Description, Icon, AvailabilityScope string
	CostPoints                                  int64
	EligibleChildIDs                            []string
}
type Redemption struct {
	ID                  string     `json:"id"`
	ChildID             string     `json:"childId"`
	ChildName           string     `json:"childName,omitempty"`
	RewardID            string     `json:"rewardId"`
	RewardTitle         string     `json:"rewardTitle"`
	RewardIcon          string     `json:"rewardIcon"`
	CostPoints          int64      `json:"costPoints"`
	State               string     `json:"state"`
	RequestedAt         time.Time  `json:"requestedAt"`
	DecidedAt           *time.Time `json:"decidedAt"`
	CancellationReason  string     `json:"cancellationReason,omitempty"`
	DebitLedgerEntryID  string     `json:"debitLedgerEntryId"`
	RefundLedgerEntryID string     `json:"refundLedgerEntryId,omitempty"`
	Version             int64      `json:"version"`
}

func parent(c context.Context, t pgx.Tx, s, f string) (string, error) {
	var u string
	e := t.QueryRow(c, `SELECT s.user_id FROM sessions s JOIN family_memberships m ON(m.family_id,m.user_id)=(s.family_id,s.user_id) JOIN families f ON f.id=s.family_id WHERE s.id=$1 AND s.family_id=$2 AND s.mode='parent' AND s.revoked_at IS NULL AND s.expires_at>now() AND m.role='owner' AND now()<s.last_activity_at+make_interval(mins=>f.parent_idle_minutes)`, s, f).Scan(&u)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrForbidden
	}
	return u, e
}
func child(c context.Context, t pgx.Tx, s, f string) (string, error) {
	var id string
	e := t.QueryRow(c, `SELECT c.id FROM sessions s JOIN children c ON c.id=s.active_child_id AND c.family_id=s.family_id AND c.archived_at IS NULL WHERE s.id=$1 AND s.family_id=$2 AND s.mode='child' AND s.revoked_at IS NULL AND s.expires_at>now() FOR UPDATE OF c`, s, f).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrForbidden
	}
	return id, e
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

const rewardCols = `id,title,coalesce(description,''),coalesce(icon,''),cost_points,availability_scope,archived_at IS NULL,version,created_at,updated_at`

func scanReward(row pgx.Row) (Reward, error) {
	var r Reward
	e := row.Scan(&r.ID, &r.Title, &r.Description, &r.Icon, &r.CostPoints, &r.AvailabilityScope, &r.Active, &r.Version, &r.CreatedAt, &r.UpdatedAt)
	r.EligibleChildIDs = []string{}
	return r, e
}
func (s *Service) eligible(c context.Context, r *Reward) error {
	rows, e := s.pool.Query(c, `SELECT child_id FROM reward_child_availability WHERE reward_id=$1 ORDER BY child_id`, r.ID)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return e
		}
		r.EligibleChildIDs = append(r.EligibleChildIDs, id)
	}
	return rows.Err()
}
func (s *Service) List(c context.Context, f string, arch bool) ([]Reward, error) {
	rows, e := s.pool.Query(c, `SELECT `+rewardCols+` FROM rewards r WHERE r.family_id=$1 AND ($2 OR r.archived_at IS NULL) ORDER BY r.archived_at NULLS FIRST,lower(r.title),r.id`, f, arch)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Reward{}
	for rows.Next() {
		r, x := scanReward(rows)
		if x != nil {
			return nil, x
		}
		if x = s.eligible(c, &r); x != nil {
			return nil, x
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func replaceEligibility(c context.Context, t pgx.Tx, f, reward, scope string, ids []string) error {
	_, e := t.Exec(c, `DELETE FROM reward_child_availability WHERE reward_id=$1`, reward)
	if e != nil {
		return e
	}
	if scope == "selected_children" {
		if len(ids) == 0 {
			return ErrUnavailable
		}
		for _, id := range ids {
			tag, x := t.Exec(c, `INSERT INTO reward_child_availability(family_id,reward_id,child_id) SELECT $1,$2,$3 WHERE EXISTS(SELECT 1 FROM children WHERE id=$3 AND family_id=$1 AND archived_at IS NULL)`, f, reward, id)
			if x != nil {
				return x
			}
			if tag.RowsAffected() != 1 {
				return ErrNotFound
			}
		}
	}
	return nil
}
func (s *Service) Create(c context.Context, sid, f, k string, h []byte, in RewardInput) (Reward, bool, error) {
	if !allowedIcons[in.Icon] {
		return Reward{}, false, ErrValidation
	}
	tx, e := s.pool.Begin(c)
	if e != nil {
		return Reward{}, false, e
	}
	defer tx.Rollback(c)
	actor, e := parent(c, tx, sid, f)
	if e != nil {
		return Reward{}, false, e
	}
	b, re, e := reserve(c, tx, f, sid, "rewards.create", k, h)
	if e != nil {
		return Reward{}, false, e
	}
	if re {
		var r Reward
		e = json.Unmarshal(b, &r)
		return r, true, e
	}
	r, e := scanReward(tx.QueryRow(c, `INSERT INTO rewards(family_id,title,description,icon,cost_points,availability_scope) VALUES($1,$2,nullif($3,''),nullif($4,''),$5,$6) RETURNING `+rewardCols, f, in.Title, in.Description, in.Icon, in.CostPoints, in.AvailabilityScope))
	if e == nil {
		e = replaceEligibility(c, tx, f, r.ID, in.AvailabilityScope, in.EligibleChildIDs)
	}
	r.EligibleChildIDs = in.EligibleChildIDs
	if e == nil {
		e = finish(c, tx, f, sid, "rewards.create", k, 201, r)
	}
	if e == nil {
		_, e = tx.Exec(c, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,after_status,idempotency_key) VALUES($1,$2,$3,'reward.created','reward',$4,'active',$5)`, f, actor, sid, r.ID, k)
	}
	if e != nil {
		return Reward{}, false, e
	}
	return r, false, tx.Commit(c)
}
func (s *Service) Update(c context.Context, sid, f, id, k string, h []byte, v int64, in RewardInput) (Reward, bool, error) {
	if !allowedIcons[in.Icon] {
		return Reward{}, false, ErrValidation
	}
	tx, e := s.pool.Begin(c)
	if e != nil {
		return Reward{}, false, e
	}
	defer tx.Rollback(c)
	actor, e := parent(c, tx, sid, f)
	if e != nil {
		return Reward{}, false, e
	}
	b, re, e := reserve(c, tx, f, sid, "rewards.update", k, h)
	if e != nil {
		return Reward{}, false, e
	}
	if re {
		var r Reward
		e = json.Unmarshal(b, &r)
		return r, true, e
	}
	var cur int64
	e = tx.QueryRow(c, `SELECT version FROM rewards WHERE id=$1 AND family_id=$2 AND archived_at IS NULL FOR UPDATE`, id, f).Scan(&cur)
	if errors.Is(e, pgx.ErrNoRows) {
		return Reward{}, false, ErrNotFound
	}
	if e != nil {
		return Reward{}, false, e
	}
	if cur != v {
		return Reward{}, false, ErrVersionConflict
	}
	r, e := scanReward(tx.QueryRow(c, `UPDATE rewards r SET title=$3,description=nullif($4,''),icon=nullif($5,''),cost_points=$6,availability_scope=$7,version=version+1,updated_at=now() WHERE id=$1 AND family_id=$2 RETURNING `+rewardCols, id, f, in.Title, in.Description, in.Icon, in.CostPoints, in.AvailabilityScope))
	if e == nil {
		e = replaceEligibility(c, tx, f, id, in.AvailabilityScope, in.EligibleChildIDs)
	}
	r.EligibleChildIDs = in.EligibleChildIDs
	if e == nil {
		e = finish(c, tx, f, sid, "rewards.update", k, 200, r)
	}
	if e == nil {
		_, e = tx.Exec(c, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status,idempotency_key) VALUES($1,$2,$3,'reward.updated','reward',$4,'active','active',$5)`, f, actor, sid, id, k)
	}
	if e != nil {
		return Reward{}, false, e
	}
	return r, false, tx.Commit(c)
}
func (s *Service) Archive(c context.Context, sid, f, id, k string, h []byte, v int64) (Reward, bool, error) {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return Reward{}, false, e
	}
	defer tx.Rollback(c)
	actor, e := parent(c, tx, sid, f)
	if e != nil {
		return Reward{}, false, e
	}
	b, re, e := reserve(c, tx, f, sid, "rewards.archive", k, h)
	if e != nil {
		return Reward{}, false, e
	}
	if re {
		var r Reward
		e = json.Unmarshal(b, &r)
		return r, true, e
	}
	var cur int64
	e = tx.QueryRow(c, `SELECT version FROM rewards WHERE id=$1 AND family_id=$2 AND archived_at IS NULL FOR UPDATE`, id, f).Scan(&cur)
	if errors.Is(e, pgx.ErrNoRows) {
		return Reward{}, false, ErrNotFound
	}
	if e != nil {
		return Reward{}, false, e
	}
	if cur != v {
		return Reward{}, false, ErrVersionConflict
	}
	r, e := scanReward(tx.QueryRow(c, `UPDATE rewards r SET archived_at=now(),version=version+1,updated_at=now() WHERE id=$1 RETURNING `+rewardCols, id))
	if e == nil {
		e = finish(c, tx, f, sid, "rewards.archive", k, 200, r)
	}
	if e == nil {
		_, e = tx.Exec(c, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status,idempotency_key) VALUES($1,$2,$3,'reward.archived','reward',$4,'active','archived',$5)`, f, actor, sid, id, k)
	}
	if e != nil {
		return Reward{}, false, e
	}
	return r, false, tx.Commit(c)
}
func (s *Service) ChildCatalog(c context.Context, sid, f string) ([]ChildReward, int64, error) {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return nil, 0, e
	}
	defer tx.Rollback(c)
	id, e := child(c, tx, sid, f)
	if e != nil {
		return nil, 0, e
	}
	var enabled bool
	e = tx.QueryRow(c, `SELECT rewards_enabled FROM families WHERE id=$1`, f).Scan(&enabled)
	if e != nil {
		return nil, 0, e
	}
	if !enabled {
		return []ChildReward{}, 0, ErrDisabled
	}
	var bal int64
	e = tx.QueryRow(c, `SELECT coalesce(sum(amount),0) FROM point_ledger WHERE family_id=$1 AND child_id=$2`, f, id).Scan(&bal)
	if e != nil {
		return nil, 0, e
	}
	rows, e := tx.Query(c, `SELECT `+rewardCols+` FROM rewards r WHERE r.family_id=$1 AND r.archived_at IS NULL AND (r.availability_scope='all_active_children' OR EXISTS(SELECT 1 FROM reward_child_availability a WHERE a.reward_id=r.id AND a.child_id=$2)) ORDER BY lower(r.title),r.id`, f, id)
	if e != nil {
		return nil, 0, e
	}
	out := []ChildReward{}
	for rows.Next() {
		r, x := scanReward(rows)
		if x != nil {
			rows.Close()
			return nil, 0, x
		}
		childReward := ChildReward{ID: r.ID, Title: r.Title, Description: r.Description, Icon: r.Icon, CostPoints: r.CostPoints, Version: r.Version, CanRedeem: bal >= r.CostPoints}
		if !childReward.CanRedeem {
			childReward.ShortfallPoints = r.CostPoints - bal
		}
		out = append(out, childReward)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return nil, 0, e
	}
	return out, bal, tx.Commit(c)
}

const redemptionCols = `rr.id,rr.child_id,coalesce(c.nickname,''),rr.reward_id,rr.reward_title_snapshot,coalesce(rr.reward_icon_snapshot,''),rr.cost_points_snapshot,rr.state::text,rr.requested_at,rr.decided_at,coalesce(rr.cancellation_reason,''),coalesce(rr.debit_ledger_entry_id::text,''),coalesce(rr.refund_ledger_entry_id::text,''),rr.version`

func scanRedemption(row pgx.Row) (Redemption, error) {
	var r Redemption
	e := row.Scan(&r.ID, &r.ChildID, &r.ChildName, &r.RewardID, &r.RewardTitle, &r.RewardIcon, &r.CostPoints, &r.State, &r.RequestedAt, &r.DecidedAt, &r.CancellationReason, &r.DebitLedgerEntryID, &r.RefundLedgerEntryID, &r.Version)
	return r, e
}
func (s *Service) Redeem(c context.Context, sid, f, reward, k string, h []byte, v, cost int64) (Redemption, bool, error) {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return Redemption{}, false, e
	}
	defer tx.Rollback(c)
	id, e := child(c, tx, sid, f)
	if e != nil {
		return Redemption{}, false, e
	}
	b, re, e := reserve(c, tx, f, sid, "redemptions.create", k, h)
	if e != nil {
		return Redemption{}, false, e
	}
	if re {
		var r Redemption
		e = json.Unmarshal(b, &r)
		return r, true, e
	}
	var enabled bool
	e = tx.QueryRow(c, `SELECT rewards_enabled FROM families WHERE id=$1 FOR SHARE`, f).Scan(&enabled)
	if e != nil {
		return Redemption{}, false, e
	}
	if !enabled {
		return Redemption{}, false, ErrDisabled
	}
	var title, icon string
	var actual, rv int64
	e = tx.QueryRow(c, `SELECT title,coalesce(icon,''),cost_points,version FROM rewards r WHERE r.id=$1 AND r.family_id=$2 AND r.archived_at IS NULL AND (r.availability_scope='all_active_children' OR EXISTS(SELECT 1 FROM reward_child_availability a WHERE a.reward_id=r.id AND a.child_id=$3)) FOR UPDATE`, reward, f, id).Scan(&title, &icon, &actual, &rv)
	if errors.Is(e, pgx.ErrNoRows) {
		return Redemption{}, false, ErrUnavailable
	}
	if e != nil {
		return Redemption{}, false, e
	}
	if rv != v || actual != cost {
		return Redemption{}, false, ErrVersionConflict
	}
	var bal int64
	e = tx.QueryRow(c, `SELECT coalesce(sum(amount),0) FROM point_ledger WHERE family_id=$1 AND child_id=$2`, f, id).Scan(&bal)
	if e != nil {
		return Redemption{}, false, e
	}
	if bal < actual {
		return Redemption{}, false, ErrInsufficient
	}
	eligibility, e := s.ensureEvaluation(c, tx, f, id, s.now())
	if e != nil {
		return Redemption{}, false, e
	}
	if eligibility.PolicyEnabled && !eligibility.CanRedeem {
		if eligibility.UnavailableReason == "redemption_limit_reached" {
			return Redemption{}, false, ErrRedemptionLimit
		}
		switch eligibility.Status {
		case "collecting", "awaiting_evaluation":
			return Redemption{}, false, ErrEligibilityNotFinal
		case "not_eligible":
			return Redemption{}, false, ErrEligibilityNotMet
		default:
			return Redemption{}, false, ErrEligibility
		}
	}
	var rid string
	var evaluationID any
	if eligibility.PolicyEnabled {
		evaluationID = eligibility.ID
	}
	e = tx.QueryRow(c, `INSERT INTO reward_redemptions(family_id,child_id,reward_id,reward_title_snapshot,reward_icon_snapshot,cost_points_snapshot,requested_by_child_id,eligibility_evaluation_id) VALUES($1,$2,$3,$4,nullif($5,''),$6,$2,$7) RETURNING id`, f, id, reward, title, icon, actual, evaluationID).Scan(&rid)
	var debit string
	if e == nil {
		e = tx.QueryRow(c, `INSERT INTO point_ledger(family_id,child_id,kind,amount,actor_child_id,reward_redemption_id) VALUES($1,$2,'reward_redemption',0-$3::bigint,$2,$4) RETURNING id`, f, id, actual, rid).Scan(&debit)
	}
	if e == nil {
		_, e = tx.Exec(c, `UPDATE reward_redemptions SET debit_ledger_entry_id=$2 WHERE id=$1`, rid, debit)
	}
	var out Redemption
	if e == nil {
		out, e = scanRedemption(tx.QueryRow(c, `SELECT `+redemptionCols+` FROM reward_redemptions rr JOIN children c ON c.id=rr.child_id WHERE rr.id=$1`, rid))
	}
	if e == nil {
		e = finish(c, tx, f, sid, "redemptions.create", k, 201, out)
	}
	if e == nil {
		_, e = tx.Exec(c, `INSERT INTO audit_events(family_id,actor_child_id,session_id,action,subject_type,subject_id,after_status,idempotency_key,metadata) VALUES($1,$2,$3,'reward_redemption.requested','reward_redemption',$4,'requested',$5,jsonb_build_object('costPoints',$6::bigint))`, f, id, sid, rid, k, actual)
	}
	if e != nil {
		return Redemption{}, false, e
	}
	return out, false, tx.Commit(c)
}

type listCursor struct {
	Time       time.Time `json:"t"`
	ID         string    `json:"i"`
	Family     string    `json:"f"`
	Projection string    `json:"p"`
	State      string    `json:"s,omitempty"`
	Child      string    `json:"c,omitempty"`
}

func decodeListCursor(raw string) (listCursor, error) {
	var v listCursor
	if raw == "" {
		return v, nil
	}
	b, e := base64.RawURLEncoding.DecodeString(raw)
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}
func encodeListCursor(v listCursor) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}
func (s *Service) ListParent(c context.Context, sid, f, state, childID, cursor string, limit int) ([]Redemption, string, error) {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return nil, "", e
	}
	defer tx.Rollback(c)
	if _, e = parent(c, tx, sid, f); e != nil {
		return nil, "", e
	}
	cur, e := decodeListCursor(cursor)
	if e != nil {
		return nil, "", ErrCursor
	}
	if cursor != "" && (cur.Family != f || cur.Projection != "parent" || cur.State != state || cur.Child != childID) {
		return nil, "", ErrCursor
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, e := tx.Query(c, `SELECT `+redemptionCols+` FROM reward_redemptions rr JOIN children c ON c.id=rr.child_id WHERE rr.family_id=$1 AND ($2='' OR rr.state::text=$2) AND ($3='' OR rr.child_id::text=$3) AND ($4='' OR (rr.requested_at,rr.id)>($5,$4::uuid)) ORDER BY rr.requested_at,rr.id LIMIT $6`, f, state, childID, cur.ID, cur.Time, limit+1)
	if e != nil {
		return nil, "", e
	}
	defer rows.Close()
	out := []Redemption{}
	for rows.Next() {
		r, x := scanRedemption(rows)
		if x != nil {
			return nil, "", x
		}
		out = append(out, r)
	}
	if e = rows.Err(); e != nil {
		return nil, "", e
	}
	next := ""
	if len(out) > limit {
		last := out[limit-1]
		next = encodeListCursor(listCursor{Time: last.RequestedAt, ID: last.ID, Family: f, Projection: "parent", State: state, Child: childID})
		out = out[:limit]
	}
	return out, next, nil
}
func (s *Service) ListChild(c context.Context, sid, f, cursor string, limit int) ([]Redemption, string, error) {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return nil, "", e
	}
	defer tx.Rollback(c)
	id, e := child(c, tx, sid, f)
	if e != nil {
		return nil, "", e
	}
	cur, e := decodeListCursor(cursor)
	if e != nil {
		return nil, "", ErrCursor
	}
	if cursor != "" && (cur.Family != f || cur.Projection != "child" || cur.Child != id) {
		return nil, "", ErrCursor
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, e := tx.Query(c, `SELECT `+redemptionCols+` FROM reward_redemptions rr JOIN children c ON c.id=rr.child_id WHERE rr.family_id=$1 AND rr.child_id=$2 AND ($3='' OR (rr.requested_at,rr.id)<($4,$3::uuid)) ORDER BY rr.requested_at DESC,rr.id DESC LIMIT $5`, f, id, cur.ID, cur.Time, limit+1)
	if e != nil {
		return nil, "", e
	}
	defer rows.Close()
	out := []Redemption{}
	for rows.Next() {
		r, x := scanRedemption(rows)
		if x != nil {
			return nil, "", x
		}
		r.ChildName = ""
		r.CancellationReason = ""
		out = append(out, r)
	}
	if e = rows.Err(); e != nil {
		return nil, "", e
	}
	next := ""
	if len(out) > limit {
		last := out[limit-1]
		next = encodeListCursor(listCursor{Time: last.RequestedAt, ID: last.ID, Family: f, Projection: "child", Child: id})
		out = out[:limit]
	}
	return out, next, nil
}
func (s *Service) Decide(c context.Context, sid, f, id, action, reason, k string, h []byte, v int64) (Redemption, bool, error) {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return Redemption{}, false, e
	}
	defer tx.Rollback(c)
	actor, e := parent(c, tx, sid, f)
	if e != nil {
		return Redemption{}, false, e
	}
	b, re, e := reserve(c, tx, f, sid, "redemptions."+action, k, h)
	if e != nil {
		return Redemption{}, false, e
	}
	if re {
		var r Redemption
		e = json.Unmarshal(b, &r)
		return r, true, e
	}
	var childID, state, debit string
	var cur, cost int64
	// Discover the scoped child without a lock, then take the common child
	// balance lock before the redemption/debit locks. This matches request,
	// award, reversal, correction and cancellation lock ordering.
	e = tx.QueryRow(c, `SELECT child_id FROM reward_redemptions WHERE id=$1 AND family_id=$2`, id, f).Scan(&childID)
	if errors.Is(e, pgx.ErrNoRows) {
		return Redemption{}, false, ErrNotFound
	}
	if e != nil {
		return Redemption{}, false, e
	}
	if _, e = tx.Exec(c, `SELECT 1 FROM children WHERE id=$1 AND family_id=$2 FOR UPDATE`, childID, f); e != nil {
		return Redemption{}, false, e
	}
	e = tx.QueryRow(c, `SELECT rr.child_id,rr.state::text,rr.version,rr.cost_points_snapshot,rr.debit_ledger_entry_id FROM reward_redemptions rr WHERE rr.id=$1 AND rr.family_id=$2 FOR UPDATE`, id, f).Scan(&childID, &state, &cur, &cost, &debit)
	if e != nil {
		return Redemption{}, false, e
	}
	if cur != v {
		return Redemption{}, false, ErrVersionConflict
	}
	if state != "requested" {
		return Redemption{}, false, ErrInvalidState
	}
	if action == "cancel" {
		var refund string
		e = tx.QueryRow(c, `INSERT INTO point_ledger(family_id,child_id,kind,amount,reason,actor_user_id,reward_redemption_id,reverses_entry_id) VALUES($1,$2,'reward_refund',$3,$4,$5,$6,$7) RETURNING id`, f, childID, cost, reason, actor, id, debit).Scan(&refund)
		if e == nil {
			_, e = tx.Exec(c, `UPDATE reward_redemptions SET state='cancelled',decided_by_user_id=$2,decided_at=now(),cancellation_reason=$3,refund_ledger_entry_id=$4,version=version+1 WHERE id=$1`, id, actor, reason, refund)
		}
	} else {
		_, e = tx.Exec(c, `UPDATE reward_redemptions SET state='fulfilled',decided_by_user_id=$2,decided_at=now(),version=version+1 WHERE id=$1`, id, actor)
	}
	var out Redemption
	if e == nil {
		out, e = scanRedemption(tx.QueryRow(c, `SELECT `+redemptionCols+` FROM reward_redemptions rr JOIN children c ON c.id=rr.child_id WHERE rr.id=$1`, id))
	}
	if e == nil {
		e = finish(c, tx, f, sid, "redemptions."+action, k, 200, out)
	}
	if e == nil {
		_, e = tx.Exec(c, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status,idempotency_key,metadata) VALUES($1,$2,$3,$4,'reward_redemption',$5,'requested',$6,$7,jsonb_build_object('costPoints',$8::bigint))`, f, actor, sid, "reward_redemption."+out.State, id, out.State, k, cost)
	}
	if e != nil {
		return Redemption{}, false, e
	}
	return out, false, tx.Commit(c)
}
