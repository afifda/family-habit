package rewards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type EligibilityPolicy struct {
	ID                          string  `json:"-"`
	Enabled                     bool    `json:"enabled"`
	Period                      string  `json:"period"`
	MinimumPoints               int     `json:"minimumPoints"`
	MinimumCompletionPercentage *int    `json:"minimumCompletionPercentage"`
	MaximumRedemptions          *int    `json:"maximumRedemptions"`
	GraceHours                  int     `json:"graceHours"`
	EffectiveFrom               *string `json:"effectiveFrom"`
	Version                     int64   `json:"version"`
	Timezone                    string  `json:"-"`
	WeekStartsOn                int     `json:"-"`
}

type EligibilityPolicyInput struct {
	Enabled                     bool
	Period                      string
	MinimumPoints               int
	MinimumCompletionPercentage *int
	MaximumRedemptions          *int
	GraceHours                  int
}

type EligibilityRule struct {
	Type   string `json:"type"`
	Target int    `json:"target"`
	Actual int    `json:"actual"`
	Passed bool   `json:"passed"`
}

type EligibilityProgress struct {
	ID                          string            `json:"id,omitempty"`
	ChildID                     string            `json:"childId"`
	ChildName                   string            `json:"childName,omitempty"`
	PolicyEnabled               bool              `json:"policyEnabled"`
	PolicyVersion               int64             `json:"policyVersion,omitempty"`
	Status                      string            `json:"status"`
	CollectionPeriodStart       string            `json:"collectionPeriodStart"`
	CollectionPeriodEnd         string            `json:"collectionPeriodEnd"`
	EvaluationAt                *time.Time        `json:"evaluationAt,omitempty"`
	EvaluatedAt                 *time.Time        `json:"evaluatedAt,omitempty"`
	PointsCollected             int64             `json:"pointsCollected"`
	MinimumPoints               int               `json:"minimumPoints"`
	AssignedCount               int               `json:"assignedCount"`
	ApprovedCount               int               `json:"approvedCount"`
	CompletionPercentage        int               `json:"completionPercentage"`
	MinimumCompletionPercentage *int              `json:"minimumCompletionPercentage"`
	EligibleFrom                *string           `json:"eligibleFrom"`
	EligibleUntil               *string           `json:"eligibleUntil"`
	RedemptionsUsed             int               `json:"redemptionsUsed"`
	MaximumRedemptions          *int              `json:"maximumRedemptions"`
	CanRedeem                   bool              `json:"canRedeem"`
	UnavailableReason           string            `json:"unavailableReason,omitempty"`
	PointsShortfall             int64             `json:"pointsShortfall"`
	Rules                       []EligibilityRule `json:"rules"`
}

func scanPolicy(row pgx.Row) (EligibilityPolicy, error) {
	var p EligibilityPolicy
	var effective time.Time
	err := row.Scan(&p.ID, &p.Enabled, &p.Period, &p.MinimumPoints, &p.MinimumCompletionPercentage, &p.MaximumRedemptions, &p.GraceHours, &p.Timezone, &p.WeekStartsOn, &effective, &p.Version)
	if err == nil {
		v := effective.Format("2006-01-02")
		p.EffectiveFrom = &v
	}
	return p, err
}

func (s *Service) GetEligibilityPolicy(ctx context.Context, sid, familyID string) (EligibilityPolicy, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return EligibilityPolicy{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = parent(ctx, tx, sid, familyID); err != nil {
		return EligibilityPolicy{}, err
	}
	p, err := scanPolicy(tx.QueryRow(ctx, `SELECT id,enabled,collection_period::text,minimum_points,minimum_completion_percentage,maximum_redemptions,grace_hours,timezone,week_starts_on,effective_from,version FROM reward_eligibility_policies WHERE family_id=$1 ORDER BY version DESC LIMIT 1`, familyID))
	if errors.Is(err, pgx.ErrNoRows) {
		p = EligibilityPolicy{Enabled: false, Period: "weekly", MinimumPoints: 100, GraceHours: 24, Version: 0}
		err = nil
	}
	if err != nil {
		return p, err
	}
	return p, tx.Commit(ctx)
}

func periodBounds(date time.Time, period string, weekStart int) (time.Time, time.Time) {
	d := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	switch period {
	case "daily":
		return d, d
	case "monthly":
		start := time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, d.Location())
		return start, start.AddDate(0, 1, -1)
	default:
		wanted := time.Sunday
		if weekStart == 1 {
			wanted = time.Monday
		}
		delta := (7 + int(d.Weekday()) - int(wanted)) % 7
		start := d.AddDate(0, 0, -delta)
		return start, start.AddDate(0, 0, 6)
	}
}

