// Package fsutil holds the one atomic-write implementation shared by
// internal/repo and internal/config.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// renameTimeout / renameBackoff* bound the retry around os.Rename. On
// Windows, MoveFileEx fails with a sharing violation or access denied while
// another handle (a reader, the file watcher, Syncthing, antivirus) holds
// the destination open; those holds are brief, so a short retry clears them.
const (
	renameTimeout        = 2 * time.Second
	renameBackoffStart   = 2 * time.Millisecond
	renameBackoffCeiling = 50 * time.Millisecond
)

// WriteFileAtomic writes data to path via a uniquely named temp file in the
// same directory + fsync + rename, so a crash mid-write never leaves a
// truncated file and two concurrent writers never share a temp file. The
// temp name ends in ".tmp" so watchers/indexers that skip "*.tmp" ignore it.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", path, err)
	}
	tmp := f.Name()

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
	// CreateTemp always creates 0600; apply the requested mode.
	if err := os.Chmod(tmp, perm); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("chmod temp %s: %w", tmp, err)
	}

	if err := Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s → %s: %w", tmp, path, err)
	}
	return nil
}

// Rename is os.Rename with a bounded retry on transient Windows
// sharing-violation / access-denied errors (see renameTimeout).
func Rename(oldpath, newpath string) error {
	deadline := time.Now().Add(renameTimeout)
	backoff := renameBackoffStart
	for {
		err := os.Rename(oldpath, newpath)
		if err == nil || !isTransientRenameError(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(backoff)
		backoff = min(backoff*2, renameBackoffCeiling)
	}
}

// KeyedMutex hands out one mutex per key (typically an absolute file path),
// so read-modify-write sequences on the same file serialize while writes to
// different files proceed in parallel. Entries are reference-counted and
// dropped once unused, so the map does not grow with every file ever touched.
type KeyedMutex struct {
	mu    sync.Mutex
	locks map[string]*keyedEntry
}

type keyedEntry struct {
	mu   sync.Mutex
	refs int
}

// Lock acquires the mutex for key and returns its unlock function.
func (k *KeyedMutex) Lock(key string) (unlock func()) {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = make(map[string]*keyedEntry)
	}
	e := k.locks[key]
	if e == nil {
		e = &keyedEntry{}
		k.locks[key] = e
	}
	e.refs++
	k.mu.Unlock()

	e.mu.Lock()
	return func() {
		e.mu.Unlock()
		k.mu.Lock()
		e.refs--
		if e.refs == 0 {
			delete(k.locks, key)
		}
		k.mu.Unlock()
	}
}
