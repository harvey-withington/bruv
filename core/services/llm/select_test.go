package llm

import (
	"context"
	"testing"

	"bruv/internal/config"
)

type testDeps struct{}

func (testDeps) Ctx() context.Context { return context.Background() }

// setup writes two providers (no API keys, so nothing touches the OS
// keychain) and the given registry into a throwaway config dir.
func setup(t *testing.T, reg config.LLMRouting) *Service {
	t.Helper()
	config.SetConfigDir(t.TempDir())
	t.Cleanup(func() { config.SetConfigDir("") })
	accounts := []config.LLMAccount{
		{ID: "sel-cloud", Label: "Cloud", Provider: "openai_compatible", BaseURL: "http://localhost:1"},
		{ID: "sel-local", Label: "Local", Provider: "ollama"},
	}
	if err := config.SaveLLMAccounts(accounts); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveLLMRouting(reg); err != nil {
		t.Fatal(err)
	}
	return New(testDeps{})
}

func registry() config.LLMRouting {
	return config.LLMRouting{
		Models: []config.LLMModel{
			{ID: "small", AccountID: "sel-local", Name: "llama3.2:3b", Tier: config.TierFast, SupportsTools: false, Enabled: true},
			{ID: "mid", AccountID: "sel-cloud", Name: "mid-model", Tier: config.TierBalanced, SupportsTools: true, Enabled: true},
			{ID: "big", AccountID: "sel-cloud", Name: "big-model", Tier: config.TierPowerful, SupportsTools: true, Enabled: true},
			{ID: "off", AccountID: "sel-cloud", Name: "off-model", Tier: config.TierPowerful, SupportsTools: true, Enabled: false},
		},
		Routers: []config.LLMRouter{{
			ID: "auto", Name: "Auto", Kind: config.RouterRules,
			Rules: []config.RoutingRule{
				{ID: "r-low", Bands: []config.ComplexityBand{config.BandLow}, Target: "tier:fast"},
				{ID: "r-high", Bands: []config.ComplexityBand{config.BandHigh}, Target: "tier:powerful"},
			},
			Fallback: "tier:balanced",
		}},
		Default: "model:mid",
		Tasks:   map[string]config.ModelRef{string(TaskAgentRun): "model:big"},
	}
}

func mustSelect(t *testing.T, s *Service, req RouteRequest) *Selection {
	t.Helper()
	sel, err := s.Select(context.Background(), req)
	if err != nil || sel == nil {
		t.Fatalf("Select(%+v) = %v, %v", req, sel, err)
	}
	return sel
}

func TestSelectPrecedence(t *testing.T) {
	s := setup(t, registry())

	if sel := mustSelect(t, s, RouteRequest{Task: TaskCardChat}); sel.Model != "mid-model" || sel.Decision.Source != SourceDefault {
		t.Errorf("default: %+v", sel.Decision)
	}
	if sel := mustSelect(t, s, RouteRequest{Task: TaskAgentRun}); sel.Model != "big-model" || sel.Decision.Source != SourceTask {
		t.Errorf("task: %+v", sel.Decision)
	}
	sel := mustSelect(t, s, RouteRequest{Task: TaskAgentRun, Choice: "model:small"})
	if sel.Model != "llama3.2:3b" || sel.Decision.Source != SourceOverride || sel.Decision.Provider != "ollama" || sel.Config.Model != "llama3.2:3b" {
		t.Errorf("override: %+v", sel.Decision)
	}
	if sel.Decision.ProviderLabel != "Local" || sel.Decision.ModelID != "small" {
		t.Errorf("decision labels: %+v", sel.Decision)
	}
}

func TestSelectSkipsUnusableChoices(t *testing.T) {
	s := setup(t, registry())
	sel := mustSelect(t, s, RouteRequest{Task: TaskAgentRun, Choice: "model:off"})
	if sel.Model != "big-model" || sel.Decision.Source != SourceTask {
		t.Errorf("disabled override should fall to task: %+v", sel.Decision)
	}
	if len(sel.Decision.Skipped) != 1 || sel.Decision.Skipped[0].Why != "disabled" || sel.Decision.Skipped[0].Source != SourceOverride {
		t.Errorf("skipped = %+v", sel.Decision.Skipped)
	}
	sel = mustSelect(t, s, RouteRequest{Task: TaskCardChat, Choice: "router:deleted"})
	if sel.Decision.Source != SourceDefault || sel.Decision.Skipped[0].Why != "missing" {
		t.Errorf("missing router: %+v", sel.Decision)
	}
}

