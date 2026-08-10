// Package workspace (core/workspace) is the Workspace engine: the FS
// abstraction adapters index through, and the Transport seam the checkout
// lifecycle uses. One package, two mounts — the Runtime service today (M1,
// local origins), the device-side ShellAPI from M3 (materialize/check-in).
package workspace

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"bruv/internal/model"

	gitignore "github.com/sabhiram/go-gitignore"
)

// MaxIndexEntries bounds the indexed tree. Beyond it the tree is truncated
// and a warning recorded — huge media workspaces are expected to stay
// Tier 0 or use coarse indexes, not to produce 100k-entry index.json files.
const MaxIndexEntries = 20000

// BruvIgnoreFile is honoured (gitignore syntax) by all adapters and
// transports, at the workspace root.
const BruvIgnoreFile = ".bruvignore"

// GitIgnoreFile is honoured too, when the workspace happens to be a repo.
// A project's own .gitignore is the best available statement of "this is
// generated junk, not my work" — written by the person whose workspace it
// is, kept current by them, and already excluding the node_modules /
// build-output / cache directories that BRUV has no business indexing.
// Far better than guessing directory names centrally (ruled 2026-08-02).
const GitIgnoreFile = ".gitignore"

// osJunk is always ignored regardless of .bruvignore.
var osJunk = map[string]bool{
	".DS_Store":   true,
	"Thumbs.db":   true,
	"desktop.ini": true,
}

// noDescendDirs are recorded as bare entries but never walked into: their
// internals are VCS/app state, not workspace content. Adapters use the bare
// entries for detection (git-repo, obsidian-vault) and drop them from the
// stored tree.
var noDescendDirs = map[string]bool{
	".git":      true,
	".obsidian": true,
}

// NOTE (2026-08-02): a blocklist of "heavy" directory names (node_modules,
// .venv, …) briefly lived here. Harvey rejected it as a band-aid, and he
// was right: the same problem arrives via a nested .git, a Rust target/, a
// folder of 80k photos — the set is unguessable. The browse tree now lists
// ONE directory at a time (ListDir below), so the cost of a directory is
// paid only when a user actually opens it, whatever it's called.

// FS abstracts "local directory" vs "remote listing + on-demand fetch" so the
// same adapter works at Tier 0 and Tier 1. M1 ships the local implementation;
// the remote one arrives with M2 transports.
type FS interface {
	// List returns the tree: slash-relative, sorted, .bruvignore and OS junk
	// excluded, no-descend dirs as bare entries, symlinks recorded not
	// followed. truncated is true when MaxIndexEntries was hit.
	//
	// This walks EVERYTHING and is therefore only for the adapter index
	// (summary + AI). Browsing uses ListDir.
	List(ctx context.Context) (entries []model.WorkspaceEntry, truncated bool, err error)
	// ListDir returns the immediate children of one directory (rel "" or
	// "." = the workspace root), same exclusions as List, sorted the same
	// way. Cost is proportional to that ONE directory, never to the tree
	// beneath it — which is what lets the UI browse a workspace containing
	// node_modules, a nested .git, or 80k photos without paying for them
	// until someone actually opens the folder.
	ListDir(ctx context.Context, rel string) ([]model.WorkspaceEntry, error)
	// Read returns up to maxBytes of one file. It must reject reads outside
	// the workspace root.
	Read(ctx context.Context, rel string, maxBytes int64) ([]byte, error)
	// LocalDir returns the on-disk root when the files are local — adapters
	// use it for capabilities that need a real directory (git shell-out).
	LocalDir() (string, bool)
}

// LocalFS is the Tier 1 FS over an on-disk directory.
type LocalFS struct {
	dir    string
	ignore *gitignore.GitIgnore // nil when neither ignore file is present
}

// NewLocalFS opens dir, honouring the workspace root's .gitignore and
// .bruvignore.
//
// Both compile into ONE matcher, .gitignore first, so .bruvignore is the
// override layer: gitignore syntax's negation means `!dist/` in
// .bruvignore un-hides something .gitignore excluded. That ordering is
// the whole point — the repo's rules are the sensible default, and BRUV's
// own file is how you disagree with them.
func NewLocalFS(dir string) (*LocalFS, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	l := &LocalFS{dir: abs}
	l.ignore = compileIgnores(abs)
	return l, nil
}

