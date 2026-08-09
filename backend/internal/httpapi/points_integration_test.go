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
	"github.com/family-habit/family-habit/backend/internal/completions"
	"github.com/family-habit/family-habit/backend/internal/database"
	"github.com/family-habit/family-habit/backend/internal/habits"
	"github.com/family-habit/family-habit/backend/internal/health"
)

func TestPointsHTTPAuthorizationPrivacyAndCSRFIntegration(t *testing.T) {
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
	authSvc := auth.NewService(pool)
	parent, parentToken, err := authSvc.Register(ctx, "phase7-http-"+suffix+"@example.test", "correct horse battery staple", "Phase 7 HTTP", "UTC", 0)
	if err != nil {
		t.Fatal(err)
	}
	other, otherToken, err := authSvc.Register(ctx, "phase7-http-other-"+suffix+"@example.test", "correct horse battery staple", "Other HTTP", "UTC", 0)
	if err != nil {
		t.Fatal(err)
	}
	cleanupCompletionHTTP(t, pool, parent, other)

	childSvc := children.NewService(pool)
	child, _, err := childSvc.Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "p7-child", []byte("p7-child"), "Maya", "fox", "#112233", "")
	if err != nil {
		t.Fatal(err)
	}
	sibling, _, err := childSvc.Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "p7-sibling", []byte("p7-sibling"), "Alex", "bear", "#223344", "")
	if err != nil {
		t.Fatal(err)
	}

	// Create a real pending completion so decision authorization is exercised at
	// the HTTP boundary, not merely stopped by request validation.
	today := time.Now().UTC().Truncate(24 * time.Hour)
	task, _, err := habits.NewService(pool).CreateTask(ctx, parent.ID, parent.UserID, parent.FamilyID, "p7-task", []byte("p7-task"), habits.TaskInput{ChildID: child.ID, Title: "Read", DueDate: today, Points: 7})
	if err != nil {
		t.Fatal(err)
	}
	var occurrenceID string
	if err = pool.QueryRow(ctx, `SELECT id FROM occurrences WHERE task_id=$1`, task.ID).Scan(&occurrenceID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL WHERE id=$1`, parent.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	pending, _, err := completions.NewService(pool).Submit(ctx, parent.ID, parent.FamilyID, child.ID, occurrenceID, "p7-submit", []byte("p7-submit"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='parent',active_child_id=NULL,parent_unlocked_at=now(),last_activity_at=now() WHERE id=$1`, parent.ID); err != nil {
		t.Fatal(err)
	}

	handler := NewApp(slog.New(slog.NewTextHandler(io.Discard, nil)), health.NewDatabaseChecker(pool, time.Second), pool, false)
	request := func(token, method, path, body, csrf, key, match string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if token != "" {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		}
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

	unauthenticated := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/review/pending"},
		{http.MethodPost, "/api/v1/completions/" + pending.ID + "/approve"},
		{http.MethodGet, "/api/v1/children/" + child.ID + "/points"},
		{http.MethodGet, "/api/v1/children/" + child.ID + "/points/ledger"},
		{http.MethodPost, "/api/v1/children/" + child.ID + "/points/corrections"},
		{http.MethodGet, "/api/v1/children/" + child.ID + "/occurrences"},
		{http.MethodGet, "/api/v1/reports/children/" + child.ID + "?period=month&anchorDate=" + today.Format("2006-01-02")},
	}
	for _, tc := range unauthenticated {
		if got := request("", tc.method, tc.path, "", "", "", ""); got.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s %s=%d %s", tc.method, tc.path, got.Code, got.Body.String())
		}
	}

	// A rejected CSRF request must not create either a ledger or audit row.
	var ledgerBefore, auditBefore int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger WHERE family_id=$1`, parent.FamilyID).Scan(&ledgerBefore)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE family_id=$1 AND action='points.corrected'`, parent.FamilyID).Scan(&auditBefore)
	noCSRF := request(parentToken, http.MethodPost, "/api/v1/children/"+child.ID+"/points/corrections", `{"points":9,"reason":"Private parent note"}`, "", "p7-no-csrf", "")
	if noCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing csrf=%d %s", noCSRF.Code, noCSRF.Body.String())
	}
	var ledgerAfter, auditAfter int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger WHERE family_id=$1`, parent.FamilyID).Scan(&ledgerAfter)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE family_id=$1 AND action='points.corrected'`, parent.FamilyID).Scan(&auditAfter)
	if ledgerAfter != ledgerBefore || auditAfter != auditBefore {
		t.Fatalf("csrf rejection wrote ledger/audit: %d/%d -> %d/%d", ledgerBefore, auditBefore, ledgerAfter, auditAfter)
	}

	correction := request(parentToken, http.MethodPost, "/api/v1/children/"+child.ID+"/points/corrections", `{"points":9,"reason":"Private parent note"}`, parent.CSRFToken, "p7-correction", "")
	if correction.Code != http.StatusCreated || !strings.Contains(correction.Body.String(), "Private parent note") {
		t.Fatalf("parent correction=%d %s", correction.Code, correction.Body.String())
	}
	for _, path := range []string{
		"/api/v1/review/pending",
		"/api/v1/children/" + child.ID + "/occurrences?from=" + today.Format("2006-01-02") + "&to=" + today.Format("2006-01-02"),
		"/api/v1/reports/children/" + child.ID + "?period=month&anchorDate=" + today.Format("2006-01-02"),
	} {
		if got := request(parentToken, http.MethodGet, path, "", "", "", ""); got.Code != http.StatusOK {
			t.Fatalf("parent read %s=%d %s", path, got.Code, got.Body.String())
		}
	}

	// Cross-household resources deliberately collapse to 404.
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/children/" + child.ID + "/points", ""},
		{http.MethodGet, "/api/v1/children/" + child.ID + "/points/ledger", ""},
		{http.MethodGet, "/api/v1/children/" + child.ID + "/occurrences", ""},
		{http.MethodGet, "/api/v1/reports/children/" + child.ID + "?period=month&anchorDate=" + today.Format("2006-01-02"), ""},
		{http.MethodPost, "/api/v1/children/" + child.ID + "/points/corrections", `{"points":1,"reason":"No"}`},
		{http.MethodPost, "/api/v1/completions/" + pending.ID + "/approve", ""},
	} {
		got := request(otherToken, tc.method, tc.path, tc.body, other.CSRFToken, "p7-cross-"+suffix+strings.ReplaceAll(tc.path, "/", "-"), "2")
		if got.Code != http.StatusNotFound {
			t.Fatalf("cross-family %s %s=%d %s", tc.method, tc.path, got.Code, got.Body.String())
		}
	}

	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL WHERE id=$1`, parent.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	ledger := request(parentToken, http.MethodGet, "/api/v1/children/"+child.ID+"/points/ledger", "", "", "", "")
	if ledger.Code != http.StatusOK || strings.Contains(ledger.Body.String(), "Private parent note") || strings.Contains(ledger.Body.String(), `"originalEntryId":"`) || !strings.Contains(ledger.Body.String(), `"displayLabel":"Points added"`) {
		t.Fatalf("child-private ledger=%d %s", ledger.Code, ledger.Body.String())
	}
	if got := request(parentToken, http.MethodGet, "/api/v1/children/"+sibling.ID+"/points/ledger", "", "", "", ""); got.Code != http.StatusNotFound {
		t.Fatalf("child sibling ledger=%d %s", got.Code, got.Body.String())
	}
	for _, tc := range []struct {
		method, path, body, key, match string
		want                           int
	}{
		{http.MethodGet, "/api/v1/review/pending", "", "", "", http.StatusForbidden},
		{http.MethodPost, "/api/v1/completions/" + pending.ID + "/approve", "", "p7-child-approve", "2", http.StatusForbidden},
		{http.MethodPost, "/api/v1/children/" + child.ID + "/points/corrections", `{"points":1,"reason":"No"}`, "p7-child-correct", "", http.StatusForbidden},
		{http.MethodGet, "/api/v1/children/" + child.ID + "/occurrences", "", "", "", http.StatusNotFound},
		{http.MethodGet, "/api/v1/reports/children/" + child.ID + "?period=month&anchorDate=" + today.Format("2006-01-02"), "", "", "", http.StatusForbidden},
	} {
		got := request(parentToken, tc.method, tc.path, tc.body, parent.CSRFToken, tc.key, tc.match)
		if got.Code != tc.want {
			t.Fatalf("child role %s %s=%d want=%d %s", tc.method, tc.path, got.Code, tc.want, got.Body.String())
		}
	}

	if _, err = pool.Exec(ctx, `UPDATE sessions SET mode='parent',active_child_id=NULL,parent_unlocked_at=now(),last_activity_at=now() WHERE id=$1`, parent.ID); err != nil {
		t.Fatal(err)
	}
	decision := request(parentToken, http.MethodPost, "/api/v1/completions/"+pending.ID+"/reject", `{"reason":"Please try again"}`, parent.CSRFToken, "p7-parent-reject", "2")
	if decision.Code != http.StatusOK || !strings.Contains(decision.Body.String(), `"attemptStatus":"rejected"`) {
		t.Fatalf("parent decision=%d %s", decision.Code, decision.Body.String())
	}
}
