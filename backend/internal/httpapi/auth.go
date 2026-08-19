package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/family-habit/family-habit/backend/internal/auth"
	"github.com/family-habit/family-habit/backend/internal/children"
	"github.com/family-habit/family-habit/backend/internal/completions"
	"github.com/family-habit/family-habit/backend/internal/habits"
	"github.com/family-habit/family-habit/backend/internal/points"
	"github.com/family-habit/family-habit/backend/internal/rewards"
	"github.com/family-habit/family-habit/backend/internal/routines"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionCookie = "habit_home_session"

type sessionKey struct{}

type authAPI struct {
	auth        *auth.Service
	pool        *pgxpool.Pool
	secure      bool
	limiter     *loginLimiter
	children    *children.Service
	habits      *habits.Service
	completions *completions.Service
	points      *points.Service
	rewards     *rewards.Service
	routines    *routines.Service
}

func (a *authAPI) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/register", a.register)
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.Handle("POST /api/v1/auth/logout", a.requireSession(a.csrf(http.HandlerFunc(a.logout))))
	mux.Handle("GET /api/v1/session", a.requireSession(http.HandlerFunc(a.current)))
	mux.Handle("POST /api/v1/session/parent/unlock", a.requireSession(a.csrf(http.HandlerFunc(a.unlockParent))))
	mux.Handle("POST /api/v1/session/parent/lock", a.requireSession(a.csrf(http.HandlerFunc(a.lockParent))))
	mux.Handle("GET /api/v1/household", a.requireParent(http.HandlerFunc(a.getHousehold)))
	mux.Handle("PATCH /api/v1/household", a.requireParent(a.csrf(http.HandlerFunc(a.updateHousehold))))
	a.childRoutes(mux)
	a.habitRoutes(mux)
	a.completionRoutes(mux)
	a.pointRoutes(mux)
	a.phase9Routes(mux)
}

type registerInput struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	HouseholdName string `json:"householdName"`
	Timezone      string `json:"timezone"`
	WeekStartsOn  string `json:"weekStartsOn"`
}

var weekdayNameToNumber = map[string]int16{
	"sunday":    0,
	"monday":    1,
	"tuesday":   2,
	"wednesday": 3,
	"thursday":  4,
	"friday":    5,
	"saturday":  6,
}

var weekdayNumberToName = map[int]string{
	0: "sunday",
	1: "monday",
	2: "tuesday",
	3: "wednesday",
	4: "thursday",
	5: "friday",
	6: "saturday",
}

func parseWeekStart(value string) (int16, bool) {
	week, ok := weekdayNameToNumber[strings.ToLower(strings.TrimSpace(value))]
	return week, ok
}

func weekStartName(value int) string {
	if name, ok := weekdayNumberToName[value]; ok {
		return name
	}
	return "sunday"
}

func (a *authAPI) register(w http.ResponseWriter, r *http.Request) {
	var in registerInput
	if !decode(w, r, &in) {
		return
	}
	in.Email = auth.NormalizeEmail(in.Email)
	in.HouseholdName = strings.TrimSpace(in.HouseholdName)
	issues := []validationIssue{}
	if !validEmail(in.Email) {
		issues = append(issues, validationIssue{"email", "invalid", "Enter a valid email address."})
	}
	if len(in.Password) < 12 || len(in.Password) > 128 {
		issues = append(issues, validationIssue{"password", "length", "Password must be 12–128 characters."})
	}
	if len(in.HouseholdName) < 1 || len(in.HouseholdName) > 80 {
		issues = append(issues, validationIssue{"householdName", "length", "Household name must be 1–80 characters."})
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		issues = append(issues, validationIssue{"timezone", "invalid", "Use a valid IANA timezone."})
	}
	week, ok := parseWeekStart(in.WeekStartsOn)
	if !ok {
		issues = append(issues, validationIssue{"weekStartsOn", "invalid", "Choose a weekday."})
	}
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	s, token, err := a.auth.Register(r.Context(), in.Email, in.Password, in.HouseholdName, in.Timezone, week)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			writeError(w, http.StatusConflict, "email_exists", "An account with this email already exists.")
			return
		}
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	a.setCookie(w, token, s.AbsoluteExpiresAt)
	writeSession(w, http.StatusCreated, s)
}

type loginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *authAPI) login(w http.ResponseWriter, r *http.Request) {
	var in loginInput
	if !decode(w, r, &in) {
		return
	}
	key := clientIP(r) + "|" + auth.NormalizeEmail(in.Email)
	if !a.limiter.Allow(key) {
		a.recordFailedLogin(r, in.Email, "rate_limited")
		w.Header().Set("Retry-After", "900")
		writeError(w, 429, "rate_limited", "Too many login attempts. Try again later.")
		return
	}
	s, token, err := a.auth.Login(r.Context(), in.Email, in.Password)
	if err != nil {
		a.limiter.Fail(key)
		a.recordFailedLogin(r, in.Email, "failed")
		writeError(w, 401, "invalid_credentials", "Email or password is incorrect.")
		return
	}
	a.limiter.Success(key)
	a.setCookie(w, token, s.AbsoluteExpiresAt)
	writeSession(w, 200, s)
}

func (a *authAPI) recordFailedLogin(r *http.Request, email, outcome string) {
	identifier := sha256.Sum256([]byte(auth.NormalizeEmail(email)))
	source := sha256.Sum256([]byte(clientIP(r)))
	_, _ = a.pool.Exec(r.Context(), `INSERT INTO authentication_attempts(identifier_hash,source_hash,outcome) VALUES($1,$2,$3)`, identifier[:], source[:], outcome)
}
func (a *authAPI) logout(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	if err := a.auth.Logout(r.Context(), s); err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	a.clearCookie(w)
	w.WriteHeader(204)
}
func (a *authAPI) current(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	csrf, err := a.auth.RotateCSRF(r.Context(), s.ID)
	if err != nil {
		writeError(w, 401, "unauthorized", "Authentication is required.")
		return
	}
	s.CSRFToken = csrf
	writeSession(w, 200, s)
}

type unlockInput struct {
	Password string `json:"password"`
	Pin      string `json:"pin"`
}

func (a *authAPI) unlockParent(w http.ResponseWriter, r *http.Request) {
	var in unlockInput
	if !decode(w, r, &in) {
		return
	}
	s := sessionFrom(r.Context())
	key := clientIP(r) + "|unlock|" + s.UserID
	if !a.limiter.Allow(key) {
		a.recordFailedLogin(r, s.UserID, "rate_limited")
		w.Header().Set("Retry-After", "900")
		writeError(w, 429, "rate_limited", "Too many unlock attempts. Try again later.")
		return
	}
	out, err := a.auth.UnlockParent(r.Context(), s, in.Password, in.Pin)
	if err != nil {
		a.limiter.Fail(key)
		a.recordFailedLogin(r, s.UserID, "failed")
		writeError(w, 401, "invalid_credentials", "Parent credentials are incorrect.")
		return
	}
	a.limiter.Success(key)
	out.CSRFToken = r.Header.Get("X-CSRF-Token")
	writeSession(w, 200, out)
}
func (a *authAPI) lockParent(w http.ResponseWriter, r *http.Request) {
	out, err := a.auth.LockParent(r.Context(), sessionFrom(r.Context()))
	if err != nil {
		writeError(w, 401, "unauthorized", "Authentication is required.")
		return
	}
	out.CSRFToken = r.Header.Get("X-CSRF-Token")
	writeSession(w, 200, out)
}

func (a *authAPI) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			writeError(w, 401, "unauthorized", "Authentication is required.")
			return
		}
		s, err := a.auth.Authenticate(r.Context(), c.Value)
		if err != nil {
			a.clearCookie(w)
			writeError(w, 401, "unauthorized", "Authentication is required.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey{}, s)))
	})
}
func (a *authAPI) requireParent(next http.Handler) http.Handler {
	return a.requireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := sessionFrom(r.Context())
		if s.Mode != "parent" {
			writeError(w, 403, "parent_mode_required", "Unlock Parent Mode to continue.")
			return
		}
		a.auth.Touch(r.Context(), s.ID)
		next.ServeHTTP(w, r)
	}))
}
func (a *authAPI) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := sessionFrom(r.Context())
		if !a.auth.CheckCSRF(r.Context(), s.ID, r.Header.Get("X-CSRF-Token")) {
			writeError(w, 403, "csrf_invalid", "The CSRF token is missing or invalid.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func sessionFrom(ctx context.Context) auth.Session {
	s, _ := ctx.Value(sessionKey{}).(auth.Session)
	return s
}

func (a *authAPI) getHousehold(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r.Context())
	a.householdResponse(w, r, s.FamilyID)
}

