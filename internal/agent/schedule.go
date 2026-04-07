package agent

import (
	"fmt"
	"regexp"
	"time"

	"github.com/robfig/cron/v3"
)

var simpleIntervalRe = regexp.MustCompile(`^(\d+[smhd])$`)

// NextRunTime computes the next run time for a schedule string.
// Supports cron expressions ("0 9 * * *"), cron shortcuts ("@daily", "@hourly", "@every 30m"),
// and simple intervals ("30m", "2h", "1d").
func NextRunTime(schedule string, from time.Time) (time.Time, error) {
	if schedule == "" {
		return time.Time{}, fmt.Errorf("empty schedule")
	}

	// Simple interval: "30m", "2h", "1d"
	if simpleIntervalRe.MatchString(schedule) {
		s := schedule
		// Convert "d" suffix to hours for time.ParseDuration
		if s[len(s)-1] == 'd' {
			s = s[:len(s)-1] + "h"
			// Multiply the numeric part by 24
			var n int
			fmt.Sscanf(schedule[:len(schedule)-1], "%d", &n)
			s = fmt.Sprintf("%dh", n*24)
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid interval %q: %w", schedule, err)
		}
		if d < time.Minute {
			return time.Time{}, fmt.Errorf("interval %q too short (minimum 1m)", schedule)
		}
		return from.Add(d), nil
	}

	// Cron expression or shortcut (@daily, @hourly, @every 5m, etc.)
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	sched, err := parser.Parse(schedule)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid schedule %q: %w", schedule, err)
	}

	return sched.Next(from), nil
}
