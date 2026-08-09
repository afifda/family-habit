package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/family-habit/family-habit/backend/internal/auth"
	"github.com/family-habit/family-habit/backend/internal/children"
)

var (
	childPINPattern = regexp.MustCompile(`^[0-9]{4,6}$`)
	colorPattern    = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	uuidPattern     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	validAvatars    = map[string]bool{"fox": true, "bear": true, "rabbit": true, "owl": true, "cat": true, "elephant": true, "panda": true, "koala": true}
)

func (a *authAPI) childRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/profiles", a.requireSession(http.HandlerFunc(a.listProfiles)))
	mux.Handle("GET /api/v1/children", a.requireParent(http.HandlerFunc(a.listChildren)))
	mux.Handle("POST /api/v1/children", a.requireParent(a.csrf(http.HandlerFunc(a.createChild))))
	mux.Handle("PATCH /api/v1/children/{childId}", a.requireParent(a.csrf(http.HandlerFunc(a.updateChild))))
	mux.Handle("DELETE /api/v1/children/{childId}", a.requireParent(a.csrf(http.HandlerFunc(a.archiveChild))))
	mux.Handle("POST /api/v1/session/child", a.requireSession(a.csrf(http.HandlerFunc(a.enterChild))))
	mux.Handle("DELETE /api/v1/session/child", a.requireSession(a.csrf(http.HandlerFunc(a.leaveChild))))
}

type childInput struct {
	Nickname, Avatar, Color, PIN string
	PINSet                       bool
}

