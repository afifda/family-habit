package completions

import (
	"testing"
	"time"
)

func TestLocalDateAtHouseholdBoundariesAndDST(t *testing.T) {
	tests := []struct{ name, instant, zone, want string }{
		{"Jakarta next day", "2026-08-09T17:00:00Z", "Asia/Jakarta", "2026-08-10"},
		{"Berlin before spring gap", "2026-03-29T00:59:59Z", "Europe/Berlin", "2026-03-29"},
		{"Berlin after spring gap", "2026-03-29T01:00:00Z", "Europe/Berlin", "2026-03-29"},
		{"Berlin first fold side", "2026-10-25T00:30:00Z", "Europe/Berlin", "2026-10-25"},
		{"Berlin second fold side", "2026-10-25T01:30:00Z", "Europe/Berlin", "2026-10-25"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instant, err := time.Parse(time.RFC3339, test.instant)
			if err != nil {
				t.Fatal(err)
			}
			got, err := localDateAt(instant, test.zone)
			if err != nil || got != test.want {
				t.Fatalf("date=%q err=%v want=%q", got, err, test.want)
			}
		})
	}
}
