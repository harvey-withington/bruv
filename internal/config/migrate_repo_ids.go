package config

import (
	"log/slog"
)

// MigrateRepoIDsToManifest aligns each registry entry's ID with its
// repo's manifest ID. Pre-unification, AppendRepo minted a fresh
// UUID for the registry while repo.InitAt minted a separate UUID for
// the manifest, so the two IDs for the same repo were always
// different — code that compared one against the other (the rename
// flow's "is this the active runtime" check, e.g.) silently failed
// the comparison.
//
// Idempotent: entries already aligned (registry.ID == manifest.ID),
// or where the manifest is missing / unreadable, are left alone. The
// repo-recents.json mapping (per-connection "last picked" pointer)
// also gets remapped so existing pointers still resolve.
func MigrateRepoIDsToManifest() {
	store, err := LoadRepos()
	if err != nil {
		slog.Warn("repo-id migration: load repos failed", "err", err)
		return
	}
	remap := map[string]string{} // oldID -> newID
	changed := false
	for i := range store.Repos {
		mID := readManifestID(store.Repos[i].Path)
		if mID == "" || mID == store.Repos[i].ID {
			continue
		}
		remap[store.Repos[i].ID] = mID
		store.Repos[i].ID = mID
		changed = true
	}
	if !changed {
		return
	}
	if err := SaveRepos(store); err != nil {
		slog.Warn("repo-id migration: save repos failed", "err", err)
		return
	}
	// Remap repo-recents.json pointers — anything pointing at an old
	// registry ID becomes a pointer at the manifest ID. Best-effort.
	recents, err := LoadRepoRecents()
	if err == nil && len(recents) > 0 {
		recentsChanged := false
		for connID, repoID := range recents {
			if newID, ok := remap[repoID]; ok {
				recents[connID] = newID
				recentsChanged = true
			}
		}
		if recentsChanged {
			if err := SaveRepoRecents(recents); err != nil {
				slog.Warn("repo-id migration: save repo-recents failed", "err", err)
			}
		}
	}
	slog.Info("repo-id migration: done", "remapped", len(remap))
}
