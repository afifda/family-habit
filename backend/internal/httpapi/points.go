package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/family-habit/family-habit/backend/internal/points"
)

func (a *authAPI) pointRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/parent/overview", a.requireParent(http.HandlerFunc(a.parentOverview)))
	mux.Handle("GET /api/v1/review/pending", a.requireParent(http.HandlerFunc(a.pendingReview)))
	mux.Handle("POST /api/v1/completions/{completionId}/approve", a.requireParent(a.csrf(http.HandlerFunc(a.approveCompletion))))
	mux.Handle("POST /api/v1/completions/{completionId}/reject", a.requireParent(a.csrf(http.HandlerFunc(a.rejectCompletion))))
	mux.Handle("POST /api/v1/completions/{completionId}/reverse", a.requireParent(a.csrf(http.HandlerFunc(a.reverseCompletion))))
	mux.Handle("GET /api/v1/children/{childId}/points", a.requireSession(http.HandlerFunc(a.balance)))
	mux.Handle("GET /api/v1/children/{childId}/points/ledger", a.requireSession(http.HandlerFunc(a.ledger)))
	mux.Handle("POST /api/v1/children/{childId}/points/corrections", a.requireParent(a.csrf(http.HandlerFunc(a.correction))))
	mux.Handle("GET /api/v1/children/{childId}/occurrences", a.requireSession(http.HandlerFunc(a.history)))
	mux.Handle("GET /api/v1/reports/children/{childId}", a.requireParent(http.HandlerFunc(a.report)))
}

func (a *authAPI) parentOverview(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	date, timezone, err := a.completions.HouseholdToday(r.Context(), s.FamilyID)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	profiles, err := a.children.List(r.Context(), s.FamilyID, false)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	items := make([]any, 0, len(profiles))
	pendingTotal := 0
	for _, child := range profiles {
		today, todayErr := a.completions.Today(r.Context(), s.ID, s.FamilyID, child.ID, date)
		if todayErr != nil {
			writeError(w, 500, "internal_error", "The request could not be completed.")
			return
		}
		completed, pending := 0, 0
		for _, occurrence := range today.Occurrences {
			if occurrence.Group == "done" {
				completed++
			}
			if occurrence.Group == "waiting_for_parent" {
				pending++
			}
		}
		pendingTotal += pending
		items = append(items, map[string]any{
			"childId": child.ID, "nickname": child.Nickname, "avatar": child.Avatar, "color": child.Color,
			"completed": completed, "total": len(today.Occurrences), "pending": pending,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"date": date, "timezone": timezone, "pending": pendingTotal, "children": items}})
}
func pageLimit(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n < 1 || n > 100 {
		return 25
	}
	return n
}
func validID(w http.ResponseWriter, id, subject string) bool {
	if !uuidPattern.MatchString(id) {
		writeError(w, 404, "not_found", subject+" not found.")
		return false
	}
	return true
}
func (a *authAPI) pendingReview(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	child := strings.TrimSpace(r.URL.Query().Get("childId"))
	if child != "" && !uuidPattern.MatchString(child) {
		writeValidation(w, []validationIssue{{"childId", "invalid", "Use a valid child identifier."}})
		return
	}
	items, next, err := a.points.Pending(r.Context(), s.ID, s.FamilyID, child, r.URL.Query().Get("cursor"), pageLimit(r))
	if handlePointError(w, err) {
		return
	}
	out := make([]any, 0, len(items))
	for _, v := range items {
		out = append(out, reviewJSON(v))
	}
	writeJSON(w, 200, map[string]any{"data": out, "page": map[string]any{"nextCursor": nilIfEmpty(next)}})
}

type decisionReasonBody struct {
	Reason string `json:"reason"`
}

