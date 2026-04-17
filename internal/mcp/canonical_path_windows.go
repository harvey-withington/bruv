//go:build windows

package mcp

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// canonicalizeTempPath normalises a path to the form both ends of the
// MCP filesystem server will agree on. The server (written in Node)
// stores the allowed-directory argument verbatim but resolves inner
// operation targets via fs.realpath, which on Windows filesystems with
// 8.3 short-names enabled will return the long form for some paths and
// the short form for others. That mismatch is fatal for string-based
// path-containment checks.
//
// The canonical fix is to pre-expand 8.3 short names to their long
// form via GetLongPathNameW before handing the path to the server.
// This bites on CI runners whose username is > 8 chars (e.g. GitHub
// Actions' "runneradmin" → "RUNNER~1"); it's invisible locally for
// any user whose profile name fits in 8 chars.
//
// Also runs the path through filepath.EvalSymlinks to collapse any
// junctions (common under %LOCALAPPDATA%\Temp). Returns the input
// unchanged on any error — we'd rather get a test that runs and fails
// loudly than one that skips silently because path expansion tripped.
func canonicalizeTempPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	short, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return p
	}
	// Two-call pattern: first call returns required buffer length
	// (in UTF-16 code units, excluding trailing NUL); second call
	// fills the buffer. A zero return from the first call means the
	// API could not expand the path and we leave it as-is.
	n, err := windows.GetLongPathName(short, nil, 0)
	if err != nil || n == 0 {
		return p
	}
	buf := make([]uint16, n)
	n, err = windows.GetLongPathName(short, &buf[0], n)
	if err != nil || n == 0 {
		return p
	}
	return windows.UTF16ToString(buf[:n])
}
