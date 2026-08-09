package httpapi

import (
	"crypto/sha256"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/family-habit/family-habit/backend/internal/completions"
)

func (a *authAPI) completionRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/children/{childId}/today", a.requireSession(http.HandlerFunc(a.childToday)))
	mux.Handle("POST /api/v1/occurrences/{occurrenceId}/completions", a.requireSession(a.csrf(http.HandlerFunc(a.submitCompletion))))
	mux.Handle("DELETE /api/v1/completions/{completionId}", a.requireSession(a.csrf(http.HandlerFunc(a.withdrawCompletion))))
}

func (a *authAPI) childToday(w http.ResponseWriter, r *http.Request) {
	childID := r.PathValue("childId")
	if !uuidPattern.MatchString(childID) {
		writeError(w, 404, "not_found", "Child not found.")
		return
	}
	session := sessionFrom(r.Context())
	dateValue := strings.TrimSpace(r.URL.Query().Get("date"))
	serverToday, _, todayErr := a.completions.HouseholdToday(r.Context(), session.FamilyID)
	if todayErr != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	if dateValue == "" {
		dateValue = serverToday
	} else if _, err := time.Parse("2006-01-02", dateValue); err != nil {
		writeValidation(w, []validationIssue{{"date", "invalid", "Use a date in YYYY-MM-DD format."}})
		return
	}
	if session.Mode == "child" && dateValue != serverToday {
		writeError(w, 403, "date_not_allowed", "Child mode can only view today.")
		return
	}
	today, err := a.completions.Today(r.Context(), session.ID, session.FamilyID, childID, dateValue)
	if handleCompletionError(w, err) {
		return
	}
	items := make([]any, 0, len(today.Occurrences))
	counts := map[string]int{"to_do": 0, "waiting_for_parent": 0, "done": 0}
	for _, item := range today.Occurrences {
		var completionID any
		if item.CompletionID != "" {
			completionID = item.CompletionID
		}
		items = append(items, map[string]any{
			"id": item.ID, "childId": item.ChildID, "type": item.Type, "localDate": item.LocalDate, "dueDate": nilIfEmpty(item.DueDate),
			"title": item.Title, "description": item.Description, "icon": item.Icon, "color": item.Color,
			"points": item.Points, "status": item.Status, "group": item.Group, "dueState": item.DueState,
			"completionId": completionID, "version": item.Version, "availableActions": item.AvailableActions,
		})
		counts[item.Group]++
	}
	writeJSON(w, 200, map[string]any{"data": map[string]any{"childId": today.ChildID, "date": today.Date, "timezone": today.Timezone, "counts": counts, "occurrences": items}})
}

func nilIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (a *authAPI) submitCompletion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("occurrenceId")
	if !uuidPattern.MatchString(id) {
		writeError(w, 404, "not_found", "Occurrence not found.")
		return
	}
	key, issues := idem(r)
	expected, versionIssues := expectedVersion(r)
	issues = append(issues, versionIssues...)
	if expected == nil {
		issues = append(issues, validationIssue{"If-Match", "required", "Provide the current occurrence version."})
	}
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	session := sessionFrom(r.Context())
	if session.Mode != "child" || session.ActiveChildID == "" {
		writeError(w, 403, "forbidden", "Child mode is required.")
		return
	}
	hash := sha256.Sum256([]byte("submit|" + id + "|" + formatVersion(expected)))
	out, replay, err := a.completions.Submit(r.Context(), session.ID, session.FamilyID, session.ActiveChildID, id, key, hash[:], *expected)
	if handleCompletionError(w, err) {
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 201, map[string]any{"data": completionJSON(out)})
}

func (a *authAPI) withdrawCompletion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("completionId")
	if !uuidPattern.MatchString(id) {
		writeError(w, 404, "not_found", "Completion not found.")
		return
	}
	key, issues := idem(r)
	expected, versionIssues := expectedVersion(r)
	issues = append(issues, versionIssues...)
	if expected == nil {
		issues = append(issues, validationIssue{"If-Match", "required", "Provide the current occurrence version."})
	}
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	session := sessionFrom(r.Context())
	if session.Mode != "child" || session.ActiveChildID == "" {
		writeError(w, 403, "forbidden", "Child mode is required.")
		return
	}
	hash := sha256.Sum256([]byte("withdraw|" + id + "|" + formatVersion(expected)))
	out, replay, err := a.completions.Withdraw(r.Context(), session.ID, session.FamilyID, session.ActiveChildID, id, key, hash[:], *expected)
	if handleCompletionError(w, err) {
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 200, map[string]any{"data": completionJSON(out)})
}

func completionJSON(c completions.Completion) map[string]any {
	var decidedAt any
	if c.DecidedAt != nil {
		decidedAt = c.DecidedAt.UTC().Format(time.RFC3339Nano)
	}
	return map[string]any{"id": c.ID, "occurrenceId": c.OccurrenceID, "childId": c.ChildID, "attemptStatus": c.AttemptStatus,
		"occurrenceStatus": c.OccurrenceStatus, "submittedAt": c.SubmittedAt.UTC().Format(time.RFC3339Nano), "decidedAt": decidedAt, "reason": nil, "occurrenceVersion": c.Version}
}

func formatVersion(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}

func handleCompletionError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var transition *completions.TransitionError
	if errors.As(err, &transition) {
		code, message := "invalid_state_transition", "The item is no longer in the required state."
		if errors.Is(err, completions.ErrVersionConflict) {
			code, message = "version_conflict", "The item changed. Refresh and try again."
		}
		writeJSON(w, 409, map[string]any{"error": map[string]any{"code": code, "message": message, "current": map[string]any{"status": transition.Status, "version": transition.Version}}})
		return true
	}
	switch {
	case errors.Is(err, completions.ErrForbidden):
		writeError(w, 403, "forbidden", "You cannot perform this action.")
	case errors.Is(err, completions.ErrNotFound):
		writeError(w, 404, "not_found", "The requested item was not found.")
	case errors.Is(err, completions.ErrInvalidState):
		writeError(w, 409, "invalid_state_transition", "The item is no longer in the required state.")
	case errors.Is(err, completions.ErrVersionConflict):
		writeError(w, 409, "version_conflict", "The item changed. Refresh and try again.")
	case errors.Is(err, completions.ErrIdempotency):
		writeError(w, 409, "idempotency_conflict", "The Idempotency-Key was already used for another request.")
	case errors.Is(err, completions.ErrFuture):
		writeError(w, 409, "not_actionable", "Future work cannot be submitted yet.")
	default:
		writeError(w, 500, "internal_error", "The request could not be completed.")
	}
	return true
}
