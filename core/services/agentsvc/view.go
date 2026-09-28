package agentsvc

// Describe and Configure are the ONE read/write surface every LLM tool
// uses for a card's agent — MCP get_card_agent / configure_card_agent
// and the card- and project-chat get_agent / configure_agent. The tool
// schemas come from one place too (llm.AgentConfigParams), so a field or
// check added here reaches every surface at once.

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/mcp"
	"bruv/internal/model"
)

// recentRuns caps the run history Describe returns.
const recentRuns = 5

// ToolOption is one id the agent's allowed_tools accepts.
type ToolOption struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Ready       bool   `json:"ready"`
}

// LLMOption is one value the agent's `llm` field accepts: a registry
// model or a router, as a config.ModelRef.
type LLMOption struct {
	Ref           string `json:"ref"`
	Label         string `json:"label"`
	Kind          string `json:"kind"` // "model" | "router"
	Tier          string `json:"tier,omitempty"`
	SupportsTools *bool  `json:"supports_tools,omitempty"`
}

// Options lists the values Configure accepts, so a caller can build a
// working config without guessing ids.
type Options struct {
	Tools          []ToolOption `json:"tools"`
	LLM            []LLMOption  `json:"llm"`
	LLMConfigured  bool         `json:"llm_configured"`
	NotifyOn       []string     `json:"notify_on"`
	NotifyChannels []string     `json:"notify_channel"`
	ScheduleSyntax string       `json:"schedule_syntax"`
}

// RunSummary is a compact run-history entry.
type RunSummary struct {
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Status     string     `json:"status"`
	Summary    string     `json:"summary,omitempty"`
	Error      string     `json:"error,omitempty"`
	ToolCalls  int        `json:"tool_calls"`
	TokensUsed int        `json:"tokens_used,omitempty"`
	ModelUsed  string     `json:"model_used,omitempty"`
}

// View is everything a caller needs to read, then change, an agent.
type View struct {
	CardID     string            `json:"card_id"`
	CardTitle  string            `json:"card_title"`
	CardType   string            `json:"card_type"`
	Config     model.AgentConfig `json:"config"`
	RecentRuns []RunSummary      `json:"recent_runs"`
	Options    Options           `json:"options"`
}

// ConfigureResult is the saved config plus anything that would stop it
// running on its own.
type ConfigureResult struct {
	CardID   string             `json:"card_id"`
	Summary  string             `json:"summary"`
	Config   *model.AgentConfig `json:"config"`
	Warnings []string           `json:"warnings,omitempty"`
}

// Describe returns the card's agent config (goal in full), its recent
// runs and the accepted option values.
func (s *Service) Describe(cardID string) (*View, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, errors.New("no repository open")
	}
	card, err := r.GetCard(cardID)
	if err != nil {
		return nil, err
	}
	af, err := s.GetConfig(cardID)
	if err != nil {
		return nil, err
	}
	runs, err := s.GetRuns(cardID)
	if err != nil {
		return nil, err
	}
	recent := make([]RunSummary, 0, recentRuns)
	for _, run := range runs {
		if len(recent) == recentRuns {
			break
		}
		recent = append(recent, RunSummary{
			StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, Status: run.Status, Summary: run.Summary,
			Error: run.Error, ToolCalls: len(run.ToolCalls), TokensUsed: run.TokensUsed, ModelUsed: run.ModelUsed,
		})
	}
	opts, err := s.options()
	if err != nil {
		return nil, err
	}
	return &View{
		CardID: cardID, CardTitle: card.Title, CardType: card.Type,
		Config: af.Config, RecentRuns: recent, Options: opts,
	}, nil
}

// Configure applies tool-call arguments (keys = AgentConfig JSON tags;
// absent keys unchanged) after checking tool ids and the model against
// the live options, and reports warnings for a config that won't run.
func (s *Service) Configure(cardID string, args map[string]any) (*ConfigureResult, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, errors.New("no repository open")
	}
	if _, err := r.GetCard(cardID); err != nil {
		return nil, err
	}
	patch, err := PatchFromArgs(args)
	if err != nil {
		return nil, err
	}
	if patch.IsEmpty() {
		return nil, errors.New("no agent settings given")
	}
	opts, err := s.options()
	if err != nil {
		return nil, err
	}
	var warnings []string
	if patch.AllowedTools != nil {
		// Store current ids: a legacy name (read_card) saves as its
		// native replacement (get_card).
		for i, id := range *patch.AllowedTools {
			(*patch.AllowedTools)[i] = llm.CanonicalAgentToolID(id)
		}
		w, err := checkAllowedTools(*patch.AllowedTools, opts.Tools, s.configuredMCPServers())
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, w...)
	}
	if patch.LLM != nil && *patch.LLM != "" {
		i := slices.IndexFunc(opts.LLM, func(o LLMOption) bool { return o.Ref == *patch.LLM })
		if i < 0 {
			return nil, fmt.Errorf("llm %q is not a configured model or router; read the agent's options.llm for valid refs", *patch.LLM)
		}
		if st := opts.LLM[i].SupportsTools; st != nil && !*st {
			warnings = append(warnings, fmt.Sprintf("model %q doesn't support tool calls, so the agent can't use any of its tools", opts.LLM[i].Label))
		}
	}
	cfg, err := s.Patch(cardID, patch)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, readinessWarnings(*cfg, opts.LLMConfigured)...)
	return &ConfigureResult{CardID: cardID, Summary: Summary(*cfg), Config: cfg, Warnings: warnings}, nil
}

