package main

// Wails-bound forwarders for LLM config, accounts, token pricing, and
// health probes. Domain logic lives in core/services/llm.
//
// loadLLMProvider / loadLLMProviderForAccount remain as App-level
// helpers because app_chat.go and app_agent.go still call them
// directly; they'll migrate to receiving the llm.Service via their
// Deps when chat and agent are extracted into services.

import (
	llmsvc "bruv/core/services/llm"
	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/notify"
)

// --- Wails-bound forwarders ---

func (a *App) GetLLMConfig() (config.LLMConfig, error)       { return a.llmService.GetConfig() }
func (a *App) SetLLMConfig(c config.LLMConfig) error         { return a.llmService.SetConfig(c) }
func (a *App) GetLLMAccounts() ([]config.LLMAccount, error)  { return a.llmService.GetAccounts() }
func (a *App) SaveLLMAccounts(x []config.LLMAccount) error   { return a.llmService.SaveAccounts(x) }
func (a *App) TestLLMAccountConnection(id string) (string, error) {
	return a.llmService.TestAccountConnection(id)
}
func (a *App) IsLLMConfigured() bool               { return a.llmService.IsConfigured() }
func (a *App) TestLLMConnection() (string, error)  { return a.llmService.TestConnection() }
func (a *App) GetTokenPricing() (map[string]config.ModelPricing, error) {
	return a.llmService.GetPricing()
}
func (a *App) SaveTokenPricing(p map[string]config.ModelPricing) error {
	return a.llmService.SavePricing(p)
}

// TestSystemNotification is wired here because the button that calls
// it sits in the LLM settings panel. It belongs in NotifyService
// conceptually but the internal/notify package already exposes it
// directly.
func (a *App) TestSystemNotification() error {
	return notify.TestSystemNotification()
}

// --- Internal helpers used by chat and agent execution paths ---

// loadLLMProvider resolves the default provider.
func (a *App) loadLLMProvider() (config.LLMConfig, llm.Provider, error) {
	return a.llmService.LoadProvider()
}

// loadLLMProviderForAccount resolves a provider with optional account
// ID and model override. Precedence documented on the service method.
func (a *App) loadLLMProviderForAccount(accountID, modelOverride string) (config.LLMConfig, llm.Provider, error) {
	return a.llmService.LoadProviderForAccount(accountID, modelOverride)
}

// defaultModelForProvider is kept as a package-level helper because
// app_chat.go and app_agent.go reference it directly. The canonical
// implementation lives in core/services/llm.
func defaultModelForProvider(provider string) string {
	return llmsvc.DefaultModelForProvider(provider)
}

// listCardTypeIDs stays on App because it's not an LLM concern — it
// returns the schema registry IDs used by agent tool builders. When
// the catalog service is extracted it will own this.
func (a *App) listCardTypeIDs() []string {
	if a.registry != nil {
		return a.registry.List()
	}
	return nil
}
