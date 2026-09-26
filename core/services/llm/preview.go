package llm

import (
	"fmt"

	"bruv/core/services/llm/routing"
	"bruv/internal/config"
)

// RoutePreview is the answer to a router's "Try it" box: how a message
// would score and which model the router would pick. No model is called.
type RoutePreview struct {
	Assessment routing.Assessment `json:"assessment"`
	Via        routing.Via        `json:"via"`
	RuleIndex  int                `json:"rule_index,omitempty"`
	RuleName   string             `json:"rule_name,omitempty"`
	ModelID    string             `json:"model_id"`
}

// PreviewRoute runs a router against a sample message. It takes the
// settings being edited (the registry and the provider order) rather
// than what is saved, so the preview reflects unsaved changes.
func (s *Service) PreviewRoute(reg config.LLMRouting, accountIDs []string, routerID, task, message string, toolsOffered bool) (RoutePreview, error) {
	cfgRouter := reg.FindRouter(routerID)
	if cfgRouter == nil {
		return RoutePreview{}, fmt.Errorf("router not found")
	}
	engine, err := routing.New(*cfgRouter)
	if err != nil {
		return RoutePreview{}, err
	}
	accounts := make([]config.LLMAccount, len(accountIDs))
	for i, id := range accountIDs {
		accounts[i] = config.LLMAccount{ID: id}
	}
	r := &resolver{reg: reg, accounts: accounts, req: RouteRequest{
		Task:         Task(task),
		UserMessage:  message,
		ToolsOffered: toolsOffered,
	}}
	cands := r.candidates()
	if len(cands) == 0 {
		return RoutePreview{Assessment: routing.Assess(r.routingRequest(), *cfgRouter)}, nil
	}
	res, err := engine.Route(s.deps.Ctx(), r.routingRequest(), cands)
	if err != nil {
		return RoutePreview{}, err
	}
	p := RoutePreview{Via: res.Via, RuleIndex: res.RuleIndex, RuleName: res.RuleName, ModelID: res.ModelID}
	if res.Assessment != nil {
		p.Assessment = *res.Assessment
	}
	return p, nil
}

// NewRulesRouter returns an unsaved rules router seeded with one rule
// per complexity band, each targeting the matching tier. The settings
// editor adds it to the registry it is editing.
func (s *Service) NewRulesRouter(name string) config.LLMRouter {
	return config.LLMRouter{
		ID:         config.NewModelID(),
		Name:       name,
		Kind:       config.RouterRules,
		Rules:      routing.DefaultRules(),
		Fallback:   config.ModelRefTo(config.RefTier, string(config.TierBalanced)),
		Thresholds: config.ComplexityThresholds{Medium: routing.DefaultMediumThreshold, High: routing.DefaultHighThreshold},
	}
}
