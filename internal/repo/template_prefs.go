package repo

// Vault-level slide-template preferences.
//
// Auto template selection (plan/2026-07-31 per-platform slide templates and
// auto matching.md) is tunable per vault: a priority order for multi-match
// URLs and per-template urlHint regex overrides. The prefs live in the repo
// — <root>/template_prefs.json, like card_types.json — because Present
// resolves templates against this vault's server, and the choice of "how my
// captured posts look" travels with the data it styles.
//
// The server never interprets these values (matching runs in the two
// renderers, which share shared/slideTemplates.ts semantics); it only loads,
// saves, and hands them to the present page inside the PresentCardJSON
// payload. Regex validity is a renderer concern — an uncompilable override
// falls back to the template's built-in hint there.

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// TemplatePrefs mirrors shared/types.ts TemplatePrefs — JSON key casing must
// stay in lockstep, the frontend consumes this verbatim.
type TemplatePrefs struct {
	// Priority for Auto multi-match: template ids, first match wins;
	// unlisted templates follow in registration order.
	Order []string `json:"order,omitempty"`
	// Per-template urlHint replacement (regex source), keyed by template id.
	URLOverrides map[string]string `json:"urlOverrides,omitempty"`
}

// TemplatePrefsStore is the on-disk wrapper — a version marker around the
// prefs so future schema additions don't break the file format.
type TemplatePrefsStore struct {
	Version int `json:"version"`
	TemplatePrefs
}

func (r *Repository) templatePrefsPath() string {
	return filepath.Join(r.Root, "template_prefs.json")
}

// LoadTemplatePrefs reads the vault's template prefs. A missing file is the
// normal fresh-vault state and returns empty prefs, not an error.
func (r *Repository) LoadTemplatePrefs() (TemplatePrefs, error) {
	data, err := os.ReadFile(r.templatePrefsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return TemplatePrefs{}, nil
		}
		return TemplatePrefs{}, err
	}
	var store TemplatePrefsStore
	if err := json.Unmarshal(data, &store); err != nil {
		return TemplatePrefs{}, err
	}
	return store.TemplatePrefs, nil
}

// SaveTemplatePrefs writes the vault's template prefs.
func (r *Repository) SaveTemplatePrefs(p TemplatePrefs) error {
	store := TemplatePrefsStore{Version: 1, TemplatePrefs: p}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.templatePrefsPath(), data, 0o644)
}
