package routines

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/family-habit/family-habit/backend/internal/auth"
	"github.com/family-habit/family-habit/backend/internal/children"
	"github.com/family-habit/family-habit/backend/internal/database"
	"github.com/family-habit/family-habit/backend/internal/habits"
)

func TestPhase9ArchiveEffectiveDatesAssignmentsAndSnapshots(t *testing.T) {
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
	parent, _, err := auth.NewService(pool).Register(ctx, "p9-routine-"+suffix+"@example.test", "correct horse battery staple", "Phase 9", "Europe/Berlin", 1)
	if err != nil {
		t.Fatal(err)
	}
	child, _, err := children.NewService(pool).Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "p9-routine-child-"+suffix, []byte("child"+suffix), "Robin", "fox", "#123456", "")
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(pool)
	morning, _, err := svc.Create(ctx, parent.ID, parent.FamilyID, "p9-morning-"+suffix, []byte("morning"+suffix), Input{Name: "Morning", SortOrder: 0})
	if err != nil {
		t.Fatal(err)
	}
	evening, _, err := svc.Create(ctx, parent.ID, parent.FamilyID, "p9-evening-"+suffix, []byte("evening"+suffix), Input{Name: "Evening", SortOrder: 1})
	if err != nil {
		t.Fatal(err)
	}
	habitSvc := habits.NewService(pool)
	habit, _, err := habitSvc.CreateHabit(ctx, parent.ID, parent.UserID, parent.FamilyID, "p9-habit-"+suffix, []byte("habit"+suffix), habits.HabitInput{Title: "Brush teeth"})
	if err != nil {
		t.Fatal(err)
	}
	start := mustDate(t, "2026-08-01")
	assignment, _, err := habitSvc.CreateAssignment(ctx, parent.ID, parent.UserID, parent.FamilyID, habit.ID, "p9-assignment-"+suffix, []byte("assignment"+suffix), habits.AssignmentInput{ChildID: child.ID, Points: 3, Kind: "daily", EffectiveDate: start, RoutineGroupID: &morning.ID, SortOrder: 2})
	if err != nil {
		t.Fatal(err)
	}
	oldDate := mustDate(t, "2026-08-04")
	if err = habitSvc.EnsureDate(ctx, parent.FamilyID, oldDate); err != nil {
		t.Fatal(err)
	}
	effective := mustDate(t, "2026-08-05")
	archived, _, err := svc.Archive(ctx, parent.ID, parent.FamilyID, morning.ID, "p9-archive-"+suffix, []byte("archive"+suffix), morning.Version, effective, &evening.ID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("group was not archived")
	}
	var oldUntil time.Time
	if err = pool.QueryRow(ctx, `SELECT effective_until FROM habit_assignments WHERE id=$1`, assignment.ID).Scan(&oldUntil); err != nil || oldUntil.Format("2006-01-02") != "2026-08-04" {
		t.Fatalf("old until=%s err=%v", oldUntil, err)
	}
	var replacement, group string
	var points, order int
	var kind string
	var weekdays []int16
	if err = pool.QueryRow(ctx, `SELECT a.id,a.routine_group_id,a.points,a.sort_order,s.kind::text,s.weekdays FROM habit_assignments a JOIN habit_schedules s ON s.assignment_id=a.id WHERE a.supersedes_assignment_id=$1`, assignment.ID).Scan(&replacement, &group, &points, &order, &kind, &weekdays); err != nil {
		t.Fatal(err)
	}
	if group != evening.ID || points != 3 || order != 2 || kind != "daily" {
		t.Fatalf("replacement=%s group=%s points/order/kind=%d/%d/%s", replacement, group, points, order, kind)
	}
	var oldSnapshot string
	if err = pool.QueryRow(ctx, `SELECT routine_group_name_snapshot FROM occurrences WHERE assignment_id=$1 AND local_date=$2`, assignment.ID, oldDate).Scan(&oldSnapshot); err != nil || oldSnapshot != "Morning" {
		t.Fatalf("old snapshot=%q err=%v", oldSnapshot, err)
	}
	if err = habitSvc.EnsureDate(ctx, parent.FamilyID, effective); err != nil {
		t.Fatal(err)
	}
	var newSnapshot, newAssignment string
	if err = pool.QueryRow(ctx, `SELECT routine_group_name_snapshot,assignment_id FROM occurrences WHERE child_id=$1 AND local_date=$2`, child.ID, effective).Scan(&newSnapshot, &newAssignment); err != nil || newSnapshot != "Evening" || newAssignment != replacement {
		t.Fatalf("new snapshot/assignment=%q/%s err=%v", newSnapshot, newAssignment, err)
	}
	if _, _, err = svc.Create(ctx, parent.ID, parent.FamilyID, "p9-after-school-"+suffix, []byte("after"+suffix), Input{Name: "After school", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.Create(ctx, parent.ID, parent.FamilyID, "p9-bedtime-"+suffix, []byte("bedtime"+suffix), Input{Name: "Bedtime", SortOrder: 2}); err != nil {
		t.Fatal(err)
	}
	var eveningVersion int64
	if err = pool.QueryRow(ctx, `SELECT version FROM routine_groups WHERE id=$1`, evening.ID).Scan(&eveningVersion); err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.Archive(ctx, parent.ID, parent.FamilyID, evening.ID, "p9-archive-first-"+suffix, []byte("archive-first"+suffix), eveningVersion, mustDate(t, "2026-08-06"), nil); err != nil {
		t.Fatalf("archive first of three active groups: %v", err)
	}
	var activeCount, distinctOrders, minOrder, maxOrder int
	if err = pool.QueryRow(ctx, `SELECT count(*),count(DISTINCT sort_order),min(sort_order),max(sort_order) FROM routine_groups WHERE family_id=$1 AND archived_at IS NULL`, parent.FamilyID).Scan(&activeCount, &distinctOrders, &minOrder, &maxOrder); err != nil {
		t.Fatal(err)
	}
	if activeCount != 2 || distinctOrders != 2 || minOrder != 0 || maxOrder != 1 {
		t.Fatalf("active routine order is not dense: count=%d distinct=%d range=%d..%d", activeCount, distinctOrders, minOrder, maxOrder)
	}
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	v, e := time.Parse("2006-01-02", s)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
