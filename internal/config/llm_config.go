package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LLMConfig holds AI-specific settings, separate from the user profile.
type LLMConfig struct {
	Context string `json:"context"` // Freeform text for LLM system prompts
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
	return os.WriteFile(path, data, 0o644)
}
