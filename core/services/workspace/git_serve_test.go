package workspace

// Publishing a workspace as git. The rule these tests exist to hold is that
// BRUV may create a repository, but must never rewrite one it didn't make:
// an existing history is the user's, and publishing is configuration.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bruv/internal/model"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
}

// gitIn runs a git command in dir, failing the test on error.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir,
		"-c", "user.name=Test", "-c", "user.email=test@localhost",
		"-c", "commit.gpgsign=false"}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestPrepareGitOriginInitialisesAPlainFolder(t *testing.T) {
	requireGit(t)
	dir := writeFiles(t, t.TempDir(), map[string]string{
		"README.md":      "# Song Alpha",
		"mix/track1.wav": "xxx",
	})

	branch, err := prepareGitOrigin(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if branch == "" {
		t.Error("a published workspace must report the branch clients should track")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("no repository created: %v", err)
	}
	// Everything present must be committed — a clone of a repo with no
	// commit, or with the files left unstaged, is an empty folder.
	if status := gitIn(t, dir, "status", "--porcelain"); status != "" {
		t.Errorf("initial commit left work uncommitted:\n%s", status)
	}
	files := gitIn(t, dir, "ls-files")
	for _, want := range []string{"README.md", "mix/track1.wav"} {
		if !strings.Contains(files, want) {
			t.Errorf("%s missing from the initial commit; got:\n%s", want, files)
		}
	}
	// Without this, a push from a checkout is refused by the host.
	if got := gitIn(t, dir, "config", "receive.denyCurrentBranch"); got != "updateInstead" {
		t.Errorf("receive.denyCurrentBranch = %q, want updateInstead", got)
	}
}

func TestPrepareGitOriginHonoursGitignore(t *testing.T) {
	requireGit(t)
	dir := writeFiles(t, t.TempDir(), map[string]string{
		".gitignore":            "node_modules/\n",
		"index.js":              "console.log(1)",
		"node_modules/dep/a.js": "junk",
	})

	if _, err := prepareGitOrigin(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if files := gitIn(t, dir, "ls-files"); strings.Contains(files, "node_modules") {
		t.Errorf("the folder's own .gitignore must govern the initial commit; got:\n%s", files)
	}
}

func TestPrepareGitOriginLeavesExistingHistoryAlone(t *testing.T) {
	requireGit(t)
	dir := writeFiles(t, t.TempDir(), map[string]string{"a.txt": "one"})
	gitIn(t, dir, "init")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-m", "the user's own commit")
	head := gitIn(t, dir, "rev-parse", "HEAD")

	// Uncommitted work at the moment of publishing: BRUV must not sweep it
	// into a commit on the user's behalf.
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("two"), 0o644)

	if _, err := prepareGitOrigin(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if now := gitIn(t, dir, "rev-parse", "HEAD"); now != head {
		t.Errorf("publishing moved HEAD from %s to %s — existing history is the user's", head, now)
	}
	if status := gitIn(t, dir, "status", "--porcelain"); !strings.Contains(status, "b.txt") {
		t.Errorf("publishing committed the user's uncommitted work; status:\n%s", status)
	}
}

func TestPrepareGitOriginIsIdempotent(t *testing.T) {
	requireGit(t)
	dir := writeFiles(t, t.TempDir(), map[string]string{"a.txt": "one"})
	if _, err := prepareGitOrigin(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	head := gitIn(t, dir, "rev-parse", "HEAD")
	if _, err := prepareGitOrigin(context.Background(), dir); err != nil {
		t.Fatalf("re-publishing must succeed: %v", err)
	}
	if now := gitIn(t, dir, "rev-parse", "HEAD"); now != head {
		t.Error("re-publishing must not add a commit")
	}
}

func TestInspectGitServeReportsWithoutWriting(t *testing.T) {
	requireGit(t)
	svc, _, b, st, p := newTestService(t)
	dir := writeFiles(t, t.TempDir(), map[string]string{
		".gitignore":  "build/\n",
		"song.md":     "lyrics",
		"build/o.bin": "generated",
	})
	if _, err := svc.Attach(context.Background(), b, st, p, dir); err != nil {
		t.Fatal(err)
	}

	rep, err := svc.InspectGitServe(context.Background(), b, st, p)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.GitAvailable {
		t.Fatal("git is on PATH but the report says otherwise")
	}
	if rep.IsRepo {
		t.Error("a plain folder must not report as a repository")
	}
	if !rep.HasGitignore {
		t.Error("the folder has a .gitignore; the report must say so")
	}
	// The estimate is what would be committed — ignored output excluded.
	if rep.Files != 2 {
		t.Errorf("files = %d, want 2 (.gitignore + song.md, build/ excluded)", rep.Files)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Error("inspecting must not create a repository")
	}
}

func TestEnableGitServeLifecycle(t *testing.T) {
	requireGit(t)
	svc, deps, b, st, p := newTestService(t)
	dir := writeFiles(t, t.TempDir(), map[string]string{"song.md": "lyrics"})
	ws, err := svc.Attach(context.Background(), b, st, p, dir)
	if err != nil {
		t.Fatal(err)
	}
	// Not published until asked: an attached workspace is not on the
	// network by default.
	if _, ok := svc.GitServeDir(ws.ID); ok {
		t.Fatal("an unpublished workspace must not be servable")
	}

	started, err := svc.EnableGitServe(context.Background(), b, st, p)
	if err != nil {
		t.Fatal(err)
	}
	if started.GitServe != model.GitServeInitializing {
		t.Errorf("state = %q, want initializing — the caller must not be left waiting", started.GitServe)
	}

	ready := waitForGitServe(t, svc, b, st, p, model.GitServeReady)
	if ready.DefaultBranch == "" {
		t.Error("a ready workspace must name the branch clients track")
	}
	if ready.GitServeError != "" {
		t.Errorf("unexpected error: %s", ready.GitServeError)
	}
	if got, ok := svc.GitServeDir(ws.ID); !ok || got != dir {
		t.Errorf("GitServeDir = %q, %v; want %q, true", got, ok, dir)
	}
	if !deps.emitted("workspace:updated") {
		t.Error("lifecycle transitions must be announced so every client sees them")
	}

	// Un-publishing is a BRUV decision and must not touch the repository.
	if _, err := svc.DisableGitServe(b, st, p); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.GitServeDir(ws.ID); ok {
		t.Error("a disabled workspace must leave the network")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Error("un-publishing deleted the user's repository")
	}
}

// waitForGitServe polls the vault record until it reaches want. The work
// runs in the background by design, so the test observes it the same way a
// client does.
func waitForGitServe(t *testing.T, svc *Service, b, st, p, want string) *model.Workspace {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		ws, err := svc.Get(b, st, p)
		if err == nil && ws.GitServe == want {
			return ws
		}
		if err == nil && ws.GitServe == model.GitServeError {
			t.Fatalf("publishing failed: %s", ws.GitServeError)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("workspace never reached state %q", want)
	return nil
}
