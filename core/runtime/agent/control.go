package agent

// Exported control surface — the methods a UI needs to inspect and
// steer the agent runtime. Same surface on desktop and headless: the
// Wails-bound App methods on the desktop forward to these directly,
// and the headless cmd/bruv-server binary exposes these via the
// JSON-RPC reflection dispatcher through its own thin forwarders.
//
// Keeping the host-specific bits (tray checkbox toggle on Windows,
// etc.) out of this file lets both hosts share identical semantics
// for "cancel this agent" / "trigger this agent" / "pause the
// scheduler" without either host leaking into the runtime package.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bruv/internal/config"
	"bruv/internal/logging"
	"bruv/internal/model"
)

// CancelAgent cancels a running agent, or resets a stuck status left
// over from a previous crash.
func (rt *Runtime) CancelAgent(cardID string) error {
	if cancelFn, ok := rt.agentCancels.Load(cardID); ok {
		cancelFn.(context.CancelFunc)()
		return nil
	}
	if rt.deps.Repo() != nil {
		af, err := rt.deps.Repo().GetAgentConfig(cardID)
		if err == nil && af.Config.Status == model.AgentStatusRunning {
			af.Config.Status = model.AgentStatusIdle
			_ = rt.deps.Repo().SaveAgentConfig(cardID, af.Config)
			if rt.deps.Index() != nil {
				nextRun := ""
				if af.Config.NextRunAt != nil {
					nextRun = af.Config.NextRunAt.Format(time.RFC3339)
				}
				rt.logIdxErr("UpdateAgentIndex", rt.deps.Index().UpdateAgentIndex(cardID, af.Config.Enabled, string(model.AgentStatusIdle), nextRun))
			}
			rt.deps.Publish("agent:completed", map[string]any{"cardID": cardID, "status": "cancelled"})
			return nil
		}
	}
	return nil
}

// TriggerAgent runs an agent immediately, bypassing the schedule.
// Falls back to a direct goroutine when the scheduler isn't running
// (e.g. a host that hasn't started it yet).
func (rt *Runtime) TriggerAgent(cardID string) error {
	if rt.scheduler != nil {
		return rt.scheduler.TriggerNow(rt.deps.Ctx(), cardID)
	}
	go func() {
		defer logging.Recover("executeAgent-direct")
		_ = rt.executeAgent(rt.deps.Ctx(), cardID)
	}()
	return nil
}

// PauseAllAgents pauses the agent scheduler and publishes a
// scheduler:paused event so UIs can reflect the new state. Tray-icon
// toggles (Windows desktop) stay on the host shell since they're
// presentation.
func (rt *Runtime) PauseAllAgents() error {
	if rt.scheduler != nil {
		rt.scheduler.Pause()
	}
	rt.deps.Publish("scheduler:paused", map[string]any{"paused": true})
	return nil
}

// ResumeAllAgents resumes the agent scheduler.
func (rt *Runtime) ResumeAllAgents() error {
	if rt.scheduler != nil {
		rt.scheduler.Resume()
	}
	rt.deps.Publish("scheduler:paused", map[string]any{"paused": false})
	return nil
}

// GetAgentSchedulerStatus returns the scheduler state: active (i.e.
// started), paused, and how many agents are currently executing.
func (rt *Runtime) GetAgentSchedulerStatus() map[string]any {
	if rt.scheduler == nil {
		return map[string]any{"active": false, "paused": false, "runningCount": 0}
	}
	return map[string]any{
		"active":       true,
		"paused":       rt.scheduler.IsPaused(),
		"runningCount": rt.scheduler.RunningCount(),
	}
}

// GetAllAgents returns a summary of every agent across all cards
// with an agent config. Skips empty shells (cards that once had an
// agent and no longer do).
func (rt *Runtime) GetAllAgents() ([]map[string]any, error) {
	r := rt.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cardsDir := filepath.Join(r.Root, "cards")
	entries, err := os.ReadDir(cardsDir)
	if err != nil {
		return nil, nil
	}
	var agents []map[string]any
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".agent.json") {
			continue
		}
		cardID := strings.TrimSuffix(name, ".agent.json")
		af, err := r.GetAgentConfig(cardID)
		if err != nil {
			continue
		}
		cfg := af.Config
		if !cfg.Enabled && cfg.Goal == "" && cfg.Schedule == "" && len(af.Runs) == 0 {
			continue
		}

		cardTitle := cardID
		if c, err := r.GetCard(cardID); err == nil {
			cardTitle = c.Title
		}

		_, isRunning := rt.agentCancels.Load(cardID)

		entry := map[string]any{
			"card_id":    cardID,
			"card_title": cardTitle,
			"enabled":    af.Config.Enabled,
			"status":     string(af.Config.Status),
			"schedule":   af.Config.Schedule,
			"goal":       af.Config.Goal,
			"is_running": isRunning,
			"one_shot":   af.Config.OneShot,
		}
		if af.Config.StartDate != nil {
			entry["start_date"] = af.Config.StartDate.Format(time.RFC3339)
		}
		if af.Config.EndDate != nil {
			entry["end_date"] = af.Config.EndDate.Format(time.RFC3339)
		}
		if af.Config.LastRunAt != nil {
			entry["last_run_at"] = af.Config.LastRunAt.Format(time.RFC3339)
		}
		if af.Config.NextRunAt != nil {
			entry["next_run_at"] = af.Config.NextRunAt.Format(time.RFC3339)
		}
		if len(af.Runs) > 0 {
			lastRun := af.Runs[0]
			entry["last_run_status"] = lastRun.Status
			entry["last_run_summary"] = lastRun.Summary
			entry["last_run_tokens"] = lastRun.TokensUsed
			if lastRun.Error != "" {
				entry["last_run_error"] = lastRun.Error
			}
		}
		agents = append(agents, entry)
	}
	return agents, nil
}

