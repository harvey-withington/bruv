package llm

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"bruv/core/services/llm/routing"
	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/model"
)

// Task is a kind of AI work. Settings attach model choices to tasks.
type Task string

const (
	TaskCardChat     Task = "card_chat"
	TaskProjectChat  Task = "project_chat"
	TaskCardPopulate Task = "card_populate" // quick capture "Create with AI"
	TaskAgentRun     Task = "agent_run"
)

// Tasks lists every task in settings order.
var Tasks = []Task{TaskCardChat, TaskProjectChat, TaskCardPopulate, TaskAgentRun}

// RouteRequest describes one AI turn to Select.
type RouteRequest struct {
	Task Task
	// Choice is the invocation's own model choice (the chat's, the
	// agent's); "" inherits the task assignment.
	Choice config.ModelRef
	// LegacyAccountID / LegacyModel are an agent's pre-routing pair,
	// honoured when Choice is empty.
	LegacyAccountID string
	LegacyModel     string

	// Routing inputs.
	UserMessage   string
	ContextTokens int
	ToolsOffered  bool
}

// Selection is the resolved model for a turn.
type Selection struct {
	// Config is the AI behaviour config (mode, context) with Provider and
	// Model set to the selected model, for prompt builders and callers
	// that read them.
	Config   config.LLMConfig
	Provider llm.Provider
	Model    string // provider model id to send
	Decision model.RouteDecision
}

// Decision sources.
const (
	SourceOverride = "override"
	SourceTask     = "task"
	SourceDefault  = "default"
	SourceFirst    = "first"
	SourceLegacy   = "legacy"
)

// Select resolves the model for an AI turn. Precedence: the request's
// own choice (or an agent's legacy account/model pair) → the task's
// assignment → the global default → the first enabled model → legacy
// single-provider config. A choice that can't be used (deleted,
// disabled, router with nothing eligible) is recorded in
// Decision.Skipped and resolution moves to the next level.
//
// Returns (nil, nil) when nothing is configured — callers treat that as
// "AI not set up". A non-nil error means config exists but can't be read
// or a provider can't be built; callers surface it.
func (s *Service) Select(ctx context.Context, req RouteRequest) (*Selection, error) {
	cfg, err := config.LoadLLMConfig()
	if err != nil {
		return nil, fmt.Errorf("load llm config: %w", err)
	}
	reg, err := config.LoadLLMRouting()
	if err != nil {
		return nil, fmt.Errorf("load llm routing: %w", err)
	}
	accounts, err := config.LoadLLMAccounts()
	if err != nil {
		return nil, fmt.Errorf("load llm accounts: %w", err)
	}
	r := &resolver{cfg: cfg, reg: reg, accounts: accounts, req: req}
	sel, err := r.resolve(ctx)
	if sel != nil {
		slog.Info("llm model selected",
			"task", req.Task, "model", sel.Decision.Model, "provider", sel.Decision.Provider,
			"source", sel.Decision.Source, "router", sel.Decision.RouterName,
			"via", sel.Decision.Via, "skipped", len(sel.Decision.Skipped))
	}
	return sel, err
}

type resolver struct {
	cfg      config.LLMConfig
	reg      config.LLMRouting
	accounts []config.LLMAccount
	req      RouteRequest
	skipped  []model.SkippedChoice
}

func (r *resolver) resolve(ctx context.Context) (*Selection, error) {
	if r.req.Choice == "" && (r.req.LegacyAccountID != "" || r.req.LegacyModel != "") {
		if sel, err := r.legacyPair(); sel != nil || err != nil {
			return sel, err
		}
	}

	levels := []struct {
		ref    config.ModelRef
		source string
	}{
		{r.req.Choice, SourceOverride},
		{r.reg.Tasks[string(r.req.Task)], SourceTask},
		{r.reg.Default, SourceDefault},
	}
	for _, l := range levels {
		if l.ref == "" {
			continue
		}
		sel, err := r.fromRef(ctx, l.ref, l.source)
		if sel != nil || err != nil {
			return sel, err
		}
	}

	for _, m := range r.orderedModels() {
		if !m.Enabled {
			continue
		}
		if acct := config.FindAccountByID(r.accounts, m.AccountID); acct != nil {
			return r.selection(acct, m.Name, &m, SourceFirst)
		}
	}

	// Legacy single-provider fields in llm_config.json.
	if r.cfg.Provider != "" {
		acct := &config.LLMAccount{Provider: r.cfg.Provider, APIKey: r.cfg.APIKey, BaseURL: r.cfg.BaseURL}
		name := r.cfg.Model
		if name == "" {
			name = config.DefaultModelForProvider(r.cfg.Provider)
		}
		return r.selection(acct, name, nil, SourceLegacy)
	}
	return nil, nil
}

// legacyPair resolves an agent's pre-routing account/model pair: the
// registry model with that account and name if there is one, else the
// name run ad hoc on that account. Returns (nil, nil) to fall through
// when the account is gone.
func (r *resolver) legacyPair() (*Selection, error) {
	accountID := r.req.LegacyAccountID
	if accountID == "" {
		if acct := config.GetDefaultAccount(r.accounts); acct != nil {
			accountID = acct.ID
		}
	}
	acct := config.FindAccountByID(r.accounts, accountID)
	if acct == nil {
		r.skip(SourceOverride, "account:"+r.req.LegacyAccountID, "no_account")
		return nil, nil
	}
	name := r.req.LegacyModel
	for i, m := range r.reg.Models {
		if m.AccountID == acct.ID && (name == "" || m.Name == name) {
			return r.selection(acct, m.Name, &r.reg.Models[i], SourceOverride)
		}
	}
	if name == "" {
		name = config.DefaultModelForProvider(acct.Provider)
	}
	return r.selection(acct, name, nil, SourceOverride)
}

