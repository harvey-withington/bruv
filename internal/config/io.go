package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// atomicWriteFile writes data to path via a temp file + rename so a
// crash mid-write never leaves a truncated/corrupt file behind. Same
// pattern as internal/repo's writeJSON, duplicated here because that
// helper is unexported and repo already depends on config.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("create temp %s: %w", tmp, err)
	}

	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()

	if writeErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("write temp %s: %w", tmp, writeErr)
	}
	if syncErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("sync temp %s: %w", tmp, syncErr)
	}
	if closeErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("close temp %s: %w", tmp, closeErr)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s → %s: %w", tmp, path, err)
	}
	return nil
}
