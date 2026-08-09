package habits

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/family-habit/family-habit/backend/internal/auth"
	"github.com/family-habit/family-habit/backend/internal/children"
	"github.com/family-habit/family-habit/backend/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSchedulingHistoryConcurrencyAndTasksIntegration(t *testing.T) {
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
	var raw [8]byte
	_, _ = rand.Read(raw[:])
	key := hex.EncodeToString(raw[:])
	session, _, err := auth.NewService(pool).Register(ctx, "phase5-"+key+"@example.test", "correct horse battery staple", "Phase 5", "America/New_York", 0)
	if err != nil {
		t.Fatal(err)
	}
	cleanupFamily(t, pool, session)
	childService := children.NewService(pool)
	first, _, err := childService.Create(ctx, session.ID, session.UserID, session.FamilyID, "c1", []byte("c1"), "First", "fox", "#112233", "")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := childService.Create(ctx, session.ID, session.UserID, session.FamilyID, "c2", []byte("c2"), "Second", "bear", "#223344", "")
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(pool)
	habit, _, err := svc.CreateHabit(ctx, session.ID, session.UserID, session.FamilyID, "h1", []byte("h1"), HabitInput{Title: "Brush teeth", Description: "Two minutes", Icon: "tooth", Color: "#334455"})
	if err != nil {
		t.Fatal(err)
	}
	start := date(t, "2026-03-01")
	a1, _, err := svc.CreateAssignment(ctx, session.ID, session.UserID, session.FamilyID, habit.ID, "a1", []byte("a1"), AssignmentInput{ChildID: first.ID, Points: 5, Kind: "daily", EffectiveDate: start})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.CreateAssignment(ctx, session.ID, session.UserID, session.FamilyID, habit.ID, "a2", []byte("a2"), AssignmentInput{ChildID: second.ID, Points: 7, Kind: "weekdays", Weekdays: []int16{0}, EffectiveDate: start})
	if err != nil {
		t.Fatal(err)
	}

	// Concurrent lazy generation converges on exactly one occurrence per assignment/date.
	day := date(t, "2026-03-08")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := svc.Materialize(ctx, session.FamilyID, first.ID, day); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM occurrences WHERE assignment_id=$1 AND local_date=$2`, a1.ID, day).Scan(&count); err != nil || count != 1 {
		t.Fatalf("concurrent occurrence count=%d err=%v", count, err)
	}
	if _, err = svc.Materialize(ctx, session.FamilyID, second.ID, day); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM occurrences WHERE child_id=$1 AND local_date=$2`, second.ID, day).Scan(&count); err != nil || count != 1 {
		t.Fatalf("Sunday weekday count=%d err=%v", count, err)
	}

	// A this-and-future presentation edit does not rewrite an already materialized snapshot.
	if _, err = svc.UpdateHabit(ctx, session.ID, session.UserID, session.FamilyID, habit.ID, date(t, "2026-03-09"), HabitInput{Title: "Brush and floss", Description: "Two minutes"}); err != nil {
		t.Fatal(err)
	}
	var oldTitle string
	if err = pool.QueryRow(ctx, `SELECT title_snapshot FROM occurrences WHERE assignment_id=$1 AND local_date=$2`, a1.ID, day).Scan(&oldTitle); err != nil || oldTitle != "Brush teeth" {
		t.Fatalf("historical title=%q err=%v", oldTitle, err)
	}
	future := date(t, "2026-03-10")
	items, err := svc.Materialize(ctx, session.FamilyID, first.ID, future)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range items {
		if o.LocalDate.Equal(future) && o.Title == "Brush and floss" {
			found = true
		}
	}
	if !found {
		t.Fatal("future occurrence did not use effective version")
	}
	var presentation string
	if err = pool.QueryRow(ctx, `SELECT description_snapshot||'/'||icon_snapshot||'/'||color_snapshot FROM occurrences WHERE assignment_id=$1 AND local_date=$2`, a1.ID, future).Scan(&presentation); err != nil || presentation != "Two minutes/tooth/#334455" {
		t.Fatalf("presentation snapshots=%q err=%v", presentation, err)
	}

	// A generated row with activity is protected, while untouched future rows
	// are discarded and regenerated from the new effective definition.
	protectedDay := date(t, "2026-03-11")
	if _, err = svc.Materialize(ctx, session.FamilyID, first.ID, protectedDay); err != nil {
		t.Fatal(err)
	}
	var protectedID string
	if err = pool.QueryRow(ctx, `SELECT id FROM occurrences WHERE assignment_id=$1 AND local_date=$2`, a1.ID, protectedDay).Scan(&protectedID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO completion_attempts(family_id,occurrence_id,child_id,attempt_number) VALUES($1,$2,$3,1)`, session.FamilyID, protectedID, first.ID); err != nil {
		t.Fatal(err)
	}
	editDay := date(t, "2026-03-10")
	if _, err = svc.UpdateHabit(ctx, session.ID, session.UserID, session.FamilyID, habit.ID, editDay, HabitInput{Title: "Final brushing", Description: "Three minutes", Icon: "stars", Color: "#556677"}); err != nil {
		t.Fatal(err)
	}
	var protectedTitle string
	if err = pool.QueryRow(ctx, `SELECT title_snapshot FROM occurrences WHERE id=$1`, protectedID).Scan(&protectedTitle); err != nil || protectedTitle != "Brush and floss" {
		t.Fatalf("protected snapshot=%q err=%v", protectedTitle, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE occurrences SET local_date=local_date+1 WHERE id=$1`, protectedID); err == nil {
		t.Fatal("protected occurrence local date was mutable")
	}
	items, err = svc.Materialize(ctx, session.FamilyID, first.ID, editDay)
	if err != nil {
		t.Fatal(err)
	}
	regenerated := false
	for _, o := range items {
		if o.LocalDate.Equal(editDay) && o.Title == "Final brushing" && o.Description == "Three minutes" {
			regenerated = true
		}
	}
	if !regenerated {
		t.Fatal("untouched future occurrence was not regenerated")
	}

	// Sunday-only schedules do not leak across the weekday boundary, and the
	// family-level helper generates all eligible children in one call.
	monday := date(t, "2026-03-09")
	if err = svc.EnsureDate(ctx, session.FamilyID, monday); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM occurrences WHERE child_id=$1 AND local_date=$2`, second.ID, monday).Scan(&count); err != nil || count != 0 {
		t.Fatalf("Monday Sunday-only count=%d err=%v", count, err)
	}
	nextSunday := date(t, "2026-03-15")
	errs = make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- svc.EnsureDate(ctx, session.FamilyID, nextSunday) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if err = pool.QueryRow(ctx, `SELECT count(DISTINCT child_id) FROM occurrences WHERE family_id=$1 AND local_date=$2 AND source_type='habit'`, session.FamilyID, nextSunday).Scan(&count); err != nil || count != 2 {
		t.Fatalf("family materialization children=%d err=%v", count, err)
	}

	// One-off tasks are materialized atomically and stay actionable when overdue.
	task, _, err := svc.CreateTask(ctx, session.ID, session.UserID, session.FamilyID, "t1", []byte("t1"), TaskInput{ChildID: first.ID, Title: "Return library book", DueDate: date(t, "2026-03-02"), Points: 9})
	if err != nil {
		t.Fatal(err)
	}
	items, err = svc.Materialize(ctx, session.FamilyID, first.ID, day)
	if err != nil {
		t.Fatal(err)
	}
	overdue := false
	for _, o := range items {
		if o.Type == "task" && o.LocalDate.Before(day) && o.Status == "not_started" {
			overdue = true
		}
	}
	if !overdue {
		t.Fatal("overdue task was not actionable")
	}
	expected := task.Version
	updated, replay, err := svc.UpdateTaskConditional(ctx, session.ID, session.UserID, session.FamilyID, task.ID, "task-update", []byte("same"), &expected, TaskInput{Title: "Return books", Description: "", DescriptionSet: true})
	if err != nil || replay || updated.Version != task.Version+1 || updated.Description != "" {
		t.Fatalf("conditional update=%+v replay=%v err=%v", updated, replay, err)
	}
	replayed, replay, err := svc.UpdateTaskConditional(ctx, session.ID, session.UserID, session.FamilyID, task.ID, "task-update", []byte("same"), &expected, TaskInput{Title: "Return books", Description: "", DescriptionSet: true})
	if err != nil || !replay || replayed.Version != updated.Version {
		t.Fatalf("update replay=%+v replay=%v err=%v", replayed, replay, err)
	}
	if _, _, err = svc.UpdateTaskConditional(ctx, session.ID, session.UserID, session.FamilyID, task.ID, "task-update", []byte("different"), &expected, TaskInput{Title: "Other"}); err != ErrIdempotency {
		t.Fatalf("hash conflict=%v", err)
	}
	concurrentExpected := updated.Version
	type mutationResult struct {
		replay bool
		err    error
	}
	results := make(chan mutationResult, 2)
	for range 2 {
		go func() {
			_, wasReplay, updateErr := svc.UpdateTaskConditional(ctx, session.ID, session.UserID, session.FamilyID, task.ID, "concurrent-update", []byte("concurrent"), &concurrentExpected, TaskInput{Title: "Return all books"})
			results <- mutationResult{wasReplay, updateErr}
		}()
	}
	replayCount := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.replay {
			replayCount++
		}
	}
	if replayCount != 1 {
		t.Fatalf("concurrent replay count=%d", replayCount)
	}
	updated.Version++
	stale := task.Version
	if _, _, err = svc.UpdateTaskConditional(ctx, session.ID, session.UserID, session.FamilyID, task.ID, "stale-update", []byte("stale"), &stale, TaskInput{Title: "Stale"}); err != ErrVersionConflict {
		t.Fatalf("version conflict=%v", err)
	}
	var occurrenceID string
	if err = pool.QueryRow(ctx, `UPDATE occurrences SET state='pending_approval' WHERE task_id=$1 RETURNING id`, task.ID).Scan(&occurrenceID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO completion_attempts(family_id,occurrence_id,child_id,attempt_number) VALUES($1,$2,$3,1)`, session.FamilyID, occurrenceID, first.ID); err != nil {
		t.Fatal(err)
	}
	cancelExpected := updated.Version
	if replay, err = svc.CancelTaskConditional(ctx, session.ID, session.UserID, session.FamilyID, task.ID, "task-cancel", []byte("cancel"), &cancelExpected, "No longer needed"); err != nil || replay {
		t.Fatal(err)
	}
	var states string
	if err = pool.QueryRow(ctx, `SELECT t.state::text||'/'||o.state::text||'/'||ca.decision::text||'/'||t.cancellation_reason FROM one_off_tasks t JOIN occurrences o ON o.task_id=t.id JOIN completion_attempts ca ON ca.occurrence_id=o.id WHERE t.id=$1`, task.ID).Scan(&states); err != nil || states != "cancelled/cancelled/cancelled/No longer needed" {
		t.Fatalf("cancel states=%q err=%v", states, err)
	}
	if replay, err = svc.CancelTaskConditional(ctx, session.ID, session.UserID, session.FamilyID, task.ID, "task-cancel", []byte("cancel"), &cancelExpected, "No longer needed"); err != nil || !replay {
		t.Fatalf("cancel replay=%v err=%v", replay, err)
	}

	// Structural scope and overlap constraints reject cross-household/overlapping definitions.
	_, _, err = svc.CreateAssignment(ctx, session.ID, session.UserID, session.FamilyID, habit.ID, "overlap", []byte("overlap"), AssignmentInput{ChildID: first.ID, Points: 4, Kind: "daily", EffectiveDate: start})
	if err != ErrConflict {
		t.Fatalf("overlap error=%v", err)
	}

	// Deactivation removes untouched generated rows instead of turning them
	// into misleading cancelled history, and retains protected snapshots.
	deactivateDay := date(t, "2026-03-16")
	if err = svc.EnsureDate(ctx, session.FamilyID, deactivateDay); err != nil {
		t.Fatal(err)
	}
	var deactivateProtectedID string
	if err = pool.QueryRow(ctx, `SELECT id FROM occurrences WHERE assignment_id IN (SELECT id FROM habit_assignments WHERE habit_id=$1 AND child_id=$2) AND local_date=$3`, habit.ID, first.ID, deactivateDay).Scan(&deactivateProtectedID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO completion_attempts(family_id,occurrence_id,child_id,attempt_number) VALUES($1,$2,$3,1)`, session.FamilyID, deactivateProtectedID, first.ID); err != nil {
		t.Fatal(err)
	}
	if err = svc.DeactivateHabit(ctx, session.ID, session.UserID, session.FamilyID, habit.ID, deactivateDay); err != nil {
		t.Fatal(err)
	}
	var protectedState string
	if err = pool.QueryRow(ctx, `SELECT state::text FROM occurrences WHERE id=$1`, deactivateProtectedID).Scan(&protectedState); err != nil || protectedState != "not_started" {
		t.Fatalf("deactivation protected state=%q err=%v", protectedState, err)
	}

	// Batch assignment takes locks and rechecks every child before writing, so
	// an archived member makes the whole multi-child operation atomic.
	batchHabit, _, err := svc.CreateHabit(ctx, session.ID, session.UserID, session.FamilyID, "batch-h", []byte("batch-h"), HabitInput{Title: "Batch habit"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE children SET archived_at=now() WHERE id=$1`, second.ID); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.CreateAssignments(ctx, session.ID, session.UserID, session.FamilyID, batchHabit.ID, "batch-a", []byte("batch-a"), []AssignmentInput{
		{ChildID: first.ID, Points: 2, Kind: "daily", EffectiveDate: start},
		{ChildID: second.ID, Points: 2, Kind: "daily", EffectiveDate: start},
	})
	if err != ErrNotFound {
		t.Fatalf("archived batch error=%v", err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM habit_assignments WHERE habit_id=$1`, batchHabit.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial batch assignments=%d err=%v", count, err)
	}
}

func date(t *testing.T, v string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", v)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func cleanupFamily(t *testing.T, pool *pgxpool.Pool, s auth.Session) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM audit_events WHERE family_id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM idempotency_records WHERE family_id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM completion_attempts WHERE family_id=$1`, s.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM point_ledger WHERE family_id=$1`, s.FamilyID)
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
