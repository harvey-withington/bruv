package workspace

import (
	"context"
	"os"
	"path/filepath"
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

func listPaths(t *testing.T, dir string) (map[string]bool, []string) {
	t.Helper()
	fsys, err := NewLocalFS(dir)
	if err != nil {
		t.Fatalf("NewLocalFS: %v", err)
	}
	entries, _, err := fsys.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	set := make(map[string]bool, len(entries))
	var notIndexed []string
	for _, e := range entries {
		set[e.Path] = true
		if e.NotIndexed {
			notIndexed = append(notIndexed, e.Path)
		}
	}
	return set, notIndexed
}

// A single node_modules can hold 30k+ files — enough on its own to
// exhaust MaxIndexEntries, which didn't just slow the tree down, it
// truncated the user's real work out of the index (2026-08-02).
func TestListRecordsButDoesNotWalkGeneratedDirs(t *testing.T) {
	dir := t.TempDir()
	mkTree(t, dir,
		"src/main.go",
		"node_modules/left-pad/index.js",
		"node_modules/.bin/tsc",
		"api/__pycache__/mod.cpython-311.pyc",
		"web/.svelte-kit/generated/root.js",
		"docs/notes.md",
	)

	paths, notIndexed := listPaths(t, dir)

	// The folders stay VISIBLE — hiding them would be its own lie.
	for _, want := range []string{"node_modules", "api/__pycache__", "web/.svelte-kit"} {
		if !paths[want] {
			t.Errorf("%s should be listed as an entry", want)
		}
	}
	// ...but nothing inside them is indexed.
	for _, unwanted := range []string{
		"node_modules/left-pad", "node_modules/left-pad/index.js", "node_modules/.bin/tsc",
		"api/__pycache__/mod.cpython-311.pyc", "web/.svelte-kit/generated/root.js",
	} {
		if paths[unwanted] {
			t.Errorf("%s must not be walked into", unwanted)
		}
	}
	// Real content is untouched.
	for _, want := range []string{"src/main.go", "docs/notes.md", "src", "docs", "api", "web"} {
		if !paths[want] {
			t.Errorf("%s missing from the index", want)
		}
	}
	// The flag is what stops the UI rendering them as empty folders.
	if len(notIndexed) != 3 {
		t.Errorf("NotIndexed entries = %v, want the 3 generated dirs", notIndexed)
	}
}

// Ambiguous names stay walkable on purpose: dist/build/target/bin/vendor
// routinely hold real content someone wants to browse, and .bruvignore is
// the escape hatch for the cases where they don't.
func TestListStillWalksAmbiguousDirs(t *testing.T) {
	dir := t.TempDir()
	mkTree(t, dir,
		"dist/bundle.js",
		"build/report.pdf",
		"target/notes.txt",
		"vendor/lib/thing.go",
	)

	paths, notIndexed := listPaths(t, dir)

	for _, want := range []string{"dist/bundle.js", "build/report.pdf", "target/notes.txt", "vendor/lib/thing.go"} {
		if !paths[want] {
			t.Errorf("%s should still be indexed (ambiguous dirs stay walkable)", want)
		}
	}
	if len(notIndexed) != 0 {
		t.Errorf("no ambiguous dir should be flagged NotIndexed, got %v", notIndexed)
	}
}

func TestListStillHonoursBruvignoreAndOSJunk(t *testing.T) {
	dir := t.TempDir()
	mkTree(t, dir, "keep/file.md", "secret/passwords.txt", ".DS_Store")
	if err := os.WriteFile(filepath.Join(dir, BruvIgnoreFile), []byte("secret/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	paths, _ := listPaths(t, dir)

	if !paths["keep/file.md"] {
		t.Error("normal content should be indexed")
	}
	if paths["secret/passwords.txt"] {
		t.Error(".bruvignore contents must stay excluded")
	}
	// Nuance worth pinning rather than assuming: a `dir/` pattern excludes
	// the CONTENTS; the bare directory entry itself is still listed (the
	// gitignore matcher doesn't match "secret" against "secret/"). Same
	// shape as the NotIndexed treatment above — you can see the folder,
	// you just can't see into it.
	if !paths["secret"] {
		t.Error("the ignored directory itself is expected to remain visible; " +
			"if that changed deliberately, update this test")
	}
	if paths[".DS_Store"] {
		t.Error("OS junk must stay excluded")
	}
}
