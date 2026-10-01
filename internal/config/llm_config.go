package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LLMConfig holds AI-specific settings, separate from the user profile.
type LLMConfig struct {
	Context          string `json:"context"`                      // Freeform text for LLM system prompts
	Provider         string `json:"provider,omitempty"`           // "openai", "anthropic", "ollama", "" (legacy, prefer accounts)
	Model            string `json:"model,omitempty"`              // e.g. "gpt-5.5" (legacy, prefer accounts)
	APIKey           string `json:"api_key,omitempty"`            // plain text (legacy, prefer accounts)
	BaseURL          string `json:"base_url,omitempty"`           // custom endpoint override (legacy, prefer accounts)
	DefaultAccountID string `json:"default_account_id,omitempty"` // references LLMAccount.ID
	AIMode           string `json:"ai_mode,omitempty"`            // "edit" (default), "suggest", or "chat"
	MinConfidence    string `json:"min_confidence,omitempty"`     // "high", "medium", "low", "" (any)
	ChatBudgetMode   string `json:"chat_budget_mode,omitempty"`   // ChatBudgetWarn (default), ChatBudgetHard, ChatBudgetOff
	ChatBudgetTokens int    `json:"chat_budget_tokens,omitempty"` // 0 = DefaultChatBudgetTokens
}

// Chat budget modes: what happens when a chat turn's output passes the
// budget.
const (
	ChatBudgetWarn = "warn" // keep going, tell the user, let them stop it
	ChatBudgetHard = "hard" // cut the reply off at the budget
	ChatBudgetOff  = "off"  // no budget
)

// DefaultChatBudgetTokens leaves a thinking model room to think and
// still write a long answer.
const DefaultChatBudgetTokens = 16000

// MaxChatOutputTokens is the most one chat call may generate: the cap
// under the warn and off modes, and the largest budget allowed.
const MaxChatOutputTokens = 64000

// ChatBudget is a chat turn's output-token budget: thinking, text and
// tool calls across every model call in the turn.
type ChatBudget struct {
	Mode   string
	Tokens int
}

// ChatBudget resolves the configured budget, defaults filled in.
func (c LLMConfig) ChatBudget() ChatBudget {
	b := ChatBudget{Mode: c.ChatBudgetMode, Tokens: c.ChatBudgetTokens}
	if b.Mode != ChatBudgetHard && b.Mode != ChatBudgetOff {
		b.Mode = ChatBudgetWarn
	}
	if b.Tokens <= 0 {
		b.Tokens = DefaultChatBudgetTokens
	}
	b.Tokens = min(b.Tokens, MaxChatOutputTokens)
	return b
}

// confidenceOrder maps confidence strings to numeric rank (higher = stricter).
var confidenceOrder = map[string]int{"": 0, "low": 1, "medium": 2, "high": 3}

// ConfidenceMeetsThreshold returns true when the suggestion confidence is at or above
// the minimum configured threshold. Empty min means accept any confidence.
func ConfidenceMeetsThreshold(confidence, min string) bool {
	minScore, ok := confidenceOrder[min]
	if !ok || minScore == 0 {
		return true
	}
	return confidenceOrder[confidence] >= minScore
}

func llmConfigPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "llm_config.json"), nil
}

// LoadLLMConfig reads the LLM config from disk, returning an empty config if not found.
func LoadLLMConfig() (LLMConfig, error) {
	var c LLMConfig
	path, err := llmConfigPath()
	if err != nil {
		return c, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return LLMConfig{}, err
	}
	return c, nil
}

// SaveLLMConfig writes the LLM config to disk.
func SaveLLMConfig(c LLMConfig) error {
	path, err := llmConfigPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data, 0o644)
}
