package localdate

import (
	"testing"
	"time"
)

func TestBerlinLocalDateAcrossMidnight(t *testing.T) {
	date, err := In(time.Date(2026, 8, 7, 22, 30, 0, 0, time.UTC), "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	if want := "2026-08-08"; date.Format(time.DateOnly) != want {
		t.Fatalf("date = %s, want %s", date.Format(time.DateOnly), want)
	}
}

func TestBerlinDSTBounds(t *testing.T) {
	tests := []struct {
		date string
		want time.Duration
	}{{"2026-03-29", 23 * time.Hour}, {"2026-10-25", 25 * time.Hour}}
	for _, tt := range tests {
		t.Run(tt.date, func(t *testing.T) {
			date, _ := time.Parse(time.DateOnly, tt.date)
			start, end, err := Bounds(date, "Europe/Berlin")
			if err != nil {
				t.Fatal(err)
			}
			if got := end.Sub(start); got != tt.want {
				t.Fatalf("duration = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestInvalidTimezone(t *testing.T) {
	if _, err := In(time.Now(), "Mars/Olympus"); err == nil {
		t.Fatal("expected error")
	}
}
