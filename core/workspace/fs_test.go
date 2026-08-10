package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkTree creates dirs/files under root from slash-relative paths; a path
// ending in "/" is a directory.
func mkTree(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(root, filepath.FromSlash(p))
		if p[len(p)-1] == '/' {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func newFS(t *testing.T, dir string) *LocalFS {
	t.Helper()
	fsys, err := NewLocalFS(dir)
	if err != nil {
		t.Fatalf("NewLocalFS: %v", err)
	}
	return fsys
}

func listDirPaths(t *testing.T, fsys *LocalFS, rel string) []string {
	t.Helper()
	entries, err := fsys.ListDir(context.Background(), rel)
	if err != nil {
		t.Fatalf("ListDir(%q): %v", rel, err)
	}
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		paths = append(paths, e.Path)
	}
	return paths
}

func eq(t *testing.T, got, want []string, ctx string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %v, want %v", ctx, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: got %v, want %v", ctx, got, want)
		}
	}
}

// ListDir is what makes lazy browsing possible: opening a directory must
// cost that directory only. Harvey, 2026-08-02, rejecting a name-based
// blocklist as a band-aid: the same problem arrives via a nested .git, a
// Rust target/, or a folder of 80k photos — the set is unguessable.
func TestListDirIsOneLevelOnly(t *testing.T) {
	dir := t.TempDir()
	mkTree(t, dir,
		"README.md",
		"src/main.go",
		"src/deep/nested/file.txt",
		"node_modules/left-pad/index.js",
		"node_modules/.bin/tsc",
	)
	fsys := newFS(t, dir)

	// Root: children only — nothing from inside src/ or node_modules/.
	eq(t, listDirPaths(t, fsys, ""), []string{"README.md", "node_modules", "src"}, "root")

	// The heavy directory is a normal entry; its contents cost nothing
	// until asked for, and then only one level of them.
	eq(t, listDirPaths(t, fsys, "node_modules"),
		[]string{"node_modules/.bin", "node_modules/left-pad"}, "node_modules")

	eq(t, listDirPaths(t, fsys, "src"), []string{"src/deep", "src/main.go"}, "src")
	eq(t, listDirPaths(t, fsys, "src/deep"), []string{"src/deep/nested"}, "src/deep")
}

func TestListDirRootAliases(t *testing.T) {
	dir := t.TempDir()
	mkTree(t, dir, "a.txt")
	fsys := newFS(t, dir)
	for _, rel := range []string{"", ".", "/"} {
		eq(t, listDirPaths(t, fsys, rel), []string{"a.txt"}, "root alias "+rel)
	}
}

func TestListDirRefusesEscapes(t *testing.T) {
	dir := t.TempDir()
	mkTree(t, dir, "a.txt")
	fsys := newFS(t, dir)
	for _, rel := range []string{"..", "../..", "../secrets"} {
		if _, err := fsys.ListDir(context.Background(), rel); err == nil {
			t.Errorf("ListDir(%q) should refuse to escape the workspace", rel)
		}
	}
}

// REGRESSION (2026-08-11): a freshly published workspace's browse tree
// showed its .git folder — noDescendDirs was dropped by the eager walk's
// adapter but not by the lazy ListDir path.
func TestListDirHidesNoDescendDirs(t *testing.T) {
	dir := t.TempDir()
	mkTree(t, dir, "keep.md", ".git/HEAD", ".git/objects/x", ".obsidian/app.json")
	fsys := newFS(t, dir)
	eq(t, listDirPaths(t, fsys, ""), []string{"keep.md"}, "root listing")
}

