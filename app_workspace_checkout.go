package main

// Device-side Workspace checkouts: cloning a published workspace from the
// machine that holds its files onto this one.
//
// These are SHELL_METHODS. The spec is explicit that transports run on the
// device doing the checkout and that the server is never a proxy for file
// bytes — so the clone happens here, in the process the user is sitting in
// front of, and the working copy lands on this disk. The BRUV server's only
// role is to serve the git protocol (transport/http/git.go); every byte
// travels over git's own pack transport.
//
// Nothing here writes to the vault. Which folder holds this device's copy
// is device-local state (internal/config/workspace_checkouts.go).

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"bruv/internal/config"
)

// Checkout lifecycle states, mirrored into the UI.
const (
	checkoutIdle    = "idle"
	checkoutCloning = "cloning"
	checkoutError   = "error"
)

// gitQuickTimeout bounds the status queries the panel makes; clones get no
// deadline beyond the user's patience (they can be gigabytes).
const gitQuickTimeout = 10 * time.Second

// WorkspaceCheckoutInfo is the panel's whole view of "does this device have
// a copy of this workspace, and what state is it in".
type WorkspaceCheckoutInfo struct {
	WorkspaceID string `json:"workspace_id"`
	// HasCopy is false when this device holds no working copy — either it
	// never made one, or the folder has since been moved or deleted.
	HasCopy   bool   `json:"has_copy"`
	LocalPath string `json:"local_path,omitempty"`
	Branch    string `json:"branch,omitempty"`
	// Status is checkoutIdle/Cloning/Error. Progress carries git's own
	// progress line while cloning ("Receiving objects: 45% …") so the user
	// sees movement rather than an unexplained wait.
	Status   string `json:"status"`
	Progress string `json:"progress,omitempty"`
	Error    string `json:"error,omitempty"`
	// Dirty, Ahead and Behind describe the copy against its origin: work
	// not yet committed, commits not yet sent back, commits waiting to come
	// down. These drive the check-in affordances.
	Dirty  bool `json:"dirty"`
	Ahead  int  `json:"ahead"`
	Behind int  `json:"behind"`
	// Diverged is ahead AND behind: both machines have commits the other
	// doesn't, so neither getting nor sending can proceed until they're
	// merged. Counts are as of the last fetch.
	Diverged bool `json:"diverged"`
	// GitAvailable is false when this device has no git binary, which the
	// UI must say plainly rather than offering an action that can't work.
	GitAvailable bool `json:"git_available"`
}

// checkoutJob is one in-flight clone.
type checkoutJob struct {
	status   string
	progress string
	err      string
}

// checkoutJobs tracks clones by workspace ID. In-memory by design: a clone
// interrupted by quitting BRUV leaves a partial folder that the next
// attempt cleans up, and resuming across restarts would be a promise this
// can't keep.
var (
	checkoutMu   sync.Mutex
	checkoutJobs = map[string]*checkoutJob{}
)

func checkoutJobFor(id string) *checkoutJob {
	checkoutMu.Lock()
	defer checkoutMu.Unlock()
	return checkoutJobs[id]
}

func setCheckoutJob(id string, mutate func(*checkoutJob)) {
	checkoutMu.Lock()
	defer checkoutMu.Unlock()
	job := checkoutJobs[id]
	if job == nil {
		job = &checkoutJob{status: checkoutIdle}
		checkoutJobs[id] = job
	}
	mutate(job)
}

// GetWorkspaceCheckout reports this device's working copy for a workspace.
// Cheap enough for the panel to poll while a clone runs.
func (a *App) GetWorkspaceCheckout(workspaceID string) (*WorkspaceCheckoutInfo, error) {
	info := &WorkspaceCheckoutInfo{WorkspaceID: workspaceID, Status: checkoutIdle}
	if _, err := exec.LookPath("git"); err == nil {
		info.GitAvailable = true
	}
	if job := checkoutJobFor(workspaceID); job != nil {
		info.Status, info.Progress, info.Error = job.status, job.progress, job.err
	}

	connID, repoID, _, _, err := activeConnectionForGit()
	if err != nil {
		return info, nil // no usable connection: simply no copy to report
	}
	co := config.GetWorkspaceCheckout(connID, repoID, workspaceID)
	if co == nil {
		return info, nil
	}
	info.HasCopy = true
	info.LocalPath = co.LocalPath
	info.Branch = co.Branch
	if info.Status == checkoutCloning || !info.GitAvailable {
		return info, nil
	}
	describeCheckout(info, co.LocalPath)
	return info, nil
}