func pointMutationInput(w http.ResponseWriter, r *http.Request, id, action string, body any) (string, *int64, []byte, bool) {
	key, issues := idem(r)
	expected, vi := expectedVersion(r)
	issues = append(issues, vi...)
	if expected == nil {
		issues = append(issues, validationIssue{"If-Match", "required", "Provide the current occurrence version."})
	}
	if len(issues) > 0 {
		writeValidation(w, issues)
		return "", nil, nil, false
	}
	raw, _ := json.Marshal(body)
	sum := sha256.Sum256(append([]byte(action+"|"+id+"|"+formatVersion(expected)+"|"), raw...))
	return key, expected, sum[:], true
}
func (a *authAPI) approveCompletion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("completionId")
	if !validID(w, id, "Completion") {
		return
	}
	key, ver, hash, ok := pointMutationInput(w, r, id, "approve", nil)
	if !ok {
		return
	}
	s := sessionFrom(r.Context())
	out, replay, err := a.points.Approve(r.Context(), s.ID, s.FamilyID, id, key, hash, *ver)
	if handlePointError(w, err) {
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 200, map[string]any{"data": reviewJSON(out)})
}
func (a *authAPI) rejectCompletion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("completionId")
	if !validID(w, id, "Completion") {
		return
	}
	var in decisionReasonBody
	if !decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Reason) > 500 {
		writeValidation(w, []validationIssue{{"reason", "length", "Reason must be at most 500 characters."}})
		return
	}
	key, ver, hash, ok := pointMutationInput(w, r, id, "reject", in)
	if !ok {
		return
	}
	s := sessionFrom(r.Context())
	out, replay, err := a.points.Reject(r.Context(), s.ID, s.FamilyID, id, in.Reason, key, hash, *ver)
	if handlePointError(w, err) {
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 200, map[string]any{"data": reviewJSON(out)})
}
func (a *authAPI) reverseCompletion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("completionId")
	if !validID(w, id, "Completion") {
		return
	}
	var in decisionReasonBody
	if !decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" || len(in.Reason) > 500 {
		writeValidation(w, []validationIssue{{"reason", "required", "Provide a reason of at most 500 characters."}})
		return
	}
	key, ver, hash, ok := pointMutationInput(w, r, id, "reverse", in)
	if !ok {
		return
	}
	s := sessionFrom(r.Context())
	out, replay, err := a.points.Reverse(r.Context(), s.ID, s.FamilyID, id, in.Reason, key, hash, *ver)
	if handlePointError(w, err) {
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 200, map[string]any{"data": reviewJSON(out)})
}
func reviewJSON(v points.Completion) map[string]any {
	var decided any
	if v.DecidedAt != nil {
		decided = v.DecidedAt.UTC().Format(time.RFC3339Nano)
	}
	return map[string]any{"id": v.ID, "occurrenceId": v.OccurrenceID, "childId": v.ChildID, "childName": v.ChildName, "childAvatar": v.ChildAvatar, "childColor": v.ChildColor, "title": v.Title, "type": v.ItemType, "localDate": v.LocalDate, "dueDate": nilIfEmpty(v.DueDate), "points": v.Points, "attemptNumber": v.AttemptNumber, "attemptStatus": v.AttemptStatus, "occurrenceStatus": v.OccurrenceStatus, "occurrenceVersion": v.Version, "submittedAt": v.SubmittedAt.UTC().Format(time.RFC3339Nano), "decidedAt": decided, "reason": nilIfEmpty(v.Reason), "ledgerEntryId": nilIfEmpty(v.LedgerEntryID), "ledgerAmount": v.LedgerAmount, "availableActions": []string{"approve", "reject"}}
}
func (a *authAPI) balance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("childId")
	if !validID(w, id, "Child") {
		return
	}
	s := sessionFrom(r.Context())
	n, err := a.points.Balance(r.Context(), s.ID, s.FamilyID, id)
	if handlePointError(w, err) {
		return
	}
	writeJSON(w, 200, map[string]any{"data": map[string]any{"childId": id, "points": n, "asOf": time.Now().UTC().Format(time.RFC3339Nano)}})
}
func ledgerJSON(v points.LedgerEntry) map[string]any {
	label := v.DisplayLabel
	if label == "" {
		switch v.Kind {
		case "award":
			label = "Points earned"
		case "approval_reversal":
			label = "Approval corrected"
		case "manual_correction":
			label = "Points added"
		}
	}
	return map[string]any{"id": v.ID, "childId": v.ChildID, "kind": v.Kind, "amount": v.Amount, "displayLabel": label, "reason": nilIfEmpty(v.Reason), "occurrenceId": nilIfEmpty(v.OccurrenceID), "title": v.Title, "originalEntryId": nilIfEmpty(v.OriginalEntryID), "createdAt": v.CreatedAt.UTC().Format(time.RFC3339Nano)}
}
func (a *authAPI) ledger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("childId")
	if !validID(w, id, "Child") {
		return
	}
	s := sessionFrom(r.Context())
	items, next, err := a.points.Ledger(r.Context(), s.ID, s.FamilyID, id, r.URL.Query().Get("cursor"), pageLimit(r))
	if handlePointError(w, err) {
		return
	}
	out := make([]any, 0, len(items))
	for _, v := range items {
		out = append(out, ledgerJSON(v))
	}
	writeJSON(w, 200, map[string]any{"data": out, "page": map[string]any{"nextCursor": nilIfEmpty(next)}})
}

