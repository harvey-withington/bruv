package fsutil

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// lockOffset puts the locked byte far past any holder text, so other
// processes can still read the file while it's locked (LockFileEx locks
// are mandatory on Windows: a locked range can't be read by others).
const lockOffset = 1 << 30

func lockFile(f *os.File) (held bool, err error) {
	ol := &windows.Overlapped{Offset: lockOffset}
	err = windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return true, nil
	}
	return false, err
}

func unlockFile(f *os.File) {
	ol := &windows.Overlapped{Offset: lockOffset}
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
}
