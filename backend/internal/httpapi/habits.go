package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/family-habit/family-habit/backend/internal/habits"
)

func (a *authAPI) habitRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/habits", a.requireParent(http.HandlerFunc(a.listHabits)))
	mux.Handle("POST /api/v1/habits", a.requireParent(a.csrf(http.HandlerFunc(a.createHabit))))
	mux.Handle("PATCH /api/v1/habits/{habitId}", a.requireParent(a.csrf(http.HandlerFunc(a.updateHabit))))
	mux.Handle("DELETE /api/v1/habits/{habitId}", a.requireParent(a.csrf(http.HandlerFunc(a.deactivateHabit))))
	mux.Handle("POST /api/v1/habits/{habitId}/assignments", a.requireParent(a.csrf(http.HandlerFunc(a.createAssignment))))
	mux.Handle("PATCH /api/v1/assignments/{assignmentId}", a.requireParent(a.csrf(http.HandlerFunc(a.updateAssignment))))
	mux.Handle("DELETE /api/v1/assignments/{assignmentId}", a.requireParent(a.csrf(http.HandlerFunc(a.deactivateAssignment))))
	mux.Handle("GET /api/v1/tasks", a.requireParent(http.HandlerFunc(a.listTasks)))
	mux.Handle("POST /api/v1/tasks", a.requireParent(a.csrf(http.HandlerFunc(a.createTask))))
	mux.Handle("PATCH /api/v1/tasks/{taskId}", a.requireParent(a.csrf(http.HandlerFunc(a.updateTask))))
	mux.Handle("DELETE /api/v1/tasks/{taskId}", a.requireParent(a.csrf(http.HandlerFunc(a.cancelTask))))
}

