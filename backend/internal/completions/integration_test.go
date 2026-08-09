package completions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/family-habit/family-habit/backend/internal/auth"
	"github.com/family-habit/family-habit/backend/internal/children"
	"github.com/family-habit/family-habit/backend/internal/database"
	"github.com/family-habit/family-habit/backend/internal/habits"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTodaySubmitWithdrawIdempotencyAndConcurrencyIntegration(t *testing.T) {
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
	session, _, err := auth.NewService(pool).Register(ctx, "phase6-"+suffix+"@example.test", "correct horse battery staple", "Phase 6", "Asia/Jakarta", 0)
	if err != nil {
		t.Fatal(err)
	}
	cleanup(t, pool, session)
	childSvc := children.NewService(pool)
	child, _, err := childSvc.Create(ctx, session.ID, session.UserID, session.FamilyID, "p6-child", []byte("p6-child"), "Maya", "fox", "#112233", "")
	if err != nil {
		t.Fatal(err)
	}
	sibling, _, err := childSvc.Create(ctx, session.ID, session.UserID, session.FamilyID, "p6-sibling", []byte("p6-sibling"), "Alex", "bear", "#223344", "")
	if err != nil {
		t.Fatal(err)
	}

	svc := NewService(pool)
	now := time.Date(2026, 8, 9, 17, 30, 0, 0, time.UTC) // 2026-08-10 in Jakarta.
	svc.now = func() time.Time { return now }
	today, timezone, err := svc.HouseholdToday(ctx, session.FamilyID)
	if err != nil || today != "2026-08-10" || timezone != "Asia/Jakarta" {
		t.Fatalf("household today=%q tz=%q err=%v", today, timezone, err)
	}

	habitSvc := habits.NewService(pool)
	habit, _, err := habitSvc.CreateHabit(ctx, session.ID, session.UserID, session.FamilyID, "p6-habit", []byte("p6-habit"), habits.HabitInput{Title: "Brush teeth", Description: "For two minutes", Icon: "tooth", Color: "#334455"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = habitSvc.CreateAssignment(ctx, session.ID, session.UserID, session.FamilyID, habit.ID, "p6-assignment", []byte("p6-assignment"), habits.AssignmentInput{ChildID: child.ID, Points: 5, Kind: "daily", EffectiveDate: mustDate(t, "2026-08-01")})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = habitSvc.CreateTask(ctx, session.ID, session.UserID, session.FamilyID, "p6-overdue", []byte("p6-overdue"), habits.TaskInput{ChildID: child.ID, Title: "Library book", Description: "Return it", DueDate: mustDate(t, "2026-08-09"), Points: 8})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = habitSvc.CreateTask(ctx, session.ID, session.UserID, session.FamilyID, "p6-due-today", []byte("p6-due-today"), habits.TaskInput{ChildID: child.ID, Title: "Pack bag", DueDate: mustDate(t, today), Points: 3})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = habitSvc.CreateTask(ctx, session.ID, session.UserID, session.FamilyID, "p6-future", []byte("p6-future"), habits.TaskInput{ChildID: child.ID, Title: "Future task", DueDate: mustDate(t, "2026-08-11"), Points: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL WHERE id=$1`, session.ID, child.ID); err != nil {
		t.Fatal(err)
	}

	view, err := svc.Today(ctx, session.ID, session.FamilyID, child.ID, today)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Occurrences) != 3 {
		t.Fatalf("today occurrences=%+v", view.Occurrences)
	}
	if view.Occurrences[0].Type != "task" || view.Occurrences[0].DueState != "overdue" || view.Occurrences[0].Group != "to_do" {
		t.Fatalf("stable overdue order=%+v", view.Occurrences)
	}
	if view.Occurrences[1].Type != "habit" || view.Occurrences[2].Title != "Pack bag" {
		t.Fatalf("habit/task order or due-today omission=%+v", view.Occurrences)
	}
	for _, item := range view.Occurrences {
		if item.Title == "Future task" {
			t.Fatal("future task leaked into Today")
		}
	}
	if _, err = svc.Today(ctx, session.ID, session.FamilyID, sibling.ID, today); err != ErrNotFound {
		t.Fatalf("sibling today error=%v", err)
	}

	var occurrenceID string
	var version int64
	if err = pool.QueryRow(ctx, `SELECT id,version FROM occurrences WHERE family_id=$1 AND child_id=$2 AND source_type='habit' AND local_date=$3`, session.FamilyID, child.ID, today).Scan(&occurrenceID, &version); err != nil {
		t.Fatal(err)
	}
	submitted, replay, err := svc.Submit(ctx, session.ID, session.FamilyID, child.ID, occurrenceID, "submit-1", []byte("submit-1-v1"), version)
	if err != nil || replay || submitted.AttemptStatus != "pending" || submitted.OccurrenceStatus != "pending_approval" || submitted.Version != version+1 {
		t.Fatalf("submit=%+v replay=%v err=%v", submitted, replay, err)
	}
	replayed, replay, err := svc.Submit(ctx, session.ID, session.FamilyID, child.ID, occurrenceID, "submit-1", []byte("submit-1-v1"), version)
	if err != nil || !replay || replayed.ID != submitted.ID || replayed.Version != submitted.Version {
		t.Fatalf("submit replay=%+v replay=%v err=%v", replayed, replay, err)
	}
	if _, _, err = svc.Submit(ctx, session.ID, session.FamilyID, child.ID, occurrenceID, "submit-1", []byte("changed"), version); err != ErrIdempotency {
		t.Fatalf("idempotency conflict=%v", err)
	}
	assertCounts(t, pool, session.FamilyID, occurrenceID, 1, 1, 0)

	withdrawn, replay, err := svc.Withdraw(ctx, session.ID, session.FamilyID, child.ID, submitted.ID, "withdraw-1", []byte("withdraw-1-v2"), submitted.Version)
	if err != nil || replay || withdrawn.AttemptStatus != "withdrawn" || withdrawn.OccurrenceStatus != "not_started" || withdrawn.Version != submitted.Version+1 {
		t.Fatalf("withdraw=%+v replay=%v err=%v", withdrawn, replay, err)
	}
	_, replay, err = svc.Withdraw(ctx, session.ID, session.FamilyID, child.ID, submitted.ID, "withdraw-1", []byte("withdraw-1-v2"), submitted.Version)
	if err != nil || !replay {
		t.Fatalf("withdraw replay=%v err=%v", replay, err)
	}
	assertCounts(t, pool, session.FamilyID, occurrenceID, 1, 2, 0)

	secondAttempt, _, err := svc.Submit(ctx, session.ID, session.FamilyID, child.ID, occurrenceID, "submit-2", []byte("submit-2-v3"), withdrawn.Version)
	if err != nil {
		t.Fatal(err)
	}
	var attemptNumber int
	if err = pool.QueryRow(ctx, `SELECT attempt_number FROM completion_attempts WHERE id=$1`, secondAttempt.ID).Scan(&attemptNumber); err != nil || attemptNumber != 2 {
		t.Fatalf("second attempt=%d err=%v", attemptNumber, err)
	}

	// Different keys race on one overdue task. The occurrence lock/version makes exactly one win.
	var taskOccurrence string
	var taskVersion int64
	if err = pool.QueryRow(ctx, `SELECT id,version FROM occurrences WHERE family_id=$1 AND child_id=$2 AND source_type='task' AND title_snapshot='Library book'`, session.FamilyID, child.ID).Scan(&taskOccurrence, &taskVersion); err != nil {
		t.Fatal(err)
	}
	type result struct {
		ok  bool
		err error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, key := range []string{"race-a", "race-b"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, _, e := svc.Submit(ctx, session.ID, session.FamilyID, child.ID, taskOccurrence, key, []byte(key), taskVersion)
			results <- result{e == nil, e}
		}(key)
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for result := range results {
		if result.ok {
			wins++
		} else if errors.Is(result.err, ErrVersionConflict) || errors.Is(result.err, ErrInvalidState) {
			conflicts++
		} else {
			t.Fatalf("race error=%v", result.err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("race wins=%d conflicts=%d", wins, conflicts)
	}
	assertCounts(t, pool, session.FamilyID, taskOccurrence, 1, 1, 0)

	// Concurrent exact retries share one idempotency reservation and response.
	var sameKeyOccurrence string
	var sameKeyVersion int64
	if err = pool.QueryRow(ctx, `SELECT id,version FROM occurrences WHERE family_id=$1 AND title_snapshot='Pack bag'`, session.FamilyID).Scan(&sameKeyOccurrence, &sameKeyVersion); err != nil {
		t.Fatal(err)
	}
	type completionResult struct {
		value  Completion
		replay bool
		err    error
	}
	sameResults := make(chan completionResult, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, wasReplay, callErr := svc.Submit(ctx, session.ID, session.FamilyID, child.ID, sameKeyOccurrence, "same-submit", []byte("same-submit"), sameKeyVersion)
			sameResults <- completionResult{value, wasReplay, callErr}
		}()
	}
	wg.Wait()
	close(sameResults)
	replays := 0
	var sameCompletion Completion
	for result := range sameResults {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.replay {
			replays++
		}
		sameCompletion = result.value
	}
	if replays != 1 {
		t.Fatalf("same-key submit replays=%d", replays)
	}
	assertCounts(t, pool, session.FamilyID, sameKeyOccurrence, 1, 1, 0)
	withdrawResults := make(chan completionResult, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, wasReplay, callErr := svc.Withdraw(ctx, session.ID, session.FamilyID, child.ID, sameCompletion.ID, "same-withdraw", []byte("same-withdraw"), sameCompletion.Version)
			withdrawResults <- completionResult{value, wasReplay, callErr}
		}()
	}
	wg.Wait()
	close(withdrawResults)
	replays = 0
	var withdrawnSame Completion
	for result := range withdrawResults {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.replay {
			replays++
		}
		withdrawnSame = result.value
	}
	if replays != 1 {
		t.Fatalf("same-key withdraw replays=%d", replays)
	}
	assertCounts(t, pool, session.FamilyID, sameKeyOccurrence, 1, 2, 0)

	// Failures after each write boundary roll attempt/state/audit/idempotency back together.
	for _, faultStage := range []string{"attempt_inserted", "occurrence_updated", "before_audit", "before_idempotency_finalize"} {
		svc.fault = func(stage string) error {
			if stage == faultStage {
				return errors.New("injected " + stage + " failure")
			}
			return nil
		}
		key := "fault-submit-" + faultStage
		if _, _, err = svc.Submit(ctx, session.ID, session.FamilyID, child.ID, sameKeyOccurrence, key, []byte(key), withdrawnSame.Version); err == nil {
			t.Fatalf("fault stage %s unexpectedly committed", faultStage)
		}
		svc.fault = nil
		var rollbackState string
		var rollbackVersion int64
		if err = pool.QueryRow(ctx, `SELECT state::text,version FROM occurrences WHERE id=$1`, sameKeyOccurrence).Scan(&rollbackState, &rollbackVersion); err != nil || rollbackState != "not_started" || rollbackVersion != withdrawnSame.Version {
			t.Fatalf("%s rollback state/version=%s/%d err=%v", faultStage, rollbackState, rollbackVersion, err)
		}
		assertCounts(t, pool, session.FamilyID, sameKeyOccurrence, 1, 2, 0)
		var reservation int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_records WHERE family_id=$1 AND session_id=$2 AND idempotency_key=$3`, session.FamilyID, session.ID, key).Scan(&reservation); err != nil || reservation != 0 {
			t.Fatalf("%s reservation=%d err=%v", faultStage, reservation, err)
		}
	}

	// Different-key concurrent withdrawals serialize on the occurrence: exactly one finalizes it.
	pendingForWithdrawRace, _, err := svc.Submit(ctx, session.ID, session.FamilyID, child.ID, sameKeyOccurrence, "withdraw-race-submit", []byte("withdraw-race-submit"), withdrawnSame.Version)
	if err != nil {
		t.Fatal(err)
	}
	differentWithdraws := make(chan completionResult, 2)
	for _, key := range []string{"withdraw-race-a", "withdraw-race-b"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			value, wasReplay, callErr := svc.Withdraw(ctx, session.ID, session.FamilyID, child.ID, pendingForWithdrawRace.ID, key, []byte(key), pendingForWithdrawRace.Version)
			differentWithdraws <- completionResult{value, wasReplay, callErr}
		}(key)
	}
	wg.Wait()
	close(differentWithdraws)
	withdrawWins, withdrawConflicts := 0, 0
	for result := range differentWithdraws {
		if result.err == nil {
			withdrawWins++
		} else if errors.Is(result.err, ErrInvalidState) || errors.Is(result.err, ErrVersionConflict) {
			withdrawConflicts++
		} else {
			t.Fatalf("different-key withdraw error=%v", result.err)
		}
	}
	if withdrawWins != 1 || withdrawConflicts != 1 {
		t.Fatalf("different-key withdraw wins/conflicts=%d/%d", withdrawWins, withdrawConflicts)
	}
	assertCounts(t, pool, session.FamilyID, sameKeyOccurrence, 2, 4, 0)

	// Submit racing parent cancellation converges on one legal cancelled terminal state.
	parentSession, _, err := auth.NewService(pool).Login(ctx, "phase6-"+suffix+"@example.test", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	decisionTask, _, err := habitSvc.CreateTask(ctx, parentSession.ID, parentSession.UserID, parentSession.FamilyID, "decision-race-task", []byte("decision-race-task"), habits.TaskInput{ChildID: child.ID, Title: "Decision race", DueDate: mustDate(t, today), Points: 2})
	if err != nil {
		t.Fatal(err)
	}
	var decisionOccurrence string
	var decisionVersion int64
	if err = pool.QueryRow(ctx, `SELECT id,version FROM occurrences WHERE task_id=$1`, decisionTask.ID).Scan(&decisionOccurrence, &decisionVersion); err != nil {
		t.Fatal(err)
	}
	decisionPending, _, err := svc.Submit(ctx, session.ID, session.FamilyID, child.ID, decisionOccurrence, "decision-race-submit", []byte("decision-race-submit"), decisionVersion)
	if err != nil {
		t.Fatal(err)
	}
	decisionRace := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _, callErr := svc.Withdraw(ctx, session.ID, session.FamilyID, child.ID, decisionPending.ID, "decision-race-withdraw", []byte("decision-race-withdraw"), decisionPending.Version)
		decisionRace <- callErr
	}()
	go func() {
		defer wg.Done()
		tx, beginErr := pool.Begin(ctx)
		if beginErr != nil {
			decisionRace <- beginErr
			return
		}
		defer tx.Rollback(ctx)
		var state, decision string
		var currentVersion int64
		queryErr := tx.QueryRow(ctx, `SELECT o.state::text,ca.decision::text,o.version FROM occurrences o JOIN completion_attempts ca ON ca.occurrence_id=o.id WHERE o.id=$1 AND ca.id=$2 FOR UPDATE OF o,ca`, decisionOccurrence, decisionPending.ID).Scan(&state, &decision, &currentVersion)
		if queryErr != nil {
			decisionRace <- queryErr
			return
		}
		if state != "pending_approval" || decision != "pending" || currentVersion != decisionPending.Version {
			decisionRace <- ErrInvalidState
			return
		}
		_, queryErr = tx.Exec(ctx, `UPDATE completion_attempts SET decision='rejected',decided_at=now(),decided_by=$2 WHERE id=$1`, decisionPending.ID, parentSession.UserID)
		if queryErr == nil {
			_, queryErr = tx.Exec(ctx, `UPDATE occurrences SET state='not_started',version=version+1,updated_at=now() WHERE id=$1`, decisionOccurrence)
		}
		if queryErr == nil {
			_, queryErr = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status) VALUES($1,$2,$3,'completion.rejected','occurrence',$4,'pending_approval','not_started')`, session.FamilyID, parentSession.UserID, parentSession.ID, decisionOccurrence)
		}
		if queryErr == nil {
			queryErr = tx.Commit(ctx)
		}
		decisionRace <- queryErr
	}()
	wg.Wait()
	close(decisionRace)
	decisionWins, decisionConflicts := 0, 0
	for raceErr := range decisionRace {
		if raceErr == nil {
			decisionWins++
		} else if errors.Is(raceErr, ErrInvalidState) || errors.Is(raceErr, ErrVersionConflict) {
			decisionConflicts++
		} else {
			t.Fatalf("withdraw/decision race error=%v", raceErr)
		}
	}
	if decisionWins != 1 || decisionConflicts != 1 {
		t.Fatalf("withdraw/decision wins/conflicts=%d/%d", decisionWins, decisionConflicts)
	}
	var finalDecision, finalOccurrenceState string
	var finalDecisionVersion, openDecisionAttempts int
	if err = pool.QueryRow(ctx, `SELECT ca.decision::text,o.state::text,o.version,(SELECT count(*) FROM completion_attempts WHERE occurrence_id=o.id AND decision='pending') FROM completion_attempts ca JOIN occurrences o ON o.id=ca.occurrence_id WHERE ca.id=$1`, decisionPending.ID).Scan(&finalDecision, &finalOccurrenceState, &finalDecisionVersion, &openDecisionAttempts); err != nil {
		t.Fatal(err)
	}
	if (finalDecision != "withdrawn" && finalDecision != "rejected") || finalOccurrenceState != "not_started" || finalDecisionVersion != int(decisionPending.Version+1) || openDecisionAttempts != 0 {
		t.Fatalf("withdraw/decision final=%s/%s/v%d pending=%d", finalDecision, finalOccurrenceState, finalDecisionVersion, openDecisionAttempts)
	}
	assertCounts(t, pool, session.FamilyID, decisionOccurrence, 1, 2, 0)

	cancelTask, _, err := habitSvc.CreateTask(ctx, parentSession.ID, parentSession.UserID, parentSession.FamilyID, "cancel-race-task", []byte("cancel-race-task"), habits.TaskInput{ChildID: child.ID, Title: "Cancel race", DueDate: mustDate(t, today), Points: 2})
	if err != nil {
		t.Fatal(err)
	}
	var cancelOccurrence string
	var cancelOccurrenceVersion int64
	if err = pool.QueryRow(ctx, `SELECT id,version FROM occurrences WHERE task_id=$1`, cancelTask.ID).Scan(&cancelOccurrence, &cancelOccurrenceVersion); err != nil {
		t.Fatal(err)
	}
	cancelRace := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _, callErr := svc.Submit(ctx, session.ID, session.FamilyID, child.ID, cancelOccurrence, "cancel-race-submit", []byte("cancel-race-submit"), cancelOccurrenceVersion)
		cancelRace <- callErr
	}()
	go func() {
		defer wg.Done()
		_, callErr := habitSvc.CancelTaskConditional(ctx, parentSession.ID, parentSession.UserID, parentSession.FamilyID, cancelTask.ID, "cancel-race-parent", []byte("cancel-race-parent"), &cancelTask.Version, "Changed plans")
		cancelRace <- callErr
	}()
	wg.Wait()
	close(cancelRace)
	for raceErr := range cancelRace {
		if raceErr != nil && !errors.Is(raceErr, ErrFuture) && !errors.Is(raceErr, ErrInvalidState) && !errors.Is(raceErr, ErrVersionConflict) {
			t.Fatalf("cancel race error=%v", raceErr)
		}
	}
	var cancelState string
	if err = pool.QueryRow(ctx, `SELECT state::text FROM occurrences WHERE id=$1`, cancelOccurrence).Scan(&cancelState); err != nil || cancelState != "cancelled" {
		t.Fatalf("cancel race state=%q err=%v", cancelState, err)
	}
	var pendingAfterCancel int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM completion_attempts WHERE occurrence_id=$1 AND decision='pending'`, cancelOccurrence).Scan(&pendingAfterCancel); err != nil || pendingAfterCancel != 0 {
		t.Fatalf("cancel race pending=%d err=%v", pendingAfterCancel, err)
	}

	// Archival racing submission cannot commit after archive and follows a deadlock-free lock order.
	raceChild, _, err := childSvc.Create(ctx, parentSession.ID, parentSession.UserID, parentSession.FamilyID, "archive-race-child", []byte("archive-race-child"), "Race", "owl", "#445566", "")
	if err != nil {
		t.Fatal(err)
	}
	archiveTask, _, err := habitSvc.CreateTask(ctx, parentSession.ID, parentSession.UserID, parentSession.FamilyID, "archive-race-task", []byte("archive-race-task"), habits.TaskInput{ChildID: raceChild.ID, Title: "Archive race", DueDate: mustDate(t, today), Points: 2})
	if err != nil {
		t.Fatal(err)
	}
	var archiveOccurrence string
	var archiveVersion int64
	if err = pool.QueryRow(ctx, `SELECT id,version FROM occurrences WHERE task_id=$1`, archiveTask.ID).Scan(&archiveOccurrence, &archiveVersion); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL WHERE id=$1`, session.ID, raceChild.ID); err != nil {
		t.Fatal(err)
	}
	archiveRace := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _, callErr := svc.Submit(ctx, session.ID, session.FamilyID, raceChild.ID, archiveOccurrence, "archive-race-submit", []byte("archive-race-submit"), archiveVersion)
		archiveRace <- callErr
	}()
	go func() {
		defer wg.Done()
		archiveRace <- childSvc.Archive(ctx, parentSession.ID, parentSession.UserID, parentSession.FamilyID, raceChild.ID)
	}()
	wg.Wait()
	close(archiveRace)
	for raceErr := range archiveRace {
		if raceErr != nil && !errors.Is(raceErr, ErrForbidden) {
			t.Fatalf("archive race error=%v", raceErr)
		}
	}
	var archived bool
	if err = pool.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM children WHERE id=$1`, raceChild.ID).Scan(&archived); err != nil || !archived {
		t.Fatalf("archive race archived=%v err=%v", archived, err)
	}
	var sessionMode string
	if err = pool.QueryRow(ctx, `SELECT mode::text FROM sessions WHERE id=$1`, session.ID).Scan(&sessionMode); err != nil || sessionMode != "shared" {
		t.Fatalf("archive race session=%q err=%v", sessionMode, err)
	}

	// No Phase 6 action ever awards points.
	var ledger int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger WHERE family_id=$1`, session.FamilyID).Scan(&ledger); err != nil || ledger != 0 {
		t.Fatalf("ledger=%d err=%v", ledger, err)
	}
}

func assertCounts(t *testing.T, pool *pgxpool.Pool, familyID, occurrenceID string, attempts, audits, ledger int) {
	t.Helper()
	var a, u, l int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM completion_attempts WHERE occurrence_id=$1`, occurrenceID).Scan(&a); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE family_id=$1 AND subject_type='occurrence' AND subject_id=$2`, familyID, occurrenceID).Scan(&u); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger WHERE family_id=$1 AND occurrence_id=$2`, familyID, occurrenceID).Scan(&l); err != nil {
		t.Fatal(err)
	}
	if a != attempts || u != audits || l != ledger {
		t.Fatalf("counts attempts/audits/ledger=%d/%d/%d", a, u, l)
	}
}

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func cleanup(t *testing.T, pool *pgxpool.Pool, s auth.Session) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM point_ledger WHERE family_id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM audit_events WHERE family_id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM idempotency_records WHERE family_id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM completion_attempts WHERE family_id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM occurrences WHERE family_id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM habit_schedules WHERE assignment_id IN(SELECT id FROM habit_assignments WHERE family_id=$1)`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM habit_assignments WHERE family_id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM habit_versions WHERE family_id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM one_off_tasks WHERE family_id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM habits WHERE family_id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM sessions WHERE family_id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM children WHERE family_id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM family_memberships WHERE family_id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM families WHERE id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, s.UserID)
	})
}
