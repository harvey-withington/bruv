package routing

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"bruv/internal/config"
)

// rulesRouter is the complexity-heuristic router: score the request,
// then walk the user's rules top to bottom; the first rule whose
// conditions all hold and whose target has an eligible model wins.
type rulesRouter struct{ cfg config.LLMRouter }

func newRulesRouter(cfg config.LLMRouter) *rulesRouter { return &rulesRouter{cfg: cfg} }

// DefaultRules seeds a new rules router: one rule per band, each
// targeting the matching tier.
func DefaultRules() []config.RoutingRule {
	return []config.RoutingRule{
		{ID: config.NewModelID(), Bands: []config.ComplexityBand{config.BandLow}, Target: config.ModelRefTo(config.RefTier, string(config.TierFast))},
		{ID: config.NewModelID(), Bands: []config.ComplexityBand{config.BandMedium}, Target: config.ModelRefTo(config.RefTier, string(config.TierBalanced))},
		{ID: config.NewModelID(), Bands: []config.ComplexityBand{config.BandHigh}, Target: config.ModelRefTo(config.RefTier, string(config.TierPowerful))},
	}
}

func (r *rulesRouter) Route(_ context.Context, req Request, candidates []config.LLMModel) (Result, error) {
	if len(candidates) == 0 {
		return Result{}, fmt.Errorf("no eligible models")
	}
	a := Assess(req, r.cfg)
	lower := strings.ToLower(req.UserMessage)

	for i, rule := range r.cfg.Rules {
		if !ruleMatches(rule, req, a, lower) {
			continue
		}
		if id := ResolveTarget(rule.Target, candidates); id != "" {
			return Result{
				ModelID:    id,
				Via:        ViaRule,
				RuleID:     rule.ID,
				RuleIndex:  i + 1,
				RuleName:   rule.Name,
				Assessment: &a,
			}, nil
		}
	}

	fallback := r.cfg.Fallback
	if fallback == "" {
		fallback = config.ModelRefTo(config.RefTier, string(config.TierBalanced))
	}
	if id := ResolveTarget(fallback, candidates); id != "" {
		return Result{ModelID: id, Via: ViaFallback, Assessment: &a}, nil
	}
	return Result{ModelID: candidates[0].ID, Via: ViaFirst, Assessment: &a}, nil
}

func ruleMatches(rule config.RoutingRule, req Request, a Assessment, lowerMsg string) bool {
	if len(rule.Tasks) > 0 && !slices.Contains(rule.Tasks, req.Task) {
		return false
	}
	if len(rule.Bands) > 0 && !slices.Contains(rule.Bands, a.Band) {
		return false
	}
	if rule.MinTokens > 0 && a.MessageTokens < rule.MinTokens {
		return false
	}
	if rule.MaxTokens > 0 && a.MessageTokens > rule.MaxTokens {
		return false
	}
	if len(rule.Keywords) > 0 && firstKeyword(lowerMsg, rule.Keywords) == "" {
		return false
	}
	switch rule.Tools {
	case config.ToolsYes:
		return req.ToolsOffered
	case config.ToolsNo:
		return !req.ToolsOffered
	}
	return true
}
