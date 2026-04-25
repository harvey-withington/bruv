// Package llm is the LLMService — config + accounts + provider
// resolution + token pricing + health probes. Named llm at the service
// layer; uses internal/llm as the underlying provider library.
//
// loadLLMProvider / loadLLMProviderForAccount are the entry points
// other services (chat, agent) use to obtain a configured provider.
// When those services are extracted they'll receive this service via
// their Deps interface; for now App forwards to it.
package llm

import (
	"bruv/internal/config"
	"bruv/internal/llm"
	"context"
	"fmt"
	"time"
)

// Deps is the narrow host contract: a context source for bounding
// test-connection probes. The service is otherwise stateless.
type Deps interface {
	Ctx() context.Context
}

// Service exposes LLM configuration and provider resolution.
type Service struct{ deps Deps }

// New constructs an LLMService.
func New(deps Deps) *Service { return &Service{deps: deps} }

// --- Config ---

func (s *Service) GetConfig() (config.LLMConfig, error) { return config.LoadLLMConfig() }
func (s *Service) SetConfig(c config.LLMConfig) error   { return config.SaveLLMConfig(c) }

// --- Accounts ---

func (s *Service) GetAccounts() ([]config.LLMAccount, error)     { return config.LoadLLMAccounts() }
func (s *Service) SaveAccounts(a []config.LLMAccount) error      { return config.SaveLLMAccounts(a) }

// TestAccountConnection probes a configured account with a minimal
// prompt and returns the model name echoed back on success.
func (s *Service) TestAccountConnection(accountID string) (string, error) {
	accounts, err := config.LoadLLMAccounts()
	if err != nil {
		return "", err
	}
	acct := config.FindAccountByID(accounts, accountID)
	if acct == nil {
		return "", fmt.Errorf("account not found")
	}
	provider, err := llm.NewProvider(acct.Provider, acct.APIKey, acct.BaseURL)
	if err != nil {
		return "", err
	}
	modelName := acct.Model
	if modelName == "" {
		modelName = DefaultModelForProvider(acct.Provider)
	}
	ctx, cancel := context.WithTimeout(s.deps.Ctx(), 30*time.Second)
	defer cancel()
	resp, err := provider.ChatCompletion(ctx, llm.ChatRequest{
		SystemPrompt: "You are a test. Reply with exactly: OK",
		Messages:     []llm.Message{{Role: "user", Content: "Hello"}},
		Model:        modelName,
	})
	if err != nil {
		return "", err
	}
	return resp.Model, nil
}

// --- Provider resolution ---

// LoadProvider resolves the default provider (no account ID, no override).
func (s *Service) LoadProvider() (config.LLMConfig, llm.Provider, error) {
	return s.LoadProviderForAccount("", "")
}

// LoadProviderForAccount resolves a provider following this precedence:
//  1. Explicit account ID (if non-empty)
//  2. Default account recorded in llm_config.json
//  3. First account in llm_accounts.json
//  4. Legacy single-provider fields in llm_config.json
//
// Returns (cfg, nil, nil) when nothing is configured — callers check
// the provider for nil.
func (s *Service) LoadProviderForAccount(accountID, modelOverride string) (config.LLMConfig, llm.Provider, error) {
	cfg, err := config.LoadLLMConfig()
	if err != nil {
		return cfg, nil, nil
	}

	accounts, _ := config.LoadLLMAccounts()
	var acct *config.LLMAccount

	if accountID != "" {
		acct = config.FindAccountByID(accounts, accountID)
	}
	if acct == nil && cfg.DefaultAccountID != "" {
		acct = config.FindAccountByID(accounts, cfg.DefaultAccountID)
	}
	if acct == nil {
		acct = config.GetDefaultAccount(accounts)
	}

	if acct != nil {
		provider, err := llm.NewProvider(acct.Provider, acct.APIKey, acct.BaseURL)
		if err != nil {
			return cfg, nil, nil
		}
		model := modelOverride
		if model == "" {
			model = acct.Model
		}
		if model == "" {
			model = DefaultModelForProvider(acct.Provider)
		}
		cfg.Model = model
		cfg.Provider = acct.Provider
		return cfg, provider, nil
	}

	// Legacy fallback
	if cfg.Provider == "" {
		return cfg, nil, nil
	}
	provider, err := llm.NewProvider(cfg.Provider, cfg.APIKey, cfg.BaseURL)
	if err != nil {
		return cfg, nil, nil
	}
	return cfg, provider, nil
}

// --- Health ---

// IsConfigured returns true when any LLM credentials are present.
func (s *Service) IsConfigured() bool {
	cfg, err := config.LoadLLMConfig()
	if err != nil {
		return false
	}
	if cfg.Provider != "" {
		return true
	}
	accounts, err := config.LoadLLMAccounts()
	if err != nil {
		return false
	}
	return len(accounts) > 0
}

// TestConnection probes the legacy single-provider configuration.
func (s *Service) TestConnection() (string, error) {
	cfg, err := config.LoadLLMConfig()
	if err != nil {
		return "", err
	}
	if cfg.Provider == "" {
		return "", fmt.Errorf("no provider configured")
	}
	provider, err := llm.NewProvider(cfg.Provider, cfg.APIKey, cfg.BaseURL)
	if err != nil {
		return "", err
	}
	modelName := cfg.Model
	if modelName == "" {
		modelName = DefaultModelForProvider(cfg.Provider)
	}
	ctx, cancel := context.WithTimeout(s.deps.Ctx(), 30*time.Second)
	defer cancel()
	resp, err := provider.ChatCompletion(ctx, llm.ChatRequest{
		SystemPrompt: "You are a test. Reply with exactly: OK",
		Messages:     []llm.Message{{Role: "user", Content: "Hello"}},
		Model:        modelName,
	})
	if err != nil {
		return "", err
	}
	return resp.Model, nil
}

// --- Token pricing ---

func (s *Service) GetPricing() (map[string]config.ModelPricing, error) {
	return config.LoadCustomPricing()
}
func (s *Service) SavePricing(p map[string]config.ModelPricing) error {
	return config.SaveCustomPricing(p)
}

// DefaultModelForProvider returns the provider's house default when
// no explicit model is configured on the account or legacy config.
func DefaultModelForProvider(provider string) string {
	switch provider {
	case "openai":
		return "gpt-4o"
	case "anthropic":
		return "claude-sonnet-4-20250514"
	case "ollama":
		return "llama3"
	default:
		return ""
	}
}