// isIgnored reports whether a path is excluded by the compiled ignore
// rules.
//
// Directories are tested BOTH bare and with a trailing slash: a
// `node_modules/` rule (the overwhelmingly common way people write it)
// does not match the bare path "node_modules", so without this the
// directory itself stayed visible and only its contents were hidden —
// i.e. an ignored folder you could still open and find empty.
func (l *LocalFS) isIgnored(slashRel string, isDir bool) bool {
	if l.ignore == nil {
		return false
	}
	if l.ignore.MatchesPath(slashRel) {
		return true
	}
	return isDir && l.ignore.MatchesPath(slashRel+"/")
}

// compileIgnores reads the root's ignore files into a single matcher.
// Returns nil when neither exists (no matcher = nothing excluded).
func compileIgnores(root string) *gitignore.GitIgnore {
	var lines []string
	for _, name := range []string{GitIgnoreFile, BruvIgnoreFile} {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue // absent or unreadable: that file simply contributes nothing
		}
		lines = append(lines, strings.Split(string(raw), "\n")...)
	}
	if len(lines) == 0 {
		return nil
	}
	return gitignore.CompileIgnoreLines(lines...)
}

// LocalDir implements FS.
func (l *LocalFS) LocalDir() (string, bool) { return l.dir, true }

// List implements FS.
func (l *LocalFS) List(ctx context.Context) ([]model.WorkspaceEntry, bool, error) {
	var entries []model.WorkspaceEntry
	truncated := false
	err := filepath.WalkDir(l.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, err := filepath.Rel(l.dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		slashRel := filepath.ToSlash(rel)
		name := d.Name()

		if osJunk[name] {
			return nil
		}
		if l.isIgnored(slashRel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if len(entries) >= MaxIndexEntries {
			truncated = true
			return filepath.SkipAll
		}

		e := model.WorkspaceEntry{Path: slashRel, IsDir: d.IsDir()}
		if d.Type()&fs.ModeSymlink != 0 {
			e.Symlink = true
			entries = append(entries, e)
			return nil // never followed
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				e.Size = info.Size()
			}
		}
		entries = append(entries, e)
		if d.IsDir() && noDescendDirs[name] {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, truncated, nil
}

// Read implements FS. rel goes through the path-safety rules indirectly: the
// caller (service) resolves via internal/workspace.Resolve before handing
// paths to anything else, but LocalFS defends itself too.
// ListDir implements FS. One directory, no recursion.
func (l *LocalFS) ListDir(ctx context.Context, rel string) ([]model.WorkspaceEntry, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || clean == string(filepath.Separator) {
		clean = ""
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path %q escapes the workspace", rel)
	}
	dir := filepath.Join(l.dir, clean)
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	slashDir := filepath.ToSlash(clean)
	entries := make([]model.WorkspaceEntry, 0, len(items))
	for _, d := range items {
		name := d.Name()
		// noDescendDirs too: the eager walk records them as bare entries
		// for adapter detection and the adapter drops them from the stored
		// tree — this lazy path feeds the browse tree directly, so without
		// the same drop a published workspace showed its .git folder.
		if osJunk[name] || noDescendDirs[name] {
			continue
		}
		slashRel := name
		if slashDir != "" {
			slashRel = slashDir + "/" + name
		}
		if l.isIgnored(slashRel, d.IsDir()) {
			continue
		}
		e := model.WorkspaceEntry{Path: slashRel, IsDir: d.IsDir()}
		if d.Type()&fs.ModeSymlink != 0 {
			e.Symlink = true // recorded, never followed
			entries = append(entries, e)
			continue
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				e.Size = info.Size()
			}
		}
		entries = append(entries, e)
	}
	// Same ordering as List: full path ascending, so a lazily-loaded level
	// reads identically to the eager tree it replaced.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func (l *LocalFS) Read(ctx context.Context, rel string, maxBytes int64) ([]byte, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path %q escapes the workspace", rel)
	}
	path := filepath.Join(l.dir, clean)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory", rel)
	}
	if maxBytes > 0 && info.Size() > maxBytes {
		return nil, fmt.Errorf("%s is %d bytes (limit %d)", rel, info.Size(), maxBytes)
	}
	return os.ReadFile(path)
}
