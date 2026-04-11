package repo

// Repo-scoped card types / templates / built-in overrides.
//
// Card types live in the repo — at <root>/.bruv/card_types.json — so that
// a BRUV repo is self-contained and shareable. Types, reusable templates,
// and the per-repo customisations of built-in types all travel with the
// project data that references them.
//
// The JSON schema is the same as it was when the store lived in the
// user's global config folder, so we reuse the config.UserTypeStore type
// directly. The repo package is allowed to depend on config; the reverse
// would create an import cycle.

import (
	"encoding/json"
	"os"
	"path/filepath"

	"bruv/internal/config"
)

// LoadUserTypeStore reads the repo-scoped card types store. Returns an
// empty store (not an error) when the file does not exist — that's the
// normal state for a fresh repo before anything has been saved.
func (r *Repository) LoadUserTypeStore() (config.UserTypeStore, error) {
	var store config.UserTypeStore
	data, err := os.ReadFile(r.cardTypesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return store, err
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return config.UserTypeStore{}, err
	}
	return store, nil
}

// SaveUserTypeStore writes the repo-scoped card types store. Ensures the
// .bruv directory exists — it normally does by the time any write
// happens, but fresh repos that skipped the metadata directory for any
// reason still get handled correctly.
func (r *Repository) SaveUserTypeStore(store config.UserTypeStore) error {
	path := r.cardTypesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
