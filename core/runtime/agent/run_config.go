package agent

// The runtime's writes to an agent's config. A run lasts minutes, and
// meanwhile the user can edit, disable or delete the agent (or its card),
// so every write here re-reads the config under the agent lock and
// patches only the fields the runtime owns: status, run timestamps, the
// next run, the retry count and the cost spent — plus Enabled when the
// runtime itself switches the agent off (budget spent, one-shot done).
// Nothing here ever re-creates an agent that is gone.

import (
	"errors"
	"time"

	agentlib "bruv/internal/agent"
	"bruv/internal/config"
	"bruv/internal/model"
	"bruv/internal/repo"
)

var (
	// errAgentCardGone aborts a config write whose card was deleted (or
	// hasn't synced yet): the agent file is left alone, never re-created.
	errAgentCardGone = errors.New("agent card not found")
	// errAgentNotEnabled aborts the run-start write for a disabled agent.
	errAgentNotEnabled = errors.New("agent is disabled")
)

// agentGone reports whether a config write failed because the agent or
// its card no longer exists.
func agentGone(err error) bool {
	return errors.Is(err, repo.ErrAgentNotFound) || errors.Is(err, errAgentCardGone)
}

// markRunning flips an enabled agent to running on a fresh read and
// returns the config the run uses. errAgentNotEnabled when the agent is
// disabled; an agentGone error when it or its card is missing.
func (rt *Runtime) markRunning(cardID string, at time.Time) (*model.AgentConfig, error) {
	r := rt.deps.Repo()
	return r.UpdateAgentConfig(cardID, func(cfg *model.AgentConfig) error {
		if !cfg.Enabled {
			return errAgentNotEnabled
		}
		if !r.CardExists(cardID) {
			return errAgentCardGone
		}
		cfg.Status = model.AgentStatusRunning
		cfg.RunStartedAt = &at
		return nil
	})
}

// finishRunConfig records a finished run on the agent's current config.
// budgetExceeded reports that this run's cost switched the agent off.
func (rt *Runtime) finishRunConfig(cardID string, run model.AgentRun, finishedAt time.Time) (cfg *model.AgentConfig, budgetExceeded bool, err error) {
	r := rt.deps.Repo()
	cfg, err = r.UpdateAgentConfig(cardID, func(cfg *model.AgentConfig) error {
		if !r.CardExists(cardID) {
			return errAgentCardGone
		}
		budgetExceeded = applyRunOutcome(cfg, run, finishedAt)
		return nil
	})
	return cfg, budgetExceeded, err
}

// applyRunOutcome patches the runtime-owned fields of cfg for a finished
// run: status, run timestamps, retry count, next run and cost spent.
// Everything else — including edits the user saved during the run — is
// left as found. Reports whether the cost budget switched the agent off.
func applyRunOutcome(cfg *model.AgentConfig, run model.AgentRun, finishedAt time.Time) (budgetExceeded bool) {
	failed := run.Status == "failure"
	cfg.Status = model.AgentStatusIdle
	if failed {
		cfg.Status = model.AgentStatusFailed
	}

	// Retry a failed run (the delay policy lives in internal/agent/retry.go).
	// A scheduled retry owns NextRunAt: the schedule must not overwrite it,
	// and a one-shot agent must not be switched off while it has retries.
	retrying := false
	if failed && cfg.MaxRetries > 0 {
		cfg.RetryCount++
		if cfg.RetryCount <= cfg.MaxRetries {
			retryAt := finishedAt.Add(agentlib.RetryDelay(run.Error, cfg.RetryBackoffMins, cfg.RetryCount))
			cfg.NextRunAt = &retryAt
			cfg.Status = model.AgentStatusIdle // allow re-scheduling
			retrying = true
		}
	} else if !failed {
		cfg.RetryCount = 0
	}

	cfg.RunStartedAt = nil // clear stuck-detection timestamp
	cfg.LastRunAt = &finishedAt

	switch {
	case retrying:
	case cfg.Schedule != "":
		opts := agentlib.ScheduleOpts{
			StartDate:         cfg.StartDate,
			EndDate:           cfg.EndDate,
			ActiveWindowStart: cfg.ActiveWindowStart,
			ActiveWindowEnd:   cfg.ActiveWindowEnd,
			OneShot:           cfg.OneShot,
			LastRunAt:         cfg.LastRunAt,
			Timezone:          cfg.Timezone,
		}
		if next, err := agentlib.NextRunTimeWithOpts(cfg.Schedule, finishedAt, opts); err == nil {
			cfg.NextRunAt = &next
		} else {
			// One-shot completed or past end date.
			cfg.NextRunAt = nil
			if cfg.OneShot {
				cfg.Enabled = false
			}
		}
	case cfg.NextRunAt != nil && !cfg.NextRunAt.After(run.StartedAt):
		// Unscheduled agent run from a pinned (or retry) time: that time
		// is used up. A later one pinned during the run is kept.
		cfg.NextRunAt = nil
	}

	// Cost tracking + budget ("0 means unlimited" lives in BudgetExceeded).
	if run.TokensUsed > 0 {
		cfg.CostSpentUSD += config.EstimateCost(run.ModelUsed, run.TokensUsed)
		if agentlib.BudgetExceeded(cfg.CostSpentUSD, cfg.CostBudgetUSD) {
			cfg.Enabled = false
			budgetExceeded = true
		}
	}

	// Disabled — by the runtime above, or by the user during the run.
	if !cfg.Enabled {
		cfg.Status = model.AgentStatusDisabled
		cfg.NextRunAt = nil
	}
	return budgetExceeded
}
