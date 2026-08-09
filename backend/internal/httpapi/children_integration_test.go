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
	"github.com/family-habit/family-habit/backend/internal/health"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestChildHTTPAuthorizationAndPrivacyIntegration(t *testing.T) {
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

	var suffix [8]byte
	if _, err = rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	key := hex.EncodeToString(suffix[:])
	authService := auth.NewService(pool)
	childService := children.NewService(pool)
	parent, token, err := authService.Register(ctx, "http-a-"+key+"@example.test", "correct horse battery staple", "HTTP A", "Asia/Jakarta", 0)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := authService.Register(ctx, "http-b-"+key+"@example.test", "correct horse battery staple", "HTTP B", "Asia/Jakarta", 0)
	if err != nil {
		t.Fatal(err)
	}
	cleanupHTTPFamilies(t, pool, parent, other)

	pinHash, err := auth.HashPassword("1234")
	if err != nil {
		t.Fatal(err)
	}
	protected, _, err := childService.Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "protected", []byte("protected"), "Protected", "fox", "#112233", pinHash)
	if err != nil {
		t.Fatal(err)
	}
	archived, _, err := childService.Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "archived", []byte("archived"), "Archived", "bear", "#223344", "")
	if err != nil {
		t.Fatal(err)
	}
	otherChild, _, err := childService.Create(ctx, other.ID, other.UserID, other.FamilyID, "other", []byte("other"), "Other", "owl", "#334455", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = childService.Archive(ctx, parent.ID, parent.UserID, parent.FamilyID, archived.ID); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewApp(logger, health.NewDatabaseChecker(pool, time.Second), pool, false)
	request := func(method, path, body, csrf, source string, authenticated bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.RemoteAddr = source + ":1234"
		if authenticated {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		return recorder
	}

	if got := request(http.MethodGet, "/api/v1/profiles", "", "", "10.0.0.1", false); got.Code != 401 {
		t.Fatalf("unauthenticated profiles status = %d", got.Code)
	}
	profiles := request(http.MethodGet, "/api/v1/profiles", "", "", "10.0.0.1", true)
	if profiles.Code != 200 || strings.Contains(profiles.Body.String(), "createdAt") || strings.Contains(profiles.Body.String(), "pinEnabled") || strings.Contains(profiles.Body.String(), archived.ID) || strings.Contains(profiles.Body.String(), otherChild.ID) {
		t.Fatalf("profile projection status/body = %d %s", profiles.Code, profiles.Body.String())
	}
	if !strings.Contains(profiles.Body.String(), `"pinRequired":true`) {
		t.Fatalf("profile projection missing PIN requirement: %s", profiles.Body.String())
	}

	if _, err = authService.LockParent(ctx, parent); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodGet, "/api/v1/children", "", "", "10.0.0.2", true); got.Code != 403 {
		t.Fatalf("shared management list status = %d", got.Code)
	}
	if got := request(http.MethodPost, "/api/v1/session/child", `{"childId":"`+protected.ID+`","pin":"1234"}`, "", "10.0.0.3", true); got.Code != 403 {
		t.Fatalf("missing CSRF status = %d", got.Code)
	}

	failures := []struct{ body, source string }{
		{`{"childId":"not-a-uuid"}`, "10.0.1.1"},
		{`{"childId":"00000000-0000-4000-8000-000000000001"}`, "10.0.1.2"},
		{`{"childId":"` + archived.ID + `"}`, "10.0.1.3"},
		{`{"childId":"` + otherChild.ID + `"}`, "10.0.1.4"},
		{`{"childId":"` + protected.ID + `"}`, "10.0.1.5"},
		{`{"childId":"` + protected.ID + `","pin":"9999"}`, "10.0.1.6"},
		{`{"childId":"` + protected.ID + `","pin":"12ab"}`, "10.0.1.7"},
	}
	var failureBody string
	for _, tc := range failures {
		got := request(http.MethodPost, "/api/v1/session/child", tc.body, parent.CSRFToken, tc.source, true)
		if got.Code != 401 {
			t.Fatalf("generic child failure status/body = %d %s", got.Code, got.Body.String())
		}
		if failureBody == "" {
			failureBody = got.Body.String()
		} else if got.Body.String() != failureBody {
			t.Fatalf("child failure oracle: %q != %q", got.Body.String(), failureBody)
		}
	}

	entered := request(http.MethodPost, "/api/v1/session/child", `{"childId":"`+protected.ID+`","pin":"1234"}`, parent.CSRFToken, "10.0.2.1", true)
	if entered.Code != 200 || !strings.Contains(entered.Body.String(), `"actor":"child"`) || !strings.Contains(entered.Body.String(), protected.ID) {
		t.Fatalf("successful child entry = %d %s", entered.Code, entered.Body.String())
	}
	if got := request(http.MethodGet, "/api/v1/children", "", "", "10.0.2.2", true); got.Code != 403 {
		t.Fatalf("child management list status = %d", got.Code)
	}
	left := request(http.MethodDelete, "/api/v1/session/child", "", parent.CSRFToken, "10.0.2.3", true)
	if left.Code != 200 || !strings.Contains(left.Body.String(), `"actor":"profile_picker"`) {
		t.Fatalf("leave child = %d %s", left.Code, left.Body.String())
	}
	leftAgain := request(http.MethodDelete, "/api/v1/session/child", "", parent.CSRFToken, "10.0.2.4", true)
	if leftAgain.Code != 200 {
		t.Fatalf("idempotent leave status = %d", leftAgain.Code)
	}
}

func cleanupHTTPFamilies(t *testing.T, pool *pgxpool.Pool, sessions ...auth.Session) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		for _, session := range sessions {
			_, _ = pool.Exec(ctx, `DELETE FROM audit_events WHERE family_id=$1`, session.FamilyID)
			_, _ = pool.Exec(ctx, `DELETE FROM idempotency_records WHERE family_id=$1`, session.FamilyID)
			_, _ = pool.Exec(ctx, `DELETE FROM sessions WHERE family_id=$1`, session.FamilyID)
			_, _ = pool.Exec(ctx, `DELETE FROM children WHERE family_id=$1`, session.FamilyID)
			_, _ = pool.Exec(ctx, `DELETE FROM family_memberships WHERE family_id=$1`, session.FamilyID)
			_, _ = pool.Exec(ctx, `DELETE FROM families WHERE id=$1`, session.FamilyID)
			_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, session.UserID)
		}
	})
}
