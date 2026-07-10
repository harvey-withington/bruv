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
	"bruv/internal/agent"
	"bruv/internal/index"
	"bruv/internal/model"
	"bruv/internal/repo"
	"fmt"
	"log/slog"
	"time"
)

// Deps is the narrow host contract for AgentService.
type Deps interface {
	Repo() *repo.Repository
	Index() *index.Index
	Publish(topic string, payload any)
}

// Service exposes agent config CRUD and schedule-preview.
type Service struct{ deps Deps }

// New constructs an AgentService.
func New(deps Deps) *Service { return &Service{deps: deps} }

// GetConfig returns the agent file for a card.
func (s *Service) GetConfig(cardID string) (*model.AgentFile, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.GetAgentConfig(cardID)
}

// SaveConfig persists agent config, recomputes NextRunAt, and updates
// the search index's agent-state row. Status is never accepted as
// 'running' from the frontend — only the executor sets that.
func (s *Service) SaveConfig(cardID string, cfg model.AgentConfig) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	if cfg.Status == model.AgentStatusRunning {
		cfg.Status = model.AgentStatusIdle
	}
	if cfg.Enabled && cfg.Schedule != "" {
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
		}
		if cfg.Status == model.AgentStatusDisabled {
			cfg.Status = model.AgentStatusIdle
		}
	} else if !cfg.Enabled {
		cfg.Status = model.AgentStatusDisabled
		cfg.NextRunAt = nil
	}
	if err := r.SaveAgentConfig(cardID, cfg); err != nil {
		return err
	}
	if idx := s.deps.Index(); idx != nil {
		nextRun := ""
		if cfg.NextRunAt != nil {
			nextRun = cfg.NextRunAt.Format(time.RFC3339)
		}
		if err := idx.UpdateAgentIndex(cardID, cfg.Enabled, string(cfg.Status), nextRun); err != nil {
			slog.Warn("update agent index failed", "card", cardID, "err", err)
		}
	}
	return nil
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