// describeCheckout fills in the dirty/ahead/behind picture. Every query is
// best-effort: a working copy the user has been editing outside BRUV (mid
// rebase, detached HEAD, no upstream yet) must still render as a checkout
// they own, not as an error.
func describeCheckout(info *WorkspaceCheckoutInfo, dir string) {
	ctx, cancel := context.WithTimeout(context.Background(), gitQuickTimeout)
	defer cancel()
	if branch, ok := gitQuery(ctx, dir, "branch", "--show-current"); ok && branch != "" {
		info.Branch = branch
	}
	if status, ok := gitQuery(ctx, dir, "status", "--porcelain"); ok {
		info.Dirty = status != ""
	}
	// Counts are against the last fetch, not the live origin — asking the
	// server on every poll would put network traffic behind a UI refresh.
	// The sync operations fetch first, so THEY see live numbers.
	if ahead, behind, ok := divergence(ctx, dir); ok {
		info.Ahead, info.Behind = ahead, behind
		// Both sides moved: neither a pull nor a push can proceed until
		// they're combined, and the UI needs to say so rather than let the
		// user press two buttons that both refuse.
		info.Diverged = ahead > 0 && behind > 0
	}
}

// divergence counts commits each side has that the other doesn't. ok is
// false when there's no upstream to compare against.
func divergence(ctx context.Context, dir string) (ahead, behind int, ok bool) {
	counts, ok := gitQuery(ctx, dir, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
	if !ok {
		return 0, 0, false
	}
	fields := strings.Fields(counts)
	if len(fields) != 2 {
		return 0, 0, false
	}
	behind, _ = strconv.Atoi(fields[0])
	ahead, _ = strconv.Atoi(fields[1])
	return ahead, behind, true
}

// MaterializeWorkspace clones a published workspace onto this device and
// returns immediately; the panel follows progress via GetWorkspaceCheckout.
//
// destParent is the folder to create the copy INSIDE — empty means the
// remembered one, or a default under the user's home. A parent rather than
// the copy's own path because the only way a user picks a folder is the
// native picker, which can only return one that already exists.
func (a *App) MaterializeWorkspace(workspaceID, brandSlug, streamSlug, projectSlug, destParent string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is not installed on this device — install git to keep a local copy of a workspace")
	}
	if job := checkoutJobFor(workspaceID); job != nil && job.status == checkoutCloning {
		return nil // already under way; polling will show it
	}
	connID, repoID, base, token, err := activeConnectionForGit()
	if err != nil {
		return err
	}
	if config.GetWorkspaceCheckout(connID, repoID, workspaceID) != nil {
		return fmt.Errorf("this device already has a copy of that workspace")
	}
	dest, err := checkoutDestination(destParent, projectSlug)
	if err != nil {
		return err
	}

	setCheckoutJob(workspaceID, func(j *checkoutJob) {
		j.status, j.progress, j.err = checkoutCloning, "", ""
	})
	remote := fmt.Sprintf("%s/repos/%s/workspaces/%s/git", strings.TrimSuffix(base, "/"), repoID, workspaceID)

	go func() {
		branch, err := cloneWorkspace(remote, token, dest, func(line string) {
			setCheckoutJob(workspaceID, func(j *checkoutJob) { j.progress = line })
		})
		if err != nil {
			// A failed clone leaves a partial folder that would block the
			// retry. Remove it — but only ever the folder this call just
			// created, never a pre-existing one (checkoutDestination
			// guarantees dest didn't exist).
			os.RemoveAll(dest)
			setCheckoutJob(workspaceID, func(j *checkoutJob) {
				j.status, j.err = checkoutError, err.Error()
			})
			return
		}
		saveErr := config.SaveWorkspaceCheckout(config.WorkspaceCheckout{
			WorkspaceID:  workspaceID,
			ConnectionID: connID,
			RepoID:       repoID,
			LocalPath:    dest,
			Branch:       branch,
			BrandSlug:    brandSlug,
			StreamSlug:   streamSlug,
			ProjectSlug:  projectSlug,
		})
		setCheckoutJob(workspaceID, func(j *checkoutJob) {
			j.progress = ""
			if saveErr != nil {
				// The files ARE cloned; only the bookkeeping failed. Say
				// exactly that, and name the folder so the work isn't lost.
				j.status = checkoutError
				j.err = fmt.Sprintf("cloned to %s, but recording it failed: %v", dest, saveErr)
				return
			}
			j.status, j.err = checkoutIdle, ""
		})
	}()
	return nil
}

