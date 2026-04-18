package repo

// Pre-v1.0b repo portability migration.
//
// Older BRUV repos were created before the repo folder was designed to be
// portable. Three things need moving on first open of an older repo so
// that sharing the repo folder actually works:
//
//  1. The manifest needs a stable repo ID. This is handled in Open()
//     itself so we can rely on r.Manifest.ID existing by the time we get
//     here; this file doesn't need to touch manifests.
//
//  2. User-defined card types + templates used to live at
//     %APPDATA%\bruv\card_types.json (global to the user). They need to
//     be copied into <repo>/.bruv/card_types.json so they travel with
//     the project. We copy (not move) because the global file is shared
//     across every repo the user has — we can't assume it's safe to
//     delete it just because one repo has been migrated.
//
//  3. Chat history (.messages.json) used to live inside the repo next
//     to each card. It needs to be moved into the OS config folder
//     keyed by repo ID so it doesn't follow the repo around when shared.
//     We move (not copy) because chats are unambiguously personal and
//     leaving them in the repo would defeat the whole point.
//
// The migration runs on every open and is idempotent — if everything is
// already in the right place it's a fast no-op. Callers pass in a
// configDir path and a function that returns the global pre-migration
// card types store; this keeps the repo package free of config imports
// (avoiding a cycle).

import (
	"bruv/internal/model"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// MigrateStats reports what a migration pass did so the caller can log
// it or surface it in the UI. Counts are cumulative for one open call.
type MigrateStats struct {
	CardTypesSeeded    bool // true if we seeded .bruv/card_types.json from somewhere
	ChatFilesMigrated  int  // count of .messages.json files moved out of the repo
	ChatFilesFailed    int  // count of files we couldn't move (logged)
}

// GlobalCardTypesProvider is called by the migration to fetch the user's
// global (pre-migration) card types store. It returns the raw JSON bytes
// of the old-location file, or nil if the file doesn't exist or can't be
// read. The app layer wires this up so the repo package doesn't depend
// on internal/config.
type GlobalCardTypesProvider func() ([]byte, error)

// ChatMover is called for each chat file we need to relocate. It
// receives the repo ID, the chat ID (either a real card ID or a
// __project__<id> synthetic), and a ChatFile loaded from the repo. The
// implementation is expected to persist the chat under the config
// folder keyed by repo. Returning an error logs and increments the
// failed count but does not abort the whole migration.
type ChatMover func(repoID, chatID string, cf *model.ChatFile) error

// MigrateOnOpen runs the pre-v1.0b portability migration for a
// freshly-opened repo. Safe to call every open; any step that's already
// done is a no-op.
//
//   - globalCardTypes: provider for the user's old global card_types.json.
//     May be nil, in which case the seed from the global location is
//     skipped and a fresh repo gets seeded from the app-level defaults
//     on first use instead.
//
//   - moveChat: destination writer for each chat file we find inside
//     the repo. Must be non-nil — if nil, chat migration is skipped.
func (r *Repository) MigrateOnOpen(globalCardTypes GlobalCardTypesProvider, moveChat ChatMover) MigrateStats {
	var stats MigrateStats

	// --- Step 1: seed .bruv/card_types.json from the global store ---
	//
	// Only runs when the repo doesn't already have its own store. New
	// repos that were created after this migration existed skip this
	// path entirely (their store is seeded by the app layer at create
	// time). Shared repos opened on another machine also skip — the
	// .bruv/card_types.json that came with the zip is authoritative.
	if _, err := os.Stat(r.cardTypesPath()); os.IsNotExist(err) {
		if globalCardTypes != nil {
			if raw, err := globalCardTypes(); err == nil && len(raw) > 0 {
				if err := os.MkdirAll(filepath.Dir(r.cardTypesPath()), 0o755); err == nil {
					if err := os.WriteFile(r.cardTypesPath(), raw, 0o644); err == nil {
						stats.CardTypesSeeded = true
						slog.Info("repo migrate: seeded card types from global",
							"path", r.cardTypesPath())
					}
				}
			}
		}
	}

	// --- Step 2: move any in-repo .messages.json chat files out ---
	//
	// We iterate the cards directory once and pick up anything ending in
	// .messages.json. Card-level chats have a real card ID prefix;
	// project-level chats use the synthetic __project__<id> prefix. The
	// mover doesn't care which — it gets the full chat ID either way.
	if moveChat != nil && r.Manifest != nil && r.Manifest.ID != "" {
		entries, err := os.ReadDir(r.cardsPath())
		if err == nil {
			for _, e := range entries {
				name := e.Name()
				if !strings.HasSuffix(name, ".messages.json") {
					continue
				}
				chatID := strings.TrimSuffix(name, ".messages.json")
				srcPath := filepath.Join(r.cardsPath(), name)

				data, err := os.ReadFile(srcPath)
				if err != nil {
					slog.Warn("repo migrate: read chat failed",
						"path", srcPath, "err", err)
					stats.ChatFilesFailed++
					continue
				}
				var cf model.ChatFile
				if err := json.Unmarshal(data, &cf); err != nil {
					slog.Warn("repo migrate: parse chat failed",
						"path", srcPath, "err", err)
					stats.ChatFilesFailed++
					continue
				}

				if err := moveChat(r.Manifest.ID, chatID, &cf); err != nil {
					slog.Warn("repo migrate: persist chat failed",
						"chat_id", chatID, "err", err)
					stats.ChatFilesFailed++
					continue
				}

				// Only remove the source after the destination write
				// succeeds. Failures leave the file in place so the
				// next open can retry.
				if err := os.Remove(srcPath); err != nil {
					slog.Warn("repo migrate: remove source chat failed",
						"path", srcPath, "err", err)
					// The chat is now in both places — not ideal, but
					// not lossy either. Count as success.
				}
				stats.ChatFilesMigrated++
			}
		}
	}

	return stats
}

// String is a compact one-line summary suitable for logging.
func (s MigrateStats) String() string {
	if !s.CardTypesSeeded && s.ChatFilesMigrated == 0 && s.ChatFilesFailed == 0 {
		return "no migration needed"
	}
	parts := []string{}
	if s.CardTypesSeeded {
		parts = append(parts, "card types seeded from global store")
	}
	if s.ChatFilesMigrated > 0 {
		parts = append(parts, fmt.Sprintf("%d chat files moved to config folder", s.ChatFilesMigrated))
	}
	if s.ChatFilesFailed > 0 {
		parts = append(parts, fmt.Sprintf("%d chat migrations failed", s.ChatFilesFailed))
	}
	return strings.Join(parts, ", ")
}
