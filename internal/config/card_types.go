package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"bruv/internal/model"
)

// UserCardType is a globally-stored card type created by the user.
type UserCardType struct {
	ID          string `json:"id"`                    // auto-slugged from label, immutable after create
	Label       string `json:"label"`
	Color       string `json:"color"`                 // hex, e.g. "#6366f1"
	Icon        string `json:"icon,omitempty"`         // lucide icon name, e.g. "rocket"
	Description string `json:"description"`
	AIHint      string `json:"ai_hint,omitempty"`
	TemplateID  string `json:"template_id,omitempty"` // references CardTemplate.ID
}

// CardTemplate is a globally-stored, reusable block layout shared across card types.
type CardTemplate struct {
	ID     string        `json:"id"`   // uuid
	Name   string        `json:"name"`
	Blocks []model.Block `json:"blocks"`
}

// BuiltinOverride stores user customisations for a built-in card type.
type BuiltinOverride struct {
	Color      string `json:"color,omitempty"`
	TemplateID string `json:"template_id,omitempty"`
}

// UserTypeStore is the on-disk root for card_types.json.
type UserTypeStore struct {
	Seeded                bool                       `json:"seeded"`
	StarterTemplatesSeeded bool                      `json:"starter_templates_seeded,omitempty"`
	Types                 []UserCardType             `json:"types"`
	Templates             []CardTemplate             `json:"templates"`
	BuiltinOverrides      map[string]BuiltinOverride `json:"builtin_overrides,omitempty"`
}

func userTypesPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "card_types.json"), nil
}

// LoadUserTypeStore reads user card types and templates from disk.
// Returns an empty store (not an error) when the file does not exist.
func LoadUserTypeStore() (UserTypeStore, error) {
	var store UserTypeStore
	path, err := userTypesPath()
	if err != nil {
		return store, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return store, err
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return UserTypeStore{}, err
	}
	return store, nil
}

// SaveUserTypeStore writes user card types and templates to disk.
func SaveUserTypeStore(store UserTypeStore) error {
	path, err := userTypesPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
