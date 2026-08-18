package points

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/family-habit/family-habit/backend/internal/auth"
	"github.com/family-habit/family-habit/backend/internal/children"
	"github.com/family-habit/family-habit/backend/internal/completions"
	"github.com/family-habit/family-habit/backend/internal/database"
	"github.com/family-habit/family-habit/backend/internal/habits"
)

// TestPointsLedgerAndReportingIntegration is opt-in because it requires a
// disposable migrated PostgreSQL database.
func TestPointsLedgerAndReportingIntegration(t *testing.T) {
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
	parent, _, err := auth.NewService(pool).Register(ctx, "phase7-"+suffix+"@example.test", "correct horse battery staple", "Phase 7", "Asia/Jakarta", 1)
	if err != nil {
		t.Fatal(err)
	}
	child, _, err := children.NewService(pool).Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "p7-child-"+suffix, []byte("child"+suffix), "Maya", "fox", "#112233", "")
	if err != nil {
		t.Fatal(err)
	}
	habitSvc := habits.NewService(pool)
	task, _, err := habitSvc.CreateTask(ctx, parent.ID, parent.UserID, parent.FamilyID, "task-"+suffix, []byte("task"+suffix), habits.TaskInput{ChildID: child.ID, Title: "Read", Description: "Ten pages", DueDate: mustPointDate(t, "2026-08-09"), Points: 7})
	if err != nil {
		t.Fatal(err)
	}
	var occurrence string
	var version int64
	if err = pool.QueryRow(ctx, `SELECT id,version FROM occurrences WHERE family_id=$1 AND task_id=$2`, parent.FamilyID, task.ID).Scan(&occurrence, &version); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL WHERE id=$1`, parent.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	completionSvc := completions.NewService(pool)
	submitted, _, err := completionSvc.Submit(ctx, parent.ID, parent.FamilyID, child.ID, occurrence, "submit-"+suffix, []byte("submit"+suffix), version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='parent',active_child_id=NULL,parent_unlocked_at=now() WHERE id=$1`, parent.ID); err != nil {
		t.Fatal(err)
	}
	svc := NewService(pool)
	queue, next, err := svc.Pending(ctx, parent.ID, parent.FamilyID, child.ID, "", 1)
	if err != nil || len(queue) != 1 || queue[0].ID != submitted.ID || next != "" {
		t.Fatalf("queue=%+v next=%q err=%v", queue, next, err)
	}
	for _, stage := range []string{"attempt_decision", "occurrence_update", "ledger_insert", "audit_insert", "idempotency_finish"} {
		svc.fault = func(got string) error {
			if got == stage {
				return errors.New("injected " + stage)
			}
			return nil
		}
		key := "fault-" + stage + suffix
		if _, _, err = svc.Approve(ctx, parent.ID, parent.FamilyID, submitted.ID, key, []byte(key), submitted.Version); err == nil {
			t.Fatalf("fault %s succeeded", stage)
		}
		var attemptState, occurrenceState string
		var currentVersion int64
		var effects int
		if err = pool.QueryRow(ctx, `SELECT ca.decision::text,o.state::text,o.version FROM completion_attempts ca JOIN occurrences o ON o.id=ca.occurrence_id WHERE ca.id=$1`, submitted.ID).Scan(&attemptState, &occurrenceState, &currentVersion); err != nil {
			t.Fatal(err)
		}
		if attemptState != "pending" || occurrenceState != "pending_approval" || currentVersion != submitted.Version {
			t.Fatalf("fault %s leaked %s/%s/v%d", stage, attemptState, occurrenceState, currentVersion)
		}
		if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM point_ledger WHERE occurrence_id=$1)+(SELECT count(*) FROM audit_events WHERE subject_id=$1 AND action='completion.approved')+(SELECT count(*) FROM idempotency_records WHERE idempotency_key=$2)`, occurrence, key).Scan(&effects); err != nil || effects != 0 {
			t.Fatalf("fault %s effects=%d err=%v", stage, effects, err)
		}
	}
	svc.fault = nil

	type result struct {
		o   Completion
		err error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			o, _, e := svc.Approve(ctx, parent.ID, parent.FamilyID, submitted.ID, "approve-"+suffix+string(rune('a'+n)), []byte{byte(n + 1)}, submitted.Version)
			results <- result{o, e}
		}(i)
	}
	wg.Wait()
	close(results)
	successes := 0
	for r := range results {
		if r.err == nil {
			successes++
			if r.o.LedgerAmount != 7 || r.o.LedgerEntryID == "" {
				t.Fatalf("approval linkage=%+v", r.o)
			}
		} else if !errors.Is(r.err, ErrVersionConflict) && !errors.Is(r.err, ErrInvalidState) {
			t.Fatalf("concurrent approve err=%v", r.err)
		}
	}
	if successes != 1 {
		t.Fatalf("approve successes=%d", successes)
	}
	childSession, _, err := auth.NewService(pool).Login(ctx, "phase7-"+suffix+"@example.test", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL WHERE id=$1`, childSession.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	makePending := func(date string) (completions.Completion, error) {
		task, _, e := habitSvc.CreateTask(ctx, parent.ID, parent.UserID, parent.FamilyID, "race-task-"+date+suffix, []byte("race-task"+date+suffix), habits.TaskInput{ChildID: child.ID, Title: "Race task " + date, DueDate: mustPointDate(t, date), Points: 7})
		if e != nil {
			return completions.Completion{}, e
		}
		var oid string
		var v int64
		if e := pool.QueryRow(ctx, `SELECT id,version FROM occurrences WHERE family_id=$1 AND child_id=$2 AND task_id=$3`, parent.FamilyID, child.ID, task.ID).Scan(&oid, &v); e != nil {
			return completions.Completion{}, e
		}
		c, _, e := completionSvc.Submit(ctx, childSession.ID, parent.FamilyID, child.ID, oid, "submit-"+date+suffix, []byte(date+suffix), v)
		return c, e
	}
	makeApproved := func(date string) (Completion, error) {
		p, e := makePending(date)
		if e != nil {
			return Completion{}, e
		}
		o, _, e := svc.Approve(ctx, parent.ID, parent.FamilyID, p.ID, "prepare-approved-"+date+suffix, []byte("prepare"+date+suffix), p.Version)
		return o, e
	}
	for i, stage := range []string{"reverse_occurrence_update", "reverse_ledger_insert", "reverse_audit_insert", "reverse_idempotency_finish"} {
		approved, e := makeApproved(fmt.Sprintf("2026-07-%02d", 20+i))
		if e != nil {
			t.Fatal(e)
		}
		svc.fault = func(got string) error {
			if got == stage {
				return errors.New("injected " + stage)
			}
			return nil
		}
		key := "fault-" + stage + suffix
		if _, _, e = svc.Reverse(ctx, parent.ID, parent.FamilyID, approved.ID, "fault", key, []byte(key), approved.Version); e == nil {
			t.Fatalf("fault %s succeeded", stage)
		}
		svc.fault = nil
		var state string
		var version int64
		var reversals, idems int
		if e = pool.QueryRow(ctx, `SELECT state::text,version,(SELECT count(*) FROM point_ledger WHERE occurrence_id=$1 AND kind='approval_reversal'),(SELECT count(*) FROM idempotency_records WHERE idempotency_key=$2) FROM occurrences WHERE id=$1`, approved.OccurrenceID, key).Scan(&state, &version, &reversals, &idems); e != nil || state != "approved" || version != approved.Version || reversals != 0 || idems != 0 {
			t.Fatalf("reverse fault %s state=%s/v%d effects=%d/%d err=%v", stage, state, version, reversals, idems, e)
		}
	}
	for _, stage := range []string{"correction_ledger_insert", "correction_audit_insert", "correction_idempotency_finish"} {
		svc.fault = func(got string) error {
			if got == stage {
				return errors.New("injected " + stage)
			}
			return nil
		}
		key := "fault-" + stage + suffix
		if _, _, e := svc.Correct(ctx, parent.ID, parent.FamilyID, child.ID, 4, "fault correction", key, []byte(key)); e == nil {
			t.Fatalf("fault %s succeeded", stage)
		}
		svc.fault = nil
		var effects int
		if e := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM point_ledger WHERE reason='fault correction' AND family_id=$1)+(SELECT count(*) FROM audit_events WHERE idempotency_key=$2)+(SELECT count(*) FROM idempotency_records WHERE idempotency_key=$2)`, parent.FamilyID, key).Scan(&effects); e != nil || effects != 0 {
			t.Fatalf("correction fault %s effects=%d err=%v", stage, effects, e)
		}
	}
	for _, tc := range []struct{ date, action string }{{"2026-07-27", "approve"}, {"2026-07-28", "reject"}} {
		pending, e := makePending(tc.date)
		if e != nil {
			t.Fatal(e)
		}
		type sameResult struct {
			replay bool
			err    error
		}
		same := make(chan sameResult, 2)
		key := "same-" + tc.action + suffix
		hash := []byte(key)
		for range 2 {
			go func() {
				var r bool
				var callErr error
				if tc.action == "approve" {
					_, r, callErr = svc.Approve(ctx, parent.ID, parent.FamilyID, pending.ID, key, hash, pending.Version)
				} else {
					_, r, callErr = svc.Reject(ctx, parent.ID, parent.FamilyID, pending.ID, "Try again", key, hash, pending.Version)
				}
				same <- sameResult{r, callErr}
			}()
		}
		a, b := <-same, <-same
		if a.err != nil || b.err != nil || a.replay == b.replay {
			t.Fatalf("same-key %s=%+v/%+v", tc.action, a, b)
		}
		auditAction := "completion.approved"
		if tc.action == "reject" {
			auditAction = "completion.rejected"
		}
		var attempts, decisionAudits, decisionLedger int
		if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM completion_attempts WHERE id=$1),(SELECT count(*) FROM audit_events WHERE subject_id=$2 AND action=$3),(SELECT count(*) FROM point_ledger WHERE occurrence_id=$2)`, pending.ID, pending.OccurrenceID, auditAction).Scan(&attempts, &decisionAudits, &decisionLedger); err != nil {
			t.Fatal(err)
		}
		expectedLedger := 0
		if tc.action == "approve" {
			expectedLedger = 1
		}
		if attempts != 1 || decisionAudits != 1 || decisionLedger != expectedLedger {
			t.Fatalf("same-key %s effects=%d/%d/%d", tc.action, attempts, decisionAudits, decisionLedger)
		}
	}
	staleReverse, err := makeApproved("2026-07-25")
	if err != nil {
		t.Fatal(err)
	}
	reverseRace := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func(n int) {
			_, _, e := svc.Reverse(ctx, parent.ID, parent.FamilyID, staleReverse.ID, "race reverse", fmt.Sprintf("reverse-race-%d-%s", n, suffix), []byte{byte(n + 1)}, staleReverse.Version)
			reverseRace <- e
		}(i)
	}
	rr1, rr2 := <-reverseRace, <-reverseRace
	if (rr1 == nil) == (rr2 == nil) {
		t.Fatalf("different-key reverse=%v/%v", rr1, rr2)
	}
	var reversalRows int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger WHERE occurrence_id=$1 AND kind='approval_reversal'`, staleReverse.OccurrenceID).Scan(&reversalRows); err != nil || reversalRows != 1 {
		t.Fatalf("reverse race rows=%d err=%v", reversalRows, err)
	}
	rejectWithdraw, err := makePending("2026-07-26")
	if err != nil {
		t.Fatal(err)
	}
	transitionRace := make(chan error, 2)
	go func() {
		_, _, e := svc.Reject(ctx, parent.ID, parent.FamilyID, rejectWithdraw.ID, "again", "reject-withdraw-race-"+suffix, []byte("reject-withdraw"+suffix), rejectWithdraw.Version)
		transitionRace <- e
	}()
	go func() {
		_, _, e := completionSvc.Withdraw(ctx, childSession.ID, parent.FamilyID, child.ID, rejectWithdraw.ID, "withdraw-reject-race-"+suffix, []byte("withdraw-reject"+suffix), rejectWithdraw.Version)
		transitionRace <- e
	}()
	rw1, rw2 := <-transitionRace, <-transitionRace
	if (rw1 == nil) == (rw2 == nil) {
		t.Fatalf("reject/withdraw race=%v/%v", rw1, rw2)
	}
	var rwAttempt, rwOccurrence string
	if err = pool.QueryRow(ctx, `SELECT ca.decision::text,o.state::text FROM completion_attempts ca JOIN occurrences o ON o.id=ca.occurrence_id WHERE ca.id=$1`, rejectWithdraw.ID).Scan(&rwAttempt, &rwOccurrence); err != nil {
		t.Fatal(err)
	}
	if !((rwAttempt == "rejected" || rwAttempt == "withdrawn") && rwOccurrence == "not_started") {
		t.Fatalf("reject/withdraw state=%s/%s", rwAttempt, rwOccurrence)
	}
	decisionRace, err := makePending("2026-07-30")
	if err != nil {
		t.Fatal(err)
	}
	traces := make(chan error, 2)
	go func() {
		_, _, e := svc.Approve(ctx, parent.ID, parent.FamilyID, decisionRace.ID, "race-approve-"+suffix, []byte("ra"), decisionRace.Version)
		traces <- e
	}()
	go func() {
		_, _, e := svc.Reject(ctx, parent.ID, parent.FamilyID, decisionRace.ID, "Try once more", "race-reject-"+suffix, []byte("rr"), decisionRace.Version)
		traces <- e
	}()
	r1, r2 := <-traces, <-traces
	if (r1 == nil) == (r2 == nil) {
		t.Fatalf("approve/reject race errors=%v/%v", r1, r2)
	}
	var raceAttempt, raceOccurrence string
	var raceLedger int
	if err = pool.QueryRow(ctx, `SELECT ca.decision::text,o.state::text,(SELECT count(*) FROM point_ledger WHERE occurrence_id=o.id) FROM completion_attempts ca JOIN occurrences o ON o.id=ca.occurrence_id WHERE ca.id=$1`, decisionRace.ID).Scan(&raceAttempt, &raceOccurrence, &raceLedger); err != nil {
		t.Fatal(err)
	}
	if !((raceAttempt == "approved" && raceOccurrence == "approved" && raceLedger == 1) || (raceAttempt == "rejected" && raceOccurrence == "not_started" && raceLedger == 0)) {
		t.Fatalf("approve/reject state=%s/%s ledger=%d", raceAttempt, raceOccurrence, raceLedger)
	}
	withdrawRace, err := makePending("2026-07-31")
	if err != nil {
		t.Fatal(err)
	}
	traces = make(chan error, 2)
	go func() {
		_, _, e := svc.Approve(ctx, parent.ID, parent.FamilyID, withdrawRace.ID, "race-approve-withdraw-"+suffix, []byte("raw"), withdrawRace.Version)
		traces <- e
	}()
	go func() {
		_, _, e := completionSvc.Withdraw(ctx, childSession.ID, parent.FamilyID, child.ID, withdrawRace.ID, "race-withdraw-"+suffix, []byte("rw"), withdrawRace.Version)
		traces <- e
	}()
	r1, r2 = <-traces, <-traces
	if (r1 == nil) == (r2 == nil) {
		t.Fatalf("approve/withdraw race errors=%v/%v", r1, r2)
	}
	if err = pool.QueryRow(ctx, `SELECT ca.decision::text,o.state::text,(SELECT count(*) FROM point_ledger WHERE occurrence_id=o.id) FROM completion_attempts ca JOIN occurrences o ON o.id=ca.occurrence_id WHERE ca.id=$1`, withdrawRace.ID).Scan(&raceAttempt, &raceOccurrence, &raceLedger); err != nil {
		t.Fatal(err)
	}
	if !((raceAttempt == "approved" && raceOccurrence == "approved" && raceLedger == 1) || (raceAttempt == "withdrawn" && raceOccurrence == "not_started" && raceLedger == 0)) {
		t.Fatalf("approve/withdraw state=%s/%s ledger=%d", raceAttempt, raceOccurrence, raceLedger)
	}
	var awards, audits int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger WHERE occurrence_id=$1 AND kind='award'`, occurrence).Scan(&awards); err != nil || awards != 1 {
		t.Fatalf("awards=%d err=%v", awards, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE subject_id=$1 AND action='completion.approved'`, occurrence).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audits=%d err=%v", audits, err)
	}

	var approvedVersion int64
	_ = pool.QueryRow(ctx, `SELECT version FROM occurrences WHERE id=$1`, occurrence).Scan(&approvedVersion)
	type reverseResult struct {
		o      Completion
		replay bool
		err    error
	}
	reverseResults := make(chan reverseResult, 2)
	for range 2 {
		go func() {
			o, r, e := svc.Reverse(ctx, parent.ID, parent.FamilyID, submitted.ID, "Parent correction", "reverse-"+suffix, []byte("reverse"+suffix), approvedVersion)
			reverseResults <- reverseResult{o, r, e}
		}()
	}
	rv1, rv2 := <-reverseResults, <-reverseResults
	if rv1.err != nil || rv2.err != nil || rv1.replay == rv2.replay {
		t.Fatalf("concurrent reverse=%+v/%+v", rv1, rv2)
	}
	reversed := rv1.o
	if rv1.replay {
		reversed = rv2.o
	}
	if reversed.LedgerAmount != -7 {
		t.Fatalf("reverse=%+v", reversed)
	}
	if _, _, err = svc.Reverse(ctx, parent.ID, parent.FamilyID, submitted.ID, "again", "reverse-other-"+suffix, []byte("other"), reversed.Version); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("second reverse=%v", err)
	}
	var awardID string
	_ = pool.QueryRow(ctx, `SELECT id FROM point_ledger WHERE occurrence_id=$1 AND kind='award'`, occurrence).Scan(&awardID)
	if _, err = pool.Exec(ctx, `UPDATE point_ledger SET amount=99 WHERE id=$1`, awardID); err == nil {
		t.Fatal("ledger update unexpectedly succeeded")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM point_ledger WHERE id=$1`, awardID); err == nil {
		t.Fatal("ledger delete unexpectedly succeeded")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO point_ledger(family_id,child_id,occurrence_id,kind,amount,reason,actor_user_id,reverses_entry_id) VALUES($1,$2,$3,'approval_reversal',-6,'wrong',$4,$5)`, parent.FamilyID, child.ID, occurrence, parent.UserID, awardID); err == nil {
		t.Fatal("inexact or duplicate reversal unexpectedly accepted")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO point_ledger(family_id,child_id,kind,amount,reason,actor_user_id) VALUES($1,$2,'manual_correction',-1,'wrong sign',$3)`, parent.FamilyID, child.ID, parent.UserID); err == nil {
		t.Fatal("negative manual correction unexpectedly accepted")
	}

	correction, replay, err := svc.Correct(ctx, parent.ID, parent.FamilyID, child.ID, 10, "Bonus", "correct-"+suffix, []byte("correct"+suffix))
	if err != nil || replay || correction.Amount != 10 {
		t.Fatalf("correction=%+v replay=%v err=%v", correction, replay, err)
	}
	if _, _, err = svc.Correct(ctx, parent.ID, parent.FamilyID, child.ID, -1, "punishment", "bad-"+suffix, []byte("bad")); !errors.Is(err, ErrValidation) {
		t.Fatalf("negative correction=%v", err)
	}
	balance, err := svc.Balance(ctx, parent.ID, parent.FamilyID, child.ID)
	var ledgerSum int64
	if scanErr := pool.QueryRow(ctx, `SELECT coalesce(sum(amount),0) FROM point_ledger WHERE family_id=$1 AND child_id=$2`, parent.FamilyID, child.ID).Scan(&ledgerSum); scanErr != nil {
		t.Fatal(scanErr)
	}
	if err != nil || balance != ledgerSum || balance < 10 {
		t.Fatalf("balance=%d err=%v", balance, err)
	}
	ledger, cursor, err := svc.Ledger(ctx, parent.ID, parent.FamilyID, child.ID, "", 1)
	if err != nil || len(ledger) != 1 || cursor == "" {
		t.Fatalf("ledger=%+v cursor=%q err=%v", ledger, cursor, err)
	}
	if _, _, err = svc.Ledger(ctx, parent.ID, parent.FamilyID, child.ID, encodeCursor(cursorValue{Kind: "ledger", Binding: cursorBinding(parent.FamilyID, "other"), Time: ledger[0].CreatedAt.Format(time.RFC3339Nano), ID: ledger[0].ID}), 1); !errors.Is(err, ErrValidation) {
		t.Fatalf("filter cursor=%v", err)
	}
	history, _, err := svc.History(ctx, parent.ID, parent.FamilyID, child.ID, "2026-08-01", "2026-08-31", "", 10)
	if err != nil || len(history) != 1 || len(history[0].Attempts) != 1 || history[0].Attempts[0].ID != submitted.ID || history[0].AwardDelta != 7 || history[0].ReversalDelta != -7 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	// Durable mixed-state report fixtures: two pending (one after a rejection),
	// one approved, one incomplete, the existing reversed occurrence, and one
	// separately cancelled task.
	if _, err = makePending("2026-08-02"); err != nil {
		t.Fatal(err)
	}
	if _, err = makeApproved("2026-08-03"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = habitSvc.CreateTask(ctx, parent.ID, parent.UserID, parent.FamilyID, "report-incomplete-"+suffix, []byte("report-incomplete"+suffix), habits.TaskInput{ChildID: child.ID, Title: "Incomplete", DueDate: mustPointDate(t, "2026-08-04"), Points: 2}); err != nil {
		t.Fatal(err)
	}
	reportRejected, err := makePending("2026-08-05")
	if err != nil {
		t.Fatal(err)
	}
	reportRejection, _, err := svc.Reject(ctx, parent.ID, parent.FamilyID, reportRejected.ID, "Try again", "report-reject-"+suffix, []byte("report-reject"+suffix), reportRejected.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = completionSvc.Submit(ctx, childSession.ID, parent.FamilyID, child.ID, reportRejected.OccurrenceID, "report-resubmit-"+suffix, []byte("report-resubmit"+suffix), reportRejection.Version); err != nil {
		t.Fatal(err)
	}
	cancelledTask, _, err := habitSvc.CreateTask(ctx, parent.ID, parent.UserID, parent.FamilyID, "report-cancel-"+suffix, []byte("report-cancel"+suffix), habits.TaskInput{ChildID: child.ID, Title: "Cancelled", DueDate: mustPointDate(t, "2026-08-06"), Points: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err = habitSvc.CancelTask(ctx, parent.ID, parent.UserID, parent.FamilyID, cancelledTask.ID); err != nil {
		t.Fatal(err)
	}
	var writesBefore, writesAfter int64
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM occurrences WHERE family_id=$1)+(SELECT count(*) FROM point_ledger WHERE family_id=$1)+(SELECT count(*) FROM audit_events WHERE family_id=$1)`, parent.FamilyID).Scan(&writesBefore); err != nil {
		t.Fatal(err)
	}
	report, err := svc.Report(ctx, parent.ID, parent.FamilyID, child.ID, "month", "2026-08-09")
	if err != nil || report.StartDate != "2026-08-01" || report.EndDate != "2026-08-31" || report.Assigned != 5 || report.Pending != 2 || report.Approved != 1 || report.Reversed != 1 || report.Incomplete != 1 || report.Cancelled != 1 || report.Submitted != 4 || report.Rejected != 1 || report.PointsEarned != 7 || report.ManualCorrections != 10 || report.NetPointsChange != 17 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM occurrences WHERE family_id=$1)+(SELECT count(*) FROM point_ledger WHERE family_id=$1)+(SELECT count(*) FROM audit_events WHERE family_id=$1)`, parent.FamilyID).Scan(&writesAfter); err != nil || writesAfter != writesBefore {
		t.Fatalf("report wrote data before/after=%d/%d err=%v", writesBefore, writesAfter, err)
	}
	week, err := svc.Report(ctx, parent.ID, parent.FamilyID, child.ID, "week", "2026-08-09")
	if err != nil || week.StartDate != "2026-08-03" || week.EndDate != "2026-08-09" {
		t.Fatalf("Jakarta week=%+v err=%v", week, err)
	}
	boundaryChild, _, err := children.NewService(pool).Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "boundary-child-"+suffix, []byte("boundary"+suffix), "Ivy", "owl", "#556677", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO point_ledger(family_id,child_id,kind,amount,reason,actor_user_id,created_at) VALUES($1,$2,'manual_correction',3,'midnight boundary',$3,'2026-08-08 17:00:00+00')`, parent.FamilyID, boundaryChild.ID, parent.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO point_ledger(family_id,child_id,kind,amount,reason,actor_user_id,created_at) VALUES($1,$2,'manual_correction',4,'before midnight',$3,'2026-08-08 16:59:59+00')`, parent.FamilyID, boundaryChild.ID, parent.UserID); err != nil {
		t.Fatal(err)
	}
	beforeBoundary, err := svc.Report(ctx, parent.ID, parent.FamilyID, boundaryChild.ID, "day", "2026-08-08")
	if err != nil || beforeBoundary.ManualCorrections != 4 || beforeBoundary.NetPointsChange != 4 {
		t.Fatalf("Jakarta before-midnight report=%+v err=%v", beforeBoundary, err)
	}
	boundary, err := svc.Report(ctx, parent.ID, parent.FamilyID, boundaryChild.ID, "day", "2026-08-09")
	if err != nil || boundary.ManualCorrections != 3 || boundary.PointsEarned != 0 || boundary.NetPointsChange != 3 {
		t.Fatalf("Jakarta midnight report=%+v err=%v", boundary, err)
	}
	other, _, err := auth.NewService(pool).Register(ctx, "phase7-other-"+suffix+"@example.test", "correct horse battery staple", "Other", "Europe/Berlin", 0)
	if err != nil {
		t.Fatal(err)
	}
	otherChild, _, err := children.NewService(pool).Create(ctx, other.ID, other.UserID, other.FamilyID, "other-child-"+suffix, []byte("other"+suffix), "Noah", "bear", "#445566", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Balance(ctx, parent.ID, parent.FamilyID, otherChild.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-family balance=%v", err)
	}
	berlin, err := svc.Report(ctx, other.ID, other.FamilyID, otherChild.ID, "day", "2026-03-29")
	if err != nil || berlin.StartDate != "2026-03-29" || berlin.EndDate != "2026-03-29" || berlin.Timezone != "Europe/Berlin" {
		t.Fatalf("Berlin DST day=%+v err=%v", berlin, err)
	}
	for i, stamp := range []string{"2026-03-28 22:59:59+00", "2026-03-28 23:00:00+00", "2026-03-29 00:59:59+00", "2026-03-29 01:00:00+00", "2026-03-29 21:59:59+00", "2026-03-29 22:00:00+00", "2026-10-25 00:30:00+00", "2026-10-25 01:30:00+00"} {
		if _, err = pool.Exec(ctx, `INSERT INTO point_ledger(family_id,child_id,kind,amount,reason,actor_user_id,created_at) VALUES($1,$2,'manual_correction',1,$3,$4,$5::timestamptz)`, other.FamilyID, otherChild.ID, fmt.Sprintf("dst-%d", i), other.UserID, stamp); err != nil {
			t.Fatal(err)
		}
	}
	spring, err := svc.Report(ctx, other.ID, other.FamilyID, otherChild.ID, "day", "2026-03-29")
	if err != nil || spring.ManualCorrections != 4 {
		t.Fatalf("Berlin spring DST=%+v err=%v", spring, err)
	}
	springBefore, _ := svc.Report(ctx, other.ID, other.FamilyID, otherChild.ID, "day", "2026-03-28")
	springAfter, _ := svc.Report(ctx, other.ID, other.FamilyID, otherChild.ID, "day", "2026-03-30")
	if springBefore.ManualCorrections != 1 || springAfter.ManualCorrections != 1 {
		t.Fatalf("Berlin spring adjacent=%+v/%+v", springBefore, springAfter)
	}
	autumn, err := svc.Report(ctx, other.ID, other.FamilyID, otherChild.ID, "day", "2026-10-25")
	if err != nil || autumn.ManualCorrections != 2 {
		t.Fatalf("Berlin autumn fold=%+v err=%v", autumn, err)
	}
	// A ledger actor must belong to the exact family; child and occurrence scope
	// are also enforced before application code can observe a hostile row.
	if _, err = pool.Exec(ctx, `INSERT INTO point_ledger(family_id,child_id,kind,amount,reason,actor_user_id) VALUES($1,$2,'manual_correction',1,'hostile',$3)`, parent.FamilyID, child.ID, other.UserID); err == nil {
		t.Fatal("cross-family ledger actor unexpectedly accepted")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO point_ledger(family_id,child_id,kind,amount,reason,actor_user_id) VALUES($1,$2,'manual_correction',1,'hostile',$3)`, parent.FamilyID, otherChild.ID, parent.UserID); err == nil {
		t.Fatal("cross-family ledger child unexpectedly accepted")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO point_ledger(family_id,child_id,occurrence_id,completion_attempt_id,kind,amount,reason,actor_user_id) SELECT family_id,child_id,occurrence_id,id,'award',7,'duplicate',$2 FROM completion_attempts WHERE id=$1`, submitted.ID, parent.UserID); err == nil {
		t.Fatal("duplicate award unexpectedly accepted")
	}

	type correctionResult struct {
		replay bool
		err    error
	}
	correctionResults := make(chan correctionResult, 2)
	for range 2 {
		go func() {
			_, replayed, e := svc.Correct(ctx, parent.ID, parent.FamilyID, child.ID, 5, "Concurrent bonus", "same-correction-"+suffix, []byte("same-correction"+suffix))
			correctionResults <- correctionResult{replayed, e}
		}()
	}
	cr1, cr2 := <-correctionResults, <-correctionResults
	if cr1.err != nil || cr2.err != nil || cr1.replay == cr2.replay {
		t.Fatalf("concurrent correction=%+v/%+v", cr1, cr2)
	}
	var correctionRows int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger WHERE family_id=$1 AND child_id=$2 AND kind='manual_correction' AND reason='Concurrent bonus'`, parent.FamilyID, child.ID).Scan(&correctionRows); err != nil || correctionRows != 1 {
		t.Fatalf("concurrent correction rows=%d err=%v", correctionRows, err)
	}
	if _, _, err = svc.Correct(ctx, parent.ID, parent.FamilyID, child.ID, 6, "Changed", "same-correction-"+suffix, []byte("changed")); !errors.Is(err, ErrIdempotency) {
		t.Fatalf("correction hash conflict=%v", err)
	}
	attributionChild, _, err := children.NewService(pool).Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "attribution-"+suffix, []byte("attribution"+suffix), "Zoe", "cat", "#667788", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET active_child_id=$2 WHERE id=$1`, childSession.ID, attributionChild.ID); err != nil {
		t.Fatal(err)
	}
	attributionTask, _, err := habitSvc.CreateTask(ctx, parent.ID, parent.UserID, parent.FamilyID, "attribution-task-"+suffix, []byte("attribution-task"+suffix), habits.TaskInput{ChildID: attributionChild.ID, Title: "July attribution", DueDate: mustPointDate(t, "2026-07-15"), Points: 9})
	if err != nil {
		t.Fatal(err)
	}
	var attributionOccurrence string
	var attributionVersion int64
	if err = pool.QueryRow(ctx, `SELECT id,version FROM occurrences WHERE task_id=$1`, attributionTask.ID).Scan(&attributionOccurrence, &attributionVersion); err != nil {
		t.Fatal(err)
	}
	attributionSubmission, _, err := completionSvc.Submit(ctx, childSession.ID, parent.FamilyID, attributionChild.ID, attributionOccurrence, "attribution-submit-"+suffix, []byte("attribution-submit"+suffix), attributionVersion)
	if err != nil {
		t.Fatal(err)
	}
	attributionApproval, _, err := svc.Approve(ctx, parent.ID, parent.FamilyID, attributionSubmission.ID, "attribution-approve-"+suffix, []byte("attribution-approve"+suffix), attributionSubmission.Version)
	if err != nil {
		t.Fatal(err)
	}
	var decisionMonth string
	if err = pool.QueryRow(ctx, `SELECT to_char(created_at,'YYYY-MM') FROM point_ledger WHERE occurrence_id=$1 AND kind='award'`, attributionOccurrence).Scan(&decisionMonth); err != nil || decisionMonth == "2026-07" {
		t.Fatalf("decision month=%q err=%v", decisionMonth, err)
	}
	julyAwarded, err := svc.Report(ctx, parent.ID, parent.FamilyID, attributionChild.ID, "month", "2026-07-15")
	if err != nil || julyAwarded.Approved != 1 || julyAwarded.PointsEarned != 9 {
		t.Fatalf("July approval attribution=%+v err=%v", julyAwarded, err)
	}
	decisionPeriodBeforeReverse, err := svc.Report(ctx, parent.ID, parent.FamilyID, attributionChild.ID, "month", decisionMonth+"-01")
	if err != nil || decisionPeriodBeforeReverse.PointsEarned != 0 {
		t.Fatalf("approval leaked into decision period=%+v err=%v", decisionPeriodBeforeReverse, err)
	}
	if _, _, err = svc.Reverse(ctx, parent.ID, parent.FamilyID, attributionSubmission.ID, "later correction", "attribution-reverse-"+suffix, []byte("attribution-reverse"+suffix), attributionApproval.Version); err != nil {
		t.Fatal(err)
	}
	julyAttribution, err := svc.Report(ctx, parent.ID, parent.FamilyID, attributionChild.ID, "month", "2026-07-15")
	if err != nil || julyAttribution.Reversed != 1 || julyAttribution.PointsEarned != 0 {
		t.Fatalf("July attribution=%+v err=%v", julyAttribution, err)
	}
	decisionPeriod, err := svc.Report(ctx, parent.ID, parent.FamilyID, attributionChild.ID, "month", decisionMonth+"-01")
	if err != nil || decisionPeriod.PointsEarned != 0 {
		t.Fatalf("decision-period leakage=%+v err=%v", decisionPeriod, err)
	}
	consistencyTask, _, err := habitSvc.CreateTask(ctx, parent.ID, parent.UserID, parent.FamilyID, "consistency-task-"+suffix, []byte("consistency-task"+suffix), habits.TaskInput{ChildID: attributionChild.ID, Title: "Consistent read", DueDate: mustPointDate(t, "2026-07-16"), Points: 6})
	if err != nil {
		t.Fatal(err)
	}
	var consistencyOccurrence string
	var consistencyVersion int64
	if err = pool.QueryRow(ctx, `SELECT id,version FROM occurrences WHERE task_id=$1`, consistencyTask.ID).Scan(&consistencyOccurrence, &consistencyVersion); err != nil {
		t.Fatal(err)
	}
	consistencySubmission, _, err := completionSvc.Submit(ctx, childSession.ID, parent.FamilyID, attributionChild.ID, consistencyOccurrence, "consistency-submit-"+suffix, []byte("consistency-submit"+suffix), consistencyVersion)
	if err != nil {
		t.Fatal(err)
	}
	reached, release := make(chan struct{}), make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	svc.fault = func(stage string) error {
		if stage == "occurrence_update" {
			close(reached)
			<-release
		}
		return nil
	}
	approveDone := make(chan error, 1)
	go func() {
		_, _, e := svc.Approve(ctx, parent.ID, parent.FamilyID, consistencySubmission.ID, "consistency-approve-"+suffix, []byte("consistency-approve"+suffix), consistencySubmission.Version)
		approveDone <- e
	}()
	<-reached
	during, err := svc.Report(ctx, parent.ID, parent.FamilyID, attributionChild.ID, "month", "2026-07-15")
	if err != nil || during.Pending != 1 || during.Approved != 0 || during.Reversed != 1 || during.PointsEarned != 0 {
		t.Fatalf("report during uncommitted approval=%+v err=%v", during, err)
	}
	close(release)
	released = true
	if err = <-approveDone; err != nil {
		t.Fatal(err)
	}
	svc.fault = nil
	after, err := svc.Report(ctx, parent.ID, parent.FamilyID, attributionChild.ID, "month", "2026-07-15")
	if err != nil || after.Pending != 0 || after.Approved != 1 || after.Reversed != 1 || after.PointsEarned != 6 {
		t.Fatalf("report after approval=%+v err=%v", after, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET active_child_id=$2 WHERE id=$1`, childSession.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	rejected, err := makePending("2026-07-29")
	if err != nil {
		t.Fatal(err)
	}
	rejection, _, err := svc.Reject(ctx, parent.ID, parent.FamilyID, rejected.ID, "Please try again", "reject-resubmit-"+suffix, []byte("reject-resubmit"+suffix), rejected.Version)
	if err != nil || rejection.OccurrenceStatus != "not_started" {
		t.Fatalf("reject=%+v err=%v", rejection, err)
	}
	resubmitted, _, err := completionSvc.Submit(ctx, childSession.ID, parent.FamilyID, child.ID, rejected.OccurrenceID, "resubmit-"+suffix, []byte("resubmit"+suffix), rejection.Version)
	if err != nil || resubmitted.AttemptNumber != 2 {
		t.Fatalf("resubmit=%+v err=%v", resubmitted, err)
	}
	if err = children.NewService(pool).Archive(ctx, parent.ID, parent.UserID, parent.FamilyID, child.ID); err != nil {
		t.Fatal(err)
	}
	archivedQueue, _, err := svc.Pending(ctx, parent.ID, parent.FamilyID, child.ID, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	foundArchived := false
	for _, item := range archivedQueue {
		if item.ID == resubmitted.ID {
			foundArchived = true
		}
	}
	if !foundArchived {
		t.Fatal("archived pending submission disappeared from parent queue")
	}
	if _, err = svc.Balance(ctx, childSession.ID, parent.FamilyID, child.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived child balance access=%v", err)
	}
	if _, err = svc.Balance(ctx, parent.ID, parent.FamilyID, child.ID); err != nil {
		t.Fatalf("archived parent balance=%v", err)
	}
	if _, _, err = svc.Approve(ctx, parent.ID, parent.FamilyID, resubmitted.ID, "approve-archived-"+suffix, []byte("approve-archived"+suffix), resubmitted.Version); err != nil {
		t.Fatalf("approve archived=%v", err)
	}
}

func mustPointDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, e := time.Parse("2006-01-02", s)
	if e != nil {
		t.Fatal(e)
	}
	return d
}
