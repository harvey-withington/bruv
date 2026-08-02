package config

// Per-device Workspace checkouts.
//
// The workspace spec splits state deliberately: the vault holds shared
// truth (origin, adapter, claim), and each device tracks its own working
// copy. This file is the device side of that split — "which folder on THIS
// machine holds my copy of workspace X, cloned from which connection".
//
// It is emphatically not vault state. Two laptops connected to the same
// server keep separate copies in separate places, and neither needs to know
// where the other put theirs.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const workspaceCheckoutsFileName = "workspace-checkouts.json"

// WorkspaceCheckout is one working copy on this device.
type WorkspaceCheckout struct {
	// WorkspaceID is the vault-side workspace UUID.
	WorkspaceID string `json:"workspace_id"`
	// ConnectionID and RepoID scope the checkout: the same workspace ID
	// could in principle be reached through two connections, and they are
	// different clones with different remotes.
	ConnectionID string `json:"connection_id"`
	RepoID       string `json:"repo_id"`
	// LocalPath is the working copy's root on this machine.
	LocalPath string `json:"local_path"`
	Branch    string `json:"branch,omitempty"`
	// ProjectSlug et al. let the UI describe a checkout without a round
	// trip, and let "forget this checkout" work when the project has since
	// been renamed or detached.
	BrandSlug      string    `json:"brand_slug,omitempty"`
	StreamSlug     string    `json:"stream_slug,omitempty"`
	ProjectSlug    string    `json:"project_slug,omitempty"`
	MaterializedAt time.Time `json:"materialized_at"`
}

// workspaceCheckoutStore is the on-disk shape. Root lives here rather than
// in Preferences because Preferences are per-*machine* and travel over RPC:
// on a remote connection GetPreferences answers for the server, and "where
// do my working copies go" is a question only this device can answer.
type workspaceCheckoutStore struct {
	// Root is the user's chosen folder for new working copies; empty means
	// use the default.
	Root string `json:"root,omitempty"`
	// Checkouts is keyed by connection+repo+workspace so lookups are exact.
	Checkouts map[string]WorkspaceCheckout `json:"checkouts"`
}

func checkoutKey(connectionID, repoID, workspaceID string) string {
	return connectionID + "|" + repoID + "|" + workspaceID
}

func workspaceCheckoutsPath() (string, error) {
	dir, err := ClientDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, workspaceCheckoutsFileName), nil
}

func loadWorkspaceCheckouts() (workspaceCheckoutStore, error) {
	out := workspaceCheckoutStore{Checkouts: map[string]WorkspaceCheckout{}}
	path, err := workspaceCheckoutsPath()
	if err != nil {
		return out, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return workspaceCheckoutStore{Checkouts: map[string]WorkspaceCheckout{}}, err
	}
	if out.Checkouts == nil {
		out.Checkouts = map[string]WorkspaceCheckout{}
	}
	return out, nil
}

func saveWorkspaceCheckouts(c workspaceCheckoutStore) error {
	path, err := workspaceCheckoutsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data, 0o644)
}

// GetWorkspaceCheckout returns this device's working copy for a workspace,
// or nil when it holds none.
//
// A checkout whose folder has been deleted or moved out from under BRUV is
// reported as absent rather than as a broken record: the user's next action
// should be "make me a copy", not an error about a path they may have
// tidied away months ago.
func GetWorkspaceCheckout(connectionID, repoID, workspaceID string) *WorkspaceCheckout {
	all, err := loadWorkspaceCheckouts()
	if err != nil {
		return nil
	}
	co, ok := all.Checkouts[checkoutKey(connectionID, repoID, workspaceID)]
	if !ok {
		return nil
	}
	if info, err := os.Stat(co.LocalPath); err != nil || !info.IsDir() {
		return nil
	}
	return &co
}

// SaveWorkspaceCheckout records a working copy on this device.
func SaveWorkspaceCheckout(co WorkspaceCheckout) error {
	all, err := loadWorkspaceCheckouts()
	if err != nil {
		return err
	}
	if co.MaterializedAt.IsZero() {
		co.MaterializedAt = time.Now()
	}
	all.Checkouts[checkoutKey(co.ConnectionID, co.RepoID, co.WorkspaceID)] = co
	return saveWorkspaceCheckouts(all)
}

// ForgetWorkspaceCheckout drops the record. The files are never touched —
// forgetting a checkout is a BRUV bookkeeping change, and deleting a user's
// working copy is not something a "forget" button may ever do.
func ForgetWorkspaceCheckout(connectionID, repoID, workspaceID string) error {
	all, err := loadWorkspaceCheckouts()
	if err != nil {
		return err
	}
	delete(all.Checkouts, checkoutKey(connectionID, repoID, workspaceID))
	return saveWorkspaceCheckouts(all)
}

// ListWorkspaceCheckouts returns every working copy recorded on this
// device, including any whose folder has since disappeared — the caller is
// managing records here, not opening files.
func ListWorkspaceCheckouts() []WorkspaceCheckout {
	all, err := loadWorkspaceCheckouts()
	if err != nil {
		return nil
	}
	out := make([]WorkspaceCheckout, 0, len(all.Checkouts))
	for _, co := range all.Checkouts {
		out = append(out, co)
	}
	return out
}

// WorkspaceRoot is where new working copies land: the user's chosen folder,
// or a single predictable one under their home directory so checkouts from
// every connection end up in one place.
func WorkspaceRoot() (string, error) {
	if all, err := loadWorkspaceCheckouts(); err == nil && all.Root != "" {
		return all.Root, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "bruv-workspaces"), nil
}

// SetWorkspaceRoot changes where future working copies land. Existing
// checkouts stay where they are — moving a user's files because they
// changed a preference would be a surprise, and their tools hold paths
// into those folders.
func SetWorkspaceRoot(root string) error {
	all, err := loadWorkspaceCheckouts()
	if err != nil {
		return err
	}
	all.Root = root
	return saveWorkspaceCheckouts(all)
}
