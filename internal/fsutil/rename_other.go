//go:build !windows

package fsutil

// POSIX rename replaces an open destination without error, so nothing is
// transient here.
func isTransientRenameError(error) bool { return false }