func nextBoundary(now time.Time, period string, weekStart int) time.Time {
	_, end := periodBounds(now, period, weekStart)
	return end.AddDate(0, 0, 1)
}

func (s *Service) PutEligibilityPolicy(ctx context.Context, sid, familyID, key string, hash []byte, expected int64, in EligibilityPolicyInput) (EligibilityPolicy, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return EligibilityPolicy{}, false, err
	}
	defer tx.Rollback(ctx)
	actor, err := parent(ctx, tx, sid, familyID)
	if err != nil {
		return EligibilityPolicy{}, false, err
	}
	body, replay, err := reserve(ctx, tx, familyID, sid, "reward-eligibility-policy.put", key, hash)
	if err != nil {
		return EligibilityPolicy{}, false, err
	}
	if replay {
		var p EligibilityPolicy
		if err = jsonUnmarshal(body, &p); err != nil {
			return p, false, err
		}
		return p, true, tx.Commit(ctx)
	}
	if _, err = tx.Exec(ctx, `SELECT 1 FROM families WHERE id=$1 FOR UPDATE`, familyID); err != nil {
		return EligibilityPolicy{}, false, err
	}
	var current int64
	err = tx.QueryRow(ctx, `SELECT coalesce(max(version),0) FROM reward_eligibility_policies WHERE family_id=$1`, familyID).Scan(&current)
	if err != nil {
		return EligibilityPolicy{}, false, err
	}
	if current != expected {
		return EligibilityPolicy{}, false, ErrVersionConflict
	}
	var tz string
	var week int
	var rewardsEnabled bool
	if err = tx.QueryRow(ctx, `SELECT timezone,week_starts_on,rewards_enabled FROM families WHERE id=$1 FOR UPDATE`, familyID).Scan(&tz, &week, &rewardsEnabled); err != nil {
		return EligibilityPolicy{}, false, err
	}
	if in.Enabled && !rewardsEnabled {
		return EligibilityPolicy{}, false, ErrDisabled
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return EligibilityPolicy{}, false, err
	}
	localNow := s.now().In(loc)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	// A not-yet-active policy may be replaced safely: it has never owned a
	// collection period or evaluation. Keeping at most one pending version
	// prevents frequency schedules from overlapping.
	if _, err = tx.Exec(ctx, `DELETE FROM reward_eligibility_policies WHERE family_id=$1 AND effective_from>$2`, familyID, today); err != nil {
		return EligibilityPolicy{}, false, err
	}
	effective := nextBoundary(localNow, in.Period, week)
	p, err := scanPolicy(tx.QueryRow(ctx, `INSERT INTO reward_eligibility_policies(family_id,enabled,collection_period,minimum_points,minimum_completion_percentage,maximum_redemptions,grace_hours,timezone,week_starts_on,effective_from,version,created_by_user_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id,enabled,collection_period::text,minimum_points,minimum_completion_percentage,maximum_redemptions,grace_hours,timezone,week_starts_on,effective_from,version`, familyID, in.Enabled, in.Period, in.MinimumPoints, in.MinimumCompletionPercentage, in.MaximumRedemptions, in.GraceHours, tz, week, effective, current+1, actor))
	if err == nil {
		err = finish(ctx, tx, familyID, sid, "reward-eligibility-policy.put", key, 200, p)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,after_status,idempotency_key,metadata) VALUES($1,$2,$3,'reward_eligibility_policy.scheduled','reward_eligibility_policy',$4,$5,$6,jsonb_build_object('effectiveFrom',$7::text,'version',$8::bigint))`, familyID, actor, sid, p.ID, map[bool]string{true: "enabled", false: "disabled"}[p.Enabled], key, effective.Format("2006-01-02"), p.Version)
	}
	if err != nil {
		return EligibilityPolicy{}, false, err
	}
	return p, false, tx.Commit(ctx)
}

// Small wrapper keeps encoding details local without adding another public API.
func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func previousPeriod(today time.Time, period string, week int) (time.Time, time.Time) {
	cur, _ := periodBounds(today, period, week)
	return periodBounds(cur.AddDate(0, 0, -1), period, week)
}

func policySnapshot(p EligibilityPolicy, tz string, week int) string {
	return fmt.Sprintf(`{"period":%q,"minimumPoints":%d,"minimumCompletionPercentage":%s,"maximumRedemptions":%s,"graceHours":%d,"timezone":%q,"weekStartsOn":%d,"version":%d}`, p.Period, p.MinimumPoints, nullableInt(p.MinimumCompletionPercentage), nullableInt(p.MaximumRedemptions), p.GraceHours, tz, week, p.Version)
}
func nullableInt(v *int) string {
	if v == nil {
		return "null"
	}
	return fmt.Sprintf("%d", *v)
}

func (s *Service) ensureEvaluation(ctx context.Context, tx pgx.Tx, familyID, childID string, now time.Time) (EligibilityProgress, error) {
	if _, err := tx.Exec(ctx, `SELECT 1 FROM children WHERE id=$1 AND family_id=$2 AND archived_at IS NULL FOR UPDATE`, childID, familyID); err != nil {
		return EligibilityProgress{}, err
	}
	var tz string
	var week int
	if err := tx.QueryRow(ctx, `SELECT timezone,week_starts_on FROM families WHERE id=$1`, familyID).Scan(&tz, &week); err != nil {
		return EligibilityProgress{}, err
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return EligibilityProgress{}, err
	}
	localNow := now.In(loc)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	p, err := scanPolicy(tx.QueryRow(ctx, `SELECT id,enabled,collection_period::text,minimum_points,minimum_completion_percentage,maximum_redemptions,grace_hours,timezone,week_starts_on,effective_from,version FROM reward_eligibility_policies WHERE family_id=$1 AND effective_from<=$2 ORDER BY effective_from DESC,version DESC LIMIT 1`, familyID, today))
	if errors.Is(err, pgx.ErrNoRows) || !p.Enabled {
		return EligibilityProgress{ChildID: childID, PolicyEnabled: false, Status: "collecting", Rules: []EligibilityRule{}, CanRedeem: true}, nil
	}
	if err != nil {
		return EligibilityProgress{}, err
	}
	policyLoc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		return EligibilityProgress{}, err
	}
	policyToday := now.In(policyLoc)
	policyToday = time.Date(policyToday.Year(), policyToday.Month(), policyToday.Day(), 0, 0, 0, 0, policyLoc)
	curStart, curEnd := periodBounds(policyToday, p.Period, p.WeekStartsOn)
	prevStart, prevEnd := previousPeriod(policyToday, p.Period, p.WeekStartsOn)
	evalAt := time.Date(prevEnd.Year(), prevEnd.Month(), prevEnd.Day()+1, 0, 0, 0, 0, policyLoc).Add(time.Duration(p.GraceHours) * time.Hour)
	progress := EligibilityProgress{ChildID: childID, PolicyEnabled: true, PolicyVersion: p.Version, Status: "awaiting_evaluation", CollectionPeriodStart: curStart.Format("2006-01-02"), CollectionPeriodEnd: curEnd.Format("2006-01-02"), EvaluationAt: &evalAt, MinimumPoints: p.MinimumPoints, MinimumCompletionPercentage: p.MinimumCompletionPercentage, MaximumRedemptions: p.MaximumRedemptions, Rules: []EligibilityRule{}, UnavailableReason: "awaiting_evaluation"}
	if err = scorePeriod(ctx, tx, familyID, childID, curStart, curEnd, now, &progress); err != nil {
		return progress, err
	}
	// The version applicable at the beginning of the closed collection period owns its evaluation.
	currentPolicy := p
	p, err = scanPolicy(tx.QueryRow(ctx, `SELECT id,enabled,collection_period::text,minimum_points,minimum_completion_percentage,maximum_redemptions,grace_hours,timezone,week_starts_on,effective_from,version FROM reward_eligibility_policies WHERE family_id=$1 AND effective_from<=$2 ORDER BY effective_from DESC,version DESC LIMIT 1`, familyID, prevStart))
	calendarChanged := err == nil && (p.Period != currentPolicy.Period || p.Timezone != currentPolicy.Timezone || (p.Period == "weekly" && p.WeekStartsOn != currentPolicy.WeekStartsOn))
	if errors.Is(err, pgx.ErrNoRows) || !p.Enabled || calendarChanged {
		progress.Status = "collecting"
		progress.CanRedeem = false
		progress.UnavailableReason = "eligibility_not_final"
		progress.EvaluationAt = nil
		return progress, nil
	}
	if err != nil {
		return progress, err
	}
	previousLoc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		return progress, err
	}
	evalAt = time.Date(prevEnd.Year(), prevEnd.Month(), prevEnd.Day()+1, 0, 0, 0, 0, previousLoc).Add(time.Duration(p.GraceHours) * time.Hour)
	progress.EvaluationAt = &evalAt
	if now.Before(evalAt) {
		return progress, nil
	}
	var eid string
	var evaluated time.Time
	var eligible bool
	var points int64
	var assigned, approved, pct int
	err = tx.QueryRow(ctx, `SELECT id,evaluated_at,eligible,points_collected,assigned_count,approved_count,completion_percentage FROM reward_period_evaluations WHERE family_id=$1 AND child_id=$2 AND policy_id=$3 AND collection_start=$4 AND collection_end=$5`, familyID, childID, p.ID, prevStart, prevEnd).Scan(&eid, &evaluated, &eligible, &points, &assigned, &approved, &pct)
	if errors.Is(err, pgx.ErrNoRows) {
		tmp := EligibilityProgress{}
		if err = scorePeriod(ctx, tx, familyID, childID, prevStart, prevEnd, evalAt, &tmp); err != nil {
			return progress, err
		}
		points, assigned, approved, pct = tmp.PointsCollected, tmp.AssignedCount, tmp.ApprovedCount, tmp.CompletionPercentage
		eligible = points >= int64(p.MinimumPoints) && (p.MinimumCompletionPercentage == nil || pct >= *p.MinimumCompletionPercentage)
		_, err = tx.Exec(ctx, `INSERT INTO reward_period_evaluations(family_id,child_id,policy_id,collection_start,collection_end,evaluation_cutoff,eligible,eligible_from,eligible_until,points_collected,assigned_count,approved_count,completion_percentage,policy_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb) ON CONFLICT DO NOTHING`, familyID, childID, p.ID, prevStart, prevEnd, evalAt, eligible, curStart, curEnd, points, assigned, approved, pct, policySnapshot(p, p.Timezone, p.WeekStartsOn))
		if err != nil {
			return progress, err
		}
		if err = tx.QueryRow(ctx, `SELECT id,evaluated_at FROM reward_period_evaluations WHERE family_id=$1 AND child_id=$2 AND policy_id=$3 AND collection_start=$4 AND collection_end=$5`, familyID, childID, p.ID, prevStart, prevEnd).Scan(&eid, &evaluated); err != nil {
			return progress, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO reward_evaluation_rule_results(evaluation_id,rule_type,target,actual,passed) VALUES($1,'minimum_points',$2::integer,$3::integer,$3::bigint>=$2::bigint) ON CONFLICT DO NOTHING`, eid, p.MinimumPoints, points)
		if err != nil {
			return progress, err
		}
		if p.MinimumCompletionPercentage != nil {
			_, err = tx.Exec(ctx, `INSERT INTO reward_evaluation_rule_results(evaluation_id,rule_type,target,actual,passed) VALUES($1,'minimum_completion_percentage',$2::integer,$3::integer,$3::integer>=$2::integer) ON CONFLICT DO NOTHING`, eid, *p.MinimumCompletionPercentage, pct)
			if err != nil {
				return progress, err
			}
		}
	} else if err != nil {
		return progress, err
	}
	var adjustPoints int64
	var adjustApproved int
	if err = tx.QueryRow(ctx, `SELECT coalesce(sum(points_delta),0),coalesce(sum(approved_count_delta),0) FROM reward_evaluation_adjustments WHERE evaluation_id=$1`, eid).Scan(&adjustPoints, &adjustApproved); err != nil {
		return progress, err
	}
	points += adjustPoints
	approved += adjustApproved
	if approved < 0 {
		approved = 0
	}
	if assigned > 0 {
		pct = approved * 100 / assigned
	} else {
		pct = 100
	}
	eligible = eligible && points >= int64(p.MinimumPoints) && (p.MinimumCompletionPercentage == nil || pct >= *p.MinimumCompletionPercentage)
	var used int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM reward_redemptions WHERE eligibility_evaluation_id=$1`, eid).Scan(&used); err != nil {
		return progress, err
	}
	status := "not_eligible"
	reason := "threshold_not_reached"
	can := false
	if eligible {
		status = "eligible"
		reason = ""
		can = true
	}
	if can && p.MaximumRedemptions != nil && used >= *p.MaximumRedemptions {
		can = false
		reason = "redemption_limit_reached"
	}
	short := int64(p.MinimumPoints) - points
	if short < 0 {
		short = 0
	}
	ef, eu := curStart.Format("2006-01-02"), curEnd.Format("2006-01-02")
	rules := []EligibilityRule{{Type: "minimum_points", Target: p.MinimumPoints, Actual: int(points), Passed: points >= int64(p.MinimumPoints)}}
	if p.MinimumCompletionPercentage != nil {
		rules = append(rules, EligibilityRule{Type: "minimum_completion_percentage", Target: *p.MinimumCompletionPercentage, Actual: pct, Passed: pct >= *p.MinimumCompletionPercentage})
	}
	return EligibilityProgress{ID: eid, ChildID: childID, PolicyEnabled: true, PolicyVersion: p.Version, Status: status, CollectionPeriodStart: prevStart.Format("2006-01-02"), CollectionPeriodEnd: prevEnd.Format("2006-01-02"), EvaluatedAt: &evaluated, PointsCollected: points, MinimumPoints: p.MinimumPoints, AssignedCount: assigned, ApprovedCount: approved, CompletionPercentage: pct, MinimumCompletionPercentage: p.MinimumCompletionPercentage, EligibleFrom: &ef, EligibleUntil: &eu, RedemptionsUsed: used, MaximumRedemptions: p.MaximumRedemptions, CanRedeem: can, UnavailableReason: reason, PointsShortfall: short, Rules: rules}, nil
}

func scorePeriod(ctx context.Context, tx pgx.Tx, familyID, childID string, start, end, cutoff time.Time, out *EligibilityProgress) error {
	err := tx.QueryRow(ctx, `SELECT coalesce((SELECT sum(pl.amount) FROM point_ledger pl JOIN occurrences po ON po.id=pl.occurrence_id WHERE pl.family_id=$1 AND pl.child_id=$2 AND pl.kind IN ('award','approval_reversal') AND pl.created_at<=$5 AND po.local_date BETWEEN $3 AND $4),0),count(*) FILTER(WHERE o.state<>'cancelled'),count(*) FILTER(WHERE EXISTS(SELECT 1 FROM point_ledger award WHERE award.occurrence_id=o.id AND award.kind='award' AND award.created_at<=$5) AND NOT EXISTS(SELECT 1 FROM point_ledger reversal WHERE reversal.occurrence_id=o.id AND reversal.kind='approval_reversal' AND reversal.created_at<=$5)) FROM occurrences o WHERE o.family_id=$1 AND o.child_id=$2 AND o.local_date BETWEEN $3 AND $4`, familyID, childID, start, end, cutoff).Scan(&out.PointsCollected, &out.AssignedCount, &out.ApprovedCount)
	if err == nil {
		out.CompletionPercentage = 100
		if out.AssignedCount > 0 {
			out.CompletionPercentage = out.ApprovedCount * 100 / out.AssignedCount
		}
	}
	return err
}

func (s *Service) ParentEligibilityProgress(ctx context.Context, sid, familyID string) ([]EligibilityProgress, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = parent(ctx, tx, sid, familyID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id,nickname FROM children WHERE family_id=$1 AND archived_at IS NULL ORDER BY lower(nickname),id`, familyID)
	if err != nil {
		return nil, err
	}
	type ch struct{ id, name string }
	children := []ch{}
	for rows.Next() {
		var v ch
		if err = rows.Scan(&v.id, &v.name); err != nil {
			rows.Close()
			return nil, err
		}
		children = append(children, v)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := make([]EligibilityProgress, 0, len(children))
	for _, v := range children {
		p, e := s.ensureEvaluation(ctx, tx, familyID, v.id, s.now())
		if e != nil {
			return nil, e
		}
		p.ChildName = v.name
		out = append(out, p)
	}
	return out, tx.Commit(ctx)
}

