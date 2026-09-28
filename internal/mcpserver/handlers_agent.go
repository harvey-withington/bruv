package mcpserver

// Agent tools: read, configure and trigger a card's autonomous agent.
// Configuration goes through agentsvc.Patch — the same path card and
// project chat use — so validation and next-run scheduling match the
// in-app surfaces exactly.

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"bruv/core/services/agentsvc"
	"bruv/core/supervisor"
	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/mcp"
	"bruv/internal/model"
)

// recentAgentRuns caps the run history get_card_agent returns.
const recentAgentRuns = 5

type agentToolOption struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Ready       bool   `json:"ready"`
}

// llmOption is one value the agent's `llm` field accepts: a registry
// model or a router, as a config.ModelRef.
type llmOption struct {
	Ref           string `json:"ref"`
	Label         string `json:"label"`
	Kind          string `json:"kind"` // "model" | "router"
	Tier          string `json:"tier,omitempty"`
	SupportsTools *bool  `json:"supports_tools,omitempty"`
}

type agentRunSummary struct {
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Status     string     `json:"status"`
	Summary    string     `json:"summary,omitempty"`
	Error      string     `json:"error,omitempty"`
	ToolCalls  int        `json:"tool_calls"`
	TokensUsed int        `json:"tokens_used,omitempty"`
	ModelUsed  string     `json:"model_used,omitempty"`
}

// agentOptions lists the values configure_card_agent accepts, so a
// client can build a working config without guessing ids.
type agentOptions struct {
	Tools          []agentToolOption `json:"tools"`
	LLM            []llmOption       `json:"llm"`
	LLMConfigured  bool              `json:"llm_configured"`
	NotifyOn       []string          `json:"notify_on"`
	NotifyChannels []string          `json:"notify_channel"`
	ScheduleSyntax string            `json:"schedule_syntax"`
}

const scheduleSyntax = "Interval ('30m', '2h', '1d'; minimum 1m), cron shortcut ('@hourly', '@daily', '@weekly', '@every 45m') " +
	"or 5-field cron ('0 9 * * 1-5' = weekdays 09:00, in `timezone`). Empty = runs only when triggered."

func hGetCardAgent(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	if cardID == "" {
		return errResult("card_id is required")
	}
	card, err := rt.GetCard(cardID)
	if err != nil {
		return errResult("%v", err)
	}
	af, err := rt.GetAgentConfig(cardID)
	if err != nil {
		return errResult("%v", err)
	}
	runs, err := rt.GetAgentRuns(cardID)
	if err != nil {
		return errResult("%v", err)
	}
	recent := make([]agentRunSummary, 0, recentAgentRuns)
	for _, r := range runs {
		if len(recent) == recentAgentRuns {
			break
		}
		recent = append(recent, agentRunSummary{
			StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, Status: r.Status, Summary: r.Summary,
			Error: r.Error, ToolCalls: len(r.ToolCalls), TokensUsed: r.TokensUsed, ModelUsed: r.ModelUsed,
		})
	}
	opts, err := buildAgentOptions(rt)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(map[string]any{
		"card_id":     cardID,
		"card_title":  card.Title,
		"card_type":   card.Type,
		"config":      af.Config,
		"recent_runs": recent,
		"options":     opts,
	})
}

func hConfigureCardAgent(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	if cardID == "" {
		return errResult("card_id is required")
	}
	if _, err := rt.GetCard(cardID); err != nil {
		return errResult("%v", err)
	}
	patch, err := agentsvc.PatchFromArgs(a)
	if err != nil {
		return errResult("%v", err)
	}
	if patch.IsEmpty() {
		return errResult("no agent settings given; see get_card_agent for the fields")
	}
	opts, err := buildAgentOptions(rt)
	if err != nil {
		return errResult("%v", err)
	}
	var warnings []string
	if patch.AllowedTools != nil {
		w, err := checkAllowedTools(*patch.AllowedTools, opts.Tools)
		if err != nil {
			return errResult("%v", err)
		}
		warnings = append(warnings, w...)
	}
	if patch.LLM != nil && *patch.LLM != "" {
		i := slices.IndexFunc(opts.LLM, func(o llmOption) bool { return o.Ref == *patch.LLM })
		if i < 0 {
			return errResult("llm %q is not a configured model or router; see get_card_agent options.llm", *patch.LLM)
		}
		if st := opts.LLM[i].SupportsTools; st != nil && !*st {
			warnings = append(warnings, fmt.Sprintf("model %q doesn't support tool calls, so the agent can't use any of its tools", opts.LLM[i].Label))
		}
	}
	cfg, err := rt.Agent.Patch(cardID, patch)
	if err != nil {
		return errResult("%v", err)
	}
	warnings = append(warnings, readinessWarnings(*cfg, opts.LLMConfigured)...)
	out := map[string]any{"card_id": cardID, "summary": agentsvc.Summary(*cfg), "config": cfg}
	if len(warnings) > 0 {
		out["warnings"] = warnings
	}
	return jsonResult(out)
}

