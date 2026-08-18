package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/family-habit/family-habit/backend/internal/rewards"
	"github.com/family-habit/family-habit/backend/internal/routines"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *authAPI) phase9Routes(m *http.ServeMux) {
	m.Handle("GET /api/v1/routine-groups", a.requireParent(http.HandlerFunc(a.listRoutineGroups)))
	m.Handle("POST /api/v1/routine-groups", a.requireParent(a.csrf(http.HandlerFunc(a.createRoutineGroup))))
	m.Handle("PATCH /api/v1/routine-groups/{id}", a.requireParent(a.csrf(http.HandlerFunc(a.updateRoutineGroup))))
	m.Handle("PUT /api/v1/routine-groups/order", a.requireParent(a.csrf(http.HandlerFunc(a.orderRoutineGroups))))
	m.Handle("POST /api/v1/routine-groups/{id}/archive", a.requireParent(a.csrf(http.HandlerFunc(a.archiveRoutineGroup))))
	m.Handle("GET /api/v1/rewards", a.requireParent(http.HandlerFunc(a.listRewards)))
	m.Handle("POST /api/v1/rewards", a.requireParent(a.csrf(http.HandlerFunc(a.createReward))))
	m.Handle("PATCH /api/v1/rewards/{id}", a.requireParent(a.csrf(http.HandlerFunc(a.updateReward))))
	m.Handle("POST /api/v1/rewards/{id}/archive", a.requireParent(a.csrf(http.HandlerFunc(a.archiveReward))))
	m.Handle("GET /api/v1/child/rewards", a.requireSession(http.HandlerFunc(a.childRewards)))
	m.Handle("POST /api/v1/child/rewards/{id}/redemptions", a.requireSession(a.csrf(http.HandlerFunc(a.redeemReward))))
	m.Handle("GET /api/v1/child/reward-redemptions", a.requireSession(http.HandlerFunc(a.childRedemptions)))
	m.Handle("GET /api/v1/reward-redemptions", a.requireParent(http.HandlerFunc(a.parentRedemptions)))
	m.Handle("POST /api/v1/reward-redemptions/{id}/fulfill", a.requireParent(a.csrf(http.HandlerFunc(a.fulfillRedemption))))
	m.Handle("POST /api/v1/reward-redemptions/{id}/cancel", a.requireParent(a.csrf(http.HandlerFunc(a.cancelRedemption))))
	m.Handle("GET /api/v1/reward-eligibility-policy", a.requireParent(http.HandlerFunc(a.getRewardEligibilityPolicy)))
	m.Handle("PUT /api/v1/reward-eligibility-policy", a.requireParent(a.csrf(http.HandlerFunc(a.putRewardEligibilityPolicy))))
	m.Handle("GET /api/v1/reward-eligibility-progress", a.requireParent(http.HandlerFunc(a.rewardEligibilityProgress)))
	m.Handle("GET /api/v1/reward-eligibility-evaluations", a.requireParent(http.HandlerFunc(a.rewardEligibilityEvaluations)))
}

type eligibilityPolicyBody struct {
	Enabled                     bool   `json:"enabled"`
	Period                      string `json:"period"`
	MinimumPoints               int    `json:"minimumPoints"`
	MinimumCompletionPercentage *int   `json:"minimumCompletionPercentage"`
	MaximumRedemptions          *int   `json:"maximumRedemptions"`
	GraceHours                  int    `json:"graceHours"`
}