// Sync outcomes. These are STATUSES, not prose: the two machines can be in
// several perfectly normal relationships, and the UI has to name each one in
// the user's language with the server's name in it. Returning git's raw
// output would put "hint: See the 'Note about fast-forwards'" in front of a
// person whose actual situation is "your laptop and RIPPED have both
// changed".
const (
	syncOK          = "ok"           // work moved
	syncUpToDate    = "up_to_date"   // nothing to do
	syncDiverged    = "diverged"     // both sides have commits the other lacks
	syncServerDirty = "server_dirty" // the host has uncommitted edits of its own
	syncConflict    = "conflict"     // a merge needs a human
)

// WorkspaceSyncResult is what a pull/push/merge actually did.
type WorkspaceSyncResult struct {
	Status string `json:"status"`
	// Detail is supporting text — git's own words, cleaned of hints, or the
	// list of files a merge couldn't reconcile. Never the whole message.
	Detail string `json:"detail,omitempty"`
}

// PullWorkspaceCheckout brings this device's copy up to date.
//
// It fetches FIRST and then decides, rather than asking git to pull and
// interpreting the wreckage: after a fetch the relationship between the two
// copies is a fact (two counts), so "you're both ahead" can be reported as
// the ordinary situation it is instead of as a fast-forward failure.
func (a *App) PullWorkspaceCheckout(workspaceID string) (*WorkspaceSyncResult, error) {
	dir, token, err := checkoutDirFor(workspaceID)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	if _, err := gitAuthed(ctx, dir, token, 0, "fetch", "--prune", "origin"); err != nil {
		return nil, err
	}
	ahead, behind, ok := divergence(ctx, dir)
	if !ok {
		// No upstream configured (someone rebuilt the remote by hand).
		// Let git judge; its error here is a real one worth showing.
		out, err := gitAuthed(ctx, dir, token, 0, "pull", "--ff-only")
		if err != nil {
			return nil, err
		}
		return &WorkspaceSyncResult{Status: syncOK, Detail: firstMeaningfulLine(out)}, nil
	}
	switch {
	case ahead > 0 && behind > 0:
		return &WorkspaceSyncResult{Status: syncDiverged}, nil
	case behind == 0:
		return &WorkspaceSyncResult{Status: syncUpToDate}, nil
	}
	out, err := gitAuthed(ctx, dir, token, 0, "merge", "--ff-only", "@{upstream}")
	if err != nil {
		return nil, err
	}
	return &WorkspaceSyncResult{Status: syncOK, Detail: firstMeaningfulLine(out)}, nil
}

// PushWorkspaceCheckout sends the copy's work back to the machine holding
// the workspace. Uncommitted edits are committed first — "Send changes"
// must mean the FILES, not "whatever was already committed with your own
// git": the status chip says "uncommitted changes" and this button is the
// only affordance most users have (field report 2026-08-11: create a file,
// press Send, get "Already up to date"). Auto-committing here honours
// "BRUV never rewrites a repo it didn't make" — the clone is BRUV's own
// sync vehicle; the server's folder is untouched by this.
//
// Two refusals are ordinary rather than exceptional, and both are
// reported as statuses: the host has its own commits (diverged), or the host
// has uncommitted edits in the folder itself (server_dirty — the protection
// that stops a push overwriting work someone typed straight onto the server).
func (a *App) PushWorkspaceCheckout(workspaceID string) (*WorkspaceSyncResult, error) {
	dir, token, err := checkoutDirFor(workspaceID)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	if status, ok := gitQuery(ctx, dir, "status", "--porcelain"); ok && status != "" {
		if _, err := gitAuthed(ctx, dir, token, gitQuickTimeout, "add", "-A"); err != nil {
			return nil, err
		}
		args := append(mergeIdentity, "commit", "-m", sendCommitMessage())
		if _, err := gitAuthed(ctx, dir, token, 0, args...); err != nil {
			return nil, err
		}
	}
	if ahead, _, ok := divergence(ctx, dir); ok && ahead == 0 {
		return &WorkspaceSyncResult{Status: syncUpToDate}, nil
	}
	out, err := gitAuthed(ctx, dir, token, 0, "push")
	if err == nil {
		return &WorkspaceSyncResult{Status: syncOK, Detail: firstMeaningfulLine(out)}, nil
	}
	switch reason := err.Error(); {
	case strings.Contains(reason, "fetch first"),
		strings.Contains(reason, "non-fast-forward"),
		strings.Contains(reason, "behind its remote"):
		return &WorkspaceSyncResult{Status: syncDiverged}, nil
	case strings.Contains(reason, "unstaged changes"),
		strings.Contains(reason, "uncommitted changes"),
		strings.Contains(reason, "denyCurrentBranch"),
		strings.Contains(reason, "not up to date"):
		return &WorkspaceSyncResult{Status: syncServerDirty}, nil
	}
	return nil, err
}

