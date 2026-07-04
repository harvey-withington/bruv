package workspace

import (
	"context"

	"bruv/internal/model"
)

// RemoteManifest is a Transport.List result: the origin's current tree with
// fingerprints, used for size estimates (materialize) and divergence
// detection (check-in).
type RemoteManifest struct {
	// Revision identifies the origin state: git head SHA, or the SHA-256 of
	// the sorted manifest for rclone origins.
	Revision string
	Files    map[string]model.WorkspaceFileStamp
}

// Transport implements exactly two verbs plus enumeration — no sync mode, no
// watch mode (spec §7). Transports execute on the device doing the checkout,
// never proxied through the BRUV server.
//
// M1 ships only the local transport (all verbs no-ops). git and rclone
// arrive with M2 (List) and M3 (Copy verbs).
type Transport interface {
	List(ctx context.Context, origin model.WorkspaceOrigin) (*RemoteManifest, error)
	CopyDown(ctx context.Context, origin model.WorkspaceOrigin, dest string) error
	CopyUp(ctx context.Context, src string, origin model.WorkspaceOrigin, files []string) error
}

// LocalTransport is the origin-is-a-local-path transport: the Workspace is
// permanently Tier 1 on the device holding the path, with no checkout /
// check-in lifecycle — all verbs are no-ops by design.
type LocalTransport struct{}

// List returns an empty manifest; a local origin has no "remote" state.
func (LocalTransport) List(context.Context, model.WorkspaceOrigin) (*RemoteManifest, error) {
	return &RemoteManifest{Files: map[string]model.WorkspaceFileStamp{}}, nil
}

// CopyDown is a no-op: the files are already where they live.
func (LocalTransport) CopyDown(context.Context, model.WorkspaceOrigin, string) error { return nil }

// CopyUp is a no-op: edits land in place; there is nothing to push.
func (LocalTransport) CopyUp(context.Context, string, model.WorkspaceOrigin, []string) error {
	return nil
}
