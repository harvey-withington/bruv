package model

import "time"

// Workspace connects a Project to a real body of work (spec: plan/2026-07-04
// BRUV workspace spec.md). 0 or 1 per Project in v1.
//
// State is split across two homes, and the split is load-bearing:
//   - Vault-side (these types, persisted under the project's workspace/ dir):
//     Workspace (origin, adapter, launch command, claim) and WorkspaceIndex.
//     Shared truth, visible to every connected client.
//   - Device-side (never in the vault): localPath, WorkspaceSnapshot, and the
//     working copy itself, kept in the materializing device's app-local data.
//
// Consequently Workspace has no localPath field; each device tracks its own
// checkout. WorkspaceSnapshot is defined here so both sides share the type,
// but it is only ever written device-side.

// WorkspaceOriginKind says where the canonical files live.
type WorkspaceOriginKind string

const (
	// OriginLocal is a local filesystem path; permanently Tier 1 on the
	// device that holds it, with no checkout/check-in lifecycle.
	OriginLocal WorkspaceOriginKind = "local"
	// OriginGit is a git remote (materialized via the system git binary).
	OriginGit WorkspaceOriginKind = "git"
	// OriginRclone is an rclone-addressable remote (NAS, S3, Drive, …).
	OriginRclone WorkspaceOriginKind = "rclone"
)

// WorkspaceOrigin locates the canonical files.
type WorkspaceOrigin struct {
	Kind WorkspaceOriginKind `json:"kind"`
	// URL is the git remote URL, or the path for local origins.
	URL string `json:"url,omitempty"`
	// Subpath scopes the Workspace to a subtree of the origin
	// (git: sparse checkout of one directory of a monorepo).
	Subpath string `json:"subpath,omitempty"`
	// RcloneRemote addresses rclone origins, e.g. "nas:projects/song-alpha".
	RcloneRemote string `json:"rclone_remote,omitempty"`
}

// Workspace claim states, mirrored vault-side for cross-device visibility
// ("laptop has unchecked-in changes"). Advisory, best-effort — same trust
// model as the claim itself.
const (
	WorkspaceStateClean = "clean"
	WorkspaceStateDirty = "dirty"
)

// WorkspaceClaim is the advisory claim registry entry: "device X materialized
// this at time T". Never a lock — any device may materialize or check in at
// any time; proceeding past the warning supersedes the claim.
type WorkspaceClaim struct {
	Device         string    `json:"device"`
	InstanceID     string    `json:"instance_id,omitempty"`
	MaterializedAt time.Time `json:"materialized_at"`
	// State is the claiming device's last self-reported lifecycle state
	// (WorkspaceStateClean/Dirty); empty = unknown.
	State    string    `json:"state,omitempty"`
	LastSeen time.Time `json:"last_seen"`
}

// Workspace is the vault-side config record (workspace/workspace.json).
type Workspace struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"project_id"`
	Origin        WorkspaceOrigin `json:"origin"`
	Adapter       string          `json:"adapter"` // "plain-folder" | "git-repo" | "obsidian-vault"
	LaunchCommand string          `json:"launch_command,omitempty"`
	Claim         *WorkspaceClaim `json:"claim,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// WorkspaceEntry is one node of the indexed file tree.
type WorkspaceEntry struct {
	Path    string `json:"path"` // slash-separated, relative to the workspace root
	IsDir   bool   `json:"is_dir,omitempty"`
	Size    int64  `json:"size,omitempty"`
	Symlink bool   `json:"symlink,omitempty"` // recorded, never followed
}

// WorkspaceIndex is the adapter's output (workspace/index.json): the tree plus
// a human/AI-readable summary. Regenerated on demand; cacheable, vault-side.
type WorkspaceIndex struct {
	WorkspaceID string    `json:"workspace_id"`
	GeneratedAt time.Time `json:"generated_at"`
	Adapter     string    `json:"adapter"`
	// Summary is the adapter's digest (branch/commits for git, note count for
	// Obsidian, …) — what the AI sees at metadata context levels.
	Summary string `json:"summary"`
	// Details are adapter-specific key/values ("branch": "main", …).
	Details  map[string]string `json:"details,omitempty"`
	Warnings []string          `json:"warnings,omitempty"`
	Tree     []WorkspaceEntry  `json:"tree"`
}

// WorkspaceFileStamp fingerprints one file in a snapshot. Hash is
// "sha256:<hex>"; files above the large-binary threshold fall back to
// size+mtime with Fuzzy set, and fuzzy mismatches reconcile as conflicts.
type WorkspaceFileStamp struct {
	Hash  string    `json:"hash,omitempty"`
	Size  int64     `json:"size"`
	MTime time.Time `json:"mtime,omitempty"`
	Fuzzy bool      `json:"fuzzy,omitempty"`
}

// WorkspaceSnapshot is the per-file manifest taken at materialize/check-in —
// the basis of all divergence detection. DEVICE-SIDE ONLY: lives in the
// materializing device's app-local data, never in the vault.
type WorkspaceSnapshot struct {
	WorkspaceID string    `json:"workspace_id"`
	TakenAt     time.Time `json:"taken_at"`
	// OriginRevision is the git head SHA at materialize time, or the SHA-256
	// of the sorted remote manifest for rclone origins.
	OriginRevision string                        `json:"origin_revision"`
	Files          map[string]WorkspaceFileStamp `json:"files"`
}