type habitBody struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	Icon           string `json:"icon"`
	Color          string `json:"color"`
	EffectiveDate  string `json:"effectiveDate"`
	descriptionSet bool
	iconSet        bool
	colorSet       bool
}
type scheduleBody struct {
	Kind     string   `json:"kind"`
	Weekdays []string `json:"weekdays"`
}
type assignmentBody struct {
	ChildID            string       `json:"childId"`
	ChildIDs           []string     `json:"childIds"`
	Points             int32        `json:"points"`
	Schedule           scheduleBody `json:"schedule"`
	EffectiveStartDate string       `json:"effectiveStartDate"`
	EffectiveDate      string       `json:"effectiveDate"`
}
type taskBody struct {
	ChildID        string `json:"childId"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	DueDate        string `json:"dueDate"`
	Points         int32  `json:"points"`
	descriptionSet bool
}
type reasonBody struct {
	Reason string `json:"reason"`
}

func (b *habitBody) UnmarshalJSON(data []byte) error {
	type plain habitBody
	var v plain
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*b = habitBody(v)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, b.descriptionSet = fields["description"]
	_, b.iconSet = fields["icon"]
	_, b.colorSet = fields["color"]
	return nil
}

func (b *taskBody) UnmarshalJSON(data []byte) error {
	type plain taskBody
	var v plain
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*b = taskBody(v)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, b.descriptionSet = fields["description"]
	return nil
}

func parseDate(value, field string) (time.Time, []validationIssue) {
	d, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, []validationIssue{{field, "invalid", "Use a date in YYYY-MM-DD format."}}
	}
	return d, nil
}
func habitIssues(in habitBody, patch bool) []validationIssue {
	out := []validationIssue{}
	title := strings.TrimSpace(in.Title)
	if (!patch || in.Title != "") && (len([]rune(title)) < 1 || len([]rune(title)) > 120) {
		out = append(out, validationIssue{"title", "length", "Title must be 1–120 characters."})
	}
	if len([]rune(in.Description)) > 500 {
		out = append(out, validationIssue{"description", "length", "Description must be at most 500 characters."})
	}
	if len([]rune(in.Icon)) > 40 {
		out = append(out, validationIssue{"icon", "length", "Icon must be at most 40 characters."})
	}
	if in.Color != "" && !colorPattern.MatchString(in.Color) {
		out = append(out, validationIssue{"color", "invalid", "Use a six-digit hex color."})
	}
	return out
}
func idem(r *http.Request) (string, []validationIssue) {
	k := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if k == "" || len(k) > 128 {
		return "", []validationIssue{{"Idempotency-Key", "required", "Provide an Idempotency-Key header of at most 128 characters."}}
	}
	return k, nil
}
func optionalIdem(r *http.Request) (string, []validationIssue) {
	k := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(k) > 128 {
		return "", []validationIssue{{"Idempotency-Key", "length", "Idempotency-Key must be at most 128 characters."}}
	}
	return k, nil
}
func expectedVersion(r *http.Request) (*int64, []validationIssue) {
	v := strings.TrimSpace(r.Header.Get("If-Match"))
	if v == "" {
		return nil, nil
	}
	v = strings.TrimPrefix(v, "W/")
	v = strings.Trim(v, "\"")
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 1 {
		return nil, []validationIssue{{"If-Match", "invalid", "If-Match must contain a positive resource version."}}
	}
	return &n, nil
}
func hashBody(v any) []byte { b, _ := json.Marshal(v); h := sha256.Sum256(b); return h[:] }

func (a *authAPI) listHabits(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	var active *bool
	if v := r.URL.Query().Get("active"); v != "" {
		b := v == "true"
		active = &b
	}
	items, err := a.habits.ListHabits(r.Context(), s.FamilyID, active)
	if handleHabitError(w, err) {
		return
	}
	data := make([]any, 0, len(items))
	for _, h := range items {
		data = append(data, habitJSON(h))
	}
	writeJSON(w, 200, map[string]any{"data": data, "page": map[string]any{"nextCursor": nil}})
}
func (a *authAPI) createHabit(w http.ResponseWriter, r *http.Request) {
	var in habitBody
	if !decode(w, r, &in) {
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	issues := habitIssues(in, false)
	key, ii := idem(r)
	issues = append(issues, ii...)
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	s := sessionFrom(r.Context())
	h, replay, err := a.habits.CreateHabit(r.Context(), s.ID, s.UserID, s.FamilyID, key, hashBody(in), habits.HabitInput{Title: in.Title, Description: in.Description, Icon: in.Icon, Color: in.Color})
	if handleHabitError(w, err) {
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 201, map[string]any{"data": habitJSON(h)})
}
func (a *authAPI) updateHabit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("habitId")
	if !uuidPattern.MatchString(id) {
		writeError(w, 404, "not_found", "Habit not found.")
		return
	}
	var in habitBody
	if !decode(w, r, &in) {
		return
	}
	issues := habitIssues(in, true)
	date, di := parseDate(in.EffectiveDate, "effectiveDate")
	issues = append(issues, di...)
	key, ki := optionalIdem(r)
	issues = append(issues, ki...)
	expected, vi := expectedVersion(r)
	issues = append(issues, vi...)
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	s := sessionFrom(r.Context())
	requestHash := hashBody(map[string]any{"id": id, "body": in, "descriptionSet": in.descriptionSet, "iconSet": in.iconSet, "colorSet": in.colorSet})
	h, replay, err := a.habits.UpdateHabitConditional(r.Context(), s.ID, s.UserID, s.FamilyID, id, key, requestHash, expected, date, habits.HabitInput{Title: strings.TrimSpace(in.Title), Description: in.Description, Icon: in.Icon, Color: in.Color, DescriptionSet: in.descriptionSet, IconSet: in.iconSet, ColorSet: in.colorSet})
	if handleHabitError(w, err) {
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	w.Header().Set("ETag", strconv.FormatInt(h.Version, 10))
	writeJSON(w, 200, map[string]any{"data": habitJSON(h)})
}
func (a *authAPI) deactivateHabit(w http.ResponseWriter, r *http.Request) {
	date, issues := parseDate(r.URL.Query().Get("effectiveDate"), "effectiveDate")
	key, ki := optionalIdem(r)
	issues = append(issues, ki...)
	expected, vi := expectedVersion(r)
	issues = append(issues, vi...)
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	s := sessionFrom(r.Context())
	id := r.PathValue("habitId")
	replay, err := a.habits.DeactivateHabitConditional(r.Context(), s.ID, s.UserID, s.FamilyID, id, key, hashBody(struct {
		ID   string
		Date time.Time
	}{id, date}), expected, date)
	if handleHabitError(w, err) {
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	w.WriteHeader(204)
}

var weekdayNum = map[string]int16{"sunday": 0, "monday": 1, "tuesday": 2, "wednesday": 3, "thursday": 4, "friday": 5, "saturday": 6}

func assignmentInput(in assignmentBody, update bool) (habits.AssignmentInput, []validationIssue) {
	issues := []validationIssue{}
	if !update && !uuidPattern.MatchString(in.ChildID) {
		issues = append(issues, validationIssue{"childId", "invalid", "Choose a valid child."})
	}
	if in.Points < 1 || in.Points > 10000 {
		if !(update && in.Points == 0) {
			issues = append(issues, validationIssue{"points", "range", "Points must be between 1 and 10,000."})
		}
	}
	if in.Schedule.Kind != "" && in.Schedule.Kind != "daily" && in.Schedule.Kind != "weekdays" {
		issues = append(issues, validationIssue{"schedule.kind", "invalid", "Choose daily or weekdays."})
	}
	days := []int16{}
	seen := map[int16]bool{}
	for _, v := range in.Schedule.Weekdays {
		d, ok := weekdayNum[v]
		if !ok || seen[d] {
			issues = append(issues, validationIssue{"schedule.weekdays", "invalid", "Choose unique valid weekdays."})
			continue
		}
		seen[d] = true
		days = append(days, d)
	}
	sort.Slice(days, func(i, j int) bool { return days[i] < days[j] })
	if in.Schedule.Kind == "daily" && len(days) > 0 {
		issues = append(issues, validationIssue{"schedule.weekdays", "invalid", "Daily schedules do not use weekdays."})
	}
	if in.Schedule.Kind == "weekdays" && len(days) == 0 {
		issues = append(issues, validationIssue{"schedule.weekdays", "required", "Choose at least one weekday."})
	}
	ds := in.EffectiveStartDate
	if update {
		ds = in.EffectiveDate
	}
	date, di := parseDate(ds, "effectiveDate")
	issues = append(issues, di...)
	return habits.AssignmentInput{ChildID: in.ChildID, Points: in.Points, Kind: in.Schedule.Kind, Weekdays: days, EffectiveDate: date}, issues
}
func (a *authAPI) createAssignment(w http.ResponseWriter, r *http.Request) {
	var in assignmentBody
	if !decode(w, r, &in) {
		return
	}
	if len(in.ChildIDs) == 0 && in.ChildID != "" {
		in.ChildIDs = []string{in.ChildID}
	}
	issues := []validationIssue{}
	if len(in.ChildIDs) == 0 {
		issues = append(issues, validationIssue{"childIds", "required", "Choose at least one child."})
	}
	seen := map[string]bool{}
	for _, id := range in.ChildIDs {
		if !uuidPattern.MatchString(id) || seen[id] {
			issues = append(issues, validationIssue{"childIds", "invalid", "Choose unique valid children."})
		}
		seen[id] = true
	}
	in.EffectiveDate = in.EffectiveStartDate
	value, more := assignmentInput(in, true)
	issues = append(issues, more...)
	key, ki := idem(r)
	issues = append(issues, ki...)
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	s := sessionFrom(r.Context())
	inputs := make([]habits.AssignmentInput, 0, len(in.ChildIDs))
	for _, id := range in.ChildIDs {
		v := value
		v.ChildID = id
		inputs = append(inputs, v)
	}
	out, replay, err := a.habits.CreateAssignments(r.Context(), s.ID, s.UserID, s.FamilyID, r.PathValue("habitId"), key, hashBody(in), inputs)
	if handleHabitError(w, err) {
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	data := make([]any, 0, len(out))
	for _, v := range out {
		data = append(data, assignmentJSON(v))
	}
	writeJSON(w, 201, map[string]any{"data": data})
}
func (a *authAPI) updateAssignment(w http.ResponseWriter, r *http.Request) {
	var in assignmentBody
	if !decode(w, r, &in) {
		return
	}
	value, issues := assignmentInput(in, true)
	key, ki := optionalIdem(r)
	issues = append(issues, ki...)
	expected, vi := expectedVersion(r)
	issues = append(issues, vi...)
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	s := sessionFrom(r.Context())
	id := r.PathValue("assignmentId")
	out, replay, err := a.habits.UpdateAssignmentConditional(r.Context(), s.ID, s.UserID, s.FamilyID, id, key, hashBody(struct {
		ID   string
		Body assignmentBody
	}{id, in}), expected, value)
	if handleHabitError(w, err) {
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	w.Header().Set("ETag", strconv.FormatInt(out.Version, 10))
	writeJSON(w, 200, map[string]any{"data": assignmentJSON(out)})
}
func (a *authAPI) deactivateAssignment(w http.ResponseWriter, r *http.Request) {
	date, issues := parseDate(r.URL.Query().Get("effectiveDate"), "effectiveDate")
	key, ki := optionalIdem(r)
	issues = append(issues, ki...)
	expected, vi := expectedVersion(r)
	issues = append(issues, vi...)
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	s := sessionFrom(r.Context())
	id := r.PathValue("assignmentId")
	replay, err := a.habits.DeactivateAssignmentConditional(r.Context(), s.ID, s.UserID, s.FamilyID, id, key, hashBody(struct {
		ID   string
		Date time.Time
	}{id, date}), expected, date)
	if handleHabitError(w, err) {
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	w.WriteHeader(204)
}

func taskIssues(in taskBody, patch bool) []validationIssue {
	out := []validationIssue{}
	if !patch && !uuidPattern.MatchString(in.ChildID) {
		out = append(out, validationIssue{"childId", "invalid", "Choose a valid child."})
	}
	if (!patch || in.Title != "") && (len([]rune(strings.TrimSpace(in.Title))) < 1 || len([]rune(strings.TrimSpace(in.Title))) > 120) {
		out = append(out, validationIssue{"title", "length", "Title must be 1–120 characters."})
	}
	if len([]rune(in.Description)) > 500 {
		out = append(out, validationIssue{"description", "length", "Description must be at most 500 characters."})
	}
	if (!patch || in.Points != 0) && (in.Points < 1 || in.Points > 10000) {
		out = append(out, validationIssue{"points", "range", "Points must be between 1 and 10,000."})
	}
	if !patch || in.DueDate != "" {
		_, di := parseDate(in.DueDate, "dueDate")
		out = append(out, di...)
	}
	return out
}
func (a *authAPI) listTasks(w http.ResponseWriter, r *http.Request) {
	child, status := r.URL.Query().Get("childId"), r.URL.Query().Get("status")
	if child != "" && !uuidPattern.MatchString(child) {
		writeValidation(w, []validationIssue{{"childId", "invalid", "Choose a valid child."}})
		return
	}
	if status != "" && status != "active" && status != "cancelled" {
		writeValidation(w, []validationIssue{{"status", "invalid", "Choose active or cancelled."}})
		return
	}
	s := sessionFrom(r.Context())
	items, err := a.habits.ListTasks(r.Context(), s.FamilyID, child, status)
	if handleHabitError(w, err) {
		return
	}
	data := make([]any, 0, len(items))
	for _, t := range items {
		data = append(data, taskJSON(t))
	}
	writeJSON(w, 200, map[string]any{"data": data, "page": map[string]any{"nextCursor": nil}})
}
func (a *authAPI) createTask(w http.ResponseWriter, r *http.Request) {
	var in taskBody
	if !decode(w, r, &in) {
		return
	}
	issues := taskIssues(in, false)
	key, ki := idem(r)
	issues = append(issues, ki...)
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	date, _ := time.Parse("2006-01-02", in.DueDate)
	s := sessionFrom(r.Context())
	t, replay, err := a.habits.CreateTask(r.Context(), s.ID, s.UserID, s.FamilyID, key, hashBody(in), habits.TaskInput{ChildID: in.ChildID, Title: strings.TrimSpace(in.Title), Description: in.Description, DueDate: date, Points: in.Points})
	if handleHabitError(w, err) {
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 201, map[string]any{"data": taskJSON(t)})
}
func (a *authAPI) updateTask(w http.ResponseWriter, r *http.Request) {
	var in taskBody
	if !decode(w, r, &in) {
		return
	}
	issues := taskIssues(in, true)
	key, ki := optionalIdem(r)
	issues = append(issues, ki...)
	expected, vi := expectedVersion(r)
	issues = append(issues, vi...)
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	var date time.Time
	if in.DueDate != "" {
		date, _ = time.Parse("2006-01-02", in.DueDate)
	}
	s := sessionFrom(r.Context())
	id := r.PathValue("taskId")
	requestHash := hashBody(map[string]any{"id": id, "body": in, "descriptionSet": in.descriptionSet})
	t, replay, err := a.habits.UpdateTaskConditional(r.Context(), s.ID, s.UserID, s.FamilyID, id, key, requestHash, expected, habits.TaskInput{Title: strings.TrimSpace(in.Title), Description: in.Description, DueDate: date, Points: in.Points, DescriptionSet: in.descriptionSet})
	if handleHabitError(w, err) {
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	w.Header().Set("ETag", strconv.FormatInt(t.Version, 10))
	writeJSON(w, 200, map[string]any{"data": taskJSON(t)})
}
func (a *authAPI) cancelTask(w http.ResponseWriter, r *http.Request) {
	var in reasonBody
	if !decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	issues := []validationIssue{}
	if len([]rune(in.Reason)) < 1 || len([]rune(in.Reason)) > 500 {
		issues = append(issues, validationIssue{"reason", "length", "Reason must be 1–500 characters."})
	}
	key, ki := optionalIdem(r)
	issues = append(issues, ki...)
	expected, vi := expectedVersion(r)
	issues = append(issues, vi...)
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	s := sessionFrom(r.Context())
	id := r.PathValue("taskId")
	replay, err := a.habits.CancelTaskConditional(r.Context(), s.ID, s.UserID, s.FamilyID, id, key, hashBody(struct {
		ID   string
		Body reasonBody
	}{id, in}), expected, in.Reason)
	if handleHabitError(w, err) {
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	w.WriteHeader(204)
}

func habitJSON(h habits.Habit) map[string]any {
	as := make([]any, 0, len(h.Assignments))
	for _, v := range h.Assignments {
		as = append(as, assignmentJSON(v))
	}
	return map[string]any{"id": h.ID, "title": h.Title, "description": h.Description, "icon": h.Icon, "color": h.Color, "active": h.Active, "version": h.Version, "createdAt": h.CreatedAt.UTC(), "updatedAt": h.UpdatedAt.UTC(), "assignments": as}
}

var weekdayName = []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}

func assignmentJSON(a habits.Assignment) map[string]any {
	days := []string{}
	for _, d := range a.Schedule.Weekdays {
		if d >= 0 && d < 7 {
			days = append(days, weekdayName[d])
		}
	}
	schedule := map[string]any{"kind": a.Schedule.Kind}
	if a.Schedule.Kind == "weekdays" {
		schedule["weekdays"] = days
	}
	return map[string]any{"id": a.ID, "habitId": a.HabitID, "childId": a.ChildID, "points": a.Points, "schedule": schedule, "effectiveStartDate": a.EffectiveStartDate.Format("2006-01-02"), "active": a.Active, "version": a.Version}
}
func taskJSON(t habits.Task) map[string]any {
	return map[string]any{"id": t.ID, "childId": t.ChildID, "title": t.Title, "description": t.Description, "dueDate": t.DueDate.Format("2006-01-02"), "points": t.Points, "status": t.Status, "version": t.Version, "createdAt": t.CreatedAt.UTC(), "updatedAt": t.UpdatedAt.UTC()}
}
func handleHabitError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, habits.ErrNotFound):
		writeError(w, 404, "not_found", "Resource not found.")
	case errors.Is(err, habits.ErrConflict):
		writeError(w, 409, "conflict", "The requested change conflicts with existing activity or dates.")
	case errors.Is(err, habits.ErrIdempotency):
		writeError(w, 409, "idempotency_conflict", "This idempotency key was already used for another request.")
	case errors.Is(err, habits.ErrVersionConflict):
		writeError(w, 409, "version_conflict", "The resource changed; refresh and retry with its current version.")
	case errors.Is(err, habits.ErrParentAuthority):
		writeError(w, 403, "parent_mode_required", "Unlock Parent Mode to continue.")
	default:
		writeError(w, 500, "internal_error", "The request could not be completed.")
	}
	return true
}
