package workspace

// Publishing a workspace as git (spec §8 materialize, adapted).
//
// The spec's model is that a client device materializes a Workspace by
// talking to its origin directly, and that "the server is never a proxy for
// file bytes". This is that, with the host's own storage as the origin: the
// machine holding the files git-ifies the folder and publishes it over the
// BRUV connection, so a laptop connected to a home server clones a real
// working copy onto its own disk. Nothing here copies bytes through an RPC —
// git's own transport does the transfer (transport/http/git.go).
//
// Everything in this file runs on the host that holds Origin.URL.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	wsengine "bruv/core/workspace"
	"bruv/internal/model"
)

// gitCmdTimeout bounds ordinary queries. Initialization gets its own,
// far larger budget — committing a large folder is legitimately slow.
const (
	gitCmdTimeout  = 15 * time.Second
	gitInitTimeout = 30 * time.Minute
)

// bruvCommitter is the identity used for BRUV's own commits, passed per
// command so the host's global git config is neither required nor touched.
var bruvCommitter = []string{
	"-c", "user.name=BRUV",
	"-c", "user.email=bruv@localhost",
	// The host may have signing configured globally; a workspace commit
	// must not fail because a signing key isn't available to a service.
	"-c", "commit.gpgsign=false",
}

// GitServeReport answers "what would publishing this workspace involve?"
// without writing anything — the same two-phase shape as capture preview.
// The counts let the UI say "12,431 files (2.1 GB) will be committed"
// before the user commits to it, rather than discovering it afterwards.
type GitServeReport struct {
	// Path is the folder on the host. Shown so the user can confirm the
	// host is looking where they think it is.
	Path string `json:"path"`
	// GitAvailable is false when the host has no git binary — publishing
	// is impossible and the UI must say so rather than offering it.
	GitAvailable bool   `json:"git_available"`
	GitVersion   string `json:"git_version,omitempty"`
	// State mirrors Workspace.GitServe.
	State string `json:"state"`
	Error string `json:"error,omitempty"`

	// IsRepo is true when the folder is already a git repository, in which
	// case publishing adds configuration but never rewrites their history.
	IsRepo     bool   `json:"is_repo"`
	HasCommits bool   `json:"has_commits"`
	Branch     string `json:"branch,omitempty"`
	// UncommittedPaths is non-zero when an existing repo has work that is
	// not committed. Clones see committed state only, so this is the one
	// surprise worth naming up front.
	UncommittedPaths int `json:"uncommitted_paths"`

	// HasGitignore reports whether the folder states its own exclusions.
	// Without one, an initial commit takes everything — build output,
	// dependencies and all — which is exactly what the user needs to know
	// before pressing the button.
	HasGitignore bool `json:"has_gitignore"`
	// Files/Bytes estimate the initial commit, honouring .gitignore where
	// present. Truncated is set when the folder exceeds the walk limit and
	// the real totals are larger.
	Files     int   `json:"files"`
	Bytes     int64 `json:"bytes"`
	Truncated bool  `json:"truncated"`
}

// InspectGitServe reports what publishing this workspace would involve.
// Read-only: it never creates a repository or writes to the folder.
func (s *Service) InspectGitServe(ctx context.Context, brandSlug, streamSlug, projectSlug string) (*GitServeReport, error) {
	ws, dir, err := s.localRoot(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	rep := &GitServeReport{Path: dir, State: ws.GitServe, Error: ws.GitServeError}

	if version, ok := gitOut(ctx, dir, "version"); ok {
		rep.GitAvailable = true
		rep.GitVersion = strings.TrimPrefix(version, "git version ")
	} else {
		return rep, nil // nothing else is answerable without git
	}

	if top, ok := gitOut(ctx, dir, "rev-parse", "--show-toplevel"); ok && top != "" {
		rep.IsRepo = true
		if _, ok := gitOut(ctx, dir, "rev-parse", "--verify", "HEAD"); ok {
			rep.HasCommits = true
		}
		rep.Branch, _ = gitOut(ctx, dir, "branch", "--show-current")
		if status, ok := gitOut(ctx, dir, "status", "--porcelain"); ok && status != "" {
			rep.UncommittedPaths = len(strings.Split(status, "\n"))
		}
	}

	// Size estimate for the initial commit. LocalFS honours .gitignore, so
	// where the folder states its own exclusions this matches what git
	// would stage. (.bruvignore is honoured too — a BRUV-only exclusion
	// makes the estimate conservative, never the reverse.)
	fs, err := wsengine.NewLocalFS(dir)
	if err != nil {
		return nil, fmt.Errorf("open workspace folder: %w", err)
	}
	if _, err := os.Stat(filepath.Join(dir, wsengine.GitIgnoreFile)); err == nil {
		rep.HasGitignore = true
	}
	tree, truncated, err := fs.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("scan workspace folder: %w", err)
	}
	rep.Truncated = truncated
	for _, e := range tree {
		if e.IsDir {
			continue
		}
		rep.Files++
		rep.Bytes += e.Size
	}
	return rep, nil
}