func hRunCardAgent(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	if cardID == "" {
		return errResult("card_id is required")
	}
	af, err := rt.GetAgentConfig(cardID)
	if err != nil {
		return errResult("%v", err)
	}
	if strings.TrimSpace(af.Config.Goal) == "" {
		return errResult("card %s has no agent goal; set one with configure_card_agent first", cardID)
	}
	if err := rt.TriggerAgent(cardID); err != nil {
		return errResult("%v", err)
	}
	return jsonResult(map[string]any{
		"card_id": cardID, "started": true,
		"note": "The run is asynchronous; call get_card_agent shortly to see its result in recent_runs.",
	})
}

func buildAgentOptions(rt *supervisor.Runtime) (agentOptions, error) {
	opts := agentOptions{
		LLMConfigured:  rt.IsLLMConfigured(),
		NotifyOn:       agentsvc.NotifyTriggers,
		NotifyChannels: agentsvc.NotifyChannels,
		ScheduleSyntax: scheduleSyntax,
		LLM:            []llmOption{},
	}
	for _, t := range llm.AgentTools(llm.BuiltinAgentToolNames()) {
		opts.Tools = append(opts.Tools, agentToolOption{ID: t.Name, Description: firstSentence(t.Description), Ready: true})
	}
	servers, err := rt.ListMCPServers()
	if err != nil {
		return opts, fmt.Errorf("list MCP servers: %w", err)
	}
	for _, s := range servers {
		ready := s.Health.Status == mcp.HealthReady
		for _, t := range s.Tools {
			opts.Tools = append(opts.Tools, agentToolOption{ID: t.NamespaceID, Description: firstSentence(t.Description), Ready: ready})
		}
	}
	// The key-free registry view: API keys never leave the machine.
	reg, err := rt.LLM.GetRegistryView()
	if err != nil {
		return opts, fmt.Errorf("load model registry: %w", err)
	}
	for _, m := range reg.Routing.Models {
		if !m.Enabled {
			continue
		}
		st := m.SupportsTools
		opts.LLM = append(opts.LLM, llmOption{
			Ref: string(config.ModelRefTo(config.RefModel, m.ID)), Label: m.DisplayLabel(),
			Kind: string(config.RefModel), Tier: string(m.Tier), SupportsTools: &st,
		})
	}
	for _, r := range reg.Routing.Routers {
		opts.LLM = append(opts.LLM, llmOption{
			Ref: string(config.ModelRefTo(config.RefRouter, r.ID)), Label: r.Name, Kind: string(config.RefRouter),
		})
	}
	return opts, nil
}

// checkAllowedTools rejects ids that name nothing. An MCP id whose
// server is configured but not currently exposing tools is accepted
// with a warning — the server may simply be restarting.
func checkAllowedTools(ids []string, known []agentToolOption) ([]string, error) {
	servers := map[string]bool{}
	for _, t := range known {
		if server, _ := mcp.SplitNamespacedTool(t.ID); server != "" {
			servers[server] = true
		}
	}
	var unknown, warnings []string
	for _, id := range ids {
		i := slices.IndexFunc(known, func(t agentToolOption) bool { return t.ID == id })
		switch {
		case i >= 0 && known[i].Ready:
		case i >= 0:
			warnings = append(warnings, fmt.Sprintf("tool %q belongs to an MCP server that is not ready; the agent can't use it until it is", id))
		default:
			if server, _ := mcp.SplitNamespacedTool(id); server != "" && servers[server] {
				warnings = append(warnings, fmt.Sprintf("tool %q is not currently offered by its MCP server", id))
				continue
			}
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown allowed_tools %v; see get_card_agent options.tools for valid ids", unknown)
	}
	return warnings, nil
}

// readinessWarnings flags a saved config that won't run on its own, so
// the client learns it now rather than from a silent scheduler.
func readinessWarnings(cfg model.AgentConfig, llmConfigured bool) []string {
	var w []string
	if !cfg.Enabled {
		return append(w, "agent is disabled; set enabled=true for it to run on its schedule")
	}
	if !llmConfigured {
		w = append(w, "no LLM account is configured in BRUV, so runs will fail until the user adds one in Settings")
	}
	switch {
	case cfg.Schedule == "":
		w = append(w, "no schedule: the agent runs only when triggered (run_card_agent)")
	case cfg.NextRunAt == nil:
		w = append(w, "schedule has no upcoming run (one-shot already ran, or the end date has passed)")
	}
	return w
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}