// MergeWorkspaceCheckout combines the two sides after they've diverged.
//
// A merge, not a rebase: rebasing rewrites commits the user already made,
// and a mid-rebase conflict in a folder of documents is a bad place to
// strand anyone. On conflict the merge is ABORTED and the files are named —
// BRUV has no conflict-resolution surface, so leaving the copy full of
// conflict markers with no way to finish would be worse than saying plainly
// that this one needs a human.
func (a *App) MergeWorkspaceCheckout(workspaceID string) (*WorkspaceSyncResult, error) {
	dir, token, err := checkoutDirFor(workspaceID)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	if _, err := gitAuthed(ctx, dir, token, 0, "fetch", "--prune", "origin"); err != nil {
		return nil, err
	}
	args := append(mergeIdentity, "merge", "--no-edit", "@{upstream}")
	out, mergeErr := gitAuthed(ctx, dir, token, 0, args...)
	if mergeErr == nil {
		return &WorkspaceSyncResult{Status: syncOK, Detail: firstMeaningfulLine(out)}, nil
	}
	conflicts, _ := gitQuery(ctx, dir, "diff", "--name-only", "--diff-filter=U")
	if strings.TrimSpace(conflicts) == "" {
		return nil, mergeErr // a real failure, not a content conflict
	}
	// Put the copy back to a state the user can act on.
	if _, err := gitAuthed(ctx, dir, token, gitQuickTimeout, "merge", "--abort"); err != nil {
		return nil, fmt.Errorf("merge failed and couldn't be undone: %w", err)
	}
	return &WorkspaceSyncResult{Status: syncConflict, Detail: strings.TrimSpace(conflicts)}, nil
}

// mergeIdentity names BRUV as the author of a merge commit, so the merge
// doesn't fail on a device with no global git identity configured.
var mergeIdentity = []string{
	"-c", "user.name=BRUV",
	"-c", "user.email=bruv@localhost",
	"-c", "commit.gpgsign=false",
}

// sendCommitMessage names the auto-commit "Send changes" makes for the
// copy's edits. The device name makes multi-device histories readable.
func sendCommitMessage() string {
	if host, err := os.Hostname(); err == nil && host != "" {
		return fmt.Sprintf("Changes from %s", host)
	}
	return "Changes sent from BRUV"
}

// ForgetWorkspaceCheckout drops BRUV's record of the copy. The folder and
// its contents are left exactly where they are.
func (a *App) ForgetWorkspaceCheckout(workspaceID string) error {
	connID, repoID, _, _, err := activeConnectionForGit()
	if err != nil {
		return err
	}
	checkoutMu.Lock()
	delete(checkoutJobs, workspaceID)
	checkoutMu.Unlock()
	return config.ForgetWorkspaceCheckout(connID, repoID, workspaceID)
}

// --- internals ---

