package config

// Model registry and routing configuration (plan/2026-09-25 multiple
// models and model routing.md).
//
// Accounts (llm_accounts.json) are the PROVIDERS: kind, key, base URL.
// This file holds what sits on top of them: the models the user has set
// up on each provider, the routers that pick between models, and which
// model or router serves each AI task.
//
// Every setting that chooses a model stores a ModelRef string, so a
// select box, a JSON field and a Go struct all carry the same value.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// ModelRef names what serves an AI request:
//
//	"model:<id>"   a model in the registry
//	"router:<id>"  a router, which picks a model per request
//	"tier:<tier>"  the first enabled model of a tier (router targets only)
//	""             inherit from the next level (task → default)
type ModelRef string

// ModelRefKind is the part of a ModelRef before the colon.
type ModelRefKind string

const (
	RefNone   ModelRefKind = ""
	RefModel  ModelRefKind = "model"
	RefRouter ModelRefKind = "router"
	RefTier   ModelRefKind = "tier"
)

// Parse splits a ref into its kind and id. An unrecognised or malformed
// ref parses as RefNone, which every caller treats as "inherit".
func (r ModelRef) Parse() (ModelRefKind, string) {
	kind, id, ok := strings.Cut(string(r), ":")
	if !ok || id == "" {
		return RefNone, ""
	}
	switch ModelRefKind(kind) {
	case RefModel, RefRouter, RefTier:
		return ModelRefKind(kind), id
	}
	return RefNone, ""
}

// ModelRefTo builds a ref of the given kind.
func ModelRefTo(kind ModelRefKind, id string) ModelRef {
	return ModelRef(string(kind) + ":" + id)
}

// ModelTier is a coarse capability/cost class routers can target.
type ModelTier string

const (
	TierFast     ModelTier = "fast"
	TierBalanced ModelTier = "balanced"
	TierPowerful ModelTier = "powerful"
)

// Valid reports whether t is one of the three known tiers.
func (t ModelTier) Valid() bool {
	return t == TierFast || t == TierBalanced || t == TierPowerful
}

// LLMModel is one usable model on a provider account.
type LLMModel struct {
	ID            string    `json:"id"`
	AccountID     string    `json:"account_id"`
	Name          string    `json:"name"` // provider model id sent on the wire
	Label         string    `json:"label,omitempty"`
	Tier          ModelTier `json:"tier"`
	SupportsTools bool      `json:"supports_tools"`
	Enabled       bool      `json:"enabled"`
}

// DisplayLabel is the label, or the provider model id when unlabelled.
func (m LLMModel) DisplayLabel() string {
	if m.Label != "" {
		return m.Label
	}
	return m.Name
}

// LLMRouterKind selects the routing engine.
type LLMRouterKind string

const (
	RouterRules LLMRouterKind = "rules"
)

// LLMRouter is a routing engine instance the user has configured.
type LLMRouter struct {
	ID   string        `json:"id"`
	Name string        `json:"name"`
	Kind LLMRouterKind `json:"kind"`

	// Rules router.
	Rules      []RoutingRule        `json:"rules,omitempty"`
	Fallback   ModelRef             `json:"fallback,omitempty"` // model or tier; "" = tier:balanced
	Thresholds ComplexityThresholds `json:"thresholds"`
	// Keyword lists for the complexity heuristic; nil = built-in defaults.
	ReasoningKeywords []string `json:"reasoning_keywords,omitempty"`
	LightKeywords     []string `json:"light_keywords,omitempty"`
}

// ComplexityThresholds are the score cut-offs between bands. Zero values
// fall back to the defaults (30 / 60).
type ComplexityThresholds struct {
	Medium int `json:"medium,omitempty"`
	High   int `json:"high,omitempty"`
}

// ComplexityBand is the heuristic's coarse verdict.
type ComplexityBand string

const (
	BandLow    ComplexityBand = "low"
	BandMedium ComplexityBand = "medium"
	BandHigh   ComplexityBand = "high"
)

// ToolsCondition restricts a rule to requests that do / don't offer tools.
type ToolsCondition string

const (
	ToolsAny ToolsCondition = ""
	ToolsYes ToolsCondition = "yes"
	ToolsNo  ToolsCondition = "no"
)

// RoutingRule is one row of a rules router. Every set condition must
// hold; unset conditions (empty slice, zero, ToolsAny) match anything.
type RoutingRule struct {
	ID        string           `json:"id"`
	Name      string           `json:"name,omitempty"`
	Tasks     []string         `json:"tasks,omitempty"`
	Bands     []ComplexityBand `json:"bands,omitempty"`
	MinTokens int              `json:"min_tokens,omitempty"` // user message, estimated
	MaxTokens int              `json:"max_tokens,omitempty"`
	Keywords  []string         `json:"keywords,omitempty"`
	Tools     ToolsCondition   `json:"tools,omitempty"`
	Target    ModelRef         `json:"target"` // model or tier
}

// LLMRouting is llm_routing.json.
type LLMRouting struct {
	Models  []LLMModel  `json:"models"`
	Routers []LLMRouter `json:"routers"`
	Default ModelRef    `json:"default,omitempty"`
	// Not omitempty: an empty map must still reach the frontend as {}
	// (omitted, it arrived as undefined and crashed the Use-for table).
	Tasks map[string]ModelRef `json:"tasks"`
}

