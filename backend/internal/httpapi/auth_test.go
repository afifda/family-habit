package httpapi

import (
	"testing"
	"time"
)

func TestLoginLimiterLocksAfterFiveFailuresAndResets(t *testing.T) {
	now := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	l := newLoginLimiter()
	l.now = func() time.Time { return now }
	for i := 0; i < 5; i++ {
		if !l.Allow("key") {
			t.Fatalf("blocked attempt %d", i+1)
		}
		l.Fail("key")
	}
	if l.Allow("key") {
		t.Fatal("sixth attempt should be blocked")
	}
	now = now.Add(16 * time.Minute)
	if !l.Allow("key") {
		t.Fatal("attempt should be allowed after window")
	}
	l.Fail("key")
	l.Success("key")
	if !l.Allow("key") {
		t.Fatal("successful login should reset limiter")
	}
}

func TestLoginLimiterBoundsAttackerControlledKeys(t *testing.T) {
	l := newLoginLimiter()
	for i := 0; i < maxLimiterKeys; i++ {
		key := string(rune(i + 1))
		if !l.Allow(key) {
			t.Fatalf("key %d unexpectedly denied", i)
		}
		l.Fail(key)
	}
	if l.Allow("one-key-too-many") {
		t.Fatal("limiter admitted more than its bounded key capacity")
	}
	if len(l.attempts) != maxLimiterKeys {
		t.Fatalf("limiter keys=%d want=%d", len(l.attempts), maxLimiterKeys)
	}
}
func TestValidEmail(t *testing.T) {
	if !validEmail("parent@example.com") {
		t.Fatal("valid email rejected")
	}
	for _, v := range []string{"", "not-an-email", "Parent <parent@example.com>"} {
		if validEmail(v) {
			t.Fatalf("invalid email accepted: %q", v)
		}
	}
}

func TestWeekStartMappingAcceptsEveryWeekday(t *testing.T) {
	for i, name := range []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"} {
		got, ok := parseWeekStart(name)
		if !ok || got != int16(i) {
			t.Fatalf("parseWeekStart(%q)=%d,%v want %d,true", name, got, ok, i)
		}
		if back := weekStartName(i); back != name {
			t.Fatalf("weekStartName(%d)=%q want %q", i, back, name)
		}
	}
	if _, ok := parseWeekStart("funday"); ok {
		t.Fatal("invalid weekday accepted")
	}
}
