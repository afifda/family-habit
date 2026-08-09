package config

import (
	"fmt"
	"os"
	"time"
)

const (
	defaultHTTPAddr        = ":8080"
	defaultReadiness       = 2 * time.Second
	defaultShutdownTimeout = 10 * time.Second
)

// Config contains process configuration supplied through environment variables.
// Secrets are never assigned defaults.
type Config struct {
	Environment      string
	HTTPAddr         string
	DatabaseURL      string
	ReadinessTimeout time.Duration
	ShutdownTimeout  time.Duration
	SecureCookies    bool
}

func Load() (Config, error) {
	readiness, err := duration("READINESS_TIMEOUT", defaultReadiness)
	if err != nil {
		return Config{}, err
	}
	shutdown, err := duration("SHUTDOWN_TIMEOUT", defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Environment:      value("APP_ENV", "development"),
		HTTPAddr:         value("HTTP_ADDR", defaultHTTPAddr),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		ReadinessTimeout: readiness,
		ShutdownTimeout:  shutdown,
		SecureCookies:    valueBool("COOKIE_SECURE", value("APP_ENV", "development") == "production"),
	}, nil
}

func valueBool(key string, fallback bool) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	return raw == "1" || raw == "true" || raw == "TRUE"
}

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func duration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return d, nil
}