type householdPatch struct {
	Name                     *string         `json:"name"`
	Timezone                 *string         `json:"timezone"`
	WeekStartsOn             *string         `json:"weekStartsOn"`
	ParentModeTimeoutMinutes *int16          `json:"parentModeTimeoutMinutes"`
	ParentPin                json.RawMessage `json:"parentPin"`
	RewardsEnabled           *bool           `json:"rewardsEnabled"`
}

func (a *authAPI) updateHousehold(w http.ResponseWriter, r *http.Request) {
	var in householdPatch
	if !decode(w, r, &in) {
		return
	}
	issues := []validationIssue{}
	var mutationKey string
	var expected *int64
	if in.RewardsEnabled != nil {
		var more []validationIssue
		mutationKey, more = idem(r)
		issues = append(issues, more...)
		expected, more = expectedVersion(r)
		issues = append(issues, more...)
		if expected == nil {
			issues = append(issues, validationIssue{"If-Match", "required", "Provide the current household version."})
		}
	}
	if in.Name != nil {
		v := strings.TrimSpace(*in.Name)
		in.Name = &v
		if len(v) < 1 || len(v) > 80 {
			issues = append(issues, validationIssue{"name", "length", "Name must be 1–80 characters."})
		}
	}
	if in.Timezone != nil {
		if _, err := time.LoadLocation(*in.Timezone); err != nil {
			issues = append(issues, validationIssue{"timezone", "invalid", "Use a valid IANA timezone."})
		}
	}
	var week *int16
	if in.WeekStartsOn != nil {
		v, ok := parseWeekStart(*in.WeekStartsOn)
		if !ok {
			issues = append(issues, validationIssue{"weekStartsOn", "invalid", "Choose a weekday."})
		}
		week = &v
	}
	if in.ParentModeTimeoutMinutes != nil && *in.ParentModeTimeoutMinutes != 5 && *in.ParentModeTimeoutMinutes != 15 && *in.ParentModeTimeoutMinutes != 30 {
		issues = append(issues, validationIssue{"parentModeTimeoutMinutes", "invalid", "Choose 5, 15, or 30."})
	}
	if len(issues) > 0 {
		writeValidation(w, issues)
		return
	}
	if in.Name == nil && in.Timezone == nil && week == nil && in.ParentModeTimeoutMinutes == nil && in.ParentPin == nil && in.RewardsEnabled == nil {
		writeValidation(w, []validationIssue{{"body", "required", "Provide at least one setting."}})
		return
	}
	var pin any
	if in.ParentPin != nil {
		if string(in.ParentPin) != "null" {
			var rawPin string
			if err := json.Unmarshal(in.ParentPin, &rawPin); err != nil || len(rawPin) != 6 || strings.Trim(rawPin, "0123456789") != "" {
				writeValidation(w, []validationIssue{{"parentPin", "invalid", "PIN must contain 6 digits."}})
				return
			}
			hash, err := auth.HashPassword(rawPin)
			if err != nil {
				writeError(w, 500, "internal_error", "The request could not be completed.")
				return
			}
			pin = hash
		}
	}
	s := sessionFrom(r.Context())
	var householdReply []byte
	var householdVersion int64
	tx, err := a.pool.Begin(r.Context())
	if err == nil && in.RewardsEnabled != nil {
		requestHash := sha256.Sum256([]byte("household.rewards|" + mutationKey + "|" + string(hashBody(in))))
		var tag pgconn.CommandTag
		if err == nil {
			tag, err = tx.Exec(r.Context(), `INSERT INTO idempotency_records(family_id,session_id,route_family,idempotency_key,request_hash,response_status,response_body,expires_at) VALUES($1,$2,'household.rewards',$3,$4,200,'{}',now()+interval '24 hours') ON CONFLICT DO NOTHING`, s.FamilyID, s.ID, mutationKey, requestHash[:])
		}
		if err == nil && tag.RowsAffected() == 0 {
			var old []byte
			err = tx.QueryRow(r.Context(), `SELECT request_hash,response_body FROM idempotency_records WHERE family_id=$1 AND session_id=$2 AND route_family='household.rewards' AND idempotency_key=$3 FOR UPDATE`, s.FamilyID, s.ID, mutationKey).Scan(&old, &householdReply)
			if err == nil && !bytes.Equal(old, requestHash[:]) {
				_ = tx.Rollback(r.Context())
				writeError(w, 409, "idempotency_conflict", "This key was already used.")
				return
			}
			if err == nil {
				_ = tx.Commit(r.Context())
				var saved struct {
					Data struct {
						Version int64 `json:"version"`
					} `json:"data"`
				}
				if len(householdReply) == 0 || json.Unmarshal(householdReply, &saved) != nil || saved.Data.Version == 0 {
					writeError(w, 500, "internal_error", "The saved response could not be restored.")
					return
				}
				w.Header().Set("Idempotent-Replayed", "true")
				w.Header().Set("ETag", strconv.FormatInt(saved.Data.Version, 10))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(householdReply)
				return
			}
		}
		var current int64
		if err == nil {
			err = tx.QueryRow(r.Context(), `SELECT version FROM families WHERE id=$1 FOR UPDATE`, s.FamilyID).Scan(&current)
		}
		if err == nil && current != *expected {
			_ = tx.Rollback(r.Context())
			writeError(w, 409, "version_conflict", "Refresh household settings and try again.")
			return
		}
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE families SET name=coalesce($2,name),timezone=coalesce($3,timezone),week_starts_on=coalesce($4,week_starts_on),parent_idle_minutes=coalesce($5,parent_idle_minutes),rewards_enabled=coalesce($6,rewards_enabled),version=version+CASE WHEN $6::boolean IS NULL THEN 0 ELSE 1 END,updated_at=now() WHERE id=$1`, s.FamilyID, in.Name, in.Timezone, week, in.ParentModeTimeoutMinutes, in.RewardsEnabled)
	}
	if err == nil && in.RewardsEnabled != nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,after_status,idempotency_key,metadata) VALUES($1,$2,$3,'household.rewards_updated','family',$1,$4,$5,jsonb_build_object('enabled',$6::boolean))`, s.FamilyID, s.UserID, s.ID, map[bool]string{true: "enabled", false: "disabled"}[*in.RewardsEnabled], mutationKey, *in.RewardsEnabled)
	}
	if err == nil && in.ParentPin != nil {
		_, err = tx.Exec(r.Context(), `UPDATE users SET parent_pin_hash=$2,updated_at=now() WHERE id=$1`, s.UserID, pin)
	}
	if err == nil && in.RewardsEnabled != nil {
		err = tx.QueryRow(r.Context(), `SELECT jsonb_build_object('data',jsonb_build_object('id',id,'name',name,'timezone',timezone,'weekStartsOn',CASE week_starts_on WHEN 1 THEN 'monday' WHEN 2 THEN 'tuesday' WHEN 3 THEN 'wednesday' WHEN 4 THEN 'thursday' WHEN 5 THEN 'friday' WHEN 6 THEN 'saturday' ELSE 'sunday' END,'parentModeTimeoutMinutes',parent_idle_minutes,'rewardsEnabled',rewards_enabled,'version',version)),version FROM families WHERE id=$1`, s.FamilyID).Scan(&householdReply, &householdVersion)
	}
	if err == nil && in.RewardsEnabled != nil {
		_, err = tx.Exec(r.Context(), `UPDATE idempotency_records SET response_body=$4::jsonb,response_status=200 WHERE family_id=$1 AND session_id=$2 AND route_family='household.rewards' AND idempotency_key=$3`, s.FamilyID, s.ID, mutationKey, householdReply)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	} else if tx != nil {
		_ = tx.Rollback(r.Context())
	}
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	if in.RewardsEnabled != nil {
		w.Header().Set("ETag", strconv.FormatInt(householdVersion, 10))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(householdReply)
		return
	}
	a.householdResponse(w, r, s.FamilyID)
}
func (a *authAPI) householdResponse(w http.ResponseWriter, r *http.Request, id string) {
	var name, tz string
	var week, idle int
	var rewardsEnabled bool
	var version int64
	err := a.pool.QueryRow(r.Context(), `SELECT name,timezone,week_starts_on,parent_idle_minutes,rewards_enabled,version FROM families WHERE id=$1`, id).Scan(&name, &tz, &week, &idle, &rewardsEnabled, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "not_found", "Household not found.")
		return
	}
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	start := weekStartName(week)
	w.Header().Set("ETag", strconv.FormatInt(version, 10))
	writeJSON(w, 200, map[string]any{"data": map[string]any{"id": id, "name": name, "timezone": tz, "weekStartsOn": start, "parentModeTimeoutMinutes": idle, "rewardsEnabled": rewardsEnabled, "version": version}})
}

