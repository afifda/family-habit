package rewards

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/family-habit/family-habit/backend/internal/auth"
	"github.com/family-habit/family-habit/backend/internal/children"
	"github.com/family-habit/family-habit/backend/internal/database"
	"github.com/family-habit/family-habit/backend/internal/points"
)

func TestPeriodicEligibilityEnforcesEvaluationAndCap(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = database.MigrateUp(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var random [8]byte
	_, _ = rand.Read(random[:])
	suffix := hex.EncodeToString(random[:])
	parentSession, _, err := auth.NewService(pool).Register(ctx, "p91-"+suffix+"@example.test", "correct horse battery staple", "Phase 9.1", "UTC", 1)
	if err != nil {
		t.Fatal(err)
	}
	childProfile, _, err := children.NewService(pool).Create(ctx, parentSession.ID, parentSession.UserID, parentSession.FamilyID, "p91-child-"+suffix, []byte("child"+suffix), "Alex", "fox", "#123456", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = points.NewService(pool).Correct(ctx, parentSession.ID, parentSession.FamilyID, childProfile.ID, 20, "Starting points", "p91-points-"+suffix, []byte("points"+suffix)); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE families SET rewards_enabled=true WHERE id=$1`, parentSession.FamilyID); err != nil {
		t.Fatal(err)
	}
	svc := NewService(pool)
	reward, _, err := svc.Create(ctx, parentSession.ID, parentSession.FamilyID, "p91-reward-"+suffix, []byte("reward"+suffix), RewardInput{Title: "Choose dinner", CostPoints: 5, AvailabilityScope: "all_active_children"})
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	previous := today.AddDate(0, 0, -1)
	var policyID, evaluationID string
	err = pool.QueryRow(ctx, `INSERT INTO reward_eligibility_policies(family_id,enabled,collection_period,minimum_points,maximum_redemptions,grace_hours,timezone,week_starts_on,effective_from,version,created_by_user_id) VALUES($1,true,'daily',10,1,0,'UTC',1,$2,1,$3) RETURNING id`, parentSession.FamilyID, previous, parentSession.UserID).Scan(&policyID)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `INSERT INTO reward_period_evaluations(family_id,child_id,policy_id,collection_start,collection_end,evaluation_cutoff,eligible,eligible_from,eligible_until,points_collected,assigned_count,approved_count,completion_percentage,policy_snapshot) VALUES($1,$2,$3,$4,$4,$6,true,$5,$5,10,1,1,100,'{}') RETURNING id`, parentSession.FamilyID, childProfile.ID, policyID, previous, today, today).Scan(&evaluationID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO reward_evaluation_rule_results(evaluation_id,rule_type,target,actual,passed) VALUES($1,'minimum_points',10,10,true)`, evaluationID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL WHERE id=$1`, parentSession.ID, childProfile.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.Redeem(ctx, parentSession.ID, parentSession.FamilyID, reward.ID, "p91-redeem-a-"+suffix, []byte("redeem-a"), reward.Version, reward.CostPoints); err != nil {
		t.Fatalf("first redemption: %v", err)
	}
	if _, _, err = svc.Redeem(ctx, parentSession.ID, parentSession.FamilyID, reward.ID, "p91-redeem-b-"+suffix, []byte("redeem-b"), reward.Version, reward.CostPoints); !errors.Is(err, ErrRedemptionLimit) {
		t.Fatalf("second redemption error=%v, want cap", err)
	}
}

func TestPeriodicEligibilityReplacesPendingPolicyAndStartsFrequencyFresh(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = database.MigrateUp(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var random [8]byte
	_, _ = rand.Read(random[:])
	suffix := hex.EncodeToString(random[:])
	session, _, err := auth.NewService(pool).Register(ctx, "p91-transition-"+suffix+"@example.test", "correct horse battery staple", "Transitions", "UTC", 1)
	if err != nil {
		t.Fatal(err)
	}
	childProfile, _, err := children.NewService(pool).Create(ctx, session.ID, session.UserID, session.FamilyID, "p91-transition-child-"+suffix, []byte("child"+suffix), "Taylor", "fox", "#123456", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE families SET rewards_enabled=true WHERE id=$1`, session.FamilyID); err != nil {
		t.Fatal(err)
	}
	svc := NewService(pool)
	svc.now = func() time.Time { return time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC) }
	first, _, err := svc.PutEligibilityPolicy(ctx, session.ID, session.FamilyID, "transition-daily-"+suffix, []byte("daily"), 0, EligibilityPolicyInput{Enabled: true, Period: "daily", MinimumPoints: 10, GraceHours: 0})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := svc.PutEligibilityPolicy(ctx, session.ID, session.FamilyID, "transition-weekly-"+suffix, []byte("weekly"), first.Version, EligibilityPolicyInput{Enabled: true, Period: "weekly", MinimumPoints: 10, GraceHours: 0})
	if err != nil {
		t.Fatal(err)
	}
	var pending int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM reward_eligibility_policies WHERE family_id=$1 AND effective_from>'2026-08-18'`, session.FamilyID).Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("pending policies=%d err=%v", pending, err)
	}
	if second.EffectiveFrom == nil || *second.EffectiveFrom != "2026-08-24" {
		t.Fatalf("weekly effective boundary=%v", second.EffectiveFrom)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM reward_eligibility_policies WHERE family_id=$1`, session.FamilyID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO reward_eligibility_policies(family_id,enabled,collection_period,minimum_points,grace_hours,timezone,week_starts_on,effective_from,version,created_by_user_id) VALUES($1,true,'daily',10,0,'UTC',1,'2026-08-17',3,$2),($1,true,'weekly',10,0,'UTC',1,'2026-08-24',4,$2)`, session.FamilyID, session.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL WHERE id=$1`, session.ID, childProfile.ID); err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC) }
	progress, err := svc.ChildEligibility(ctx, session.ID, session.FamilyID)
	if err != nil {
		t.Fatal(err)
	}
	if !progress.PolicyEnabled || progress.Status != "collecting" || progress.CanRedeem {
		t.Fatalf("frequency transition progress=%+v", progress)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM reward_eligibility_policies WHERE family_id=$1`, session.FamilyID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO reward_eligibility_policies(family_id,enabled,collection_period,minimum_points,grace_hours,timezone,week_starts_on,effective_from,version,created_by_user_id) VALUES($1,true,'weekly',10,0,'UTC',1,'2026-08-17',5,$2),($1,true,'weekly',20,0,'UTC',1,'2026-08-24',6,$2)`, session.FamilyID, session.UserID); err != nil {
		t.Fatal(err)
	}
	progress, err = svc.ChildEligibility(ctx, session.ID, session.FamilyID)
	if err != nil {
		t.Fatal(err)
	}
	if progress.Status != "not_eligible" || progress.PolicyVersion != 5 || progress.MinimumPoints != 10 {
		t.Fatalf("same-calendar transition lost previous evaluation: %+v", progress)
	}
}
