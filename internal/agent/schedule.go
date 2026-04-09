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

// ScheduleOpts configures optional scheduling constraints.
type ScheduleOpts struct {
	StartDate         *time.Time
	EndDate           *time.Time
	ActiveWindowStart string // "HH:MM"
	ActiveWindowEnd   string // "HH:MM"
	OneShot           bool
	LastRunAt         *time.Time
	Timezone          string // IANA timezone name
}

// NextRunTimeWithOpts computes the next run time with optional constraints.
func NextRunTimeWithOpts(schedule string, from time.Time, opts ScheduleOpts) (time.Time, error) {
	// Resolve timezone
	loc := time.Local
	if opts.Timezone != "" {
		var err error
		loc, err = time.LoadLocation(opts.Timezone)
		if err != nil {
			loc = time.Local
		}
	}

	// One-shot: if already ran, no next run
	if opts.OneShot && opts.LastRunAt != nil {
		return time.Time{}, fmt.Errorf("one-shot agent already ran")
	}

	// Clamp from to StartDate if needed
	if opts.StartDate != nil && from.Before(*opts.StartDate) {
		from = *opts.StartDate
	}

	// Get base next time using existing logic
	next, err := NextRunTime(schedule, from)
	if err != nil {
		return time.Time{}, err
	}

	// Check EndDate
	if opts.EndDate != nil && next.After(*opts.EndDate) {
		return time.Time{}, fmt.Errorf("next run after end date")
	}

	// Active window check - advance if outside window
	if opts.ActiveWindowStart != "" && opts.ActiveWindowEnd != "" {
		next = adjustToActiveWindow(next, opts.ActiveWindowStart, opts.ActiveWindowEnd, loc)
		// Re-check end date after adjustment
		if opts.EndDate != nil && next.After(*opts.EndDate) {
			return time.Time{}, fmt.Errorf("next run after end date")
		}
	}

	return next, nil
}

// adjustToActiveWindow moves a time into the active window if outside it.
func adjustToActiveWindow(t time.Time, startHM, endHM string, loc *time.Location) time.Time {
	localT := t.In(loc)
	startH, startM := ParseHM(startHM)
	endH, endM := ParseHM(endHM)

	dayStart := time.Date(localT.Year(), localT.Month(), localT.Day(), startH, startM, 0, 0, loc)
	dayEnd := time.Date(localT.Year(), localT.Month(), localT.Day(), endH, endM, 0, 0, loc)

	if localT.Before(dayStart) {
		return dayStart
	}
	if localT.After(dayEnd) {
		// Move to next day's window start
		return dayStart.AddDate(0, 0, 1)
	}
	return t
}

// ParseHM parses an "HH:MM" string into hour and minute.
func ParseHM(hm string) (int, int) {
	var h, m int
	fmt.Sscanf(hm, "%d:%d", &h, &m)
	return h, m
}
