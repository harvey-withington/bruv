package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDueDateStateIsPerRepo: two repos' scanners must not share (and
// overwrite) one notified-state file.
func TestDueDateStateIsPerRepo(t *testing.T) {
	cfg := t.TempDir()
	a := NewDueDateScanner(filepath.Join(t.TempDir(), "cards"), cfg, nil, nil)
	b := NewDueDateScanner(filepath.Join(t.TempDir(), "cards"), cfg, nil, nil)
	if a.notifiedPath == b.notifiedPath {
		t.Fatalf("scanners share state file %s", a.notifiedPath)
	}

	a.notified["card-a:0s"] = time.Now()
	a.saveNotified()
	if _, err := os.Stat(a.notifiedPath); err != nil {
		t.Fatalf("state not written: %v", err)
	}
	// Only the state file itself is left behind — no temp files.
	entries, _ := os.ReadDir(filepath.Dir(a.notifiedPath))
	if len(entries) != 1 {
		t.Fatalf("state dir has %d entries, want 1", len(entries))
	}

	reloaded := NewDueDateScanner(a.cardsDir, cfg, nil, nil)
	if _, ok := reloaded.notified["card-a:0s"]; !ok {
		t.Fatal("state not reloaded for the same repo")
	}
}

// TestDueDateLegacyStateSeedsNewFile: the pre-per-repo shared file is
// read when a repo has no state of its own yet, so upgrading doesn't
// re-fire every notification.
func TestDueDateLegacyStateSeedsNewFile(t *testing.T) {
	cfg := t.TempDir()
	legacy := `{"card-x:alarm:b1": "2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(cfg, "due_notified.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewDueDateScanner(filepath.Join(t.TempDir(), "cards"), cfg, nil, nil)
	if _, ok := s.notified["card-x:alarm:b1"]; !ok {
		t.Fatal("legacy state not loaded")
	}
}

// TestDueDateStopWaitsForScan: Stop must not return while a scan is
// still able to stamp a card.
func TestDueDateStopWaitsForScan(t *testing.T) {
	cardsDir := t.TempDir()
	card := `{"id":"card-1","title":"T","blocks":[{"id":"b1","type":"alarm","meta":{"alarm_time":"2020-01-01T00:00:00Z"}}]}`
	if err := os.WriteFile(filepath.Join(cardsDir, "card-0001-long-id.json"), []byte(card), 0o644); err != nil {
		t.Fatal(err)
	}
	inFire := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	s := NewDueDateScanner(cardsDir, t.TempDir(), func(string, string, time.Duration, bool) {}, func(string, string) {
		close(inFire)
		<-release
		close(finished)
	})
	s.Configure(true, []string{"0"}, "")
	// Drive scan through the same done-channel contract Start uses.
	s.started = true
	go func() {
		defer close(s.done)
		s.scan()
	}()
	<-inFire

	stopped := make(chan struct{})
	go func() { s.Stop(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("Stop returned while a scan was mid-write")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-stopped
	select {
	case <-finished:
	default:
		t.Fatal("Stop returned before the scan finished")
	}
}