func (a *authAPI) setCookie(w http.ResponseWriter, token string, expiry time.Time) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, Expires: expiry, MaxAge: int(time.Until(expiry).Seconds())})
}
func (a *authAPI) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}
func writeSession(w http.ResponseWriter, status int, s auth.Session) {
	actor := "profile_picker"
	if s.Mode == "parent" {
		actor = "parent"
	} else if s.Mode == "child" {
		actor = "child"
	}
	var idleExpiresAt any
	if !s.IdleExpiresAt.IsZero() {
		idleExpiresAt = s.IdleExpiresAt.UTC().Format(time.RFC3339)
	}
	var childID any
	if s.ActiveChildID != "" {
		childID = s.ActiveChildID
	}
	writeJSON(w, status, map[string]any{"data": map[string]any{"actor": actor, "userId": s.UserID, "householdId": s.FamilyID, "childId": childID, "parentMode": s.Mode == "parent", "csrfToken": s.CSRFToken, "idleExpiresAt": idleExpiresAt, "absoluteExpiresAt": s.AbsoluteExpiresAt.UTC().Format(time.RFC3339)}})
}

type validationIssue struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeValidation(w http.ResponseWriter, v []validationIssue) {
	writeJSON(w, 422, map[string]any{"error": map[string]any{"code": "validation_failed", "message": "Check the highlighted fields.", "validation": v}})
}
func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}
func decode(w http.ResponseWriter, r *http.Request, d any) bool {
	if r.ContentLength > maxJSONBodyBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request body must be at most 1 MiB.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(d); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request body must be at most 1 MiB.")
			return false
		}
		writeError(w, 400, "invalid_json", "Request body must be valid JSON.")
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, 400, "invalid_json", "Request body must contain one JSON value.")
		return false
	}
	return true
}
func validEmail(v string) bool {
	a, e := mail.ParseAddress(v)
	return e == nil && a.Address == v && len(v) <= 254
}
func clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	return host
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	now      func() time.Time
}

const maxLimiterKeys = 10000

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: map[string][]time.Time{}, now: time.Now}
}
func (l *loginLimiter) Allow(k string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := l.now().Add(-15 * time.Minute)
	values, exists := l.attempts[k]
	kept := values[:0]
	for _, v := range values {
		if v.After(cut) {
			kept = append(kept, v)
		}
	}
	if len(kept) == 0 {
		delete(l.attempts, k)
	} else {
		l.attempts[k] = kept
	}
	if !exists && len(l.attempts) >= maxLimiterKeys {
		for key, attempts := range l.attempts {
			if len(attempts) == 0 || !attempts[len(attempts)-1].After(cut) {
				delete(l.attempts, key)
			}
		}
		if len(l.attempts) >= maxLimiterKeys {
			return false
		}
	}
	return len(kept) < 5
}
func (l *loginLimiter) Fail(k string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts[k] = append(l.attempts[k], l.now())
}
func (l *loginLimiter) Success(k string) { l.mu.Lock(); defer l.mu.Unlock(); delete(l.attempts, k) }
