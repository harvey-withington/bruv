// Package workspace holds the path-safety chokepoint for Workspace file
// access. Every RPC, shell method, and AI tool that touches a file inside a
// workspace resolves the path through Resolve — it is the single gateway
// between "a relative path from the outside world" and "a filesystem path we
// will actually open".
package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrPathEscapes is wrapped by every rejection Resolve produces.
var ErrPathEscapes = errors.New("path escapes the workspace root")

// Resolve validates rel against root and returns the absolute path to open.
//
// Rejected: absolute paths, drive-qualified paths ("C:x"), paths that clean
// to outside the root (".."), and paths whose existing ancestors resolve
// through a symlink to somewhere outside the root (symlinks inside a
// workspace are never followed out of it).
//
// rel is slash- or backslash-separated; "" and "." mean the root itself.
func Resolve(root, rel string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}

	// Reject anything that isn't a plain relative path: absolute paths,
	// drive-qualified paths ("C:x"), and rooted paths ("/etc", `\win`) —
	// the latter aren't IsAbs on Windows but still express absolute intent.
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" ||
		(rel != "" && (rel[0] == '/' || rel[0] == '\\')) {
		return "", fmt.Errorf("%q is not relative: %w", rel, ErrPathEscapes)
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q: %w", rel, ErrPathEscapes)
	}

	abs := rootAbs
	if clean != "." {
		abs = filepath.Join(rootAbs, clean)
	}

	// Symlink escape: resolve the deepest existing ancestor of abs and the
	// root, and require containment of the *resolved* paths. A symlink inside
	// the workspace pointing outside must not be traversable.
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	real, err := evalExisting(abs)
	if err != nil {
		return "", err
	}
	if !contains(rootReal, real) {
		return "", fmt.Errorf("%q resolves outside the workspace: %w", rel, ErrPathEscapes)
	}
	return abs, nil
}

// evalExisting resolves symlinks over the longest existing prefix of path and
// re-joins the (not-yet-existing) remainder, so paths about to be created are
// checked against where they would really land.
func evalExisting(path string) (string, error) {
	remainder := ""
	cur := path
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			if remainder == "" {
				return resolved, nil
			}
			return filepath.Join(resolved, remainder), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			// Walked to the volume root without finding an existing dir.
			return path, nil
		}
		if remainder == "" {
			remainder = filepath.Base(cur)
		} else {
			remainder = filepath.Join(filepath.Base(cur), remainder)
		}
		cur = parent
	}
}

// contains reports whether path is root or inside root. Case-insensitive on
// Windows and macOS (their default filesystems are).
func contains(root, path string) bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		root = strings.ToLower(root)
		path = strings.ToLower(path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
