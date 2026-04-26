package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// MigrateRecentsToRegistry imports the legacy recent.json into
// repos.json on first boot post-unification. After the user opened
// each repo at least once on the new code path, the recents list is
// what the old picker used as the user's "set of repos on this
// machine"; the registry now serves that role. Repos that no longer
// exist on disk get logged + skipped — we don't want to register
// dangling paths.
//
// Idempotent: deletes recent.json after a successful migration so it
// doesn't run again. Safe to call on every startup.
func MigrateRecentsToRegistry() {
	clientDir, err := ClientDataDir()
	if err != nil {
		return
	}
	srcPath := filepath.Join(clientDir, "recent.json")
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return // nothing to migrate (or unreadable — either way, no-op)
	}
	var recents []legacyRecentRepo
	if err := json.Unmarshal(data, &recents); err != nil {
		slog.Warn("recents migration: parse failed", "err", err)
		return
	}
	imported := 0
	for _, r := range recents {
		if r.Path == "" {
			continue
		}
		// Skip vanished paths — registering them would surface as
		// noisy "unreachable" entries in the picker.
		if _, statErr := os.Stat(r.Path); statErr != nil {
			slog.Info("recents migration: skip missing path", "path", r.Path)
			continue
		}
		if _, appendErr := AppendRepo(r.Path, r.Name); appendErr != nil {
			slog.Warn("recents migration: append failed", "path", r.Path, "err", appendErr)
			continue
		}
		imported++
	}
	if removeErr := os.Remove(srcPath); removeErr != nil {
		slog.Warn("recents migration: remove old file failed", "err", removeErr)
	}
	slog.Info("recents migration: done", "imported", imported, "total", len(recents))
}

// legacyRecentRepo mirrors the old RecentRepo shape — kept here, NOT
// in the public package surface, since recent.go is being deleted.
// LastOpened is ignored — the registry doesn't track open recency.
type legacyRecentRepo struct {
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	LastOpened time.Time `json:"last_opened"`
}