func (a *authAPI) getRewardEligibilityPolicy(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	p, e := a.rewards.GetEligibilityPolicy(r.Context(), s.ID, s.FamilyID)
	if phase9Error(w, e) {
		return
	}
	writeJSON(w, 200, map[string]any{"data": p})
}
func (a *authAPI) putRewardEligibilityPolicy(w http.ResponseWriter, r *http.Request) {
	var b eligibilityPolicyBody
	if !decode(w, r, &b) {
		return
	}
	issues := []validationIssue{}
	if b.Period != "daily" && b.Period != "weekly" && b.Period != "monthly" {
		issues = append(issues, validationIssue{"period", "invalid", "Choose daily, weekly, or monthly."})
	}
	if b.MinimumPoints < 1 || b.MinimumPoints > 1000000 {
		issues = append(issues, validationIssue{"minimumPoints", "range", "Minimum points must be 1–1,000,000."})
	}
	if b.MinimumCompletionPercentage != nil && (*b.MinimumCompletionPercentage < 1 || *b.MinimumCompletionPercentage > 100) {
		issues = append(issues, validationIssue{"minimumCompletionPercentage", "range", "Completion percentage must be 1–100."})
	}
	if b.MaximumRedemptions != nil && (*b.MaximumRedemptions < 1 || *b.MaximumRedemptions > 100) {
		issues = append(issues, validationIssue{"maximumRedemptions", "range", "Maximum redemptions must be 1–100."})
	}
	if b.Period == "daily" && b.GraceHours != 0 {
		issues = append(issues, validationIssue{"graceHours", "daily_zero", "Daily collection requires a zero-hour grace period."})
	} else if b.Period != "daily" && b.GraceHours != 0 && b.GraceHours != 12 && b.GraceHours != 24 && b.GraceHours != 48 {
		issues = append(issues, validationIssue{"graceHours", "invalid", "Choose 0, 12, 24, or 48 hours."})
	}
	k, ki := phase9Key(r)
	issues = append(issues, ki...)
	raw := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.Header.Get("If-Match")), "W/"), "\"")
	expected, e := strconv.ParseInt(raw, 10, 64)
	if e != nil || expected < 0 {
		issues = append(issues, validationIssue{"If-Match", "invalid", "If-Match must contain the current version, or 0 for initial setup."})
	}
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	s := sessionFrom(r.Context())
	p, re, err := a.rewards.PutEligibilityPolicy(r.Context(), s.ID, s.FamilyID, k, hashBody(b), expected, rewards.EligibilityPolicyInput{Enabled: b.Enabled, Period: b.Period, MinimumPoints: b.MinimumPoints, MinimumCompletionPercentage: b.MinimumCompletionPercentage, MaximumRedemptions: b.MaximumRedemptions, GraceHours: b.GraceHours})
	if phase9Error(w, err) {
		return
	}
	if re {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	w.Header().Set("ETag", strconv.FormatInt(p.Version, 10))
	writeJSON(w, 200, map[string]any{"data": p})
}
func (a *authAPI) rewardEligibilityProgress(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	x, e := a.rewards.ParentEligibilityProgress(r.Context(), s.ID, s.FamilyID)
	if phase9Error(w, e) {
		return
	}
	writeJSON(w, 200, map[string]any{"data": x})
}
func (a *authAPI) rewardEligibilityEvaluations(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	childID := r.URL.Query().Get("childId")
	if childID != "" && !uuidPattern.MatchString(childID) {
		writeValidation(w, []validationIssue{{"childId", "invalid", "Choose a valid child."}})
		return
	}
	x, e := a.rewards.EligibilityHistory(r.Context(), s.ID, s.FamilyID, childID, pageLimit(r))
	if phase9Error(w, e) {
		return
	}
	writeJSON(w, 200, map[string]any{"data": x, "page": map[string]any{"nextCursor": nil}})
}

type routineBody struct {
	Name          string `json:"name"`
	Icon          string `json:"icon"`
	Color         string `json:"color"`
	StartsAtLocal string `json:"startsAtLocal"`
	EndsAtLocal   string `json:"endsAtLocal"`
	SortOrder     int    `json:"sortOrder"`
}

type optionalString struct {
	Set   bool
	Value *string
}

func (o *optionalString) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

type routinePatchBody struct {
	Name          *string        `json:"name"`
	Icon          optionalString `json:"icon"`
	Color         optionalString `json:"color"`
	StartsAtLocal optionalString `json:"startsAtLocal"`
	EndsAtLocal   optionalString `json:"endsAtLocal"`
	SortOrder     *int           `json:"sortOrder"`
	sortOrderSet  bool
}

func (b *routinePatchBody) UnmarshalJSON(data []byte) error {
	type plain routinePatchBody
	var v plain
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*b = routinePatchBody(v)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, b.sortOrderSet = fields["sortOrder"]
	return nil
}

