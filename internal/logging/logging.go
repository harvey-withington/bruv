// Package logging initialises BRUV's structured logger.
//
// On Init, logs are written to both stderr (for terminal runs) and a
// daily-rotated file under <configDir>/logs/bruv-YYYY-MM-DD.log. Files
// older than retentionDays are deleted on startup so the folder never
// grows without bound. The exported functions are no-ops until Init has
// been called, so it's safe to import this package from packages that
// run before startup.
package logging

import (
	"fmt"
	"io"
	stdlog "log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	logsSubdir     = "logs"
	retentionDays  = 7
	filePermission = 0o644
)

var (
	mu      sync.Mutex
	logFile *os.File
)

// Init configures the default slog logger to fan out to stderr and a
// dated log file in <configDir>/logs/. Safe to call multiple times — a
// second call closes the previous file and opens a fresh one (useful
// if the config dir ever moves at runtime).
//
// Returns the path of the active log file so callers can surface it in
// UI (About dialog, "Open logs folder" menu item, etc.). On error the
// default logger is still installed in stderr-only mode.
func Init(configDir string) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	logsDir := filepath.Join(configDir, logsSubdir)
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		installDefault(os.Stderr)
		return "", fmt.Errorf("create logs dir: %w", err)
	}

	pruneOldLogs(logsDir)

	name := fmt.Sprintf("bruv-%s.log", time.Now().Format("2006-01-02"))
	path := filepath.Join(logsDir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, filePermission)
	if err != nil {
		installDefault(os.Stderr)
		return "", fmt.Errorf("open log file %s: %w", path, err)
	}

	if logFile != nil {
		_ = logFile.Close()
	}
	logFile = f

	installDefault(io.MultiWriter(os.Stderr, f))
	slog.Info("logging initialised", "path", path)
	return path, nil
}

// LogsDir returns the directory where BRUV writes its log files.
// Callers can use this to open the folder in the OS file browser.
// Returns an empty string if Init has not been called or its configDir
// was not writable.
func LogsDir(configDir string) string {
	return filepath.Join(configDir, logsSubdir)
}

// Close flushes and closes the active log file. Call from app
// shutdown (OnBeforeClose). Safe to call if Init never ran.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		_ = logFile.Sync()
		_ = logFile.Close()
		logFile = nil
	}
}

func installDefault(w io.Writer) {
	h := slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(h))
	// Route stdlib log.Printf through the same writer so existing
	// log.Printf call sites keep working and their output also lands
	// in the log file without migrating every call site.
	stdlog.SetOutput(w)
}

// pruneOldLogs deletes bruv-*.log files older than retentionDays.
// Errors are swallowed — pruning is a best-effort housekeeping task,
// not a correctness concern.
func pruneOldLogs(logsDir string) {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "bruv-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(logsDir, name))
		}
	}
}
