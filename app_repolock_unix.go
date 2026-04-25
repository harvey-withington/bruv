//go:build !windows

package main

import "syscall"

// syscallSignalZero is the Unix "is this process alive" probe signal.
// Sending signal 0 to a PID returns nil if the process exists and we
// have permission to signal it, or an error otherwise — exactly the
// liveness check we want.
var syscallSignalZero = syscall.Signal(0)
