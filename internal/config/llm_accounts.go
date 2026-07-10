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

// loadRawLLMAccounts reads the JSON file without hydrating keys from the
// keychain. Used by the migration path, which needs to see the raw disk
// state before rewriting anything.
func loadRawLLMAccounts() ([]LLMAccount, error) {
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

// LoadLLMAccounts reads the accounts list from disk and hydrates any API
// keys from the OS keychain. An account whose APIKey is already populated
// in the JSON (legacy pre-migration state) is returned as-is. An account
// whose APIKey is empty is looked up in the keychain and filled in if a
// matching entry exists.
func LoadLLMAccounts() ([]LLMAccount, error) {
	accounts, err := loadRawLLMAccounts()
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		if accounts[i].APIKey == "" {
			if secret, err := getKeychainSecret(accounts[i].ID); err == nil && secret != "" {
				accounts[i].APIKey = secret
			}
		}
	}
	return accounts, nil
}

// SaveLLMAccounts writes the accounts list to disk. For each account, the
// API key is moved into the OS keychain and blanked out of the JSON
// representation before writing. Accounts that have been removed since
// the last save have their keychain entries purged.
//
// If the OS keychain is unavailable, the code falls back to writing the
// API key in plaintext exactly as before — the goal is to upgrade
// security when possible, never to lose data.
//
// Enforces exactly one default if the list is non-empty.
func SaveLLMAccounts(accounts []LLMAccount) error {
	if len(accounts) > 0 {
		ensureOneDefault(accounts)
	}

	// Diff against the previous on-disk state to catch removals. Keychain
	// entries for accounts that no longer exist in the new list are purged
	// so we don't accumulate orphaned secrets over time.
	previous, _ := loadRawLLMAccounts()
	newIDs := make(map[string]bool, len(accounts))
	for _, a := range accounts {
		newIDs[a.ID] = true
	}
	for _, old := range previous {
		if !newIDs[old.ID] {
			deleteKeychainSecret(old.ID)
		}
	}

	// Build the JSON-facing copy. Each account's key is stored in the
	// keychain if possible; if that fails we keep the plaintext APIKey so
	// the account stays functional.
	jsonAccounts := make([]LLMAccount, len(accounts))
	for i, a := range accounts {
		jsonAccounts[i] = a
		if a.APIKey != "" {
			if err := storeKeychainSecret(a.ID, a.APIKey); err == nil {
				jsonAccounts[i].APIKey = ""
			}
		}
	}

	path, err := llmAccountsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(jsonAccounts, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data, 0o600)
}

// MigrateLLMKeysToKeychain moves any plaintext API keys from llm_accounts.json
// into the OS keychain. Idempotent and safe to call on every startup: if
// the keychain is unavailable it's a no-op, and if no plaintext keys are
// present the file is not rewritten.
//
// This is the one-way upgrade path for users who installed BRUV before
// the keychain backend existed. Once a user's keys have been migrated,
// subsequent saves keep them in the keychain automatically via the
// normal SaveLLMAccounts path.
func MigrateLLMKeysToKeychain() error {
	if !KeychainAvailable() {
		return nil
	}
	accounts, err := loadRawLLMAccounts()
	if err != nil || len(accounts) == 0 {
		return err
	}
	needsRewrite := false
	for _, a := range accounts {
		if a.APIKey != "" {
			needsRewrite = true
			break
		}
	}
	if !needsRewrite {
		return nil
	}
	// Re-save through the normal path, which moves keys into the keychain
	// and blanks them out of the JSON.
	return SaveLLMAccounts(accounts)
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
