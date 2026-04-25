package main

// Advisory lock on an open repo. Prevents a second BRUV process from
// mutating the same filesystem concurrently (which would race the
// atomic-write layer and corrupt the index). Implementation is a
// simple PID-based lock file at <repo>/.bruv/lock — no OS-level
// flock, because cross-platform flock adds dependency weight for
// marginal benefit at this scale. Stale locks (process no longer
// alive) are automatically overwritten so a crash doesn't brick a
// repo.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// lockFilePath returns the canonical path where a repo's lock file
// should live: <repo>/.bruv/lock.
func lockFilePath(repoRoot string) string {
	return filepath.Join(repoRoot, ".bruv", "lock")
}

// acquireRepoLock writes our PID into the repo's lock file. If an
// existing lock file holds a PID of a process that's still alive,
// returns an error. Stale locks (dead PID) are overwritten silently.
func (a *App) acquireRepoLock(repoRoot string) error {
	path := lockFilePath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir .bruv: %w", err)
	}

	if data, err := os.ReadFile(path); err == nil {
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		if pid > 0 && pid != os.Getpid() && processAlive(pid) {
			return fmt.Errorf("repo already open by PID %d (lock at %s)", pid, path)
		}
	}

	pid := strconv.Itoa(os.Getpid())
	return os.WriteFile(path, []byte(pid), 0o644)
}

// releaseRepoLock removes the lock file if it still contains our PID.
// Idempotent — safe to call with no lock held.
func (a *App) releaseRepoLock() {
	if a.repo == nil {
		return
	}
	path := lockFilePath(a.repo.Root)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if pid != os.Getpid() {
		return // someone else owns it now
	}
	_ = os.Remove(path)
}

// processAlive returns true if a process with the given PID is
// currently running. Implementation is platform-dependent; on both
// Unix and Windows, os.FindProcess followed by a zero-signal probe
// reports liveness without side effects.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix: signal 0 is the "are you alive" probe. On Windows:
	// os.FindProcess always succeeds but Signal(nil) returns an
	// error for dead processes, which is what we want.
	return p.Signal(syscallSignalZero) == nil
}
