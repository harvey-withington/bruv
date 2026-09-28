package agentsvc

// Partial agent-config updates for LLM callers (the MCP server, card and
// project chat). The UI saves a whole AgentConfig through SaveConfig;
// tool callers only name the fields they want to change, so they go
// through Patch, which validates the merged result before persisting it.

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"bruv/internal/agent"
	"bruv/internal/model"
)

// Accepted values for the notification settings. "in-app" is always
// implicit; it is tolerated in NotifyChannel but never required.
var (
	NotifyTriggers = []string{"success", "failure"}
	NotifyChannels = []string{"system", "email", "webhook"}
)

// MaxRetriesLimit mirrors the Agent tab's input bound.
const MaxRetriesLimit = 10

var hhmmRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// ConfigPatch is a partial AgentConfig update: nil fields are left
// unchanged. For StartDate / EndDate a zero time clears the bound.
// Runtime bookkeeping (status, last/next run, retry count, cost spent)
// is deliberately absent — the executor owns it.
type ConfigPatch struct {
	Enabled           *bool
	Goal              *string
	Schedule          *string
	AllowedTools      *[]string
	NotifyOn          *[]string
	NotifyChannels    *[]string
	LLMAccountID      *string
	LLMModel          *string
	MaxTokensBudget   *int
	MinIntervalMins   *int
	MaxRetries        *int
	RetryBackoffMins  *int
	CostBudgetUSD     *float64
	StartDate         *time.Time
	EndDate           *time.Time
	ActiveWindowStart *string
	ActiveWindowEnd   *string
	OneShot           *bool
	Timezone          *string
	// NextRunAt pins the next run to an exact time instead of the one
	// derived from the schedule. Only honoured while the agent is enabled.
	NextRunAt *time.Time
}

// Apply merges the patch into cfg in place.
func (p ConfigPatch) Apply(cfg *model.AgentConfig) {
	setIf(&cfg.Enabled, p.Enabled)
	setIf(&cfg.Goal, p.Goal)
	setIf(&cfg.Schedule, p.Schedule)
	setIf(&cfg.AllowedTools, p.AllowedTools)
	setIf(&cfg.NotifyOn, p.NotifyOn)
	if p.NotifyChannels != nil {
		cfg.NotifyChannel = strings.Join(*p.NotifyChannels, ",")
	}
	setIf(&cfg.LLMAccountID, p.LLMAccountID)
	setIf(&cfg.LLMModel, p.LLMModel)
	setIf(&cfg.MaxTokensBudget, p.MaxTokensBudget)
	setIf(&cfg.MinIntervalMins, p.MinIntervalMins)
	setIf(&cfg.MaxRetries, p.MaxRetries)
	setIf(&cfg.RetryBackoffMins, p.RetryBackoffMins)
	setIf(&cfg.CostBudgetUSD, p.CostBudgetUSD)
	setTimeIf(&cfg.StartDate, p.StartDate)
	setTimeIf(&cfg.EndDate, p.EndDate)
	setIf(&cfg.ActiveWindowStart, p.ActiveWindowStart)
	setIf(&cfg.ActiveWindowEnd, p.ActiveWindowEnd)
	setIf(&cfg.OneShot, p.OneShot)
	setIf(&cfg.Timezone, p.Timezone)
}

// IsEmpty reports whether the patch changes nothing.
func (p ConfigPatch) IsEmpty() bool { return p == ConfigPatch{} }

func setIf[T any](dst *T, v *T) {
	if v != nil {
		*dst = *v
	}
}

func setTimeIf(dst **time.Time, v *time.Time) {
	switch {
	case v == nil:
	case v.IsZero():
		*dst = nil
	default:
		t := *v
		*dst = &t
	}
}