func (i *childInput) UnmarshalJSON(b []byte) error {
	var v struct {
		Nickname string `json:"nickname"`
		Avatar   string `json:"avatar"`
		Color    string `json:"color"`
		PIN      string `json:"pin"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	i.Nickname = v.Nickname
	i.Avatar = v.Avatar
	i.Color = v.Color
	i.PIN = v.PIN
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return err
	}
	_, i.PINSet = fields["pin"]
	return nil
}

func validateChild(nickname, avatar, color string, pin *string) []validationIssue {
	issues := []validationIssue{}
	if n := len([]rune(strings.TrimSpace(nickname))); n < 1 || n > 40 {
		issues = append(issues, validationIssue{"nickname", "length", "Nickname must be 1–40 characters."})
	}
	if !validAvatars[avatar] {
		issues = append(issues, validationIssue{"avatar", "invalid", "Choose an available avatar."})
	}
	if !colorPattern.MatchString(color) {
		issues = append(issues, validationIssue{"color", "invalid", "Use a six-digit hex color."})
	}
	if pin != nil && !childPINPattern.MatchString(*pin) {
		issues = append(issues, validationIssue{"pin", "invalid", "PIN must contain 4–6 digits."})
	}
	return issues
}

func (a *authAPI) listChildren(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	include := r.URL.Query().Get("includeArchived") == "true"
	items, err := a.children.List(r.Context(), s.FamilyID, include)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	data := make([]any, 0, len(items))
	for _, c := range items {
		data = append(data, childJSON(c))
	}
	writeJSON(w, 200, map[string]any{"data": data, "page": map[string]any{"nextCursor": nil}})
}

func (a *authAPI) listProfiles(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	items, err := a.children.List(r.Context(), s.FamilyID, false)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	data := make([]any, 0, len(items))
	for _, c := range items {
		data = append(data, childPickerJSON(c))
	}
	writeJSON(w, 200, map[string]any{"data": data, "page": map[string]any{"nextCursor": nil}})
}

func (a *authAPI) createChild(w http.ResponseWriter, r *http.Request) {
	var in childInput
	if !decode(w, r, &in) {
		return
	}
	in.Nickname = children.NormalizeNickname(in.Nickname)
	var pin *string
	if in.PINSet {
		pin = &in.PIN
	}
	issues := validateChild(in.Nickname, in.Avatar, in.Color, pin)
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || len(key) > 128 {
		writeValidation(w, []validationIssue{{"Idempotency-Key", "required", "Provide an Idempotency-Key header of at most 128 characters."}})
		return
	}
	pinHash := ""
	var err error
	if in.PIN != "" {
		pinHash, err = auth.HashPassword(in.PIN)
		if err != nil {
			writeError(w, 500, "internal_error", "The request could not be completed.")
			return
		}
	}
	canonical, _ := json.Marshal(in)
	sum := sha256.Sum256(canonical)
	s := sessionFrom(r.Context())
	c, replayed, err := a.children.Create(r.Context(), s.ID, s.UserID, s.FamilyID, key, sum[:], in.Nickname, in.Avatar, in.Color, pinHash)
	if handleChildError(w, err) {
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, 201, map[string]any{"data": childJSON(c)})
}

type childPatch struct {
	Nickname, Avatar, Color *string
	PIN                     json.RawMessage `json:"pin"`
}

func (a *authAPI) updateChild(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("childId")
	if !uuidPattern.MatchString(id) {
		writeError(w, 404, "not_found", "Child not found.")
		return
	}
	var in childPatch
	if !decode(w, r, &in) {
		return
	}
	if in.Nickname == nil && in.Avatar == nil && in.Color == nil && in.PIN == nil {
		writeValidation(w, []validationIssue{{"body", "required", "Provide at least one field."}})
		return
	}
	var nickname, avatar, color string
	if in.Nickname != nil {
		nickname = children.NormalizeNickname(*in.Nickname)
		in.Nickname = &nickname
	}
	if in.Avatar != nil {
		avatar = *in.Avatar
	} else {
		avatar = "fox"
	}
	if in.Color != nil {
		color = *in.Color
	} else {
		color = "#000000"
	}
	issues := []validationIssue{}
	if in.Nickname != nil {
		n := len([]rune(nickname))
		if n < 1 || n > 40 {
			issues = append(issues, validationIssue{"nickname", "length", "Nickname must be 1–40 characters."})
		}
	}
	if in.Avatar != nil && !validAvatars[avatar] {
		issues = append(issues, validationIssue{"avatar", "invalid", "Choose an available avatar."})
	}
	if in.Color != nil && !colorPattern.MatchString(color) {
		issues = append(issues, validationIssue{"color", "invalid", "Use a six-digit hex color."})
	}
	upd := children.Update{Nickname: in.Nickname, Avatar: in.Avatar, Color: in.Color}
	if in.PIN != nil {
		upd.PINSet = true
		if string(in.PIN) != "null" {
			var pin string
			if json.Unmarshal(in.PIN, &pin) != nil || !childPINPattern.MatchString(pin) {
				issues = append(issues, validationIssue{"pin", "invalid", "PIN must contain 4–6 digits or be null."})
			} else {
				hash, err := auth.HashPassword(pin)
				if err != nil {
					writeError(w, 500, "internal_error", "The request could not be completed.")
					return
				}
				upd.PINHash = &hash
			}
		}
	}
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	s := sessionFrom(r.Context())
	c, err := a.children.Update(r.Context(), s.ID, s.UserID, s.FamilyID, id, upd)
	if handleChildError(w, err) {
		return
	}
	writeJSON(w, 200, map[string]any{"data": childJSON(c)})
}

func (a *authAPI) archiveChild(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("childId")
	if !uuidPattern.MatchString(id) {
		writeError(w, 404, "not_found", "Child not found.")
		return
	}
	s := sessionFrom(r.Context())
	if handleChildError(w, a.children.Archive(r.Context(), s.ID, s.UserID, s.FamilyID, id)) {
		return
	}
	w.WriteHeader(204)
}

type enterChildInput struct {
	ChildID string `json:"childId"`
	PIN     string `json:"pin"`
}

func (a *authAPI) enterChild(w http.ResponseWriter, r *http.Request) {
	var in enterChildInput
	if !decode(w, r, &in) {
		return
	}
	s := sessionFrom(r.Context())
	key := "child-session|" + s.ID + "|" + in.ChildID
	sourceKey := "child-source|" + clientIP(r)
	if !a.limiter.Allow(key) || !a.limiter.Allow(sourceKey) {
		a.auditPINFailure(r, s, in.ChildID, "rate_limited")
		w.Header().Set("Retry-After", "900")
		writeError(w, 429, "rate_limited", "Too many PIN attempts. Try again later.")
		return
	}
	if !uuidPattern.MatchString(in.ChildID) || (in.PIN != "" && !childPINPattern.MatchString(in.PIN)) {
		a.limiter.Fail(key)
		a.limiter.Fail(sourceKey)
		a.auditPINFailure(r, s, in.ChildID, "failed")
		writeChildEntryFailure(w)
		return
	}
	c, err := a.children.Enter(r.Context(), s.ID, s.UserID, s.FamilyID, in.ChildID, in.PIN)
	if errors.Is(err, children.ErrProfileUnavailable) {
		a.limiter.Fail(key)
		a.limiter.Fail(sourceKey)
		a.auditPINFailure(r, s, in.ChildID, "failed")
		writeChildEntryFailure(w)
		return
	}
	if handleChildError(w, err) {
		return
	}
	a.limiter.Success(key)
	a.limiter.Success(sourceKey)
	s.Mode = "child"
	s.ActiveChildID = c.ID
	s.IdleExpiresAt = s.IdleExpiresAt.Add(-s.IdleExpiresAt.Sub(s.IdleExpiresAt))
	s.CSRFToken = r.Header.Get("X-CSRF-Token")
	writeSession(w, 200, s)
}

func (a *authAPI) leaveChild(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	if err := a.children.Leave(r.Context(), s.ID, s.UserID, s.FamilyID); err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	s.Mode = "shared"
	s.ActiveChildID = ""
	s.IdleExpiresAt = s.IdleExpiresAt.Add(-s.IdleExpiresAt.Sub(s.IdleExpiresAt))
	s.CSRFToken = r.Header.Get("X-CSRF-Token")
	writeSession(w, 200, s)
}

func (a *authAPI) auditPINFailure(r *http.Request, s auth.Session, childID, outcome string) {
	identifier := sha256.Sum256([]byte(childID))
	source := sha256.Sum256([]byte(clientIP(r)))
	_, _ = a.pool.Exec(r.Context(), `INSERT INTO authentication_attempts(identifier_hash,source_hash,outcome) VALUES($1,$2,$3)`, identifier[:], source[:], outcome)
	_, _ = a.pool.Exec(r.Context(), `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,metadata) VALUES($1,$2,$3,'child.profile_verification_failed','session',$3,jsonb_build_object('outcome',$4,'sourceHash',encode($5::bytea,'hex')))`, s.FamilyID, s.UserID, s.ID, outcome, source[:])
}

func childJSON(c children.Child) map[string]any {
	return map[string]any{"id": c.ID, "nickname": c.Nickname, "avatar": c.Avatar, "color": c.Color, "pinEnabled": c.PINEnabled, "active": c.Active, "createdAt": c.CreatedAt.UTC(), "updatedAt": c.UpdatedAt.UTC()}
}
func childPickerJSON(c children.Child) map[string]any {
	return map[string]any{"id": c.ID, "nickname": c.Nickname, "avatar": c.Avatar, "color": c.Color, "pinRequired": c.PINEnabled}
}
func writeChildEntryFailure(w http.ResponseWriter) {
	writeError(w, 401, "child_profile_unavailable", "The child profile or PIN could not be verified.")
}
func handleChildError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, children.ErrNotFound):
		writeError(w, 404, "not_found", "Child not found.")
	case errors.Is(err, children.ErrNicknameExists):
		writeError(w, 409, "nickname_conflict", "An active child already uses this nickname.")
	case errors.Is(err, children.ErrIdempotency):
		writeError(w, 409, "idempotency_conflict", "This idempotency key was already used for another request.")
	case errors.Is(err, children.ErrParentAuthority):
		writeError(w, 403, "parent_mode_required", "Unlock Parent Mode to continue.")
	default:
		writeError(w, 500, "internal_error", "The request could not be completed.")
	}
	return true
}
