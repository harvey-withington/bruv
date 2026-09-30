package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrLocked is wrapped by LockHeldError, so callers can test with
// errors.Is(err, fsutil.ErrLocked).
var ErrLocked = errors.New("locked by another process")

// LockHeldError reports that another process holds the lock. Holder is
// whatever that process wrote into the lock file when it took the lock
// (empty if it couldn't be read).
type LockHeldError struct {
	Path   string
	Holder string
}

func (e *LockHeldError) Error() string {
	if e.Holder == "" {
		return fmt.Sprintf("%s: %v", e.Path, ErrLocked)
	}
	return fmt.Sprintf("%s: %v (%s)", e.Path, ErrLocked, e.Holder)
}

func (e *LockHeldError) Unwrap() error { return ErrLocked }

// TryLock takes an exclusive, non-blocking, OS-level lock on path
// (created if missing) and records holder in it. The OS drops the lock
// when the process exits, crash included, so a stale lock file never
// blocks anyone. Returns a *LockHeldError if another process — or
// another handle in this process — holds it; call release to unlock.
//
// The lock covers a byte range far past the file's content, so other
// processes can still read the holder text while it's held.
func TryLock(path, holder string) (release func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", path, err)
	}
	held, err := lockFile(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	if held {
		data, _ := os.ReadFile(path)
		f.Close()
		return nil, &LockHeldError{Path: path, Holder: strings.TrimSpace(string(data))}
	}
	// Best effort: the holder text is diagnostics only.
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(holder+"\n"), 0)
	}
	return func() {
		_ = f.Truncate(0)
		unlockFile(f)
		f.Close()
	}, nil
}
