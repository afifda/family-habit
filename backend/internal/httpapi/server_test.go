package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type checkerFunc func(context.Context) error

func (f checkerFunc) Check(ctx context.Context) error { return f(ctx) }

func TestLiveness(t *testing.T) {
	handler := New(slog.New(slog.NewTextHandler(io.Discard, nil)), checkerFunc(func(context.Context) error { return nil }))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestReadinessUnavailable(t *testing.T) {
	handler := New(slog.New(slog.NewTextHandler(io.Discard, nil)), checkerFunc(func(context.Context) error { return errors.New("down") }))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestRequestIDIsGeneratedAndLoggedWithStatus(t *testing.T) {
	var logs bytes.Buffer
	handler := New(slog.New(slog.NewJSONHandler(&logs, nil)), checkerFunc(func(context.Context) error { return errors.New("down") }))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	requestID := response.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Fatal("expected X-Request-ID response header")
	}
	if !bytes.Contains(logs.Bytes(), []byte(`"request_id":"`+requestID+`"`)) {
		t.Fatalf("log does not contain request ID: %s", logs.String())
	}
	if !bytes.Contains(logs.Bytes(), []byte(`"status":503`)) {
		t.Fatalf("log does not contain response status: %s", logs.String())
	}
}

func TestRequestIDIsPropagated(t *testing.T) {
	handler := New(slog.New(slog.NewTextHandler(io.Discard, nil)), checkerFunc(func(context.Context) error { return nil }))
	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	request.Header.Set("X-Request-ID", "client-request-42")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if got := response.Header().Get("X-Request-ID"); got != "client-request-42" {
		t.Fatalf("X-Request-ID = %q, want client-request-42", got)
	}
}

func TestSecurityHeadersAreApplied(t *testing.T) {
	handler := New(slog.New(slog.NewTextHandler(io.Discard, nil)), checkerFunc(func(context.Context) error { return nil }))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1", nil))

	for name, want := range map[string]string{
		"Cache-Control":                "no-store",
		"Content-Security-Policy":      "default-src 'none'; frame-ancestors 'none'",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Referrer-Policy":              "no-referrer",
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
	} {
		if got := response.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if got := response.Header().Get("Permissions-Policy"); !strings.Contains(got, "camera=()") {
		t.Errorf("Permissions-Policy = %q", got)
	}
}

func TestUnsafeRequestIDIsReplacedAndNotLogged(t *testing.T) {
	var logs bytes.Buffer
	handler := New(slog.New(slog.NewJSONHandler(&logs, nil)), checkerFunc(func(context.Context) error { return nil }))
	request := httptest.NewRequest(http.MethodGet, "/health/live?token=must-not-be-logged", nil)
	request.Header.Set("X-Request-ID", "unsafe request id")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if got := response.Header().Get("X-Request-ID"); got == "unsafe request id" || !safeRequestID.MatchString(got) {
		t.Fatalf("unsafe request ID was not replaced: %q", got)
	}
	if strings.Contains(logs.String(), "must-not-be-logged") {
		t.Fatalf("query secret appeared in logs: %s", logs.String())
	}
}

func TestDecodeRejectsBodiesLargerThanOneMiB(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"value":"`+strings.Repeat("x", int(maxJSONBodyBytes))+`"}`))
	var body map[string]string
	if decode(response, request, &body) {
		t.Fatal("oversized request unexpectedly decoded")
	}
	if response.Code != http.StatusRequestEntityTooLarge || !strings.Contains(response.Body.String(), `"code":"request_too_large"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestDecodeRejectsMalformedUnknownAndTrailingJSON(t *testing.T) {
	for name, input := range map[string]string{
		"malformed": `{`,
		"unknown":   `{"unexpected":true}`,
		"trailing":  `{} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(input))
			var body struct {
				Value string `json:"value"`
			}
			if decode(response, request, &body) || response.Code != http.StatusBadRequest {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestDecodeAcceptsBodyBelowLimit(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"value":"ok"}`))
	var body map[string]string
	if !decode(response, request, &body) || body["value"] != "ok" {
		t.Fatalf("valid request rejected: %d %s", response.Code, response.Body.String())
	}
}
