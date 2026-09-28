// Package llm is the LLMService — AI behaviour config, provider
// accounts, the model registry, model selection (Select, select.go),
// model discovery and health probes. Named llm at the service layer;
// uses internal/llm as the underlying provider library.
package llm

import (
	"bruv/internal/config"
	"bruv/internal/llm"
	"context"
	"fmt"
	"time"
)

// Deps is the narrow host contract: a context source for bounding
// probes. The service is otherwise stateless.
type Deps interface {
	Ctx() context.Context
}

// Service exposes LLM configuration and model selection.
type Service struct{ deps Deps }

// New constructs an LLMService.
func New(deps Deps) *Service { return &Service{deps: deps} }

// --- Config ---

func (s *Service) GetConfig() (config.LLMConfig, error) { return config.LoadLLMConfig() }
func (s *Service) SetConfig(c config.LLMConfig) error   { return config.SaveLLMConfig(c) }

// --- Providers (accounts) and the model registry ---

func (s *Service) GetAccounts() ([]config.LLMAccount, error) { return config.LoadLLMAccounts() }
func (s *Service) SaveAccounts(a []config.LLMAccount) error  { return config.SaveLLMAccounts(a) }

func (s *Service) GetRouting() (config.LLMRouting, error) { return config.LoadLLMRouting() }
func (s *Service) SaveRouting(r config.LLMRouting) error  { return config.SaveLLMRouting(r) }

// DiscoveredModel is a model a provider offers, with a tier guessed
// from its name for the settings UI to pre-fill.
type DiscoveredModel struct {
	ID    string           `json:"id"`
	Label string           `json:"label,omitempty"`
	Tier  config.ModelTier `json:"tier"`
}

// DiscoverModels lists the models a provider account offers.
func (s *Service) DiscoverModels(accountID string) ([]DiscoveredModel, error) {
	acct, err := findAccount(accountID)
	if err != nil {
		return nil, err
	}
	found, err := llm.ListModels(s.deps.Ctx(), acct.Provider, acct.APIKey, acct.BaseURL)
	if err != nil {
		return nil, err
	}
	out := make([]DiscoveredModel, len(found))
	for i, m := range found {
		out[i] = DiscoveredModel{ID: m.ID, Label: m.Label, Tier: config.GuessModelTier(m.ID)}
	}
	return out, nil
}

// TestModel probes one registry model with a minimal prompt and returns
// the model name the provider echoes back.
func (s *Service) TestModel(modelID string) (string, error) {
	routing, err := config.LoadLLMRouting()
	if err != nil {
		return "", err
	}
	m := routing.FindModel(modelID)
	if m == nil {
		return "", fmt.Errorf("model not found")
	}
	acct, err := findAccount(m.AccountID)
	if err != nil {
		return "", err
	}
	provider, err := llm.NewProvider(acct.Provider, acct.APIKey, acct.BaseURL)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(s.deps.Ctx(), 30*time.Second)
	defer cancel()
	resp, err := provider.ChatCompletion(ctx, llm.ChatRequest{
		SystemPrompt: "You are a test. Reply with exactly: OK",
		Messages:     []llm.Message{{Role: "user", Content: "Hello"}},
		Model:        m.Name,
	})
	if err != nil {
		return "", err
	}
	return resp.Model, nil
}

func findAccount(id string) (*config.LLMAccount, error) {
	accounts, err := config.LoadLLMAccounts()
	if err != nil {
		return nil, err
	}
	acct := config.FindAccountByID(accounts, id)
	if acct == nil {
		return nil, fmt.Errorf("provider not found")
	}
	return acct, nil
}

// --- Health ---

// IsConfigured returns true when any usable model or legacy provider
// is configured.
func (s *Service) IsConfigured() bool {
	cfg, err := config.LoadLLMConfig()
	if err != nil {
		return false
	}
	if cfg.Provider != "" {
		return true
	}
	routing, err := config.LoadLLMRouting()
	if err != nil {
		return false
	}
	for _, m := range routing.Models {
		if m.Enabled {
			return true
		}
	}
	return false
}

// ProviderSummary is a provider without its credentials — what model
// pickers need for group labels and order.
type ProviderSummary struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Provider string `json:"provider"`
}

// RegistryView is the model registry for pickers: routing plus the
// providers, with no API keys, so chat and agent pickers (including the
// phone) never receive secrets.
type RegistryView struct {
	Routing   config.LLMRouting `json:"routing"`
	Providers []ProviderSummary `json:"providers"`
}

// GetRegistryView loads the key-free registry view.
func (s *Service) GetRegistryView() (RegistryView, error) {
	routing, err := config.LoadLLMRouting()
	if err != nil {
		return RegistryView{}, err
	}
	accounts, err := config.LoadLLMAccounts()
	if err != nil {
		return RegistryView{}, err
	}
	providers := make([]ProviderSummary, len(accounts))
	for i, a := range accounts {
		providers[i] = ProviderSummary{ID: a.ID, Label: a.Label, Provider: a.Provider}
	}
	return RegistryView{Routing: routing, Providers: providers}, nil
}
