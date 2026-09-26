// Package routing holds BRUV's model routing engines: given an AI
// request and the models eligible to serve it, a Router picks one.
//
// Engines are pure decision-makers. They never construct providers or
// call a model for the answer itself; the LLM service resolves the
// chosen model id into a provider (core/services/llm/select.go).
package routing

import (
	"context"
	"fmt"

	"bruv/internal/config"
)

// Request is what a router sees of an AI turn.
type Request struct {
	Task          string
	UserMessage   string
	ContextTokens int // system prompt + history, estimated
	ToolsOffered  bool
}

// Via says how a router arrived at its pick. Structured rather than
// prose so each surface can localize the explanation.
type Via string

const (
	ViaRule     Via = "rule"     // a rule matched
	ViaFallback Via = "fallback" // no rule matched; the router's fallback
	ViaFirst    Via = "first"    // nothing usable matched; first eligible model
)

// Result is a router's pick.
type Result struct {
	ModelID    string
	Via        Via
	RuleID     string // rules router: the rule that matched
	RuleIndex  int    // 1-based position of that rule
	RuleName   string
	Assessment *Assessment // set by engines that score complexity
}

// Router picks a model for a request from eligible candidates, given in
// preference order (provider order, then model order).
type Router interface {
	Route(ctx context.Context, req Request, candidates []config.LLMModel) (Result, error)
}

// New builds the engine for a configured router.
func New(r config.LLMRouter) (Router, error) {
	switch r.Kind {
	case config.RouterRules:
		return newRulesRouter(r), nil
	default:
		return nil, fmt.Errorf("unknown router kind %q", r.Kind)
	}
}

// ResolveTarget turns a router target (model or tier ref) into a
// candidate model id. Returns "" when the target names no eligible
// model: a model that is not a candidate, or a tier with none.
func ResolveTarget(target config.ModelRef, candidates []config.LLMModel) string {
	kind, id := target.Parse()
	switch kind {
	case config.RefModel:
		for _, c := range candidates {
			if c.ID == id {
				return c.ID
			}
		}
	case config.RefTier:
		for _, c := range candidates {
			if string(c.Tier) == id {
				return c.ID
			}
		}
	}
	return ""
}

// EstimateTokens is the chars/4 rule of thumb used for routing
// decisions; close enough to rank prompts, never used for billing.
func EstimateTokens(s string) int {
	return (len([]rune(s)) + 3) / 4
}