// initInFlight guards against two clients starting the same initialization.
// Keyed by workspace ID; the value is unused.
var initInFlight sync.Map

// EnableGitServe publishes the workspace as git and returns immediately.
//
// The work — git init, the initial commit, receive configuration — happens
// in the background because committing a large folder takes minutes, and a
// client should be told "initializing" rather than left holding an RPC
// open. Progress is the vault-side GitServe state: every transition is
// persisted and announced, so all connected clients (the phone included)
// see the same lifecycle.
//
// Idempotent: calling it on a Ready workspace re-verifies configuration and
// returns; calling it while initializing is a no-op.
func (s *Service) EnableGitServe(ctx context.Context, brandSlug, streamSlug, projectSlug string) (*model.Workspace, error) {
	ws, dir, err := s.localRoot(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git is not installed on the machine holding these files — install git there, or attach a folder this device can see")
	}
	// Deliberately NOT gated on the persisted "initializing" state: if the
	// host restarted mid-initialization, that state is stale and gating on
	// it would leave the workspace preparing forever with no way back.
	// The in-flight map is this process's own truth, so a re-request either
	// joins a live run or starts a fresh one — and prepareGitOrigin is
	// idempotent, so starting fresh costs nothing.
	if _, busy := initInFlight.LoadOrStore(ws.ID, struct{}{}); busy {
		return ws, nil
	}

	ws.GitServe = model.GitServeInitializing
	ws.GitServeError = ""
	if err := s.saveWorkspace(brandSlug, streamSlug, projectSlug, ws); err != nil {
		initInFlight.Delete(ws.ID)
		return nil, err
	}
	s.emit("workspace:updated", brandSlug, streamSlug, projectSlug)

	go func() {
		defer initInFlight.Delete(ws.ID)
		// Deliberately not the caller's context: the RPC that started this
		// returns immediately, and cancelling the clone setup because the
		// caller hung up would leave a half-initialized repository.
		bg, cancel := context.WithTimeout(context.Background(), gitInitTimeout)
		defer cancel()
		branch, err := prepareGitOrigin(bg, dir)
		s.finishGitServe(brandSlug, streamSlug, projectSlug, branch, err)
	}()
	return ws, nil
}

// finishGitServe records the outcome of initialization and announces it.
// It re-reads the workspace so a concurrent edit (rename, launch command)
// isn't clobbered by the copy captured when initialization started.
func (s *Service) finishGitServe(brandSlug, streamSlug, projectSlug, branch string, initErr error) {
	r, err := s.repo()
	if err != nil {
		return
	}
	ws, err := r.GetWorkspace(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return // detached while we worked; nothing to record
	}
	if initErr != nil {
		ws.GitServe = model.GitServeError
		ws.GitServeError = initErr.Error()
	} else {
		ws.GitServe = model.GitServeReady
		ws.GitServeError = ""
		ws.DefaultBranch = branch
	}
	if err := s.saveWorkspace(brandSlug, streamSlug, projectSlug, ws); err != nil {
		return
	}
	s.emit("workspace:updated", brandSlug, streamSlug, projectSlug)
}

