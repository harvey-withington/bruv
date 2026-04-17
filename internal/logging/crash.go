package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

// crashSubdir is a sibling to `logs/` so crash reports are trivial to
// find when users attach them to bug reports. Keeping them separate
// from the rolling daily log also means retention pruning doesn't
// sweep crashes away after 7 days — the slow-burn bugs that only
// reproduce every few weeks deserve a longer tail.
const crashSubdir = "crashes"

// crashDir is set by InitCrashReporting and read by Recover. Stored
// as a package-level var so goroutines spawned across the app don't
// need to plumb the config dir through as an arg.
var (
	crashMu  sync.Mutex
	crashDir string
)

// BuildInfo holds version metadata included in crash reports. Set
// from main during startup so every crash is tagged with the
// version the user was actually running.
type BuildInfo struct {
	Version   string
	BuildDate string
}

var buildInfo BuildInfo

// InitCrashReporting configures the crash-report output directory.
// Call from app startup after logging.Init. Safe to call before the
// directory exists — it's created lazily on the first crash.
//
// buildVersion/buildDate are stamped into every crash report so users
// filing a bug report don't have to remember which build they were on.
func InitCrashReporting(configDir, buildVersion, buildDate string) {
	crashMu.Lock()
	defer crashMu.Unlock()
	crashDir = filepath.Join(configDir, crashSubdir)
	buildInfo = BuildInfo{Version: buildVersion, BuildDate: buildDate}
}

// Recover is a panic handler intended to be deferred at the top of
// any goroutine that might outlive the caller's stack frame. It
// writes a timestamped crash report to <configDir>/crashes/ with
// the full stack, logs a warning via slog so the live log also
// shows the event, and then returns — the goroutine is terminated
// but the rest of the app keeps running.
//
// Usage:
//
//	go func() {
//	    defer logging.Recover("scheduler")
//	    // goroutine body…
//	}()
//
// The `name` arg identifies the goroutine in both the slog line and
// the crash-report filename (so users who see BRUV misbehaving can
// send us "the most recent crashes/*scheduler*.log" file rather
// than wading through the whole folder).
func Recover(name string) {
	r := recover()
	if r == nil {
		return
	}
	WriteCrash(name, r)
}

// WriteCrash is the Recover helper for call sites that already have
// their own recover() block (e.g. because they need to run cleanup
// logic alongside the crash-report write). Call WriteCrash(name, r)
// where `r` is the value returned by recover(). No-op if r is nil.
//
// Recover and WriteCrash diverge only at the recover() call itself —
// after that they do the same thing. Having both lets code with
// pre-existing cleanup-on-panic logic upgrade to disk-based crash
// reports without having to restructure its defer stack.
func WriteCrash(name string, panicVal any) {
	if panicVal == nil {
		return
	}
	stack := debug.Stack()
	slog.Error("goroutine panic recovered",
		"goroutine", name,
		"panic", fmt.Sprintf("%v", panicVal))

	path, err := writeCrashReport(name, panicVal, stack)
	if err != nil {
		// Crash logging itself failed — fall back to stderr so we
		// at least have something. The slog.Error above is also
		// preserved in the daily log.
		fmt.Fprintf(os.Stderr, "crash report write failed: %v\npanic: %v\n%s\n", err, panicVal, stack)
		return
	}
	slog.Error("crash report written", "path", path, "goroutine", name)
}

// writeCrashReport produces a self-contained crash log that's
// useful without the surrounding context. Includes build metadata,
// timestamp, runtime info, the panic value, and the full stack.
func writeCrashReport(goroutine string, panicVal any, stack []byte) (string, error) {
	crashMu.Lock()
	dir := crashDir
	info := buildInfo
	crashMu.Unlock()

	if dir == "" {
		return "", fmt.Errorf("crash reporting not initialised")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create crash dir: %w", err)
	}

	ts := time.Now().Format("2006-01-02_15-04-05")
	// Sanitise the goroutine name for the filename — callers pass
	// short identifiers like "scheduler" already but be defensive.
	safeName := sanitiseForFilename(goroutine)
	filename := fmt.Sprintf("crash-%s-%s.log", ts, safeName)
	path := filepath.Join(dir, filename)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, filePermission)
	if err != nil {
		return "", fmt.Errorf("open crash file: %w", err)
	}
	defer f.Close()

	fmt.Fprintf(f, "BRUV crash report\n")
	fmt.Fprintf(f, "=================\n\n")
	fmt.Fprintf(f, "Time:       %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "Version:    %s\n", info.Version)
	fmt.Fprintf(f, "Build date: %s\n", info.BuildDate)
	fmt.Fprintf(f, "OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(f, "Go:         %s\n", runtime.Version())
	fmt.Fprintf(f, "Goroutine:  %s\n\n", goroutine)
	fmt.Fprintf(f, "Panic value\n-----------\n%v\n\n", panicVal)
	fmt.Fprintf(f, "Stack trace\n-----------\n%s\n", stack)
	return path, nil
}

// CrashDir returns the directory where crash reports are written.
// Returns an empty string if InitCrashReporting has not been called.
// Callers can surface this in UI (e.g. "Open crash folder" in About)
// or attach the most recent report to a bug report.
func CrashDir() string {
	crashMu.Lock()
	defer crashMu.Unlock()
	return crashDir
}

// sanitiseForFilename drops characters that don't belong in a
// cross-platform filename. Keeps ASCII letters, digits, dash and
// underscore.
func sanitiseForFilename(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "unnamed"
	}
	return string(out)
}