// FindModel returns the model with the given id, or nil.
func (r *LLMRouting) FindModel(id string) *LLMModel {
	for i := range r.Models {
		if r.Models[i].ID == id {
			return &r.Models[i]
		}
	}
	return nil
}

// FindRouter returns the router with the given id, or nil.
func (r *LLMRouting) FindRouter(id string) *LLMRouter {
	for i := range r.Routers {
		if r.Routers[i].ID == id {
			return &r.Routers[i]
		}
	}
	return nil
}

var llmRoutingMu sync.Mutex

func llmRoutingPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "llm_routing.json"), nil
}

// LoadLLMRouting reads llm_routing.json. When the file does not exist yet
// it is built from the provider accounts (each account's model becomes a
// model entry, the default account's model the default) and saved, so
// every startup path — desktop, service, dev server — migrates on first
// read.
func LoadLLMRouting() (LLMRouting, error) {
	llmRoutingMu.Lock()
	defer llmRoutingMu.Unlock()

	path, err := llmRoutingPath()
	if err != nil {
		return LLMRouting{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		accounts, aerr := loadRawLLMAccounts()
		if aerr != nil {
			return LLMRouting{}, aerr
		}
		routing := routingFromAccounts(accounts, defaultAccountID(accounts))
		if err := writeLLMRouting(path, routing); err != nil {
			return LLMRouting{}, err
		}
		return routing, nil
	}
	if err != nil {
		return LLMRouting{}, err
	}
	var routing LLMRouting
	if err := json.Unmarshal(data, &routing); err != nil {
		return LLMRouting{}, err
	}
	normalizeRouting(&routing)
	return routing, nil
}

// SaveLLMRouting writes llm_routing.json.
func SaveLLMRouting(routing LLMRouting) error {
	llmRoutingMu.Lock()
	defer llmRoutingMu.Unlock()
	path, err := llmRoutingPath()
	if err != nil {
		return err
	}
	normalizeRouting(&routing)
	return writeLLMRouting(path, routing)
}

func writeLLMRouting(path string, routing LLMRouting) error {
	data, err := json.MarshalIndent(routing, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data, 0o600)
}

// normalizeRouting keeps the JSON shape stable for the frontend: nil
// slices become empty arrays, unknown tiers become balanced.
func normalizeRouting(r *LLMRouting) {
	if r.Models == nil {
		r.Models = []LLMModel{}
	}
	if r.Routers == nil {
		r.Routers = []LLMRouter{}
	}
	if r.Tasks == nil {
		r.Tasks = map[string]ModelRef{}
	}
	for i := range r.Models {
		if !r.Models[i].Tier.Valid() {
			r.Models[i].Tier = GuessModelTier(r.Models[i].Name)
		}
	}
}

// defaultAccountID mirrors the legacy precedence: llm_config.json's
// default_account_id, then the account flagged is_default, then the first.
func defaultAccountID(accounts []LLMAccount) string {
	if cfg, err := LoadLLMConfig(); err == nil && cfg.DefaultAccountID != "" {
		if FindAccountByID(accounts, cfg.DefaultAccountID) != nil {
			return cfg.DefaultAccountID
		}
	}
	if acct := GetDefaultAccount(accounts); acct != nil {
		return acct.ID
	}
	return ""
}

// routingFromAccounts builds the initial registry from pre-routing
// accounts: one model per account (its configured model, or the
// provider's house default), the default account's model as default.
func routingFromAccounts(accounts []LLMAccount, defaultID string) LLMRouting {
	routing := LLMRouting{}
	for _, a := range accounts {
		name := a.Model
		if name == "" {
			name = DefaultModelForProvider(a.Provider)
		}
		if name == "" {
			continue
		}
		m := LLMModel{
			ID:            NewModelID(),
			AccountID:     a.ID,
			Name:          name,
			Tier:          GuessModelTier(name),
			SupportsTools: true,
			Enabled:       true,
		}
		routing.Models = append(routing.Models, m)
		if a.ID == defaultID {
			routing.Default = ModelRefTo(RefModel, m.ID)
		}
	}
	normalizeRouting(&routing)
	return routing
}

// NewModelID returns a short id for a registry model or router.
func NewModelID() string { return uuid.New().String()[:8] }

// DefaultModelForProvider returns the provider's house default when no
// explicit model is configured. Keep in sync with shared/llmDefaults.ts
// and DefaultPricing in pricing.go. Refreshed 2026-09-06 against each
// provider's live model list.
func DefaultModelForProvider(provider string) string {
	switch provider {
	case "openai":
		return "gpt-5.5"
	case "anthropic":
		return "claude-opus-5"
	case "ollama":
		// llama3.1 is the smallest Llama with tool support, which BRUV's
		// chat and agents rely on; plain llama3 silently ignores tools.
		return "llama3.1"
	default:
		return ""
	}
}

// GuessModelTier classifies a provider model id by its name. Only a
// starting point: the user sets the tier on the model row.
func GuessModelTier(name string) ModelTier {
	n := strings.ToLower(name)
	for _, s := range []string{"mini", "nano", "haiku", "flash", "lite", "small", "tiny", ":1b", ":3b", "-1b", "-3b", "8b"} {
		if strings.Contains(n, s) {
			return TierFast
		}
	}
	for _, s := range []string{"opus", "fable", "-pro", "ultra", "large", "70b", "405b"} {
		if strings.Contains(n, s) {
			return TierPowerful
		}
	}
	return TierBalanced
}
