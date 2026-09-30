package agent

// The runtime's agent-config writes patch only runtime-owned fields on a
// fresh read, and never re-create a removed agent (pre-release sweep
// 2026-09-29, §2).

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"bruv/internal/model"
)

func saveAgent(t *testing.T, a *Runtime, cardID string, cfg model.AgentConfig) {
	t.Helper()
	if err := a.deps.Repo().SaveAgentConfig(cardID, cfg); err != nil {
		t.Fatalf("SaveAgentConfig: %v", err)
	}
}

func loadAgent(t *testing.T, a *Runtime, cardID string) model.AgentConfig {
	t.Helper()
	af, err := a.deps.Repo().GetAgentConfig(cardID)
	if err != nil {
		t.Fatalf("GetAgentConfig: %v", err)
	}
	return af.Config
}

func TestFinishRunKeepsEditsMadeDuringTheRun(t *testing.T) {
	a, r := testRuntime(t)
	id := testCard(t, r, "Watcher", nil)
	saveAgent(t, a, id, model.AgentConfig{Enabled: true, Goal: "old goal", AllowedTools: []string{}})

	if _, err := a.markRunning(id, time.Now().UTC()); err != nil {
		t.Fatalf("markRunning: %v", err)
	}
	// The user edits and disables the agent mid-run.
	cfg := loadAgent(t, a, id)
	cfg.Goal, cfg.Enabled = "new goal", false
	saveAgent(t, a, id, cfg)

	run := newRun(id)
	run.Status = "success"
	if _, _, err := a.finishRunConfig(id, run, time.Now().UTC()); err != nil {
		t.Fatalf("finishRunConfig: %v", err)
	}
	got := loadAgent(t, a, id)
	if got.Goal != "new goal" || got.Enabled {
		t.Errorf("user edits reverted: goal %q enabled %v", got.Goal, got.Enabled)
	}
	if got.Status != model.AgentStatusDisabled || got.RunStartedAt != nil || got.LastRunAt == nil {
		t.Errorf("runtime fields not settled: %+v", got)
	}
}

func TestFinishRunNeverRecreatesARemovedAgent(t *testing.T) {
	for name, remove := range map[string]func(a *Runtime, id string) error{
		"agent deleted": func(a *Runtime, id string) error { return a.deps.Repo().DeleteAgentFile(id) },
		"card deleted":  func(a *Runtime, id string) error { return a.deps.Repo().DeleteCard(id) },
	} {
		t.Run(name, func(t *testing.T) {
			a, r := testRuntime(t)
			id := testCard(t, r, "Watcher", nil)
			saveAgent(t, a, id, model.AgentConfig{Enabled: true, Goal: "g", AllowedTools: []string{}})
			if _, err := a.markRunning(id, time.Now().UTC()); err != nil {
				t.Fatalf("markRunning: %v", err)
			}
			if err := remove(a, id); err != nil {
				t.Fatal(err)
			}
			run := newRun(id)
			a.finishRun(id, run, time.Now().UTC(), nil, model.AgentConfig{Enabled: true})
			if _, err := os.Stat(filepath.Join(r.Root, "cards", id+".agent.json")); !os.IsNotExist(err) {
				t.Errorf("agent file re-created: %v", err)
			}
		})
	}
}

func TestMarkRunningSkipsDisabledAndOrphanedAgents(t *testing.T) {
	a, r := testRuntime(t)
	id := testCard(t, r, "Watcher", nil)
	saveAgent(t, a, id, model.AgentConfig{Enabled: false, Goal: "g"})
	if _, err := a.markRunning(id, time.Now().UTC()); err != errAgentNotEnabled {
		t.Errorf("disabled agent: err %v, want errAgentNotEnabled", err)
	}

	// An agent file whose card hasn't arrived (yet) is left alone.
	const orphan = "11111111-2222-3333-4444-555555555555"
	saveAgent(t, a, orphan, model.AgentConfig{Enabled: true, Goal: "g"})
	if _, err := a.markRunning(orphan, time.Now().UTC()); !agentGone(err) {
		t.Errorf("orphan agent: err %v, want agent gone", err)
	}
	if got := loadAgent(t, a, orphan); got.Status == model.AgentStatusRunning {
		t.Error("orphan agent marked running")
	}
}

func TestApplyRunOutcome(t *testing.T) {
	finished := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	failed := model.AgentRun{Status: "failure", Error: "boom", StartedAt: finished.Add(-time.Minute)}

	t.Run("a scheduled retry is not overwritten by the schedule", func(t *testing.T) {
		cfg := model.AgentConfig{Enabled: true, Schedule: "1d", MaxRetries: 2}
		applyRunOutcome(&cfg, failed, finished)
		if cfg.NextRunAt == nil || cfg.NextRunAt.Sub(finished) > 2*time.Hour {
			t.Errorf("next run %v, want the retry time", cfg.NextRunAt)
		}
		if cfg.RetryCount != 1 || cfg.Status != model.AgentStatusIdle {
			t.Errorf("retry count %d status %q", cfg.RetryCount, cfg.Status)
		}
	})

	t.Run("a one-shot agent keeps its retries", func(t *testing.T) {
		cfg := model.AgentConfig{Enabled: true, Schedule: "1d", OneShot: true, MaxRetries: 1}
		applyRunOutcome(&cfg, failed, finished)
		if !cfg.Enabled || cfg.NextRunAt == nil {
			t.Errorf("one-shot disabled before its retry: %+v", cfg)
		}
		// The retry fails too: retries are spent, the one-shot is done.
		applyRunOutcome(&cfg, failed, finished.Add(time.Hour))
		if cfg.Enabled || cfg.Status != model.AgentStatusDisabled {
			t.Errorf("spent one-shot still enabled: %+v", cfg)
		}
	})

	t.Run("an unscheduled agent's used-up run time is cleared", func(t *testing.T) {
		pinned := failed.StartedAt.Add(-time.Second)
		cfg := model.AgentConfig{Enabled: true, NextRunAt: &pinned}
		applyRunOutcome(&cfg, model.AgentRun{Status: "success", StartedAt: failed.StartedAt}, finished)
		if cfg.NextRunAt != nil {
			t.Errorf("next run %v kept, would re-run every tick", cfg.NextRunAt)
		}
	})
}