// Summary is a one-line description of a config for tool results.
func Summary(cfg model.AgentConfig) string {
	state := "disabled"
	if cfg.Enabled {
		state = "enabled"
	}
	schedule := cfg.Schedule
	if schedule == "" {
		schedule = "none (runs only when triggered)"
	}
	tools := "all built-in"
	if len(cfg.AllowedTools) > 0 {
		tools = strings.Join(cfg.AllowedTools, ", ")
	}
	out := fmt.Sprintf("Agent %s — schedule: %s, tools: %s", state, schedule, tools)
	if cfg.NextRunAt != nil {
		out += ", next run: " + cfg.NextRunAt.Format(time.RFC3339)
	}
	return out
}

// Validate checks a config for values the scheduler would silently
// ignore or misread. All problems are reported together so an LLM
// caller can fix them in one retry.
func Validate(cfg model.AgentConfig) error {
	var errs []error
	if cfg.Enabled && strings.TrimSpace(cfg.Goal) == "" {
		errs = append(errs, errors.New("an enabled agent needs a goal"))
	}
	if cfg.Schedule != "" {
		if _, err := agent.NextRunTime(cfg.Schedule, time.Now()); err != nil {
			errs = append(errs, fmt.Errorf("schedule: %w", err))
		}
	}
	if cfg.Timezone != "" {
		if _, err := time.LoadLocation(cfg.Timezone); err != nil {
			errs = append(errs, fmt.Errorf("timezone %q is not an IANA zone name", cfg.Timezone))
		}
	}
	if (cfg.ActiveWindowStart == "") != (cfg.ActiveWindowEnd == "") {
		errs = append(errs, errors.New("active window needs both a start and an end (or neither)"))
	}
	for _, hm := range []string{cfg.ActiveWindowStart, cfg.ActiveWindowEnd} {
		if hm != "" && !hhmmRe.MatchString(hm) {
			errs = append(errs, fmt.Errorf("active window time %q is not HH:MM (24-hour)", hm))
		}
	}
	if cfg.StartDate != nil && cfg.EndDate != nil && !cfg.EndDate.After(*cfg.StartDate) {
		errs = append(errs, errors.New("end date must be after start date"))
	}
	for _, v := range cfg.NotifyOn {
		if !slices.Contains(NotifyTriggers, v) {
			errs = append(errs, fmt.Errorf("notify_on %q is not one of %v", v, NotifyTriggers))
		}
	}
	for _, v := range strings.Split(cfg.NotifyChannel, ",") {
		if v = strings.TrimSpace(v); v != "" && v != "in-app" && !slices.Contains(NotifyChannels, v) {
			errs = append(errs, fmt.Errorf("notify channel %q is not one of %v", v, NotifyChannels))
		}
	}
	for name, n := range map[string]int{
		"max_tokens_budget":     cfg.MaxTokensBudget,
		"min_interval_minutes":  cfg.MinIntervalMins,
		"max_retries":           cfg.MaxRetries,
		"retry_backoff_minutes": cfg.RetryBackoffMins,
	} {
		if n < 0 {
			errs = append(errs, fmt.Errorf("%s must not be negative", name))
		}
	}
	if cfg.MaxRetries > MaxRetriesLimit {
		errs = append(errs, fmt.Errorf("max_retries must be at most %d", MaxRetriesLimit))
	}
	if cfg.CostBudgetUSD < 0 {
		errs = append(errs, errors.New("cost_budget_usd must not be negative"))
	}
	return errors.Join(errs...)
}

// Patch applies a partial update to a card's agent, validates the merged
// config, and saves it through SaveConfig (which recomputes the next run
// and the index row). Publishes card:updated so an open Agent tab
// re-fetches. Returns the saved config.
func (s *Service) Patch(cardID string, p ConfigPatch) (*model.AgentConfig, error) {
	af, err := s.GetConfig(cardID)
	if err != nil {
		return nil, err
	}
	cfg := af.Config
	p.Apply(&cfg)
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	normalizeForSave(&cfg)
	if p.NextRunAt != nil && !p.NextRunAt.IsZero() && cfg.Enabled {
		t := *p.NextRunAt
		cfg.NextRunAt = &t
	}
	if err := s.persist(cardID, cfg); err != nil {
		return nil, err
	}
	s.deps.Publish("card:updated", map[string]any{"cardID": cardID})
	return &cfg, nil
}
