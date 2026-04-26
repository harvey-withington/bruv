package config

// One-shot migration that promotes the implicit "Local" connection
// (Active=="" sentinel) into a real entry in connections.json with
// the stable ID "local". Runs at every desktop boot via NewApp;
// idempotent — second boot is a no-op.
//
// Why: pre-2026-04-26 the desktop's loopback ran in single-repo
// transport mode and "Local" lived as the empty-string Active
// sentinel. The duplication-audit refactor switches the desktop to
// multi-repo transport and treats Local as just another connection
// in connections.json — same shape as any Remote entry — which lets
// every Local-vs-Remote branch in the frontend / Shell-bridge
// disappear. This migration coerces existing installs into that
// shape so callers can rely on Active != "".
//
// The Local entry's URL + DeviceToken are placeholders here
// (filled at boot by app.refreshLocalConnection in the main package
// after the loopback HTTP server has bound a port and the desktop
// device token has been resolved/issued). Users see an empty URL
// only between this migration and that boot step — invisible to
// anyone not poking at connections.json by hand.
//
// repo-recents.json is migrated alongside: any value previously
// keyed by "" (the Local sentinel) is moved to the "local" key so
// reopen-last-repo on Local keeps working post-migration.

import (
	"log/slog"
)

// LocalConnectionID is the stable, well-known ID used for the
// desktop's loopback connection in connections.json. Hard-coded
// (not generated) so callers can recognise it on disk + in logs.
const LocalConnectionID = "local"

// LocalConnectionName is the user-facing label rendered in the
// picker / connection chip when this connection is active. Frontend
// has its own t('connection.local') hook for i18n; this default
// only matters when the entry first lands on disk.
const LocalConnectionName = "Local"

// MigrateLocalConnection ensures connections.json contains a
// canonical Local entry with ID "local", coerces a legacy
// Active=="" pointer to Active=="local", and renames any
// empty-string key in repo-recents.json to "local". Safe to call
// on every boot — second + subsequent calls are no-ops.
func MigrateLocalConnection() {
	store, err := LoadConnections()
	if err != nil {
		slog.Warn("migrate local connection: load connections failed", "err", err)
		return
	}

	changed := false
	hasLocal := false
	for _, c := range store.Connections {
		if c.ID == LocalConnectionID {
			hasLocal = true
			break
		}
	}
	if !hasLocal {
		// Prepend so the picker renders Local first. URL + DeviceToken
		// stay empty here; the desktop fills them in once the loopback
		// HTTP server is up. AddedAt left zero so it's distinguishable
		// from a user-added Remote in logs (zero time vs UTC now).
		entry := Connection{
			ID:   LocalConnectionID,
			Name: LocalConnectionName,
		}
		store.Connections = append([]Connection{entry}, store.Connections...)
		changed = true
	}
	if store.Active == "" {
		store.Active = LocalConnectionID
		changed = true
	}
	if changed {
		if err := SaveConnections(store); err != nil {
			slog.Warn("migrate local connection: save connections failed", "err", err)
			// Fall through — repo-recents migration can still happen.
		}
	}

	// Rename the legacy "" key in repo-recents.json to "local". The
	// SetRecentRepoForConnection helper accepts the empty-string key
	// (it's the legitimate Local sentinel pre-migration), so the old
	// install may have a value there.
	recents, err := LoadRepoRecents()
	if err != nil {
		slog.Warn("migrate local connection: load repo-recents failed", "err", err)
		return
	}
	legacyValue, hasLegacy := recents[""]
	if !hasLegacy {
		return // already migrated or never had a Local last-opened
	}
	delete(recents, "")
	if _, hasNew := recents[LocalConnectionID]; !hasNew {
		// Only adopt the legacy value if the new key isn't already
		// populated — protects against double-runs of an aborted
		// migration where the post-rename write happened but the
		// pre-rename delete didn't.
		recents[LocalConnectionID] = legacyValue
	}
	if err := SaveRepoRecents(recents); err != nil {
		slog.Warn("migrate local connection: save repo-recents failed", "err", err)
	}
}
