package config

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
)

// UserProfile holds the user's editable identity, visible to collaborators and LLMs.
type UserProfile struct {
	DisplayName string   `json:"display_name"`
	Role        string   `json:"role"`         // e.g. "Software Engineer", "Content Creator"
	Bio         string   `json:"bio"`          // Short personal description
	Expertise   []string `json:"expertise"`    // Areas of knowledge / skill
	AvatarURL   string   `json:"avatar_url"`   // Profile picture URL or path
}

func profilePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profile.json"), nil
}

// LoadProfile reads the user profile from disk.
// On first load (no file), auto-populates DisplayName from the OS account.
func LoadProfile() (UserProfile, error) {
	var p UserProfile
	path, err := profilePath()
	if err != nil {
		return p, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			p.DisplayName = osDisplayName()
			return p, nil
		}
		return p, err
	}

	// Unmarshal into a raw map first to detect legacy "context" field
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return UserProfile{}, err
	}

	// Migrate legacy "context" field to llm_config.json
	if ctxRaw, ok := raw["context"]; ok {
		var ctxStr string
		if json.Unmarshal(ctxRaw, &ctxStr) == nil && ctxStr != "" {
			existing, _ := LoadLLMConfig()
			if existing.Context == "" {
				existing.Context = ctxStr
				_ = SaveLLMConfig(existing)
			}
		}
		delete(raw, "context")
		// Re-marshal without context and rewrite profile
		cleaned, _ := json.MarshalIndent(raw, "", "  ")
		_ = os.WriteFile(path, cleaned, 0o644)
	}

	if err := json.Unmarshal(data, &p); err != nil {
		return UserProfile{}, err
	}
	return p, nil
}

// osDisplayName returns the current OS user's display name, or empty string on failure.
func osDisplayName() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	if u.Name != "" {
		return u.Name
	}
	return u.Username
}

// SaveProfile writes the user profile to disk.
func SaveProfile(p UserProfile) error {
	path, err := profilePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
