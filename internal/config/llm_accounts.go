package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// LLMAccount represents a configured AI provider with credentials.
type LLMAccount struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Provider  string `json:"provider"`
	Model     string `json:"model,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	IsDefault bool   `json:"is_default"`
}

func llmAccountsPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "llm_accounts.json"), nil
}

// LoadLLMAccounts reads the accounts list from disk, returning an empty slice if not found.
func LoadLLMAccounts() ([]LLMAccount, error) {
	path, err := llmAccountsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []LLMAccount{}, nil
		}
		return nil, err
	}
	var accounts []LLMAccount
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil, err
	}
	if accounts == nil {
		accounts = []LLMAccount{}
	}
	return accounts, nil
}

// SaveLLMAccounts writes the accounts list to disk.
// Enforces exactly one default if the list is non-empty.
func SaveLLMAccounts(accounts []LLMAccount) error {
	if len(accounts) > 0 {
		ensureOneDefault(accounts)
	}
	path, err := llmAccountsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// GetDefaultAccount returns the default account, or nil if none.
func GetDefaultAccount(accounts []LLMAccount) *LLMAccount {
	for i := range accounts {
		if accounts[i].IsDefault {
			return &accounts[i]
		}
	}
	if len(accounts) > 0 {
		return &accounts[0]
	}
	return nil
}

// FindAccountByID returns the account with the given ID, or nil.
func FindAccountByID(accounts []LLMAccount, id string) *LLMAccount {
	for i := range accounts {
		if accounts[i].ID == id {
			return &accounts[i]
		}
	}
	return nil
}

// MigrateLegacyLLMConfig checks if llm_accounts.json exists; if not and
// llm_config.json has a provider configured, creates an initial account.
func MigrateLegacyLLMConfig() error {
	path, err := llmAccountsPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}

	cfg, err := LoadLLMConfig()
	if err != nil {
		return SaveLLMAccounts([]LLMAccount{})
	}

	if cfg.Provider == "" {
		return SaveLLMAccounts([]LLMAccount{})
	}

	// Create account from legacy config
	account := LLMAccount{
		ID:        uuid.New().String()[:8],
		Label:     capitalizeProvider(cfg.Provider),
		Provider:  cfg.Provider,
		Model:     cfg.Model,
		APIKey:    cfg.APIKey,
		BaseURL:   cfg.BaseURL,
		IsDefault: true,
	}

	// Update llm_config with the new account ID reference
	cfg.DefaultAccountID = account.ID
	_ = SaveLLMConfig(cfg)

	return SaveLLMAccounts([]LLMAccount{account})
}

func capitalizeProvider(p string) string {
	switch p {
	case "openai":
		return "OpenAI"
	case "anthropic":
		return "Anthropic"
	case "ollama":
		return "Ollama"
	default:
		if len(p) > 0 {
			return strings.ToUpper(p[:1]) + p[1:]
		}
		return p
	}
}

func ensureOneDefault(accounts []LLMAccount) {
	hasDefault := false
	for _, a := range accounts {
		if a.IsDefault {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		accounts[0].IsDefault = true
	}
}