// activeConnectionForGit resolves everything a git command needs about the
// current connection: which connection and repo, the base URL, and the
// device token to authenticate with.
func activeConnectionForGit() (connID, repoID, base, token string, err error) {
	active, _ := config.ActiveConnection()
	if active == nil {
		return "", "", "", "", fmt.Errorf("this workspace's files are already on this device — a local copy only applies to a remote connection")
	}
	repoID = config.GetRecentRepoForConnection(active.ID)
	if repoID == "" {
		return "", "", "", "", fmt.Errorf("no repo is open on this connection")
	}
	return active.ID, repoID, strings.TrimSuffix(active.URL, "/"), active.DeviceToken, nil
}

// checkoutDirFor resolves a recorded checkout to its folder plus the token
// its git commands should authenticate with.
func checkoutDirFor(workspaceID string) (dir, token string, err error) {
	connID, repoID, _, token, err := activeConnectionForGit()
	if err != nil {
		return "", "", err
	}
	co := config.GetWorkspaceCheckout(connID, repoID, workspaceID)
	if co == nil {
		return "", "", fmt.Errorf("this device has no copy of that workspace")
	}
	return co.LocalPath, token, nil
}

// checkoutDestination picks the folder for a new working copy inside parent,
// guaranteeing the result does not already exist — so a failed clone can be
// cleaned up without any chance of deleting something that was already there.
//
// An explicitly chosen parent becomes the remembered default: picking a
// folder once is a statement about where this user keeps their work, and
// making them re-pick every time would be the annoying half of a setting
// without the setting.
func checkoutDestination(parent, projectSlug string) (string, error) {
	root := strings.TrimSpace(parent)
	if root != "" {
		abs, err := filepath.Abs(root)
		if err != nil {
			return "", err
		}
		root = abs
		if err := config.SetWorkspaceRoot(root); err != nil {
			return "", err
		}
	} else {
		var err error
		if root, err = config.WorkspaceRoot(); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create workspace folder %s: %w", root, err)
	}
	name := projectSlug
	if name == "" {
		name = "workspace"
	}
	candidate := filepath.Join(root, name)
	for i := 2; ; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
		candidate = filepath.Join(root, fmt.Sprintf("%s-%d", name, i))
		if i > 100 {
			return "", fmt.Errorf("couldn't find a free folder name under %s", root)
		}
	}
}

// progressLine matches the counter lines git writes while transferring, so
// the panel can show the same numbers a terminal would.
var progressLine = regexp.MustCompile(`^(Receiving objects|Resolving deltas|Counting objects|Compressing objects|Updating files):`)

// cloneWorkspace runs the clone, reporting progress lines as they arrive,
// and returns the branch that was checked out.
func cloneWorkspace(remote, token, dest string, onProgress func(string)) (string, error) {
	args := append(authArgs(token), "clone", "--progress", remote, dest)
	cmd := noPromptEnv(exec.Command("git", args...))
	hideConsole(cmd)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	// git writes progress to stderr, updating in place with carriage
	// returns; the tail is also where any error message appears.
	tail := scanGitProgress(stderr, onProgress)
	if err := cmd.Wait(); err != nil {
		if tail != "" {
			return "", fmt.Errorf("%s", tail)
		}
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitQuickTimeout)
	defer cancel()
	branch, _ := gitQuery(ctx, dest, "branch", "--show-current")
	return branch, nil
}

// scanGitProgress streams git's stderr, forwarding progress lines and
// keeping the last non-progress line as the error candidate.
func scanGitProgress(r io.Reader, onProgress func(string)) string {
	scanner := bufio.NewScanner(r)
	scanner.Split(scanCRLines)
	lastError := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if progressLine.MatchString(line) {
			onProgress(line)
			continue
		}
		// "fatal: …", "remote: …", "error: …" — whatever git ends on is
		// the most useful thing to show when the clone fails.
		lastError = line
	}
	return lastError
}

// scanCRLines splits on \r as well as \n: git's in-place progress updates
// are carriage-return separated, so a newline-only scanner would sit silent
// for the whole transfer and then emit one enormous line.
func scanCRLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// authArgs carries the device token as a request header rather than baking
// it into the remote URL. The token therefore never lands in .git/config,
// never shows up in `git remote -v`, and a re-paired device keeps working
// because every command reads the current token.
//
// It also disables git's credential helpers for BRUV's own commands. The
// server answers a rejected token with a Basic challenge (so a user CAN
// clone by hand), and a helper seeing that challenge would pop a system
// credential dialog behind the app — leaving a clone that never finishes
// and never explains itself. Failing immediately is the honest outcome.
func authArgs(token string) []string {
	args := []string{"-c", "credential.helper="}
	if token != "" {
		args = append(args, "-c", "http.extraHeader=Authorization: Bearer "+token)
	}
	return args
}

