package agent

import (
	"fmt"
	"testing"
	"time"
)

func TestNextRunTime(t *testing.T) {
	now := time.Date(2026, 4, 7, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		schedule string
		minDiff  time.Duration
	}{
		{"@hourly", 29 * time.Minute},
		{"@daily", 9 * time.Hour},
		{"30m", 30 * time.Minute},
		{"2h", 2 * time.Hour},
		{"0 9 * * *", 18 * time.Hour},
	}

	for _, tc := range tests {
		next, err := NextRunTime(tc.schedule, now)
		if err != nil {
			t.Errorf("NextRunTime(%q): %v", tc.schedule, err)
			continue
		}
		diff := next.Sub(now)
		fmt.Printf("schedule=%q  now=%s  next=%s  diff=%s\n", tc.schedule, now.Format(time.RFC3339), next.Format(time.RFC3339), diff)
		if diff < tc.minDiff {
			t.Errorf("NextRunTime(%q): diff %s < expected min %s", tc.schedule, diff, tc.minDiff)
		}
		if diff <= 0 {
			t.Errorf("NextRunTime(%q): next is not in the future (diff=%s)", tc.schedule, diff)
		}
	}
}
