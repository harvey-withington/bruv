package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// brokenWriter stands in for os.Stderr under the Windows SCM: a valid
// io.Writer whose underlying handle is dead, so every Write errors.
// The existing Init tests can't catch this — `go test` has a working
// stderr, which is precisely how the bug below survived to production.
type brokenWriter struct{ writes int }

func (b *brokenWriter) Write(p []byte) (int, error) {
	b.writes++
	return 0, errors.New("the handle is invalid")
}

// The regression that left the home server with EMPTY log files: with
// io.MultiWriter a failing console aborted the write before it reached
// the log file. Every destination must be attempted independently.
func TestFanoutSurvivesADeadWriter(t *testing.T) {
	broken := &brokenWriter{}
	var file bytes.Buffer

	n, err := fanout(&file, broken).Write([]byte("hello"))
	if err != nil {
		t.Fatalf("write returned %v, want nil (one destination succeeded)", err)
	}
	if n != len("hello") {
		t.Errorf("n = %d, want %d", n, len("hello"))
	}
	if file.String() != "hello" {
		t.Errorf("file writer got %q, want %q", file.String(), "hello")
	}
	if broken.writes != 1 {
		t.Errorf("broken writer attempted %d times, want 1", broken.writes)
	}

	// Order must not matter — the dead writer FIRST is the production case.
	file.Reset()
	if _, err := fanout(broken, &file).Write([]byte("second")); err != nil {
		t.Fatalf("write returned %v, want nil", err)
	}
	if file.String() != "second" {
		t.Errorf("file writer got %q, want %q", file.String(), "second")
	}
}

func TestFanoutFailsOnlyWhenEveryWriterFails(t *testing.T) {
	if _, err := fanout(&brokenWriter{}, &brokenWriter{}).Write([]byte("x")); err == nil {
		t.Fatal("expected an error when every destination fails")
	}
}

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
