package workspace

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	wsengine "bruv/core/workspace"
	"bruv/internal/model"
)

// gitAdapter indexes git working copies: branch, remote, recent commits,
// dirty/clean status (local only, via the system git binary — feature-
// detected, degrading to tree-only output with a warning when absent).
type gitAdapter struct{}

func (*gitAdapter) Name() string { return "git-repo" }

func (*gitAdapter) Detect(tree []model.WorkspaceEntry) float64 {
	if hasRootEntry(tree, ".git", true) {
		return 0.9
	}
	return 0
}

func (*gitAdapter) Index(ctx context.Context, fs wsengine.FS) (*model.WorkspaceIndex, error) {
	tree, truncated, err := fs.List(ctx)
	if err != nil {
		return nil, err
	}
	tree = dropStateDirs(tree, ".git", ".obsidian")
	files, _, size := treeStats(tree)

	idx := &model.WorkspaceIndex{
		Adapter: "git-repo",
		Details: map[string]string{"languages": extHistogram(tree, 5)},
		Tree:    tree,
	}
	if truncated {
		idx.Warnings = append(idx.Warnings, fmt.Sprintf("tree truncated at %d entries", wsengine.MaxIndexEntries))
	}

	summary := fmt.Sprintf("Git repository with %d files (%s).", files, humanBytes(size))
	dir, isLocal := fs.LocalDir()
	if !isLocal {
		idx.Summary = summary
		return idx, nil // remote FS: tree-only until M2's origin listing
	}
	if _, err := exec.LookPath("git"); err != nil {
		idx.Warnings = append(idx.Warnings, "git binary not found — branch/commit details unavailable")
		idx.Summary = summary
		return idx, nil
	}

	if branch, ok := gitOut(ctx, dir, "branch", "--show-current"); ok && branch != "" {
		idx.Details["branch"] = branch
		summary += " Branch: " + branch + "."
	}
	if remote, ok := gitOut(ctx, dir, "remote", "get-url", "origin"); ok && remote != "" {
		idx.Details["remote"] = remote
	}
	if status, ok := gitOut(ctx, dir, "status", "--porcelain"); ok {
		if status == "" {
			idx.Details["status"] = "clean"
			summary += " Working tree clean."
		} else {
			n := len(strings.Split(status, "\n"))
			idx.Details["status"] = fmt.Sprintf("dirty (%d paths)", n)
			summary += fmt.Sprintf(" %d uncommitted paths.", n)
		}
	}
	if log, ok := gitOut(ctx, dir, "log", "--pretty=format:%h %s", "-n", "5"); ok && log != "" {
		idx.Details["recent_commits"] = log
	}
	idx.Summary = summary
	return idx, nil
}

// gitOut runs one git query with a hard timeout, hidden console window, and
// trimmed output. ok is false on any failure — callers degrade quietly.
func gitOut(ctx context.Context, dir string, args ...string) (string, bool) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "git", append([]string{"-C", dir}, args...)...)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}