func routineIssues(b routineBody) []validationIssue {
	o := []validationIssue{}
	n := strings.TrimSpace(b.Name)
	if len([]rune(n)) < 1 || len([]rune(n)) > 60 {
		o = append(o, validationIssue{"name", "length", "Name must be 1–60 characters."})
	}
	if len([]rune(b.Icon)) > 40 {
		o = append(o, validationIssue{"icon", "length", "Icon must be at most 40 characters."})
	}
	if !safePhase9Icon(b.Icon) {
		o = append(o, validationIssue{"icon", "invalid", "Use a short text or emoji icon without markup or control characters."})
	}
	if b.Color != "" && !colorPattern.MatchString(b.Color) {
		o = append(o, validationIssue{"color", "invalid", "Use a six-digit hex color."})
	}
	if b.SortOrder < 0 {
		o = append(o, validationIssue{"sortOrder", "range", "Sort order must not be negative."})
	}
	if (b.StartsAtLocal == "") != (b.EndsAtLocal == "") {
		o = append(o, validationIssue{"startsAtLocal", "paired", "Provide both time hints or neither."})
	}
	for f, v := range map[string]string{"startsAtLocal": b.StartsAtLocal, "endsAtLocal": b.EndsAtLocal} {
		if v != "" {
			if _, e := time.Parse("15:04", v); e != nil {
				o = append(o, validationIssue{f, "invalid", "Use HH:mm time."})
			}
		}
	}
	if b.StartsAtLocal != "" && b.StartsAtLocal == b.EndsAtLocal {
		o = append(o, validationIssue{"endsAtLocal", "invalid", "Time hints cannot be equal."})
	}
	return o
}

var phase9Icons = map[string]bool{"": true, "🌅": true, "☀️": true, "🏫": true, "🌆": true, "🌙": true, "⭐": true, "🎁": true, "🍦": true, "🎮": true, "🎬": true, "📚": true, "🚲": true, "🍕": true, "🎨": true, "⚽": true}

func safePhase9Icon(v string) bool { return phase9Icons[v] }
func phase9Key(r *http.Request) (string, []validationIssue) {
	k := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(k) < 8 || len(k) > 128 {
		return "", []validationIssue{{"Idempotency-Key", "length", "Provide an Idempotency-Key of 8–128 characters."}}
	}
	return k, nil
}
func phase9Version(r *http.Request) (int64, []validationIssue) {
	v, x := expectedVersion(r)
	if len(x) > 0 {
		return 0, x
	}
	if v == nil {
		return 0, []validationIssue{{"If-Match", "required", "Provide the current resource version."}}
	}
	return *v, nil
}
func (a *authAPI) listRoutineGroups(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	x, e := a.routines.List(r.Context(), s.FamilyID, r.URL.Query().Get("includeArchived") == "true")
	if phase9Error(w, e) {
		return
	}
	writeJSON(w, 200, map[string]any{"data": x})
}
func (a *authAPI) createRoutineGroup(w http.ResponseWriter, r *http.Request) {
	var b routineBody
	if !decode(w, r, &b) {
		return
	}
	b.Name = strings.TrimSpace(b.Name)
	is := routineIssues(b)
	k, ki := phase9Key(r)
	is = append(is, ki...)
	if len(is) > 0 {
		writeValidation(w, is)
		return
	}
	s := sessionFrom(r.Context())
	x, re, e := a.routines.Create(r.Context(), s.ID, s.FamilyID, k, hashBody(b), routines.Input{Name: b.Name, Icon: b.Icon, Color: b.Color, StartsAtLocal: b.StartsAtLocal, EndsAtLocal: b.EndsAtLocal, SortOrder: b.SortOrder})
	if phase9Error(w, e) {
		return
	}
	if re {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 201, map[string]any{"data": x})
}
func (a *authAPI) updateRoutineGroup(w http.ResponseWriter, r *http.Request) {
	var b routinePatchBody
	if !decode(w, r, &b) {
		return
	}
	is := []validationIssue{}
	if b.Name != nil {
		*b.Name = strings.TrimSpace(*b.Name)
		if n := len([]rune(*b.Name)); n < 1 || n > 60 {
			is = append(is, validationIssue{"name", "length", "Name must be 1–60 characters."})
		}
	}
	if b.Icon.Set && b.Icon.Value != nil && len([]rune(*b.Icon.Value)) > 40 {
		is = append(is, validationIssue{"icon", "length", "Icon must be at most 40 characters."})
	}
	if b.Color.Set && b.Color.Value != nil && !colorPattern.MatchString(*b.Color.Value) {
		is = append(is, validationIssue{"color", "invalid", "Use a six-digit hex color."})
	}
	if b.SortOrder != nil && *b.SortOrder < 0 {
		is = append(is, validationIssue{"sortOrder", "range", "Sort order must not be negative."})
	}
	if b.sortOrderSet {
		is = append(is, validationIssue{"sortOrder", "managed", "Use the routine order endpoint to change ordering."})
	}
	if b.StartsAtLocal.Set != b.EndsAtLocal.Set {
		is = append(is, validationIssue{"startsAtLocal", "paired", "Update both time hints together."})
	}
	if b.StartsAtLocal.Set && ((b.StartsAtLocal.Value == nil) != (b.EndsAtLocal.Value == nil)) {
		is = append(is, validationIssue{"startsAtLocal", "paired", "Provide both time hints or clear both."})
	}
	if b.StartsAtLocal.Value != nil && b.EndsAtLocal.Value != nil {
		if _, e := time.Parse("15:04", *b.StartsAtLocal.Value); e != nil {
			is = append(is, validationIssue{"startsAtLocal", "invalid", "Use HH:mm time."})
		}
		if _, e := time.Parse("15:04", *b.EndsAtLocal.Value); e != nil {
			is = append(is, validationIssue{"endsAtLocal", "invalid", "Use HH:mm time."})
		}
		if *b.StartsAtLocal.Value == *b.EndsAtLocal.Value {
			is = append(is, validationIssue{"endsAtLocal", "invalid", "Time hints cannot be equal."})
		}
	}
	if b.Name == nil && !b.Icon.Set && !b.Color.Set && !b.StartsAtLocal.Set && b.SortOrder == nil {
		is = append(is, validationIssue{"request", "empty", "Provide at least one field to update."})
	}
	k, ki := phase9Key(r)
	is = append(is, ki...)
	v, vi := phase9Version(r)
	is = append(is, vi...)
	if len(is) > 0 {
		writeValidation(w, is)
		return
	}
	s := sessionFrom(r.Context())
	x, re, e := a.routines.Update(r.Context(), s.ID, s.FamilyID, r.PathValue("id"), k, hashBody(b), v, routines.UpdateInput{Name: b.Name, Icon: b.Icon.Value, Color: b.Color.Value, StartsAtLocal: b.StartsAtLocal.Value, EndsAtLocal: b.EndsAtLocal.Value, IconSet: b.Icon.Set, ColorSet: b.Color.Set, StartsAtSet: b.StartsAtLocal.Set, EndsAtSet: b.EndsAtLocal.Set, SortOrder: b.SortOrder})
	if phase9Error(w, e) {
		return
	}
	if re {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	w.Header().Set("ETag", strconv.FormatInt(x.Version, 10))
	writeJSON(w, 200, map[string]any{"data": x})
}

type orderBody struct {
	OrderedIDs []string `json:"orderedIds"`
	Items      []struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	} `json:"items"`
}

