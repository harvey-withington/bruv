package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const maxRecent = 10

type RecentRepo struct {
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	LastOpened time.Time `json:"last_opened"`
}

// ConfigDir returns the BRUV config directory (e.g. %APPDATA%/bruv).
func ConfigDir() (string, error) {
	return configDir()
}

// configDir returns the BRUV config directory (e.g. %APPDATA%/bruv).
func configDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, "bruv")
	return p, os.MkdirAll(p, 0o755)
}

const recentFileName = "recent.json"

func recentFilePath() (string, error) {
	// One-shot migration: pre-split builds wrote recent.json at the
	// root of the config dir. The recent-repos list is genuinely
	// per-device — each machine opens repos at its own filesystem
	// paths — so it belongs in clientdata/.
	migrateToClientData(recentFileName)

	dir, err := ClientDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, recentFileName), nil
}

// LoadRecent reads the recent repos list from disk.
func LoadRecent() ([]RecentRepo, error) {
	path, err := recentFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var repos []RecentRepo
	if err := json.Unmarshal(data, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

// AddRecent adds or bumps a repo to the top of the recent list.
func AddRecent(repoPath, name string) error {
	repos, _ := LoadRecent()

	// Remove if already present
	filtered := make([]RecentRepo, 0, len(repos))
	for _, r := range repos {
		if r.Path != repoPath {
			filtered = append(filtered, r)
		}
	}

	// Prepend
	entry := RecentRepo{
		Path:       repoPath,
		Name:       name,
		LastOpened: time.Now().UTC(),
	}
	filtered = append([]RecentRepo{entry}, filtered...)

	// Trim to max
	if len(filtered) > maxRecent {
		filtered = filtered[:maxRecent]
	}

	return saveRecent(filtered)
}

// RemoveRecent removes a repo from the recent list.
func RemoveRecent(repoPath string) error {
	repos, _ := LoadRecent()
	filtered := make([]RecentRepo, 0, len(repos))
	for _, r := range repos {
		if r.Path != repoPath {
			filtered = append(filtered, r)
		}
	}
	return saveRecent(filtered)
}

func saveRecent(repos []RecentRepo) error {
	path, err := recentFilePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(repos, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
