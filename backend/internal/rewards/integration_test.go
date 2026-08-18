package rewards

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/family-habit/family-habit/backend/internal/auth"
	"github.com/family-habit/family-habit/backend/internal/children"
	"github.com/family-habit/family-habit/backend/internal/database"
	"github.com/family-habit/family-habit/backend/internal/points"
)

func TestPhase9RewardBalanceSerializationAndLedgerActors(t *testing.T) {
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
	parent, _, err := auth.NewService(pool).Register(ctx, "p9-rewards-"+suffix+"@example.test", "correct horse battery staple", "Phase 9", "Europe/Berlin", 1)
	if err != nil {
		t.Fatal(err)
	}
	child, _, err := children.NewService(pool).Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "p9-child-"+suffix, []byte("child"+suffix), "Alex", "fox", "#123456", "")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := children.NewService(pool).Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "p9-other-"+suffix, []byte("other"+suffix), "Jamie", "bear", "#654321", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = points.NewService(pool).Correct(ctx, parent.ID, parent.FamilyID, child.ID, 10, "Starting points", "p9-points-"+suffix, []byte("points"+suffix)); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE families SET rewards_enabled=true WHERE id=$1`, parent.FamilyID); err != nil {
		t.Fatal(err)
	}
	svc := NewService(pool)
	reward, _, err := svc.Create(ctx, parent.ID, parent.FamilyID, "p9-reward-"+suffix, []byte("reward"+suffix), RewardInput{Title: "Choose dinner", CostPoints: 10, AvailabilityScope: "selected_children", EligibleChildIDs: []string{child.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL WHERE id=$1`, parent.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	catalog, balance, err := svc.ChildCatalog(ctx, parent.ID, parent.FamilyID)
	if err != nil || balance != 10 || len(catalog) != 1 || catalog[0].ID != reward.ID {
		t.Fatalf("catalog=%+v balance=%d err=%v", catalog, balance, err)
	}
	type result struct {
		redemption Redemption
		err        error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r, _, e := svc.Redeem(ctx, parent.ID, parent.FamilyID, reward.ID, "p9-redeem-"+suffix+string(rune('a'+n)), []byte{byte(n + 1)}, reward.Version, reward.CostPoints)
			results <- result{r, e}
		}(i)
	}
	wg.Wait()
	close(results)
	var requested Redemption
	successes, insufficient := 0, 0
	for got := range results {
		if got.err == nil {
			successes++
			requested = got.redemption
		} else if errors.Is(got.err, ErrInsufficient) {
			insufficient++
		} else {
			t.Fatalf("unexpected redeem error: %v", got.err)
		}
	}
	if successes != 1 || insufficient != 1 {
		t.Fatalf("success=%d insufficient=%d", successes, insufficient)
	}
	var signed int64
	var debitChild *string
	var debitUser *string
	if err = pool.QueryRow(ctx, `SELECT sum(amount),max(actor_child_id::text),max(actor_user_id::text) FROM point_ledger WHERE family_id=$1 AND child_id=$2`, parent.FamilyID, child.ID).Scan(&signed, &debitChild, &debitUser); err != nil || signed != 0 || debitChild == nil || *debitChild != child.ID {
		t.Fatalf("balance=%d childActor=%v userActor=%v err=%v", signed, debitChild, debitUser, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='parent',active_child_id=NULL,parent_unlocked_at=now() WHERE id=$1`, parent.ID); err != nil {
		t.Fatal(err)
	}
	cancelled, _, err := svc.Decide(ctx, parent.ID, parent.FamilyID, requested.ID, "cancel", "Parent-only reason", "p9-cancel-"+suffix, []byte("cancel"+suffix), requested.Version)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.State != "cancelled" || cancelled.RefundLedgerEntryID == "" {
		t.Fatalf("cancelled=%+v", cancelled)
	}
	var refundAmount int64
	var refundUser *string
	var refundChild *string
	if err = pool.QueryRow(ctx, `SELECT amount,actor_user_id::text,actor_child_id::text FROM point_ledger WHERE id=$1`, cancelled.RefundLedgerEntryID).Scan(&refundAmount, &refundUser, &refundChild); err != nil || refundAmount != 10 || refundUser == nil || *refundUser != parent.UserID || refundChild != nil {
		t.Fatalf("refund=%d/%v/%v err=%v", refundAmount, refundUser, refundChild, err)
	}
	if err = pool.QueryRow(ctx, `SELECT sum(amount) FROM point_ledger WHERE family_id=$1 AND child_id=$2`, parent.FamilyID, child.ID).Scan(&signed); err != nil || signed != 10 {
		t.Fatalf("refunded balance=%d err=%v", signed, err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO point_ledger(family_id,child_id,kind,amount,reward_redemption_id) VALUES($1,$2,'reward_redemption',-10,$3)`, parent.FamilyID, child.ID, requested.ID); err == nil {
		t.Fatal("expected actor/duplicate debit constraint rejection")
	}
	// The selected catalog must not leak to another active child.
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL WHERE id=$1`, parent.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	otherCatalog, _, err := svc.ChildCatalog(ctx, parent.ID, parent.FamilyID)
	if err != nil || len(otherCatalog) != 0 {
		t.Fatalf("other catalog=%+v err=%v", otherCatalog, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL WHERE id=$1`, parent.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.Redeem(ctx, parent.ID, parent.FamilyID, reward.ID, "p9-redeem-again-"+suffix, []byte("redeem-again"+suffix), reward.Version, reward.CostPoints); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='parent',active_child_id=NULL,parent_unlocked_at=now() WHERE id=$1`, parent.ID); err != nil {
		t.Fatal(err)
	}
	_, cursor, err := svc.ListParent(ctx, parent.ID, parent.FamilyID, "", "", "", 1)
	if err != nil || cursor == "" {
		t.Fatalf("parent cursor=%q err=%v", cursor, err)
	}
	if _, _, err = svc.ListParent(ctx, parent.ID, parent.FamilyID, "requested", "", cursor, 1); !errors.Is(err, ErrCursor) {
		t.Fatalf("cross-filter cursor error=%v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL WHERE id=$1`, parent.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.ListChild(ctx, parent.ID, parent.FamilyID, cursor, 1); !errors.Is(err, ErrCursor) {
		t.Fatalf("cross-projection cursor error=%v", err)
	}
}