func (s *Service) options() (Options, error) {
	opts := Options{
		NotifyOn:       NotifyTriggers,
		NotifyChannels: NotifyChannels,
		ScheduleSyntax: llm.AgentScheduleSyntax,
		LLM:            []LLMOption{},
	}
	for _, t := range append(llm.AgentBuiltinTools(), s.deps.NativeToolDefs()...) {
		if !llm.AgentMayUse(t.Name) {
			continue // agents never start or reconfigure agents
		}
		opts.Tools = append(opts.Tools, ToolOption{ID: t.Name, Description: firstSentence(t.Description), Ready: true})
	}
	if reg := s.deps.MCPRegistry(); reg != nil {
		for _, t := range reg.Tools() { // ready servers only
			desc := t.Tool.Description
			if desc == "" {
				desc = t.Tool.Title
			}
			opts.Tools = append(opts.Tools, ToolOption{ID: t.NamespaceID, Description: firstSentence(desc), Ready: true})
		}
	}
	if svc := s.deps.LLM(); svc != nil {
		opts.LLMConfigured = svc.IsConfigured()
		// The key-free registry view: API keys never leave the machine.
		reg, err := svc.GetRegistryView()
		if err != nil {
			return opts, fmt.Errorf("load model registry: %w", err)
		}
		for _, m := range reg.Routing.Models {
			if !m.Enabled {
				continue
			}
			st := m.SupportsTools
			opts.LLM = append(opts.LLM, LLMOption{
				Ref: string(config.ModelRefTo(config.RefModel, m.ID)), Label: m.DisplayLabel(),
				Kind: string(config.RefModel), Tier: string(m.Tier), SupportsTools: &st,
			})
		}
		for _, rt := range reg.Routing.Routers {
			opts.LLM = append(opts.LLM, LLMOption{
				Ref: string(config.ModelRefTo(config.RefRouter, rt.ID)), Label: rt.Name, Kind: string(config.RefRouter),
			})
		}
	}
	return opts, nil
}

// configuredMCPServers names every registered MCP server, ready or not.
func (s *Service) configuredMCPServers() map[string]bool {
	out := map[string]bool{}
	if reg := s.deps.MCPRegistry(); reg != nil {
		for _, h := range reg.Health() {
			out[h.Name] = true
		}
	}
	return out
}

// checkAllowedTools rejects ids that name nothing. An MCP id whose
// server is registered but not currently exposing it is accepted with a
// warning — the server may simply be restarting.
func checkAllowedTools(ids []string, known []ToolOption, servers map[string]bool) ([]string, error) {
	var unknown, warnings []string
	for _, id := range ids {
		if !llm.AgentMayUse(id) {
			return nil, fmt.Errorf("agents can't be granted %s: an agent must never start or reconfigure agents", id)
		}
		if slices.ContainsFunc(known, func(t ToolOption) bool { return t.ID == id }) {
			continue
		}
		if server, _ := mcp.SplitNamespacedTool(id); server != "" && servers[server] {
			warnings = append(warnings, fmt.Sprintf("tool %q is not currently offered by its MCP server; the agent can't use it until it is", id))
			continue
		}
		unknown = append(unknown, id)
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown allowed_tools %v; read the agent's options.tools for valid ids", unknown)
	}
	return warnings, nil
}

// readinessWarnings flags a saved config that won't run on its own, so
// the caller learns it now rather than from a silent scheduler.
func readinessWarnings(cfg model.AgentConfig, llmConfigured bool) []string {
	if !cfg.Enabled {
		return []string{"agent is disabled; set enabled=true for it to run on its schedule"}
	}
	var w []string
	if len(cfg.AllowedTools) == 0 {
		w = append(w, "no tools are allowed, so the agent can only reply in text; grant the tools its goal needs in allowed_tools")
	}
	if !llmConfigured {
		w = append(w, "no LLM account is configured in BRUV, so runs will fail until the user adds one in Settings")
	}
	switch {
	case cfg.Schedule == "":
		w = append(w, "no schedule: the agent runs only when triggered")
	case cfg.NextRunAt == nil:
		w = append(w, "schedule has no upcoming run (one-shot already ran, or the end date has passed)")
	}
	return w
}

// firstSentence trims a tool description for the options list. It ends
// at ". " followed by an upper-case letter, so "e.g. one" doesn't cut.
func firstSentence(s string) string {
	for i := 0; i+2 < len(s); i++ {
		if s[i] == '.' && s[i+1] == ' ' && s[i+2] >= 'A' && s[i+2] <= 'Z' {
			return s[:i+1]
		}
	}
	return strings.TrimSpace(s)
}
