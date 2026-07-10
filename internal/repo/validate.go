package repo

import (
	"fmt"
	"path/filepath"
	"strings"
)

// RPC-supplied identifiers (card IDs, attachment IDs, slugs) flow into
// filepath.Join under the repo root. Without validation an ID like
// "../../../x" reads or writes files outside the repository — under
// the shared home-server model that's any file the service account
// can touch. internal/workspace has its own Resolve chokepoint for
// workspace paths; this is the equivalent for repo-object IDs.

// isSafeSeg reports whether s can be used as a single path segment
// under the repo root: non-empty, no separators, no traversal, no
// Windows drive/device tricks (filepath.IsLocal covers "..", ".",
// reserved device names, and drive-relative forms).
func isSafeSeg(s string) bool {
	// "." passes filepath.IsLocal but as a segment resolves to the
	// parent itself — pinsDirPath(".") would be the whole pins dir.
	if s == "" || s == "." || s == ".." {
		return false
	}
	if strings.ContainsAny(s, `/\`) || strings.ContainsRune(s, 0) {
		return false
	}
	return filepath.IsLocal(s)
}

// safeSeg returns s unchanged when it is a safe path segment, and a
// sentinel that cannot exist on any filesystem otherwise (NUL is
// invalid in paths on both Windows and Unix, so every os call on the
// resulting path fails cleanly instead of escaping the repo). Used
// inside the string-returning path builders so that every caller is
// covered without signature churn; the public entry points also call
// validID for a readable error on the common routes.
func safeSeg(s string) string {
	if isSafeSeg(s) {
		return s
	}
	return "\x00invalid-path-segment"
}

// validID rejects identifiers that are not safe path segments.
func validID(id string) error {
	if !isSafeSeg(id) {
		return fmt.Errorf("invalid id %q", id)
	}
	return nil
}