func (a *authAPI) orderRoutineGroups(w http.ResponseWriter, r *http.Request) {
	var b orderBody
	if !decode(w, r, &b) {
		return
	}
	is := []validationIssue{}
	vs := map[string]int64{}
	ordered := map[string]bool{}
	if len(b.OrderedIDs) == 0 || len(b.Items) != len(b.OrderedIDs) {
		is = append(is, validationIssue{"items", "complete", "Provide every active group and version."})
	}
	for _, x := range b.Items {
		if !uuidPattern.MatchString(x.ID) || x.Version < 1 || vs[x.ID] > 0 {
			is = append(is, validationIssue{"items", "invalid", "Provide unique group IDs and positive versions."})
		}
		vs[x.ID] = x.Version
	}
	for _, id := range b.OrderedIDs {
		if !uuidPattern.MatchString(id) || ordered[id] || vs[id] < 1 {
			is = append(is, validationIssue{"orderedIds", "invalid", "Ordered IDs must exactly match the unique versioned items."})
		}
		ordered[id] = true
	}
	k, ki := phase9Key(r)
	is = append(is, ki...)
	if len(is) > 0 {
		writeValidation(w, is)
		return
	}
	s := sessionFrom(r.Context())
	x, re, e := a.routines.Reorder(r.Context(), s.ID, s.FamilyID, k, hashBody(b), b.OrderedIDs, vs)
	if phase9Error(w, e) {
		return
	}
	if re {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 200, map[string]any{"data": x})
}

type archiveRoutineBody struct {
	EffectiveFrom        string  `json:"effectiveFrom"`
	MoveToRoutineGroupID *string `json:"moveToRoutineGroupId"`
	destinationSet       bool
}

