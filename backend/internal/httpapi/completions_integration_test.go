package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/family-habit/family-habit/backend/internal/auth"
	"github.com/family-habit/family-habit/backend/internal/children"
	"github.com/family-habit/family-habit/backend/internal/database"
	"github.com/family-habit/family-habit/backend/internal/habits"
	"github.com/family-habit/family-habit/backend/internal/health"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCompletionHTTPChildScopeVersionAndReplayIntegration(t *testing.T) {
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
	parent, token, err := auth.NewService(pool).Register(ctx, "phase6-http-"+suffix+"@example.test", "correct horse battery staple", "Phase 6 HTTP", "UTC", 0)
	if err != nil {
		t.Fatal(err)
	}
	otherParent, _, err := auth.NewService(pool).Register(ctx, "phase6-http-other-"+suffix+"@example.test", "correct horse battery staple", "Other HTTP", "UTC", 0)
	if err != nil {
		t.Fatal(err)
	}
	cleanupCompletionHTTP(t, pool, parent, otherParent)
	childSvc := children.NewService(pool)
	child, _, err := childSvc.Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "http-child", []byte("http-child"), "Maya", "fox", "#112233", "")
	if err != nil {
		t.Fatal(err)
	}
	sibling, _, err := childSvc.Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "http-sibling", []byte("http-sibling"), "Alex", "bear", "#223344", "")
	if err != nil {
		t.Fatal(err)
	}
	otherChild, _, err := childSvc.Create(ctx, otherParent.ID, otherParent.UserID, otherParent.FamilyID, "http-other", []byte("http-other"), "Other", "owl", "#334455", "")
	if err != nil {
		t.Fatal(err)
	}
	habitSvc := habits.NewService(pool)
	habit, _, err := habitSvc.CreateHabit(ctx, parent.ID, parent.UserID, parent.FamilyID, "http-habit", []byte("http-habit"), habits.HabitInput{Title: "Make bed", Description: "Straighten the blanket", Icon: "bed", Color: "#334455"})
	if err != nil {
		t.Fatal(err)
	}
	date := time.Now().UTC().Format("2006-01-02")
	parsed, _ := time.Parse("2006-01-02", date)
	_, _, err = habitSvc.CreateAssignment(ctx, parent.ID, parent.UserID, parent.FamilyID, habit.ID, "http-assignment", []byte("http-assignment"), habits.AssignmentInput{ChildID: child.ID, Points: 4, Kind: "daily", EffectiveDate: parsed})
	if err != nil {
		t.Fatal(err)
	}

	handler := NewApp(slog.New(slog.NewTextHandler(io.Discard, nil)), health.NewDatabaseChecker(pool, time.Second), pool, false)
	activeToken := token
	request := func(method, path, csrf, key, match string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(nil))
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: activeToken})
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		if match != "" {
			req.Header.Set("If-Match", match)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		return recorder
	}
	parentToday := request(http.MethodGet, "/api/v1/children/"+child.ID+"/today?date="+date, "", "", "")
	if parentToday.Code != 200 || !strings.Contains(parentToday.Body.String(), `"description":"Straighten the blanket"`) || !strings.Contains(parentToday.Body.String(), `"availableActions":[]`) {
		t.Fatalf("parent today=%d %s", parentToday.Code, parentToday.Body.String())
	}
	var occurrenceID string
	var version int64
	if err = pool.QueryRow(ctx, `SELECT id,version FROM occurrences WHERE family_id=$1 AND child_id=$2 AND local_date=$3`, parent.FamilyID, child.ID, date).Scan(&occurrenceID, &version); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL WHERE id=$1`, parent.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodGet, "/api/v1/children/"+child.ID+"/today", "", "", ""); got.Code != 200 || !strings.Contains(got.Body.String(), `"availableActions":["submit"]`) {
		t.Fatalf("child today=%d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/api/v1/children/"+sibling.ID+"/today", "", "", ""); got.Code != 404 {
		t.Fatalf("sibling today=%d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/api/v1/children/00000000-0000-4000-8000-000000000001/today", "", "", ""); got.Code != 404 {
		t.Fatalf("nonexistent today=%d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/api/v1/children/"+otherChild.ID+"/today", "", "", ""); got.Code != 404 {
		t.Fatalf("cross-family today=%d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/api/v1/children/"+child.ID+"/today?date=2020-01-01", "", "", ""); got.Code != 403 || !strings.Contains(got.Body.String(), "date_not_allowed") {
		t.Fatalf("child historical date=%d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodPost, "/api/v1/occurrences/"+occurrenceID+"/completions", parent.CSRFToken, "submit-http", ""); got.Code != 422 {
		t.Fatalf("missing version=%d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodPost, "/api/v1/occurrences/"+occurrenceID+"/completions", "", "csrf-no-write", "1"); got.Code != 403 {
		t.Fatalf("missing csrf=%d %s", got.Code, got.Body.String())
	}
	var beforeSubmit int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM completion_attempts WHERE occurrence_id=$1`, occurrenceID).Scan(&beforeSubmit); err != nil || beforeSubmit != 0 {
		t.Fatalf("csrf wrote attempts=%d err=%v", beforeSubmit, err)
	}
	submit := request(http.MethodPost, "/api/v1/occurrences/"+occurrenceID+"/completions", parent.CSRFToken, "submit-http", "1")
	if submit.Code != 201 || !strings.Contains(submit.Body.String(), `"attemptStatus":"pending"`) || !strings.Contains(submit.Body.String(), `"occurrenceStatus":"pending_approval"`) {
		t.Fatalf("submit=%d %s", submit.Code, submit.Body.String())
	}
	replay := request(http.MethodPost, "/api/v1/occurrences/"+occurrenceID+"/completions", parent.CSRFToken, "submit-http", "1")
	if replay.Code != 201 || replay.Header().Get("Idempotent-Replayed") != "true" || replay.Body.String() != submit.Body.String() {
		t.Fatalf("replay=%d %s", replay.Code, replay.Body.String())
	}
	var completionID string
	if err = pool.QueryRow(ctx, `SELECT id FROM completion_attempts WHERE occurrence_id=$1 AND decision='pending'`, occurrenceID).Scan(&completionID); err != nil {
		t.Fatal(err)
	}
	withdraw := request(http.MethodDelete, "/api/v1/completions/"+completionID, parent.CSRFToken, "withdraw-http", "2")
	if withdraw.Code != 200 || !strings.Contains(withdraw.Body.String(), `"attemptStatus":"withdrawn"`) || !strings.Contains(withdraw.Body.String(), `"occurrenceStatus":"not_started"`) {
		t.Fatalf("withdraw=%d %s", withdraw.Code, withdraw.Body.String())
	}
	if version != 1 {
		t.Fatalf("unexpected starting version=%d", version)
	}
	var ledger int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger WHERE family_id=$1`, parent.FamilyID).Scan(&ledger); err != nil || ledger != 0 {
		t.Fatalf("ledger=%d err=%v", ledger, err)
	}
	// Expired and revoked child sessions fail authentication before any transition write.
	login := auth.NewService(pool)
	expiredSession, expiredToken, err := login.Login(ctx, "phase6-http-"+suffix+"@example.test", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL,created_at=now()-interval '2 days',expires_at=now()-interval '1 day' WHERE id=$1`, expiredSession.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	activeToken = expiredToken
	if got := request(http.MethodPost, "/api/v1/occurrences/"+occurrenceID+"/completions", expiredSession.CSRFToken, "expired-no-write", "3"); got.Code != 401 {
		t.Fatalf("expired submit=%d %s", got.Code, got.Body.String())
	}
	revokedSession, revokedToken, err := login.Login(ctx, "phase6-http-"+suffix+"@example.test", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL,revoked_at=now() WHERE id=$1`, revokedSession.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	activeToken = revokedToken
	if got := request(http.MethodPost, "/api/v1/occurrences/"+occurrenceID+"/completions", revokedSession.CSRFToken, "revoked-no-write", "3"); got.Code != 401 {
		t.Fatalf("revoked submit=%d %s", got.Code, got.Body.String())
	}
	var afterInvalidSessions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM completion_attempts WHERE occurrence_id=$1`, occurrenceID).Scan(&afterInvalidSessions); err != nil || afterInvalidSessions != 1 {
		t.Fatalf("invalid sessions wrote attempts=%d err=%v", afterInvalidSessions, err)
	}
}

func cleanupCompletionHTTP(t *testing.T, pool *pgxpool.Pool, sessions ...auth.Session) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		for _, s := range sessions {
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
		}
	})
}
