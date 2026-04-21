package main

// LLM provider wiring: config I/O, accounts CRUD, provider resolution,
// test-connection probes, and token pricing.
//
// Resolution order for "which provider answers this call" lives in
// loadLLMProviderForAccount — explicit account ID wins, then the
// configured default account, then the first account of the right
// provider, then the legacy single-provider fields in llm_config.json.
// The multi-account path is the long-term home; the legacy fallback
// stays for users still on pre-multi-account configs.
//
// Extracted from app.go so changes to provider loading don't require
// scrolling through 7k lines of unrelated method bodies.

import (
	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/notify"
	"context"
	"fmt"
	"time"
)

// --- LLM Config ---

func (a *App) GetLLMConfig() (config.LLMConfig, error) {
	return config.LoadLLMConfig()
}

func (a *App) SetLLMConfig(c config.LLMConfig) error {
	return config.SaveLLMConfig(c)
}

// --- LLM Accounts (multi-account support) ---

// GetLLMAccounts returns all configured AI accounts.
func (a *App) GetLLMAccounts() ([]config.LLMAccount, error) {
	return config.LoadLLMAccounts()
}

// SaveLLMAccounts persists the AI accounts list.
func (a *App) SaveLLMAccounts(accounts []config.LLMAccount) error {
	return config.SaveLLMAccounts(accounts)
}

// TestLLMAccountConnection tests connectivity for a specific account by ID.
func (a *App) TestLLMAccountConnection(accountID string) (string, error) {
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
		modelName = defaultModelForProvider(acct.Provider)
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
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

// loadLLMProvider loads config and creates a provider. Returns (cfg, provider, err).
// If LLM is not configured, provider is nil and err is nil.
func (a *App) loadLLMProvider() (config.LLMConfig, llm.Provider, error) {
	return a.loadLLMProviderForAccount("", "")
}

// loadLLMProviderForAccount resolves the LLM provider from:
// 1. Specific account (if accountID is set)
// 2. Default account from llm_accounts.json
// 3. Legacy fields in llm_config.json (backward compat)
func (a *App) loadLLMProviderForAccount(accountID, modelOverride string) (config.LLMConfig, llm.Provider, error) {
	cfg, err := config.LoadLLMConfig()
	if err != nil {
		return cfg, nil, nil
	}

	// Try accounts-based resolution
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
		// Use account credentials
		provider, err := llm.NewProvider(acct.Provider, acct.APIKey, acct.BaseURL)
		if err != nil {
			return cfg, nil, nil
		}
		// Determine model: override > account default > provider default
		model := modelOverride
		if model == "" {
			model = acct.Model
		}
		if model == "" {
			model = defaultModelForProvider(acct.Provider)
		}
		cfg.Model = model
		cfg.Provider = acct.Provider
		return cfg, provider, nil
	}

	// Legacy fallback: use fields directly from llm_config.json
	if cfg.Provider == "" {
		return cfg, nil, nil
	}
	provider, err := llm.NewProvider(cfg.Provider, cfg.APIKey, cfg.BaseURL)
	if err != nil {
		return cfg, nil, nil
	}
	return cfg, provider, nil
}

// --- Health checks ---

func (a *App) IsLLMConfigured() bool {
	cfg, err := config.LoadLLMConfig()
	if err != nil {
		return false
	}
	if cfg.Provider != "" {
		return true
	}
	// Check multi-account setup
	accounts, err := config.LoadLLMAccounts()
	if err != nil {
		return false
	}
	return len(accounts) > 0
}

func (a *App) TestLLMConnection() (string, error) {
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
		modelName = defaultModelForProvider(cfg.Provider)
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
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

// TestSystemNotification sends a test OS notification to verify desktop notifications work.
func (a *App) TestSystemNotification() error {
	return notify.TestSystemNotification()
}

// defaultModelForProvider returns the provider's house default when no
// model is set on the account or config. Used by both loadLLMProvider
// resolution and the test-connection probes.
func defaultModelForProvider(provider string) string {
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

// --- Token pricing ---

// GetTokenPricing returns the current token pricing configuration.
func (a *App) GetTokenPricing() (map[string]config.ModelPricing, error) {
	return config.LoadCustomPricing()
}

// SaveTokenPricing saves custom token pricing overrides.
func (a *App) SaveTokenPricing(pricing map[string]config.ModelPricing) error {
	return config.SaveCustomPricing(pricing)
}

// --- Card type IDs (LLM tool helper) ---

// listCardTypeIDs returns all registered card type IDs. Used by the
// chat/agent tool builders to expose the set of valid card types to
// the LLM in set_card_type and similar tools.
func (a *App) listCardTypeIDs() []string {
	if a.registry != nil {
		return a.registry.List()
	}
	return nil
}