func (b *archiveRoutineBody) UnmarshalJSON(data []byte) error {
	type plain archiveRoutineBody
	var v plain
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*b = archiveRoutineBody(v)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, b.destinationSet = fields["moveToRoutineGroupId"]
	return nil
}

func (a *authAPI) archiveRoutineGroup(w http.ResponseWriter, r *http.Request) {
	var b archiveRoutineBody
	if !decode(w, r, &b) {
		return
	}
	is := []validationIssue{}
	if !b.destinationSet {
		is = append(is, validationIssue{"moveToRoutineGroupId", "required", "Choose another routine or Other."})
	}
	if _, e := time.Parse("2006-01-02", b.EffectiveFrom); e != nil {
		is = append(is, validationIssue{"effectiveFrom", "invalid", "Use YYYY-MM-DD."})
	}
	if b.MoveToRoutineGroupID != nil && *b.MoveToRoutineGroupID != "" && !uuidPattern.MatchString(*b.MoveToRoutineGroupID) {
		is = append(is, validationIssue{"moveToRoutineGroupId", "invalid", "Choose a valid routine group."})
	}
	k, ki := phase9Key(r)
	is = append(is, ki...)
	v, vi := phase9Version(r)
	is = append(is, vi...)
	if len(is) > 0 {
		writeValidation(w, is)
		return
	}
	s := sessionFrom(r.Context())
	effective, _ := time.Parse("2006-01-02", b.EffectiveFrom)
	x, re, e := a.routines.Archive(r.Context(), s.ID, s.FamilyID, r.PathValue("id"), k, hashBody(b), v, effective, b.MoveToRoutineGroupID)
	if phase9Error(w, e) {
		return
	}
	if re {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 200, map[string]any{"data": x})
}

type rewardBody struct {
	Title             string   `json:"title"`
	Description       string   `json:"description"`
	Icon              string   `json:"icon"`
	CostPoints        int64    `json:"costPoints"`
	AvailabilityScope string   `json:"availabilityScope"`
	EligibleChildIDs  []string `json:"eligibleChildIds"`
}

func rewardIssues(b rewardBody) []validationIssue {
	o := []validationIssue{}
	if n := len([]rune(strings.TrimSpace(b.Title))); n < 1 || n > 80 {
		o = append(o, validationIssue{"title", "length", "Title must be 1–80 characters."})
	}
	if len([]rune(b.Description)) > 500 {
		o = append(o, validationIssue{"description", "length", "Description must be at most 500 characters."})
	}
	if len([]rune(b.Icon)) > 40 || !safePhase9Icon(b.Icon) {
		o = append(o, validationIssue{"icon", "invalid", "Use a short text or emoji icon without markup or control characters."})
	}
	if b.CostPoints < 1 || b.CostPoints > 10000 {
		o = append(o, validationIssue{"costPoints", "range", "Cost must be 1–10,000 points."})
	}
	if b.AvailabilityScope != "all_active_children" && b.AvailabilityScope != "selected_children" {
		o = append(o, validationIssue{"availabilityScope", "invalid", "Choose a valid availability scope."})
	}
	if b.AvailabilityScope == "selected_children" && len(b.EligibleChildIDs) == 0 {
		o = append(o, validationIssue{"eligibleChildIds", "required", "Choose at least one child."})
	}
	return o
}
func (a *authAPI) listRewards(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	x, e := a.rewards.List(r.Context(), s.FamilyID, r.URL.Query().Get("includeArchived") == "true")
	if phase9Error(w, e) {
		return
	}
	writeJSON(w, 200, map[string]any{"data": x})
}
func (a *authAPI) createReward(w http.ResponseWriter, r *http.Request) {
	var b rewardBody
	if !decode(w, r, &b) {
		return
	}
	is := rewardIssues(b)
	k, ki := phase9Key(r)
	is = append(is, ki...)
	if len(is) > 0 {
		writeValidation(w, is)
		return
	}
	s := sessionFrom(r.Context())
	x, re, e := a.rewards.Create(r.Context(), s.ID, s.FamilyID, k, hashBody(b), rewards.RewardInput{Title: strings.TrimSpace(b.Title), Description: b.Description, Icon: b.Icon, AvailabilityScope: b.AvailabilityScope, CostPoints: b.CostPoints, EligibleChildIDs: b.EligibleChildIDs})
	if phase9Error(w, e) {
		return
	}
	if re {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 201, map[string]any{"data": x})
}
func (a *authAPI) updateReward(w http.ResponseWriter, r *http.Request) {
	var b rewardBody
	if !decode(w, r, &b) {
		return
	}
	is := rewardIssues(b)
	k, ki := phase9Key(r)
	is = append(is, ki...)
	v, vi := phase9Version(r)
	is = append(is, vi...)
	if len(is) > 0 {
		writeValidation(w, is)
		return
	}
	s := sessionFrom(r.Context())
	x, re, e := a.rewards.Update(r.Context(), s.ID, s.FamilyID, r.PathValue("id"), k, hashBody(b), v, rewards.RewardInput{Title: strings.TrimSpace(b.Title), Description: b.Description, Icon: b.Icon, AvailabilityScope: b.AvailabilityScope, CostPoints: b.CostPoints, EligibleChildIDs: b.EligibleChildIDs})
	if phase9Error(w, e) {
		return
	}
	if re {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 200, map[string]any{"data": x})
}
func (a *authAPI) archiveReward(w http.ResponseWriter, r *http.Request) {
	k, is := phase9Key(r)
	v, vi := phase9Version(r)
	is = append(is, vi...)
	if len(is) > 0 {
		writeValidation(w, is)
		return
	}
	s := sessionFrom(r.Context())
	x, re, e := a.rewards.Archive(r.Context(), s.ID, s.FamilyID, r.PathValue("id"), k, hashBody(r.PathValue("id")), v)
	if phase9Error(w, e) {
		return
	}
	if re {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 200, map[string]any{"data": x})
}
func (a *authAPI) childRewards(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	x, b, e := a.rewards.ChildCatalog(r.Context(), s.ID, s.FamilyID)
	if phase9Error(w, e) {
		return
	}
	eligibility, e := a.rewards.ChildEligibility(r.Context(), s.ID, s.FamilyID)
	if phase9Error(w, e) {
		return
	}
	if eligibility.PolicyEnabled && !eligibility.CanRedeem {
		for i := range x {
			x[i].CanRedeem = false
		}
	}
	writeJSON(w, 200, map[string]any{"data": x, "balance": b, "eligibility": eligibility})
}

