package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestTryLockIsExclusiveAndReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".bruv", "instance.lock")

	release, err := TryLock(path, "BRUV Server, pid 1")
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}

	// A second handle — the same thing another process would do — is refused
	// and can read who holds it.
	_, err = TryLock(path, "BRUV desktop, pid 2")
	var held *LockHeldError
	if !errors.As(err, &held) || !errors.Is(err, ErrLocked) {
		t.Fatalf("second lock = %v, want LockHeldError", err)
	}
	if held.Holder != "BRUV Server, pid 1" {
		t.Errorf("holder = %q", held.Holder)
	}

	release()
	release2, err := TryLock(path, "BRUV desktop, pid 2")
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	release2()
}

// A lock file left behind by a crashed process (content, no live lock)
// never blocks the next opener.
func TestTryLockIgnoresStaleFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.lock")
	if err := os.WriteFile(path, []byte("BRUV desktop, pid 999 (crashed)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	release, err := TryLock(path, "BRUV desktop, pid 3")
	if err != nil {
		t.Fatalf("stale file blocked the lock: %v", err)
	}
	release()
}
