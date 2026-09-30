// Package agentsvc is the AgentService — config CRUD, run-history
// reads, and the schedule-preview helper. Named agentsvc to avoid
// colliding with internal/agent.
//
// The agent runtime (scheduler loop, executeAgent, tool dispatch,
// MCP tool bridging, due-date scanner) stays on App for now. It's
// ~4000 lines entangled with the LLM chat loop; extracting it into a
// service is a future multi-file pass under an LLM-runtime package
// that groups chat + agent + tool execution.
package agentsvc

import (
	llmsvc "bruv/core/services/llm"
	"bruv/internal/agent"
	"bruv/internal/index"
	"bruv/internal/llm"
	"bruv/internal/mcp"
	"bruv/internal/model"
	"bruv/internal/repo"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Deps is the narrow host contract for AgentService.
type Deps interface {
	Repo() *repo.Repository
	Index() *index.Index
	Publish(topic string, payload any)
	// LLM and MCPRegistry feed Describe/Configure's option lists (models,
	// routers, MCP tool ids). Either may be nil; the lists are then empty.
	LLM() *llmsvc.Service
	MCPRegistry() *mcp.Registry
	// NativeToolDefs is BRUV's native board tool set (core/boardtools),
	// which agents may be granted alongside their built-ins.
	NativeToolDefs() []llm.ToolDef
}

// Service exposes agent config CRUD and schedule-preview.
type Service struct{ deps Deps }

// New constructs an AgentService.
func New(deps Deps) *Service { return &Service{deps: deps} }

// GetConfig returns the agent file for a card. allowed_tools comes back
// with current ids, so a saved legacy name (read_card) reads as its
// native replacement (get_card) on every surface, including the UI.
func (s *Service) GetConfig(cardID string) (*model.AgentFile, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	af, err := r.GetAgentConfig(cardID)
	if err != nil {
		return nil, err
	}
	for i, id := range af.Config.AllowedTools {
		af.Config.AllowedTools[i] = llm.CanonicalAgentToolID(id)
	}
	return af, nil
}

// SaveConfig saves the Agent tab's whole config, recomputes NextRunAt,
// and updates the search index's agent-state row. The runtime-owned
// fields (status, run timestamps, retry count, cost spent) come from the
// config on disk, not from the caller: the tab holds a copy loaded before
// any runs since, so taking its values would re-fire one-shot agents,
// reset the cost budget and flip a running agent to idle. The one
// exception is a cost of 0 over a non-zero spend — the tab's "reset cost".
func (s *Service) SaveConfig(cardID string, cfg model.AgentConfig) error {
	_, err := s.write(cardID, func(disk *model.AgentConfig) error {
		*disk = mergeUIConfig(*disk, cfg)
		normalizeForSave(disk)
		return nil
	})
	return err
}

// mergeUIConfig is incoming with disk's runtime-owned fields.
func mergeUIConfig(disk, incoming model.AgentConfig) model.AgentConfig {
	out := incoming
	out.Status = disk.Status
	out.LastRunAt = disk.LastRunAt
	out.RunStartedAt = disk.RunStartedAt
	out.RetryCount = disk.RetryCount
	if incoming.CostSpentUSD != 0 {
		out.CostSpentUSD = disk.CostSpentUSD
	}
	return out
}

// write is every config save: apply edits the current config on disk
// under the agent lock (runtime writes made meanwhile are kept), then the
// index row is updated. With no agent file yet, apply edits a fresh
// config, which is created.
func (s *Service) write(cardID string, apply func(cfg *model.AgentConfig) error) (*model.AgentConfig, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	saved, err := r.UpdateAgentConfig(cardID, apply)
	if errors.Is(err, repo.ErrAgentNotFound) {
		cfg := model.AgentConfig{Status: model.AgentStatusDisabled, AllowedTools: []string{}}
		if err := apply(&cfg); err != nil {
			return nil, err
		}
		if err := r.SaveAgentConfig(cardID, cfg); err != nil {
			return nil, err
		}
		saved, err = &cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if idx := s.deps.Index(); idx != nil {
		nextRun := ""
		if saved.NextRunAt != nil {
			nextRun = saved.NextRunAt.Format(time.RFC3339)
		}
		if err := idx.UpdateAgentIndex(cardID, saved.Enabled, string(saved.Status), nextRun); err != nil {
			slog.Warn("update agent index failed", "card", cardID, "err", err)
		}
	}
	return saved, nil
}

// normalizeForSave derives status and NextRunAt from the enabled flag
// and schedule, so every save path schedules an agent the same way. A
// running agent stays running — only the executor ends a run.
func normalizeForSave(cfg *model.AgentConfig) {
	running := cfg.Status == model.AgentStatusRunning
	if !cfg.Enabled {
		if !running {
			cfg.Status = model.AgentStatusDisabled
		}
		cfg.NextRunAt = nil
		return
	}
	if cfg.Status == model.AgentStatusDisabled {
		cfg.Status = model.AgentStatusIdle
	}
	if cfg.Schedule == "" {
		// Enabled but unscheduled: runs only when triggered.
		cfg.NextRunAt = nil
		return
	}
	opts := agent.ScheduleOpts{
		StartDate:         cfg.StartDate,
		EndDate:           cfg.EndDate,
		ActiveWindowStart: cfg.ActiveWindowStart,
		ActiveWindowEnd:   cfg.ActiveWindowEnd,
		OneShot:           cfg.OneShot,
		LastRunAt:         cfg.LastRunAt,
		Timezone:          cfg.Timezone,
	}
	if next, err := agent.NextRunTimeWithOpts(cfg.Schedule, time.Now(), opts); err == nil {
		cfg.NextRunAt = &next
	} else {
		// Nothing left to run (one-shot already fired, past end date).
		cfg.NextRunAt = nil
	}
}

// ValidateSchedulePreview returns the next N run times for a schedule.
func (s *Service) ValidateSchedulePreview(schedule, startDate, endDate, timezone string, count int) ([]string, error) {
	if schedule == "" {
		return nil, fmt.Errorf("empty schedule")
	}
	if count <= 0 || count > 10 {
		count = 5
	}

	var sd, ed *time.Time
	if startDate != "" {
		if t, err := time.Parse(time.RFC3339, startDate); err == nil {
			sd = &t
		}
	}
	if endDate != "" {
		if t, err := time.Parse(time.RFC3339, endDate); err == nil {
			ed = &t
		}
	}

	opts := agent.ScheduleOpts{StartDate: sd, EndDate: ed, Timezone: timezone}

	var result []string
	from := time.Now()
	for i := 0; i < count; i++ {
		next, err := agent.NextRunTimeWithOpts(schedule, from, opts)
		if err != nil {
			break
		}
		result = append(result, next.Format(time.RFC3339))
		from = next.Add(time.Second)
	}
	return result, nil
}

// GetRuns returns the run history for a card's agent.
func (s *Service) GetRuns(cardID string) ([]model.AgentRun, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.GetAgentRuns(cardID)
}

// ClearRuns drops the run history for a card's agent.
func (s *Service) ClearRuns(cardID string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	return r.ClearAgentRuns(cardID)
}

// Delete removes a card's agent entirely, turning it back into a
// plain card. repo.DeleteAgentFile covers both storage layouts: the
// in-repo config file (which in legacy merged mode also embeds the
// run history) and the split server-side runs file. The search
// index's agent columns are cleared so dashboards and due-agent
// queries stop seeing it, and card:updated is published — the same
// event agent-config mutations emit — so open card UIs refresh.
func (s *Service) Delete(cardID string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	if err := r.DeleteAgentFile(cardID); err != nil {
		return err
	}
	if idx := s.deps.Index(); idx != nil {
		if err := idx.UpdateAgentIndex(cardID, false, "", ""); err != nil {
			slog.Warn("clear agent index failed", "card", cardID, "err", err)
		}
	}
	s.deps.Publish("card:updated", map[string]any{"cardID": cardID})
	return nil
}