type redeemBody struct {
	RewardVersion       int64 `json:"rewardVersion"`
	ConfirmedCostPoints int64 `json:"confirmedCostPoints"`
}

func (a *authAPI) redeemReward(w http.ResponseWriter, r *http.Request) {
	var b redeemBody
	if !decode(w, r, &b) {
		return
	}
	is := []validationIssue{}
	if b.RewardVersion < 1 || b.ConfirmedCostPoints < 1 {
		is = append(is, validationIssue{"rewardVersion", "invalid", "Confirm the current reward and cost."})
	}
	k, ki := phase9Key(r)
	is = append(is, ki...)
	if len(is) > 0 {
		writeValidation(w, is)
		return
	}
	s := sessionFrom(r.Context())
	x, re, e := a.rewards.Redeem(r.Context(), s.ID, s.FamilyID, r.PathValue("id"), k, hashBody(b), b.RewardVersion, b.ConfirmedCostPoints)
	if phase9Error(w, e) {
		return
	}
	if re {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 201, map[string]any{"data": x})
}
func (a *authAPI) parentRedemptions(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	x, next, e := a.rewards.ListParent(r.Context(), s.ID, s.FamilyID, r.URL.Query().Get("state"), r.URL.Query().Get("childId"), r.URL.Query().Get("cursor"), pageLimit(r))
	if phase9Error(w, e) {
		return
	}
	writeJSON(w, 200, map[string]any{"data": x, "page": map[string]any{"nextCursor": nilIfEmpty(next)}})
}
func (a *authAPI) childRedemptions(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	x, next, e := a.rewards.ListChild(r.Context(), s.ID, s.FamilyID, r.URL.Query().Get("cursor"), pageLimit(r))
	if phase9Error(w, e) {
		return
	}
	data := make([]any, 0, len(x))
	for _, v := range x {
		data = append(data, map[string]any{"id": v.ID, "rewardId": v.RewardID, "rewardTitle": v.RewardTitle, "rewardIcon": v.RewardIcon, "costPoints": v.CostPoints, "state": v.State, "requestedAt": v.RequestedAt, "decidedAt": v.DecidedAt, "reservedPoints": -v.CostPoints, "refundedPoints": map[bool]int64{true: v.CostPoints, false: 0}[v.State == "cancelled"], "version": v.Version})
	}
	writeJSON(w, 200, map[string]any{"data": data, "page": map[string]any{"nextCursor": nilIfEmpty(next)}})
}
func (a *authAPI) fulfillRedemption(w http.ResponseWriter, r *http.Request) {
	a.decideRedemption(w, r, "fulfill", "")
}
func (a *authAPI) cancelRedemption(w http.ResponseWriter, r *http.Request) {
	var b reasonBody
	if !decode(w, r, &b) {
		return
	}
	b.Reason = strings.TrimSpace(b.Reason)
	if len([]rune(b.Reason)) < 1 || len([]rune(b.Reason)) > 500 {
		writeValidation(w, []validationIssue{{"reason", "length", "Reason must be 1–500 characters."}})
		return
	}
	a.decideRedemption(w, r, "cancel", b.Reason)
}
func (a *authAPI) decideRedemption(w http.ResponseWriter, r *http.Request, action, reason string) {
	k, is := phase9Key(r)
	v, vi := phase9Version(r)
	is = append(is, vi...)
	if len(is) > 0 {
		writeValidation(w, is)
		return
	}
	s := sessionFrom(r.Context())
	x, re, e := a.rewards.Decide(r.Context(), s.ID, s.FamilyID, r.PathValue("id"), action, reason, k, hashBody(map[string]any{"id": r.PathValue("id"), "action": action, "reason": reason, "version": v}), v)
	if phase9Error(w, e) {
		return
	}
	if re {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 200, map[string]any{"data": x})
}
func phase9Error(w http.ResponseWriter, e error) bool {
	if e == nil {
		return false
	}
	switch {
	case errors.Is(e, routines.ErrNotFound), errors.Is(e, rewards.ErrNotFound), errors.Is(e, rewards.ErrUnavailable):
		writeError(w, 404, "not_found", "Resource not found.")
	case errors.Is(e, routines.ErrVersionConflict), errors.Is(e, rewards.ErrVersionConflict):
		writeError(w, 409, "version_conflict", "Refresh and try again.")
	case errors.Is(e, routines.ErrIdempotency), errors.Is(e, rewards.ErrIdempotency):
		writeError(w, 409, "idempotency_conflict", "This key was already used for another request.")
	case errors.Is(e, routines.ErrConflict):
		writeError(w, 409, "group_in_use", "The routine group cannot be changed.")
	case errors.Is(e, rewards.ErrDisabled):
		writeError(w, 409, "rewards_disabled", "Rewards are not enabled.")
	case errors.Is(e, rewards.ErrInsufficient):
		writeError(w, 409, "insufficient_points", "There are not enough points.")
	case errors.Is(e, rewards.ErrEligibility):
		writeError(w, 409, "eligibility_required", "The current collection-period rules have not unlocked rewards.")
	case errors.Is(e, rewards.ErrEligibilityNotFinal):
		writeError(w, 409, "eligibility_not_final", "This collection period has not been evaluated yet.")
	case errors.Is(e, rewards.ErrEligibilityNotMet):
		writeError(w, 409, "eligibility_not_met", "The previous collection period did not unlock rewards.")
	case errors.Is(e, rewards.ErrEligibilityExpired):
		writeError(w, 409, "eligibility_expired", "This reward eligibility window has ended.")
	case errors.Is(e, rewards.ErrRedemptionLimit):
		writeError(w, 409, "redemption_limit_reached", "The redemption limit for this eligibility period has been reached.")
	case errors.Is(e, rewards.ErrInvalidState):
		writeError(w, 409, "invalid_state_transition", "This request was already decided.")
	case errors.Is(e, rewards.ErrCursor):
		writeValidation(w, []validationIssue{{"cursor", "invalid", "Use the cursor returned by the previous page."}})
	case errors.Is(e, routines.ErrValidation), errors.Is(e, rewards.ErrValidation):
		writeValidation(w, []validationIssue{{"icon", "invalid", "Choose an approved icon."}})
	case errors.Is(e, routines.ErrForbidden), errors.Is(e, rewards.ErrForbidden):
		writeError(w, 403, "forbidden", "This action is unavailable.")
	default:
		var p *pgconn.PgError
		if errors.As(e, &p) && (p.Code == "23505" || p.Code == "23514") {
			writeError(w, 409, "conflict", "The resource conflicts with existing data.")
		} else {
			writeError(w, 500, "internal_error", "The request could not be completed.")
		}
	}
	return true
}
