package points

import (
	"errors"
	"testing"
	"time"
)

func TestFilterBoundCursor(t *testing.T) {
	binding := cursorBinding("family-a", "child-a", "2026-01-01", "2026-01-31")
	raw := encodeCursor(cursorValue{Kind: "history", Binding: binding, Date: "2026-01-20", ID: "00000000-0000-0000-0000-000000000001"})
	got, err := decodeCursor(raw, "history", binding)
	if err != nil || got.Date != "2026-01-20" {
		t.Fatalf("decode=%+v err=%v", got, err)
	}
	if _, err = decodeCursor(raw, "history", cursorBinding("family-a", "child-b", "2026-01-01", "2026-01-31")); !errors.Is(err, ErrValidation) {
		t.Fatalf("cross-filter cursor err=%v", err)
	}
	if _, err = decodeCursor(raw, "ledger", binding); !errors.Is(err, ErrValidation) {
		t.Fatalf("cross-kind cursor err=%v", err)
	}
	if _, err = decodeCursor("not-base64", "history", binding); !errors.Is(err, ErrValidation) {
		t.Fatalf("malformed cursor err=%v", err)
	}
}

func TestReportPeriodCalendarBoundaries(t *testing.T) {
	anchor := time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		period     string
		week       int
		start, end string
	}{
		{"day", "day", 0, "2028-02-29", "2028-02-29"},
		{"sunday week", "week", 0, "2028-02-27", "2028-03-04"},
		{"monday week", "week", 1, "2028-02-28", "2028-03-05"},
		{"leap month", "month", 0, "2028-02-01", "2028-02-29"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := reportPeriod(tt.period, anchor, tt.week)
			if err != nil || start.Format("2006-01-02") != tt.start || end.Format("2006-01-02") != tt.end {
				t.Fatalf("%s..%s err=%v", start, end, err)
			}
		})
	}
	if _, _, err := reportPeriod("year", anchor, 0); !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid period err=%v", err)
	}
}