func (r *resolver) fromRef(ctx context.Context, ref config.ModelRef, source string) (*Selection, error) {
	kind, id := ref.Parse()
	switch kind {
	case config.RefModel:
		m := r.reg.FindModel(id)
		if m == nil {
			r.skip(source, string(ref), "missing")
			return nil, nil
		}
		if !m.Enabled {
			r.skip(source, string(ref), "disabled")
			return nil, nil
		}
		acct := config.FindAccountByID(r.accounts, m.AccountID)
		if acct == nil {
			r.skip(source, string(ref), "no_account")
			return nil, nil
		}
		return r.selection(acct, m.Name, m, source)

	case config.RefTier:
		cands := r.candidates()
		mid := routing.ResolveTarget(ref, cands)
		if mid == "" {
			r.skip(source, string(ref), "no_eligible")
			return nil, nil
		}
		m := r.reg.FindModel(mid)
		return r.selection(config.FindAccountByID(r.accounts, m.AccountID), m.Name, m, source)

	case config.RefRouter:
		cfgRouter := r.reg.FindRouter(id)
		if cfgRouter == nil {
			r.skip(source, string(ref), "missing")
			return nil, nil
		}
		engine, err := routing.New(*cfgRouter)
		if err != nil {
			r.skip(source, string(ref), "router_error")
			return nil, nil
		}
		cands := r.candidates()
		if len(cands) == 0 {
			r.skip(source, string(ref), "no_eligible")
			return nil, nil
		}
		res, err := engine.Route(ctx, r.routingRequest(), cands)
		if err != nil {
			slog.Warn("llm router failed", "router", cfgRouter.Name, "err", err)
			r.skip(source, string(ref), "router_error")
			return nil, nil
		}
		m := r.reg.FindModel(res.ModelID)
		if m == nil {
			r.skip(source, string(ref), "router_error")
			return nil, nil
		}
		sel, err := r.selection(config.FindAccountByID(r.accounts, m.AccountID), m.Name, m, source)
		if sel != nil {
			applyRouterResult(&sel.Decision, cfgRouter, res)
		}
		return sel, err
	}
	r.skip(source, string(ref), "missing")
	return nil, nil
}

func applyRouterResult(d *model.RouteDecision, cfgRouter *config.LLMRouter, res routing.Result) {
	d.RouterID = cfgRouter.ID
	d.RouterName = cfgRouter.Name
	d.Via = string(res.Via)
	d.RuleIndex = res.RuleIndex
	d.RuleName = res.RuleName
	if res.Assessment != nil {
		score := res.Assessment.Score
		d.Score = &score
		d.Band = string(res.Assessment.Band)
	}
}

func (r *resolver) routingRequest() routing.Request {
	return routing.Request{
		Task:          string(r.req.Task),
		UserMessage:   r.req.UserMessage,
		ContextTokens: r.req.ContextTokens,
		ToolsOffered:  r.req.ToolsOffered,
	}
}

// orderedModels returns registry models in preference order: provider
// (account) order first — the user drags providers to rank them — then
// the model's own order within the registry.
func (r *resolver) orderedModels() []config.LLMModel {
	rank := make(map[string]int, len(r.accounts))
	for i, a := range r.accounts {
		rank[a.ID] = i
	}
	models := slices.Clone(r.reg.Models)
	slices.SortStableFunc(models, func(a, b config.LLMModel) int {
		ra, oka := rank[a.AccountID]
		rb, okb := rank[b.AccountID]
		if !oka {
			ra = len(r.accounts)
		}
		if !okb {
			rb = len(r.accounts)
		}
		return ra - rb
	})
	return models
}

// candidates are the models a router may pick for this request: enabled,
// on an existing provider, and tool-capable when the turn offers tools.
func (r *resolver) candidates() []config.LLMModel {
	var out []config.LLMModel
	for _, m := range r.orderedModels() {
		if !m.Enabled || config.FindAccountByID(r.accounts, m.AccountID) == nil {
			continue
		}
		if r.req.ToolsOffered && !m.SupportsTools {
			continue
		}
		out = append(out, m)
	}
	return out
}

func (r *resolver) skip(source, ref, why string) {
	r.skipped = append(r.skipped, model.SkippedChoice{Source: source, Ref: ref, Why: why})
}

func (r *resolver) selection(acct *config.LLMAccount, name string, m *config.LLMModel, source string) (*Selection, error) {
	provider, err := llm.NewProvider(acct.Provider, acct.APIKey, acct.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("create %s provider: %w", acct.Provider, err)
	}
	cfg := r.cfg
	cfg.Provider = acct.Provider
	cfg.Model = name
	d := model.RouteDecision{
		Model:         name,
		Provider:      acct.Provider,
		ProviderLabel: acct.Label,
		Source:        source,
		Skipped:       r.skipped,
	}
	if m != nil {
		d.ModelID = m.ID
		d.ModelLabel = m.Label
	}
	return &Selection{Config: cfg, Provider: provider, Model: name, Decision: d}, nil
}
