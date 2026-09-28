package agentsvc

import (
	"strings"
	"testing"
	"time"

	"bruv/internal/model"
)

func mustPatchArgs(t *testing.T, a map[string]any) ConfigPatch {
	t.Helper()
	p, err := PatchFromArgs(a)
	if err != nil {
		t.Fatalf("PatchFromArgs: %v", err)
	}
	return p
}

// A fresh card enabled with a goal + schedule must come out idle with a
// next run — the scheduler skips any agent whose next_run_at is nil.
func TestPatchSchedulesNewAgent(t *testing.T) {
	svc, deps := newTestService(t)
	cfg, err := svc.Patch("c1", mustPatchArgs(t, map[string]any{
		"enabled": true, "goal": "Check prices", "schedule": "@daily",
		"allowed_tools": []any{"web_search", "update_self"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Status != model.AgentStatusIdle || cfg.NextRunAt == nil {
		t.Fatalf("status=%q next=%v, want idle with a next run", cfg.Status, cfg.NextRunAt)
	}
	if !deps.emitted("card:updated") {
		t.Error("patch must publish card:updated")
	}
	af, _ := svc.GetConfig("c1")
	if af.Config.Goal != "Check prices" || len(af.Config.AllowedTools) != 2 {
		t.Errorf("not persisted: %+v", af.Config)
	}
}

// Absent keys are left alone; a changed schedule moves the next run.
func TestPatchIsPartial(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.Patch("c1", mustPatchArgs(t, map[string]any{
		"enabled": true, "goal": "g", "schedule": "1d", "notify_on": []any{"failure"},
	})); err != nil {
		t.Fatal(err)
	}
	cfg, err := svc.Patch("c1", mustPatchArgs(t, map[string]any{"schedule": "30m"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Goal != "g" || !cfg.Enabled || len(cfg.NotifyOn) != 1 {
		t.Errorf("untouched fields changed: %+v", cfg)
	}
	if cfg.NextRunAt == nil || time.Until(*cfg.NextRunAt) > time.Hour {
		t.Errorf("next run %v not recomputed from the 30m schedule", cfg.NextRunAt)
	}
}

func TestPatchDisableClearsNextRun(t *testing.T) {
	svc, _ := newTestService(t)
	_, _ = svc.Patch("c1", mustPatchArgs(t, map[string]any{"enabled": true, "goal": "g", "schedule": "@hourly"}))
	cfg, err := svc.Patch("c1", mustPatchArgs(t, map[string]any{"enabled": false}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Status != model.AgentStatusDisabled || cfg.NextRunAt != nil {
		t.Errorf("status=%q next=%v, want disabled with no next run", cfg.Status, cfg.NextRunAt)
	}
}

func TestPatchNextRunOverride(t *testing.T) {
	svc, _ := newTestService(t)
	at := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)
	cfg, err := svc.Patch("c1", mustPatchArgs(t, map[string]any{
		"enabled": true, "goal": "g", "schedule": "@daily", "next_run_at": at.Format(time.RFC3339),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NextRunAt == nil || !cfg.NextRunAt.Equal(at) {
		t.Errorf("next run = %v, want pinned %v", cfg.NextRunAt, at)
	}
}

// Invalid input is rejected whole, with every problem named, and
// nothing is written.
func TestPatchRejectsInvalid(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.Patch("c1", mustPatchArgs(t, map[string]any{
		"enabled": true, "schedule": "every tuesday", "timezone": "Mars/Base",
		"active_window_start": "25:00", "notify_on": []any{"always"}, "max_retries": float64(99),
	}))
	if err == nil {
		t.Fatal("expected a validation error")
	}
	for _, want := range []string{"goal", "schedule", "timezone", "active window", "notify_on", "max_retries"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
	af, _ := svc.GetConfig("c1")
	if af.Config.Enabled {
		t.Error("invalid patch was persisted")
	}
}

func TestPatchFromArgsTypeErrors(t *testing.T) {
	_, err := PatchFromArgs(map[string]any{"enabled": "yes", "max_retries": 1.5, "start_date": "next week"})
	if err == nil {
		t.Fatal("expected type errors")
	}
	for _, want := range []string{"enabled", "max_retries", "start_date"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

// A zone-less datetime-local value ("2026-09-27T14:30") is what an LLM
// or a date picker naturally sends; it must parse as local wall-clock
// time rather than fail, matching shared/dateTimeInput on the UI side.
func TestPatchFromArgsAcceptsZonelessDateTime(t *testing.T) {
	p := mustPatchArgs(t, map[string]any{"start_date": "2026-09-27T14:30", "end_date": "2026-10-01"})
	want := time.Date(2026, 9, 27, 14, 30, 0, 0, time.Local)
	if p.StartDate == nil || !p.StartDate.Equal(want) {
		t.Errorf("start_date = %v, want %v", p.StartDate, want)
	}
	if p.EndDate == nil || !p.EndDate.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)) {
		t.Errorf("end_date = %v, want local midnight 2026-10-01", p.EndDate)
	}
}

func TestPatchFromArgsClearsDates(t *testing.T) {
	svc, _ := newTestService(t)
	_, _ = svc.Patch("c1", mustPatchArgs(t, map[string]any{"end_date": "2099-01-01"}))
	cfg, err := svc.Patch("c1", mustPatchArgs(t, map[string]any{"end_date": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EndDate != nil {
		t.Errorf("end date = %v, want cleared", cfg.EndDate)
	}
}
