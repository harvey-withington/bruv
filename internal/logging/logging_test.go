package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInitCreatesLogFileAndAcceptsWrites(t *testing.T) {
	dir := t.TempDir()
	path, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(Close)

	if !strings.HasPrefix(filepath.Base(path), "bruv-") {
		t.Errorf("expected bruv-*.log, got %s", path)
	}
	slog.Error("test log line", "case", "unit")
	Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "test log line") {
		t.Errorf("log file missing expected line, got: %q", data)
	}
}

func TestInitPrunesOldFiles(t *testing.T) {
	dir := t.TempDir()
	logsDir := filepath.Join(dir, logsSubdir)
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	old := filepath.Join(logsDir, "bruv-2020-01-01.log")
	if err := os.WriteFile(old, []byte("ancient"), 0o644); err != nil {
		t.Fatal(err)
	}
	aged := time.Now().AddDate(0, 0, -30)
	if err := os.Chtimes(old, aged, aged); err != nil {
		t.Fatal(err)
	}

	recent := filepath.Join(logsDir, "bruv-recent.log")
	if err := os.WriteFile(recent, []byte("fresh"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(Close)

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old log not pruned: err=%v", err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("recent log unexpectedly pruned: %v", err)
	}
}