type correctionBody struct {
	Points int64  `json:"points"`
	Reason string `json:"reason"`
}

func (a *authAPI) correction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("childId")
	if !validID(w, id, "Child") {
		return
	}
	var in correctionBody
	if !decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	issues := []validationIssue{}
	if in.Points < 1 || in.Points > 10000 {
		issues = append(issues, validationIssue{"points", "range", "Points must be between 1 and 10000."})
	}
	if in.Reason == "" || len(in.Reason) > 500 {
		issues = append(issues, validationIssue{"reason", "required", "Provide a reason of at most 500 characters."})
	}
	key, ii := idem(r)
	issues = append(issues, ii...)
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(append([]byte("correction|"+id+"|"), raw...))
	s := sessionFrom(r.Context())
	out, replay, err := a.points.Correct(r.Context(), s.ID, s.FamilyID, id, in.Points, in.Reason, key, sum[:])
	if handlePointError(w, err) {
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 201, map[string]any{"data": ledgerJSON(out)})
}
func (a *authAPI) history(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("childId")
	if !validID(w, id, "Child") {
		return
	}
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	for name, v := range map[string]string{"from": from, "to": to} {
		if v != "" {
			if _, err := time.Parse("2006-01-02", v); err != nil {
				writeValidation(w, []validationIssue{{name, "invalid", "Use YYYY-MM-DD."}})
				return
			}
		}
	}
	s := sessionFrom(r.Context())
	items, next, err := a.points.History(r.Context(), s.ID, s.FamilyID, id, from, to, r.URL.Query().Get("cursor"), pageLimit(r))
	if handlePointError(w, err) {
		return
	}
	writeJSON(w, 200, map[string]any{"data": items, "page": map[string]any{"nextCursor": nilIfEmpty(next)}})
}
func (a *authAPI) report(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("childId")
	if !validID(w, id, "Child") {
		return
	}
	s := sessionFrom(r.Context())
	out, err := a.points.Report(r.Context(), s.ID, s.FamilyID, id, r.URL.Query().Get("period"), r.URL.Query().Get("anchorDate"))
	if handlePointError(w, err) {
		return
	}
	writeJSON(w, 200, map[string]any{"data": out})
}
func handlePointError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var t *points.TransitionError
	if errors.As(err, &t) {
		code := "invalid_state_transition"
		if errors.Is(err, points.ErrVersionConflict) {
			code = "version_conflict"
		}
		writeJSON(w, 409, map[string]any{"error": map[string]any{"code": code, "message": "The item changed. Refresh and try again.", "current": map[string]any{"status": t.Status, "version": t.Version}}})
		return true
	}
	switch {
	case errors.Is(err, points.ErrForbidden):
		writeError(w, 403, "forbidden", "You cannot perform this action.")
	case errors.Is(err, points.ErrNotFound):
		writeError(w, 404, "not_found", "The requested item was not found.")
	case errors.Is(err, points.ErrIdempotency):
		writeError(w, 409, "idempotency_conflict", "The Idempotency-Key was already used for another request.")
	case errors.Is(err, points.ErrValidation):
		writeValidation(w, []validationIssue{{"request", "invalid", "Check the request values."}})
	default:
		writeError(w, 500, "internal_error", "The request could not be completed.")
	}
	return true
}