func (s *Service) ChildEligibility(ctx context.Context, sid, familyID string) (EligibilityProgress, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return EligibilityProgress{}, err
	}
	defer tx.Rollback(ctx)
	childID, err := child(ctx, tx, sid, familyID)
	if err != nil {
		return EligibilityProgress{}, err
	}
	p, err := s.ensureEvaluation(ctx, tx, familyID, childID, s.now())
	if err != nil {
		return p, err
	}
	return p, tx.Commit(ctx)
}

func (s *Service) EligibilityHistory(ctx context.Context, sid, familyID, childID string, limit int) ([]EligibilityProgress, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = parent(ctx, tx, sid, familyID); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, err := tx.Query(ctx, `SELECT e.id,e.child_id,c.nickname,p.version,e.collection_start,e.collection_end,e.evaluated_at,e.eligible,e.eligible_from,e.eligible_until,e.points_collected,e.assigned_count,e.approved_count,e.completion_percentage,p.minimum_points,p.minimum_completion_percentage,p.maximum_redemptions,(SELECT count(*) FROM reward_redemptions rr WHERE rr.eligibility_evaluation_id=e.id),coalesce((SELECT sum(points_delta) FROM reward_evaluation_adjustments a WHERE a.evaluation_id=e.id),0),coalesce((SELECT sum(approved_count_delta) FROM reward_evaluation_adjustments a WHERE a.evaluation_id=e.id),0) FROM reward_period_evaluations e JOIN children c ON c.id=e.child_id JOIN reward_eligibility_policies p ON p.id=e.policy_id WHERE e.family_id=$1 AND ($2='' OR e.child_id::text=$2) ORDER BY e.collection_start DESC,e.id DESC LIMIT $3`, familyID, childID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EligibilityProgress{}
	for rows.Next() {
		var p EligibilityProgress
		var cs, ce, ef, eu time.Time
		var eligible bool
		var adjPoints int64
		var adjApproved int
		if err = rows.Scan(&p.ID, &p.ChildID, &p.ChildName, &p.PolicyVersion, &cs, &ce, &p.EvaluatedAt, &eligible, &ef, &eu, &p.PointsCollected, &p.AssignedCount, &p.ApprovedCount, &p.CompletionPercentage, &p.MinimumPoints, &p.MinimumCompletionPercentage, &p.MaximumRedemptions, &p.RedemptionsUsed, &adjPoints, &adjApproved); err != nil {
			return nil, err
		}
		p.PolicyEnabled = true
		p.CollectionPeriodStart = cs.Format("2006-01-02")
		p.CollectionPeriodEnd = ce.Format("2006-01-02")
		x, y := ef.Format("2006-01-02"), eu.Format("2006-01-02")
		p.EligibleFrom = &x
		p.EligibleUntil = &y
		p.PointsCollected += adjPoints
		p.ApprovedCount += adjApproved
		if p.ApprovedCount < 0 {
			p.ApprovedCount = 0
		}
		if p.AssignedCount > 0 {
			p.CompletionPercentage = p.ApprovedCount * 100 / p.AssignedCount
		} else {
			p.CompletionPercentage = 100
		}
		eligible = eligible && p.PointsCollected >= int64(p.MinimumPoints) && (p.MinimumCompletionPercentage == nil || p.CompletionPercentage >= *p.MinimumCompletionPercentage)
		p.Status = "not_eligible"
		p.UnavailableReason = "threshold_not_reached"
		if eligible {
			p.Status = "eligible"
			p.CanRedeem = true
			p.UnavailableReason = ""
		}
		if p.CanRedeem && p.MaximumRedemptions != nil && p.RedemptionsUsed >= *p.MaximumRedemptions {
			p.CanRedeem = false
			p.UnavailableReason = "redemption_limit_reached"
		}
		p.PointsShortfall = int64(p.MinimumPoints) - p.PointsCollected
		if p.PointsShortfall < 0 {
			p.PointsShortfall = 0
		}
		p.Rules = []EligibilityRule{{Type: "minimum_points", Target: p.MinimumPoints, Actual: int(p.PointsCollected), Passed: p.PointsCollected >= int64(p.MinimumPoints)}}
		if p.MinimumCompletionPercentage != nil {
			p.Rules = append(p.Rules, EligibilityRule{Type: "minimum_completion_percentage", Target: *p.MinimumCompletionPercentage, Actual: p.CompletionPercentage, Passed: p.CompletionPercentage >= *p.MinimumCompletionPercentage})
		}
		out = append(out, p)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}
