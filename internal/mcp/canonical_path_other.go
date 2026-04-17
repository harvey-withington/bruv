//go:build !windows

package mcp

// canonicalizeTempPath is a no-op on non-Windows platforms — only
// Windows has the 8.3 short-name quirk that the MCP filesystem server
// trips over. See canonical_path_windows.go for the problem.
func canonicalizeTempPath(p string) string {
	return p
}
