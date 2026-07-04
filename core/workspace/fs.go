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

// FS abstracts "local directory" vs "remote listing + on-demand fetch" so the
// same adapter works at Tier 0 and Tier 1. M1 ships the local implementation;
// the remote one arrives with M2 transports.
type FS interface {
	// List returns the tree: slash-relative, sorted, .bruvignore and OS junk
	// excluded, no-descend dirs as bare entries, symlinks recorded not
	// followed. truncated is true when MaxIndexEntries was hit.
	List(ctx context.Context) (entries []model.WorkspaceEntry, truncated bool, err error)
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
	ignore *gitignore.GitIgnore // nil when no .bruvignore
}

// NewLocalFS opens dir, loading .bruvignore if present.
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
	if ign, err := gitignore.CompileIgnoreFile(filepath.Join(abs, BruvIgnoreFile)); err == nil {
		l.ignore = ign
	}
	return l, nil
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
		if l.ignore != nil && l.ignore.MatchesPath(slashRel) {
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
