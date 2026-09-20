package report

import (
	"testing"
	"time"
)

func TestFreshnessStates(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		name   string
		status string
		age    time.Duration
		want   string
	}{
		{"never scanned", "", 0, FreshNever},
		{"fresh", "complete", time.Hour, FreshCurrent},
		{"partial but fresh is still current", "partial", time.Hour, FreshCurrent},
		{"final fifth is due soon", "complete", 20 * time.Hour, FreshDueSoon},
		{"past cadence is stale", "complete", 25 * time.Hour, FreshStale},
		{"interrupted regardless of age", "interrupted", time.Minute, FreshInterrupted},
		{"unavailable regardless of age", "unavailable", time.Minute, FreshUnavailable},
		{"running", "running", time.Minute, FreshRunning},
	}
	for _, c := range cases {
		if got := Classify(c.status, c.age, day); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}
