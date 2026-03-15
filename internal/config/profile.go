package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// UserProfile holds personal context that can be shared with LLMs.
type UserProfile struct {
	DisplayName string   `json:"display_name"`
	Role        string   `json:"role"`        // e.g. "Software Engineer", "Content Creator"
	Bio         string   `json:"bio"`         // Short personal description
	Expertise   []string `json:"expertise"`   // Areas of knowledge / skill
	Context     string   `json:"context"`     // Freeform text for additional LLM context
}

func profilePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profile.json"), nil
}

// LoadProfile reads the user profile from disk, returning an empty profile if not found.
func LoadProfile() (UserProfile, error) {
	var p UserProfile
	path, err := profilePath()
	if err != nil {
		return p, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return p, nil
		}
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return UserProfile{}, err
	}
	return p, nil
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
