package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoverWritesCrashReport(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(Close)
	InitCrashReporting(dir, "v0.1.0-test", "2026-04-18")

	// Run a panicking goroutine that Recover catches.
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer Recover("unit-test")
		panic("intentional panic for unit test")
	}()
	<-done

	// The crashes dir should contain exactly one crash report tagged
	// with our goroutine name.
	files, err := os.ReadDir(filepath.Join(dir, crashSubdir))
	if err != nil {
		t.Fatalf("read crashes dir: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 crash file, got %d", len(files))
	}
	name := files[0].Name()
	if !strings.Contains(name, "unit-test") {
		t.Errorf("expected filename to contain 'unit-test', got %q", name)
	}
	if !strings.HasSuffix(name, ".log") {
		t.Errorf("expected .log suffix, got %q", name)
	}

	body, err := os.ReadFile(filepath.Join(dir, crashSubdir, name))
	if err != nil {
		t.Fatalf("read crash file: %v", err)
	}
	s := string(body)
	for _, want := range []string{
		"BRUV crash report",
		"Version:    v0.1.0-test",
		"Goroutine:  unit-test",
		"intentional panic for unit test",
		"Stack trace",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("crash file missing %q; got:\n%s", want, s)
		}
	}
}

func TestRecoverWithoutPanicIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(Close)
	InitCrashReporting(dir, "v0.1.0-test", "2026-04-18")

	// Deferring Recover in a function that does NOT panic must not
	// write a crash file.
	func() {
		defer Recover("clean-goroutine")
	}()

	entries, err := os.ReadDir(filepath.Join(dir, crashSubdir))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read crashes dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no crash files, got %d", len(entries))
	}
}

func TestSanitiseForFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"scheduler", "scheduler"},
		{"bounds-poller", "bounds-poller"},
		{"with spaces/and:slashes", "with_spaces_and_slashes"},
		{"", "unnamed"},
		{"ASCII_123-ok", "ASCII_123-ok"},
	}
	for _, c := range cases {
		if got := sanitiseForFilename(c.in); got != c.want {
			t.Errorf("sanitiseForFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
