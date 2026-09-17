package workspace

// Commit-on-save for published workspaces.
//
// A published workspace accepts pushes from clones through
// receive.denyCurrentBranch=updateInstead, which refuses while the host's
// working tree is dirty. Every write BRUV itself makes on the host
// (editor save, new file, template generation) would otherwise leave the
// tree dirty and block the next push from a laptop — BRUV's own edit
// locking out BRUV's own clone. So when the workspace opts in, each write
// is committed immediately, as BRUV, scoped to the paths it touched.

import (
	"context"
	"log/slog"
	"strings"

	"bruv/internal/model"
)

// SetCommitOnSave toggles the behaviour. Only meaningful on a published
// workspace; stored regardless so the choice survives un-/re-publishing.
func (s *Service) SetCommitOnSave(brandSlug, streamSlug, projectSlug string, on bool) (*model.Workspace, error) {
	ws, _, err := s.localRoot(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	ws.CommitOnSave = on
	if err := s.saveWorkspace(brandSlug, streamSlug, projectSlug, ws); err != nil {
		return nil, err
	}
	s.emit("workspace:updated", brandSlug, streamSlug, projectSlug)
	return ws, nil
}

// commitIfEnabled commits rels (workspace-relative, slash form) on the host
// when the workspace is published with CommitOnSave. Best-effort: the file
// write already succeeded and is the user's result; a commit failure is
// logged, never surfaced as a failed save. Ignored paths are skipped so a
// save into an ignored folder doesn't error on `git add`.
func (s *Service) commitIfEnabled(ctx context.Context, ws *model.Workspace, root string, rels []string, message string) {
	if ws == nil || !ws.CommitOnSave || ws.GitServe != model.GitServeReady {
		return
	}
	var paths []string
	for _, rel := range rels {
		if rel == "" || rel == "." {
			continue
		}
		if _, ignored := gitOut(ctx, root, "check-ignore", "-q", "--", rel); ignored {
			continue
		}
		paths = append(paths, rel)
	}
	if len(paths) == 0 {
		return
	}
	addArgs := append([]string{"add", "-A", "--"}, paths...)
	if _, err := gitRunArgs(ctx, root, gitCmdTimeout, bruvCommitter, addArgs...); err != nil {
		slog.Warn("commit on save: git add failed", "err", err)
		return
	}
	// Commit only the touched paths, so a dirty tree elsewhere (the user's
	// own uncommitted work on the host) is left exactly as it was.
	commitArgs := append([]string{"commit", "-q", "-m", message, "--"}, paths...)
	if _, err := gitRunArgs(ctx, root, gitCmdTimeout, bruvCommitter, commitArgs...); err != nil {
		if strings.Contains(err.Error(), "nothing to commit") || strings.Contains(err.Error(), "no changes added") {
			return
		}
		slog.Warn("commit on save: git commit failed", "err", err)
	}
}