func TestListDirHonoursIgnoresAndJunk(t *testing.T) {
	dir := t.TempDir()
	mkTree(t, dir, "keep.md", "secret/pw.txt", ".DS_Store", "logs/app.log")
	if err := os.WriteFile(filepath.Join(dir, BruvIgnoreFile), []byte("secret/\nlogs/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fsys := newFS(t, dir)

	got := listDirPaths(t, fsys, "")
	for _, unwanted := range []string{".DS_Store"} {
		for _, g := range got {
			if g == unwanted {
				t.Errorf("%s should be excluded", unwanted)
			}
		}
	}
	// The .bruvignore'd directories' CONTENTS are excluded when listed.
	if inner := listDirPaths(t, fsys, "secret"); len(inner) != 0 {
		t.Errorf("ignored dir contents should be excluded, got %v", inner)
	}
	if !contains(got, "keep.md") || !contains(got, BruvIgnoreFile) {
		t.Errorf("normal files missing from root listing: %v", got)
	}
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// A workspace that's a git repo already declares what's junk. Honouring
// it is why BRUV doesn't need a central list of "heavy" directory names
// (ruled 2026-08-02) — the person whose workspace it is has already said.
func TestGitignoreIsHonoured(t *testing.T) {
	dir := t.TempDir()
	mkTree(t, dir,
		"src/main.ts",
		"node_modules/left-pad/index.js",
		"dist/bundle.js",
		"notes.md",
	)
	if err := os.WriteFile(filepath.Join(dir, GitIgnoreFile),
		[]byte("node_modules/\ndist/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fsys := newFS(t, dir)

	got := listDirPaths(t, fsys, "")
	for _, unwanted := range []string{"node_modules", "dist"} {
		if contains(got, unwanted) {
			t.Errorf("%s is gitignored and should not be listed; got %v", unwanted, got)
		}
	}
	for _, want := range []string{"src", "notes.md"} {
		if !contains(got, want) {
			t.Errorf("%s should still be listed; got %v", want, got)
		}
	}

	// The whole-tree index honours it too, so the AI summary isn't
	// drowned in dependency noise either.
	entries, _, err := fsys.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Path, "node_modules") || strings.HasPrefix(e.Path, "dist") {
			t.Errorf("gitignored path %s leaked into the index", e.Path)
		}
	}
}

// .bruvignore is layered AFTER .gitignore, so gitignore-syntax negation
// lets a user disagree with their repo's rules — "I do want dist/ in
// BRUV" — without editing .gitignore.
func TestBruvignoreOverridesGitignore(t *testing.T) {
	dir := t.TempDir()
	mkTree(t, dir, "dist/report.pdf", "node_modules/pkg/index.js", "src/a.ts")
	if err := os.WriteFile(filepath.Join(dir, GitIgnoreFile),
		[]byte("node_modules/\ndist/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, BruvIgnoreFile),
		[]byte("!dist/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fsys := newFS(t, dir)

	got := listDirPaths(t, fsys, "")
	if !contains(got, "dist") {
		t.Errorf("!dist/ in .bruvignore should un-hide it; got %v", got)
	}
	if contains(got, "node_modules") {
		t.Errorf("node_modules was not un-hidden and should stay excluded; got %v", got)
	}
}

func TestNoIgnoreFilesListsEverything(t *testing.T) {
	dir := t.TempDir()
	mkTree(t, dir, "a.txt", "b/c.txt")
	got := listDirPaths(t, newFS(t, dir), "")
	if len(got) != 2 {
		t.Errorf("with no ignore files everything should list; got %v", got)
	}
}

// The whole-tree List still exists for the adapter index (summary + AI);
// it must keep its exclusions. Browsing no longer uses it.
func TestListStillWalksEverythingForTheIndex(t *testing.T) {
	dir := t.TempDir()
	mkTree(t, dir, "src/main.go", "docs/notes.md", "deep/a/b/c.txt")
	fsys := newFS(t, dir)

	entries, truncated, err := fsys.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if truncated {
		t.Error("small tree should not report truncation")
	}
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Path] = true
	}
	for _, want := range []string{"src/main.go", "docs/notes.md", "deep/a/b/c.txt"} {
		if !seen[want] {
			t.Errorf("%s missing from the whole-tree index", want)
		}
	}
}
