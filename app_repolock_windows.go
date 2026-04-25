//go:build windows

package main

import "os"

// On Windows there is no signal 0 equivalent. Passing a nil os.Signal
// to Process.Signal returns an error for dead processes and nil for
// live ones — the behaviour we want. Typed as os.Signal so the
// signature matches the Unix sibling.
var syscallSignalZero os.Signal = nil