// noPromptEnv stops git falling back to an interactive terminal prompt for
// credentials, for the same reason authArgs clears the helpers.
func noPromptEnv(cmd *exec.Cmd) *exec.Cmd {
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0")
	return cmd
}

// gitAuthed runs a git command against the BRUV remote. timeout 0 means no
// deadline — fetches and pushes are as slow as the work is large.
func gitAuthed(ctx context.Context, dir, token string, timeout time.Duration, args ...string) (string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	full := append([]string{"-C", dir}, authArgs(token)...)
	full = append(full, args...)
	cmd := noPromptEnv(exec.CommandContext(ctx, "git", full...))
	hideConsole(cmd)
	out, err := cmd.Output()
	combined := strings.TrimSpace(string(out))
	if err != nil {
		detail := combined
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			detail = strings.TrimSpace(string(ee.Stderr))
		}
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], gitReason(detail))
	}
	return combined, nil
}

// gitQuery is a best-effort read-only git question; ok is false on any
// failure, and callers degrade rather than surfacing an error.
func gitQuery(ctx context.Context, dir string, args ...string) (string, bool) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	hideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// splitLines splits git output on either line ending, so parsing behaves
// the same whether git wrote CRLF (Windows) or LF.
func splitLines(out string) []string {
	return strings.FieldsFunc(out, func(r rune) bool { return r == 0x0A || r == 0x0D })
}

// gitReason pulls the one useful sentence out of a git failure.
//
// The naive "last non-empty line" is wrong for the case that matters most: a
// rejected push puts the reason in its `!` line and then FIVE lines of hints
// after it, so the last line is "hint: See the 'Note about fast-forwards' in
// 'git push --help' for details" — advice about a manual page, handed to
// someone whose actual problem is that a second laptop got there first.
//
// Preference order: the rejection line's parenthesised reason, then fatal:,
// then error: (skipping git's generic "failed to push some refs" wrapper),
// then the first line that isn't noise.
func gitReason(out string) string {
	var rejected, fatal, errLine, first string
	for _, raw := range splitLines(out) {
		line := strings.TrimSpace(raw)
		switch {
		case line == "",
			strings.HasPrefix(line, "hint:"),
			strings.HasPrefix(line, "warning:"),
			strings.HasPrefix(line, "Warning:"),
			strings.HasPrefix(line, "To "): // the remote URL echo
			continue
		}
		if rejected == "" && strings.Contains(line, "! [") && strings.Contains(line, "rejected]") {
			rejected = parenthesised(line)
			continue
		}
		if fatal == "" && strings.HasPrefix(line, "fatal:") {
			fatal = strings.TrimSpace(strings.TrimPrefix(line, "fatal:"))
			continue
		}
		if errLine == "" && strings.HasPrefix(line, "error:") {
			detail := strings.TrimSpace(strings.TrimPrefix(line, "error:"))
			// "failed to push some refs to '<url>'" restates the exit code.
			if !strings.HasPrefix(detail, "failed to push some refs") {
				errLine = detail
			}
			continue
		}
		if first == "" {
			first = line
		}
	}
	for _, candidate := range []string{rejected, fatal, errLine, first} {
		if candidate != "" {
			return candidate
		}
	}
	return strings.TrimSpace(out)
}

// parenthesised returns the text inside the last (…) of a git rejection
// line — "! [remote rejected] main -> main (Working directory has unstaged
// changes)" — falling back to the whole line when there is none.
func parenthesised(line string) string {
	open := strings.LastIndex(line, "(")
	closed := strings.LastIndex(line, ")")
	if open >= 0 && closed > open {
		return strings.TrimSpace(line[open+1 : closed])
	}
	return line
}

// firstMeaningfulLine summarises a SUCCESSFUL git run for the UI ("Fast
// forward", "Updating a1b2c3..d4e5f6"). Same noise filter, no error shapes.
func firstMeaningfulLine(out string) string {
	for _, raw := range splitLines(out) {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "hint:") || strings.HasPrefix(line, "warning:") {
			continue
		}
		return line
	}
	return ""
}
