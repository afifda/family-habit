package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/family-habit/family-habit/backend/internal/auth"
	"github.com/family-habit/family-habit/backend/internal/database"
	"github.com/family-habit/family-habit/backend/internal/health"
)

// TestParentChildParentJourneyIntegration exercises the release-critical journey
// only through the HTTP contract while using the real Go services and PostgreSQL.
// Frontend component tests cover the same API calls and user interactions.
func TestParentChildParentJourneyIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, databaseURL)
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
	handler := NewApp(slog.New(slog.NewTextHandler(io.Discard, nil)), health.NewDatabaseChecker(pool, time.Second), pool, false)
	var token, csrf string

	request := func(method, path, body, idem, match string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if token != "" {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		}
		if csrf != "" && method != http.MethodGet {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if idem != "" {
			req.Header.Set("Idempotency-Key", idem)
		}
		if match != "" {
			req.Header.Set("If-Match", match)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	decodeData := func(rec *httptest.ResponseRecorder) any {
		t.Helper()
		var envelope map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode %d response: %v: %s", rec.Code, err, rec.Body.String())
		}
		return envelope["data"]
	}
	mustStatus := func(rec *httptest.ResponseRecorder, want int) {
		t.Helper()
		if rec.Code != want {
			t.Fatalf("status=%d want=%d body=%s", rec.Code, want, rec.Body.String())
		}
	}

	password := "correct horse battery staple"
	registered := request(http.MethodPost, "/api/v1/auth/register", `{"email":"phase8-`+suffix+`@example.test","password":"`+password+`","householdName":"Phase 8 Journey","timezone":"UTC","weekStartsOn":"sunday"}`, "", "")
	mustStatus(registered, http.StatusCreated)
	for _, cookie := range registered.Result().Cookies() {
		if cookie.Name == sessionCookie {
			token = cookie.Value
		}
	}
	sessionData := decodeData(registered).(map[string]any)
	csrf = sessionData["csrfToken"].(string)
	session := auth.Session{ID: "", UserID: sessionData["userId"].(string), FamilyID: sessionData["householdId"].(string)}
	cleanupCompletionHTTP(t, pool, session)

	household := request(http.MethodGet, "/api/v1/household", "", "", "")
	mustStatus(household, http.StatusOK)
	householdVersion := int(decodeData(household).(map[string]any)["version"].(float64))
	toggleKey := "journey-rewards-enable-" + suffix
	enabled := request(http.MethodPatch, "/api/v1/household", `{"rewardsEnabled":true}`, toggleKey, strconv.Itoa(householdVersion))
	mustStatus(enabled, http.StatusOK)
	enabledBody := enabled.Body.String()
	enabledVersion := int(decodeData(enabled).(map[string]any)["version"].(float64))
	disabled := request(http.MethodPatch, "/api/v1/household", `{"rewardsEnabled":false}`, "journey-rewards-disable-"+suffix, strconv.Itoa(enabledVersion))
	mustStatus(disabled, http.StatusOK)
	replayedToggle := request(http.MethodPatch, "/api/v1/household", `{"rewardsEnabled":true}`, toggleKey, strconv.Itoa(householdVersion))
	mustStatus(replayedToggle, http.StatusOK)
	if replayedToggle.Header().Get("Idempotent-Replayed") != "true" || replayedToggle.Body.String() != enabledBody {
		t.Fatalf("household toggle replay changed: first=%s replay=%s", enabledBody, replayedToggle.Body.String())
	}

	childResponse := request(http.MethodPost, "/api/v1/children", `{"nickname":"Maya","avatar":"fox","color":"#336699"}`, "journey-child-"+suffix, "")
	mustStatus(childResponse, http.StatusCreated)
	childID := decodeData(childResponse).(map[string]any)["id"].(string)

	habitResponse := request(http.MethodPost, "/api/v1/habits", `{"title":"Make the bed","description":"Straighten the blanket","icon":"bed","color":"#336699"}`, "journey-habit-"+suffix, "")
	mustStatus(habitResponse, http.StatusCreated)
	habitID := decodeData(habitResponse).(map[string]any)["id"].(string)
	today := time.Now().UTC().Format("2006-01-02")
	assignmentResponse := request(http.MethodPost, "/api/v1/habits/"+habitID+"/assignments", `{"childIds":["`+childID+`"],"points":7,"schedule":{"kind":"daily","weekdays":[]},"effectiveStartDate":"`+today+`"}`, "journey-assignment-"+suffix, "")
	mustStatus(assignmentResponse, http.StatusCreated)
	overviewBefore := request(http.MethodGet, "/api/v1/parent/overview", "", "", "")
	mustStatus(overviewBefore, http.StatusOK)
	overviewBeforeData := decodeData(overviewBefore).(map[string]any)
	if overviewBeforeData["date"] != today || len(overviewBeforeData["children"].([]any)) != 1 {
		t.Fatalf("overview before child journey=%s", overviewBefore.Body.String())
	}

	entered := request(http.MethodPost, "/api/v1/session/child", `{"childId":"`+childID+`"}`, "", "")
	mustStatus(entered, http.StatusOK)
	csrf = decodeData(entered).(map[string]any)["csrfToken"].(string)
	mustStatus(request(http.MethodGet, "/api/v1/parent/overview", "", "", ""), http.StatusForbidden)
	todayResponse := request(http.MethodGet, "/api/v1/children/"+childID+"/today", "", "", "")
	mustStatus(todayResponse, http.StatusOK)
	occurrences := decodeData(todayResponse).(map[string]any)["occurrences"].([]any)
	if len(occurrences) != 1 {
		t.Fatalf("today occurrences=%d body=%s", len(occurrences), todayResponse.Body.String())
	}
	occurrence := occurrences[0].(map[string]any)
	occurrenceID := occurrence["id"].(string)
	version := occurrence["version"].(float64)
	submitted := request(http.MethodPost, "/api/v1/occurrences/"+occurrenceID+"/completions", "", "journey-submit-"+suffix, json.Number(versionString(version)).String())
	mustStatus(submitted, http.StatusCreated)
	completionData := decodeData(submitted).(map[string]any)
	completionID := completionData["id"].(string)
	version = completionData["occurrenceVersion"].(float64)

	left := request(http.MethodDelete, "/api/v1/session/child", "", "", "")
	mustStatus(left, http.StatusOK)
	csrf = decodeData(left).(map[string]any)["csrfToken"].(string)
	unlocked := request(http.MethodPost, "/api/v1/session/parent/unlock", `{"password":"`+password+`"}`, "", "")
	mustStatus(unlocked, http.StatusOK)
	csrf = decodeData(unlocked).(map[string]any)["csrfToken"].(string)
	queue := request(http.MethodGet, "/api/v1/review/pending", "", "", "")
	mustStatus(queue, http.StatusOK)
	if len(decodeData(queue).([]any)) != 1 {
		t.Fatalf("pending queue body=%s", queue.Body.String())
	}
	overviewPending := request(http.MethodGet, "/api/v1/parent/overview", "", "", "")
	mustStatus(overviewPending, http.StatusOK)
	pendingChildOverview := decodeData(overviewPending).(map[string]any)["children"].([]any)[0].(map[string]any)
	if pendingChildOverview["waitingPointsToday"].(float64) != 7 || pendingChildOverview["approvedPointsToday"].(float64) != 0 {
		t.Fatalf("overview pending points=%s", overviewPending.Body.String())
	}
	approved := request(http.MethodPost, "/api/v1/completions/"+completionID+"/approve", "", "journey-approve-"+suffix, versionString(version))
	mustStatus(approved, http.StatusOK)
	overviewAfter := request(http.MethodGet, "/api/v1/parent/overview", "", "", "")
	mustStatus(overviewAfter, http.StatusOK)
	childOverview := decodeData(overviewAfter).(map[string]any)["children"].([]any)[0].(map[string]any)
	if childOverview["completed"].(float64) != 1 || childOverview["total"].(float64) != 1 || childOverview["pending"].(float64) != 0 || childOverview["approvedPointsToday"].(float64) != 7 || childOverview["waitingPointsToday"].(float64) != 0 {
		t.Fatalf("overview after approval=%s", overviewAfter.Body.String())
	}
	balance := request(http.MethodGet, "/api/v1/children/"+childID+"/points", "", "", "")
	mustStatus(balance, http.StatusOK)
	if got := decodeData(balance).(map[string]any)["points"].(float64); got != 7 {
		t.Fatalf("balance=%v want=7 body=%s", got, balance.Body.String())
	}
	report := request(http.MethodGet, "/api/v1/reports/children/"+childID+"?period=day&anchorDate="+today, "", "", "")
	mustStatus(report, http.StatusOK)
	if got := decodeData(report).(map[string]any)["approved"].(float64); got != 1 {
		t.Fatalf("approved report count=%v body=%s", got, report.Body.String())
	}
}

func versionString(version float64) string {
	return strconv.FormatInt(int64(version), 10)
}
