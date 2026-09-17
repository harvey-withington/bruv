package workspace

// Commit-on-save: BRUV's own writes to a published workspace must not leave
// the host tree dirty, or a clone's next push is refused by updateInstead.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bruv/internal/model"
)

func publishedTestWorkspace(t *testing.T) (*Service, string, string, string, string) {
	t.Helper()
	requireGit(t)
	svc, _, b, st, p := newTestService(t)
	dir := writeFiles(t, t.TempDir(), map[string]string{
		".gitignore": "scratch/\n",
		"notes.md":   "# notes",
	})
	if _, err := svc.Attach(context.Background(), b, st, p, dir); err != nil {
		t.Fatal(err)
	}
	branch, created, err := prepareGitOrigin(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	svc.finishGitServe(b, st, p, branch, created, nil)
	ws, err := svc.Get(b, st, p)
	if err != nil {
		t.Fatal(err)
	}
	if ws.GitServe != model.GitServeReady || !ws.CommitOnSave {
		t.Fatalf("a repository BRUV created must publish with commit-on-save on: %+v", ws)
	}
	return svc, dir, b, st, p
}

func TestCommitOnSaveKeepsHostTreeClean(t *testing.T) {
	svc, dir, b, st, p := publishedTestWorkspace(t)
	ctx := context.Background()
	head := gitIn(t, dir, "rev-parse", "HEAD")

	// Editor save → a commit, tree clean afterwards.
	if _, err := svc.SaveFile(ctx, b, st, p, "notes.md", "# notes\n\nedited in BRUV", ""); err != nil {
		t.Fatal(err)
	}
	if status := gitIn(t, dir, "status", "--porcelain"); status != "" {
		t.Errorf("save left the host tree dirty:\n%s", status)
	}
	if now := gitIn(t, dir, "rev-parse", "HEAD"); now == head {
		t.Error("save did not commit")
	}
	if msg := gitIn(t, dir, "log", "-1", "--format=%s"); !strings.Contains(msg, "notes.md") {
		t.Errorf("commit message should name the file, got %q", msg)
	}

	// New file → committed too.
	if _, err := svc.CreateFile(ctx, b, st, p, "chapters/01.md"); err != nil {
		t.Fatal(err)
	}
	if status := gitIn(t, dir, "status", "--porcelain"); status != "" {
		t.Errorf("create left the host tree dirty:\n%s", status)
	}

	// An ignored path is skipped, not an error.
	if _, err := svc.CreateFile(ctx, b, st, p, "scratch/tmp.md"); err != nil {
		t.Fatalf("saving into an ignored folder must still succeed: %v", err)
	}
	if files := gitIn(t, dir, "ls-files"); strings.Contains(files, "scratch/") {
		t.Errorf("ignored path was committed:\n%s", files)
	}
}

func TestCommitOnSaveLeavesTheUsersOwnWorkAlone(t *testing.T) {
	svc, dir, b, st, p := publishedTestWorkspace(t)
	ctx := context.Background()

	// The user is mid-edit on another file, uncommitted, on the host.
	if err := os.WriteFile(filepath.Join(dir, "draft.md"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveFile(ctx, b, st, p, "notes.md", "# notes v2", ""); err != nil {
		t.Fatal(err)
	}
	status := gitIn(t, dir, "status", "--porcelain")
	if !strings.Contains(status, "draft.md") {
		t.Errorf("BRUV's commit must not sweep the user's uncommitted draft.md in; status:\n%s", status)
	}
	if strings.Contains(status, "notes.md") {
		t.Errorf("notes.md should have been committed; status:\n%s", status)
	}
}

func TestCommitOnSaveOffLeavesTreeDirty(t *testing.T) {
	svc, dir, b, st, p := publishedTestWorkspace(t)
	ctx := context.Background()
	if _, err := svc.SetCommitOnSave(b, st, p, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveFile(ctx, b, st, p, "notes.md", "# notes v3", ""); err != nil {
		t.Fatal(err)
	}
	if status := gitIn(t, dir, "status", "--porcelain"); !strings.Contains(status, "notes.md") {
		t.Errorf("with commit-on-save off the save must stay uncommitted; status:\n%s", status)
	}
}