// DisableGitServe stops publishing the workspace. The repository itself is
// left completely alone — un-publishing is a BRUV-level decision and must
// never delete a user's git history.
func (s *Service) DisableGitServe(brandSlug, streamSlug, projectSlug string) (*model.Workspace, error) {
	ws, _, err := s.localRoot(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	ws.GitServe = model.GitServeOff
	ws.GitServeError = ""
	if err := s.saveWorkspace(brandSlug, streamSlug, projectSlug, ws); err != nil {
		return nil, err
	}
	s.emit("workspace:updated", brandSlug, streamSlug, projectSlug)
	return ws, nil
}

// prepareGitOrigin makes dir cloneable and returns the branch to track.
//
// Existing repositories keep their history: BRUV only ensures there is a
// commit to clone and that pushes from a checkout are accepted. A folder
// that isn't a repository yet gets one, with everything committed.
func prepareGitOrigin(ctx context.Context, dir string) (string, error) {
	isRepo := false
	if top, ok := gitOut(ctx, dir, "rev-parse", "--show-toplevel"); ok && top != "" {
		isRepo = true
	}
	if !isRepo {
		// -b needs git 2.28+; older git still works, it just names the
		// branch by its own default.
		if _, err := gitRun(ctx, dir, gitCmdTimeout, "init", "-b", "main"); err != nil {
			if _, err := gitRun(ctx, dir, gitCmdTimeout, "init"); err != nil {
				return "", err
			}
		}
	}

	hasCommits := false
	if _, ok := gitOut(ctx, dir, "rev-parse", "--verify", "HEAD"); ok {
		hasCommits = true
	}
	if !hasCommits {
		// A repository with no commits cannot be cloned usefully, so this
		// is the one case where BRUV commits on the user's behalf — and
		// only ever as the *first* commit of a repository it just made.
		if _, err := gitRunArgs(ctx, dir, gitInitTimeout, bruvCommitter, "add", "-A"); err != nil {
			return "", err
		}
		if _, err := gitRunArgs(ctx, dir, gitInitTimeout, bruvCommitter,
			"commit", "-m", "Initial commit (published by BRUV)", "--allow-empty"); err != nil {
			return "", err
		}
	}

	// Accept pushes from checkouts. A normal (non-bare) repository refuses
	// a push to the branch it has checked out; updateInstead accepts it and
	// fast-forwards the working tree, but only when that tree is clean — so
	// edits made directly on the host are never overwritten by a push.
	if _, err := gitRun(ctx, dir, gitCmdTimeout, "config", "receive.denyCurrentBranch", "updateInstead"); err != nil {
		return "", err
	}

	branch, _ := gitOut(ctx, dir, "branch", "--show-current")
	if branch == "" {
		branch = "main"
	}
	return branch, nil
}

// GitServeDir returns the on-disk repository for a published workspace,
// looked up by workspace ID. The git transport handler resolves requests
// through this — ok is false unless the workspace exists, is published and
// is Ready, so an unpublished workspace is simply not on the network.
func (s *Service) GitServeDir(workspaceID string) (dir string, ok bool) {
	r, err := s.repo()
	if err != nil {
		return "", false
	}
	refs, err := r.ListWorkspaces()
	if err != nil {
		return "", false
	}
	for _, ref := range refs {
		ws := ref.Workspace
		if ws.ID != workspaceID {
			continue
		}
		if ws.GitServe != model.GitServeReady || ws.Origin.Kind != model.OriginLocal {
			return "", false
		}
		return ws.Origin.URL, true
	}
	return "", false
}

// saveWorkspace persists a mutated workspace record.
func (s *Service) saveWorkspace(brandSlug, streamSlug, projectSlug string, ws *model.Workspace) error {
	r, err := s.repo()
	if err != nil {
		return err
	}
	return r.SaveWorkspace(brandSlug, streamSlug, projectSlug, ws)
}

// gitRun runs one git command and returns a useful error. Unlike gitOut
// (which degrades quietly for optional index details) every caller here
// needs to know precisely why something failed — these errors reach the
// user as the reason their workspace didn't publish.
func gitRun(ctx context.Context, dir string, timeout time.Duration, args ...string) (string, error) {
	return gitRunArgs(ctx, dir, timeout, nil, args...)
}

// gitRunArgs is gitRun with leading `-c key=value` configuration.
func gitRunArgs(ctx context.Context, dir string, timeout time.Duration, config []string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	full := append([]string{"-C", dir}, config...)
	full = append(full, args...)
	cmd := exec.CommandContext(cctx, "git", full...)
	hideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		if cctx.Err() == context.DeadlineExceeded {
			detail = fmt.Sprintf("timed out after %s (%s)", timeout, detail)
		}
		return "", fmt.Errorf("git %s: %s", args[0], firstLine(detail))
	}
	return strings.TrimSpace(string(out)), nil
}

// firstLine keeps multi-line git errors readable in a toast.
func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
