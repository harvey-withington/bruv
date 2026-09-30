//go:build !windows

package fsutil

import (
	"errors"
	"os"
	"syscall"
)

// flock locks are advisory and per open file description: another
// open() of the same path — in this process or another — conflicts, and
// reading the file stays possible.
func lockFile(f *os.File) (held bool, err error) {
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return true, nil
	}
	return false, err
}

func unlockFile(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
