//go:build !windows

package mcp

import "path/filepath"

// canonicalizeTempPath resolves symlinks so the MCP filesystem
// server's path-containment check works consistently. macOS trips
// on this: t.TempDir() returns /var/folders/..., but /var is a
// symlink to /private/var, and the server stores the resolved form
// (/private/var/...) for its allowed directory while comparing
// operation targets to the unresolved form we pass in tool args.
// Pre-resolving on our side makes both strings match.
//
// Linux is usually a no-op but harmless — if a distro ever puts
// /tmp behind a symlink (e.g. /tmp → /private/tmp-style), we're
// covered.
//
// On any EvalSymlinks error we return the input unchanged; a failing
// canonicalization should surface as a loud test failure rather than
// a silent skip.
func canonicalizeTempPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return p
}
