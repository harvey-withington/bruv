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
	"errors"
	"os"

	"bruv/internal/config"
	"bruv/internal/fsutil"
)

// LoadUserTypeStore reads the repo-scoped card types store. Returns an
// empty store (not an error) only when the file does not exist — that's
// the normal state for a fresh repo before anything has been saved. Any
// other read/parse failure is returned, and callers must not save (or
// reseed) over a store they failed to read.
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

// SaveUserTypeStore atomically writes the repo-scoped card types store.
// It is a blind replace; for a read-modify-write use UpdateUserTypeStore
// so concurrent edits aren't lost.
func (r *Repository) SaveUserTypeStore(store config.UserTypeStore) error {
	unlock := lockPath(r.cardTypesPath())
	defer unlock()
	return r.writeUserTypeStoreLocked(store)
}

// UpdateUserTypeStore loads the card types store under its file lock,
// applies fn and saves the result. A failed load aborts before fn runs,
// so an unreadable store is never overwritten. An error from fn aborts
// without writing; ErrNoChange from fn skips the write (nil error).
func (r *Repository) UpdateUserTypeStore(fn func(store *config.UserTypeStore) error) (config.UserTypeStore, error) {
	unlock := lockPath(r.cardTypesPath())
	defer unlock()

	store, err := r.LoadUserTypeStore()
	if err != nil {
		return config.UserTypeStore{}, err
	}
	if err := fn(&store); err != nil {
		if errors.Is(err, ErrNoChange) {
			return store, nil
		}
		return config.UserTypeStore{}, err
	}
	if err := r.writeUserTypeStoreLocked(store); err != nil {
		return config.UserTypeStore{}, err
	}
	return store, nil
}

func (r *Repository) writeUserTypeStoreLocked(store config.UserTypeStore) error {
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(r.cardTypesPath(), data, 0o644)
}