// GetAllAgentRuns returns a unified run history across all agents,
// most recent first, capped at limit (default 100 when <= 0).
func (rt *Runtime) GetAllAgentRuns(limit int) ([]map[string]any, error) {
	r := rt.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	if limit <= 0 {
		limit = 100
	}
	cardsDir := filepath.Join(r.Root, "cards")
	entries, err := os.ReadDir(cardsDir)
	if err != nil {
		return nil, nil
	}

	type runWithContext struct {
		run       model.AgentRun
		cardTitle string
	}
	var allRuns []runWithContext

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".agent.json") {
			continue
		}
		cardID := strings.TrimSuffix(name, ".agent.json")
		af, err := r.GetAgentConfig(cardID)
		if err != nil || len(af.Runs) == 0 {
			continue
		}
		cardTitle := cardID
		if c, err := r.GetCard(cardID); err == nil {
			cardTitle = c.Title
		}
		for _, run := range af.Runs {
			run.CardID = cardID
			allRuns = append(allRuns, runWithContext{run: run, cardTitle: cardTitle})
		}
	}

	for i := 0; i < len(allRuns)-1; i++ {
		for j := i + 1; j < len(allRuns); j++ {
			if allRuns[j].run.StartedAt.After(allRuns[i].run.StartedAt) {
				allRuns[i], allRuns[j] = allRuns[j], allRuns[i]
			}
		}
	}

	if len(allRuns) > limit {
		allRuns = allRuns[:limit]
	}

	var result []map[string]any
	for _, rc := range allRuns {
		run := rc.run
		entry := map[string]any{
			"id":             run.ID,
			"card_id":        run.CardID,
			"card_title":     rc.cardTitle,
			"started_at":     run.StartedAt.Format(time.RFC3339),
			"status":         run.Status,
			"tokens_used":    run.TokensUsed,
			"tool_count":     len(run.ToolCalls),
			"model_used":     run.ModelUsed,
			"estimated_cost": config.EstimateCost(run.ModelUsed, run.TokensUsed),
		}
		if run.FinishedAt != nil {
			entry["finished_at"] = run.FinishedAt.Format(time.RFC3339)
			entry["duration_secs"] = int(run.FinishedAt.Sub(run.StartedAt).Seconds())
		}
		if run.Summary != "" {
			entry["summary"] = run.Summary
		}
		if run.Error != "" {
			entry["error"] = run.Error
		}
		result = append(result, entry)
	}
	return result, nil
}

// GetAgentAnalytics returns aggregate statistics across all agents:
// totals, success/failure split, token + cost breakdowns, and cost
// windows (today / 7-day) for the dashboard.
func (rt *Runtime) GetAgentAnalytics() (map[string]any, error) {
	r := rt.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cardsDir := filepath.Join(r.Root, "cards")
	entries, err := os.ReadDir(cardsDir)
	if err != nil {
		return nil, nil
	}

	var totalAgents, enabledAgents, totalRuns, successRuns, failedRuns, totalTokens int
	var totalCost, costToday, cost7d float64
	costByModel := make(map[string]float64)

	now := time.Now()
	now7d := now.AddDate(0, 0, -7)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".agent.json") {
			continue
		}
		cardID := strings.TrimSuffix(name, ".agent.json")
		af, err := r.GetAgentConfig(cardID)
		if err != nil {
			continue
		}
		cfg := af.Config
		if !cfg.Enabled && cfg.Goal == "" && cfg.Schedule == "" && len(af.Runs) == 0 {
			continue
		}
		totalAgents++
		if cfg.Enabled {
			enabledAgents++
		}
		for _, run := range af.Runs {
			totalRuns++
			totalTokens += run.TokensUsed
			if run.Status == "success" {
				successRuns++
			} else if run.Status == "failure" {
				failedRuns++
			}
			runCost := config.EstimateCost(run.ModelUsed, run.TokensUsed)
			totalCost += runCost
			if run.ModelUsed != "" {
				costByModel[run.ModelUsed] += runCost
			}
			if run.StartedAt.After(todayStart) {
				costToday += runCost
			}
			if run.StartedAt.After(now7d) {
				cost7d += runCost
			}
		}
	}

	return map[string]any{
		"total_agents":   totalAgents,
		"enabled_agents": enabledAgents,
		"total_runs":     totalRuns,
		"success_runs":   successRuns,
		"failed_runs":    failedRuns,
		"total_tokens":   totalTokens,
		"total_cost":     totalCost,
		"cost_today":     costToday,
		"cost_7d":        cost7d,
		"cost_by_model":  costByModel,
	}, nil
}
