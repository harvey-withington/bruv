package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bruv/internal/fsutil"
)

// validPathSegment rejects values that cannot be used as a single
// path segment under the config directory (empty, separators,
// traversal, Windows device names). RPC-supplied IDs (chat IDs, repo
// IDs) must pass through this before reaching filepath.Join.
func validPathSegment(s string) error {
	if s == "" || s == "." || s == ".." ||
		strings.ContainsAny(s, `/\`) || strings.ContainsRune(s, 0) || !filepath.IsLocal(s) {
		return fmt.Errorf("invalid path segment %q", s)
	}
	return nil
}

// atomicWriteFile writes data to path via a unique temp file + rename so a
// crash mid-write never leaves a truncated/corrupt file behind. One shared
// implementation (with the Windows rename retry) with internal/repo.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	return fsutil.WriteFileAtomic(path, data, perm)
}
