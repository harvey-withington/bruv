package main

import (
	"path/filepath"

	"bruv/internal/config"
	"bruv/internal/repo"
)

// configureRunsDir points the open Repository at its server-side run
// history directory — <serverdata>/runs/<repoManifestID>/. Creating
// the split this way keeps run history out of the repo tree so git /
// Syncthing pulls don't drag another user's agent runs into yours
// when sharing a repo. See internal/repo/agent.go for the storage
// contract.
//
// Uses the repo's stable Manifest.ID rather than a name or path slug
// so renames / moves don't orphan the runs directory.
func (a *App) configureRunsDir(r *repo.Repository) error {
	serverDir, err := config.ServerDataDir()
	if err != nil {
		return err
	}
	runsDir := filepath.Join(serverDir, "runs", r.Manifest.ID)
	return r.SetRunsDir(runsDir)
}
