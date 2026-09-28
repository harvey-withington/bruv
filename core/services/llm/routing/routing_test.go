package routing

import (
	"context"
	"strings"
	"testing"

	"bruv/internal/config"
)

func TestAssessBands(t *testing.T) {
	r := config.LLMRouter{}
	low := Assess(Request{Task: "card_chat", UserMessage: "rename this card to Groceries"}, r)
	if low.Band != config.BandLow {
		t.Errorf("rename: band %s score %d, want low", low.Band, low.Score)
	}
	hard := Assess(Request{
		Task:         "project_chat",
		ToolsOffered: true,
		UserMessage:  "Can you design a plan for the migration? What are the trade-offs between the two approaches?",
	}, r)
	if hard.Band != config.BandHigh {
		t.Errorf("design question: band %s score %d (%+v), want high", hard.Band, hard.Score, hard.Signals)
	}
	if hard.Score > 100 || low.Score < 0 {
		t.Error("score not clamped")
	}
}

func TestAssessWholeWordKeywords(t *testing.T) {
	a := Assess(Request{UserMessage: "move this to the next stage"}, config.LLMRouter{})
	for _, s := range a.Signals {
		if s.Key == "light_keyword" {
			t.Errorf("'tag' matched inside 'stage': %+v", s)
		}
	}
}

func TestAssessCustomThresholds(t *testing.T) {
	r := config.LLMRouter{Thresholds: config.ComplexityThresholds{Medium: 5, High: 10}}
	a := Assess(Request{Task: "project_chat"}, r) // +10
	if a.Band != config.BandHigh {
		t.Errorf("band = %s, want high with custom thresholds", a.Band)
	}
}

func TestAssessLongMessage(t *testing.T) {
	a := Assess(Request{UserMessage: strings.Repeat("word ", 1300)}, config.LLMRouter{})
	if a.Score != 30 || a.Signals[0].Key != "message_long" {
		t.Errorf("long message: %+v", a)
	}
}

var candidates = []config.LLMModel{
	{ID: "fast1", Tier: config.TierFast},
	{ID: "bal1", Tier: config.TierBalanced},
	{ID: "pow1", Tier: config.TierPowerful},
}

func TestRulesRouterDefaultRules(t *testing.T) {
	router, err := New(config.LLMRouter{Kind: config.RouterRules, Rules: DefaultRules()})
	if err != nil {
		t.Fatal(err)
	}
	res, err := router.Route(context.Background(), Request{Task: "card_chat", UserMessage: "fix the typo"}, candidates)
	if err != nil || res.ModelID != "fast1" || res.Via != ViaRule || res.RuleIndex != 1 {
		t.Errorf("easy: %+v, %v", res, err)
	}
	res, _ = router.Route(context.Background(), Request{
		Task: "agent_run", ToolsOffered: true,
		UserMessage: "Analyse last week's cards and plan the next sprint. Why did velocity drop? What should we cut?",
	}, candidates)
	if res.ModelID != "pow1" || res.Assessment.Band != config.BandHigh {
		t.Errorf("hard: %+v", res)
	}
}

func TestRulesRouterConditionsAndFallback(t *testing.T) {
	cfg := config.LLMRouter{
		Kind: config.RouterRules,
		Rules: []config.RoutingRule{
			{ID: "r1", Name: "agents", Tasks: []string{"agent_run"}, Target: "model:pow1"},
			{ID: "r2", Keywords: []string{"summarise"}, Target: "tier:fast"},
			{ID: "r3", Tools: config.ToolsNo, MaxTokens: 5, Target: "model:gone"}, // target not eligible → skipped
		},
		Fallback: "model:bal1",
	}
	router, _ := New(cfg)
	ctx := context.Background()

	if res, _ := router.Route(ctx, Request{Task: "agent_run"}, candidates); res.ModelID != "pow1" || res.RuleName != "agents" {
		t.Errorf("task rule: %+v", res)
	}
	if res, _ := router.Route(ctx, Request{Task: "card_chat", UserMessage: "Summarise this"}, candidates); res.ModelID != "fast1" || res.RuleIndex != 2 {
		t.Errorf("keyword rule: %+v", res)
	}
	if res, _ := router.Route(ctx, Request{Task: "card_chat", UserMessage: "hi"}, candidates); res.ModelID != "bal1" || res.Via != ViaFallback {
		t.Errorf("fallback: %+v", res)
	}

	cfg.Fallback = "tier:nope"
	router, _ = New(cfg)
	if res, _ := router.Route(ctx, Request{Task: "card_chat", UserMessage: "hi"}, candidates); res.ModelID != "fast1" || res.Via != ViaFirst {
		t.Errorf("first eligible: %+v", res)
	}

	if _, err := router.Route(ctx, Request{}, nil); err == nil {
		t.Error("expected error with no candidates")
	}
}

func TestNewUnknownKind(t *testing.T) {
	if _, err := New(config.LLMRouter{Kind: "psychic"}); err == nil {
		t.Error("expected error for unknown router kind")
	}
}