func TestSelectRouter(t *testing.T) {
	s := setup(t, registry())

	// Easy, no tools: the fast local model is eligible.
	sel := mustSelect(t, s, RouteRequest{Task: TaskCardChat, Choice: "router:auto", UserMessage: "rename it"})
	if sel.Model != "llama3.2:3b" || sel.Decision.RouterName != "Auto" || sel.Decision.Via != "rule" || sel.Decision.RuleIndex != 1 || sel.Decision.Score == nil {
		t.Errorf("easy: %+v", sel.Decision)
	}

	// Easy but tools offered: the fast model can't call tools, so the
	// low rule has no eligible target and the fallback (balanced) wins.
	sel = mustSelect(t, s, RouteRequest{Task: TaskCardChat, Choice: "router:auto", UserMessage: "rename it", ToolsOffered: true})
	if sel.Model != "mid-model" || sel.Decision.Via != "fallback" {
		t.Errorf("tools: %+v", sel.Decision)
	}

	// Hard: powerful tier, skipping the disabled powerful model.
	sel = mustSelect(t, s, RouteRequest{
		Task: TaskProjectChat, Choice: "router:auto", ToolsOffered: true,
		UserMessage: "Plan and design the Q4 roadmap. What are the trade-offs? Which should go first?",
	})
	if sel.Model != "big-model" || sel.Decision.Band != "high" {
		t.Errorf("hard: %+v", sel.Decision)
	}
}

func TestSelectTierRefAndLegacyPair(t *testing.T) {
	s := setup(t, registry())
	reg := registry()
	reg.Tasks[string(TaskCardPopulate)] = "tier:powerful"
	if err := config.SaveLLMRouting(reg); err != nil {
		t.Fatal(err)
	}
	if sel := mustSelect(t, s, RouteRequest{Task: TaskCardPopulate}); sel.Model != "big-model" {
		t.Errorf("tier ref: %+v", sel.Decision)
	}

	// Pre-routing agent pair matching a registry model.
	sel := mustSelect(t, s, RouteRequest{Task: TaskAgentRun, LegacyAccountID: "sel-cloud", LegacyModel: "mid-model"})
	if sel.Decision.ModelID != "mid" || sel.Decision.Source != SourceOverride {
		t.Errorf("legacy match: %+v", sel.Decision)
	}
	// ... and one naming a model not in the registry runs ad hoc.
	sel = mustSelect(t, s, RouteRequest{Task: TaskAgentRun, LegacyAccountID: "sel-cloud", LegacyModel: "adhoc-model"})
	if sel.Model != "adhoc-model" || sel.Decision.ModelID != "" {
		t.Errorf("legacy ad hoc: %+v", sel.Decision)
	}
}

func TestSelectFirstEnabledAndUnconfigured(t *testing.T) {
	reg := registry()
	reg.Default = ""
	s := setup(t, reg)
	// Provider order: sel-cloud before sel-local, so "mid" is first.
	if sel := mustSelect(t, s, RouteRequest{Task: TaskCardChat}); sel.Model != "mid-model" || sel.Decision.Source != SourceFirst {
		t.Errorf("first: %+v", sel.Decision)
	}

	if err := config.SaveLLMRouting(config.LLMRouting{}); err != nil {
		t.Fatal(err)
	}
	sel, err := s.Select(context.Background(), RouteRequest{Task: TaskCardChat})
	if err != nil || sel != nil {
		t.Errorf("unconfigured: %+v, %v", sel, err)
	}
}

func TestPreviewRouteUsesUnsavedSettings(t *testing.T) {
	s := setup(t, config.LLMRouting{})
	reg := registry()
	p, err := s.PreviewRoute(reg, []string{"sel-cloud", "sel-local"}, "auto", string(TaskCardChat), "fix the typo", false)
	if err != nil {
		t.Fatal(err)
	}
	if p.ModelID != "small" || p.Via != "rule" || p.Assessment.Band != config.BandLow {
		t.Errorf("preview: %+v", p)
	}
}
