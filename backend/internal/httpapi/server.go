package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/family-habit/family-habit/backend/internal/auth"
	"github.com/family-habit/family-habit/backend/internal/children"
	"github.com/family-habit/family-habit/backend/internal/completions"
	"github.com/family-habit/family-habit/backend/internal/habits"
	"github.com/family-habit/family-habit/backend/internal/health"
	"github.com/family-habit/family-habit/backend/internal/points"
	"github.com/family-habit/family-habit/backend/internal/rewards"
	"github.com/family-habit/family-habit/backend/internal/routines"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxJSONBodyBytes int64 = 1 << 20

var safeRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

type Server struct {
	logger    *slog.Logger
	readiness health.Checker
}

func New(logger *slog.Logger, readiness health.Checker) http.Handler {
	s := &Server{logger: logger, readiness: readiness}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", s.live)
	mux.HandleFunc("GET /health/ready", s.ready)
	mux.HandleFunc("GET /api/v1", s.apiRoot)
	return requestLogger(logger, securityHeaders(mux))
}

func NewApp(logger *slog.Logger, readiness health.Checker, pool *pgxpool.Pool, secureCookies bool) http.Handler {
	s := &Server{logger: logger, readiness: readiness}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", s.live)
	mux.HandleFunc("GET /health/ready", s.ready)
	mux.HandleFunc("GET /api/v1", s.apiRoot)
	a := &authAPI{auth: auth.NewService(pool), pool: pool, secure: secureCookies, limiter: newLoginLimiter(), children: children.NewService(pool), habits: habits.NewService(pool), completions: completions.NewService(pool), points: points.NewService(pool), rewards: rewards.NewService(pool), routines: routines.NewService(pool)}
	a.routes(mux)
	return requestLogger(logger, securityHeaders(mux))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if err := s.readiness.Check(r.Context()); err != nil {
		s.logger.Warn("readiness check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) apiRoot(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"name": "Habit Home API", "version": "v1"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if !safeRequestID.MatchString(requestID) {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		response := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(response, r)
		logger.Info("request",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", response.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "req-" + time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(value[:])
}
