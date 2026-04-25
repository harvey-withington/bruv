package reposync

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// recorder implements Publisher by appending every event to a slice
// under lock. Used by tests to assert what the watcher surfaced.
type recorder struct {
	mu     sync.Mutex
	events []event
}

type event struct {
	topic   string
	payload any
}

func (r *recorder) Publish(topic string, payload any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event{topic, payload})
}

func (r *recorder) snapshot() []event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]event, len(r.events))
	copy(out, r.events)
	return out
}

// waitFor polls for a condition within the given timeout. Returns true
// if the condition held; false on timeout.
func waitFor(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// setupRepo builds a minimal repo-shaped directory tree the watcher
// can chew on. Real repo.Init does far more, but for watcher tests
// we only care about the directory shape.
func setupRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range []string{
		"cards",
		"brands/acme",
		"brands/acme/streams/youtube",
		"brands/acme/streams/youtube/projects/tutorials",
		"brands/acme/streams/youtube/projects/tutorials/categories",
		".bruv",
	} {
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
	}
	return root
}

func TestCardWriteFiresCardUpdated(t *testing.T) {
	root := setupRepo(t)
	r := &recorder{}
	w, err := Start(root, r)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	cardPath := filepath.Join(root, "cards", "abc123.json")
	if err := os.WriteFile(cardPath, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write card: %v", err)
	}

	if !waitFor(time.Second, func() bool {
		for _, ev := range r.snapshot() {
			if ev.topic == "card:updated" {
				if p, ok := ev.payload.(map[string]any); ok && p["cardID"] == "abc123" {
					return true
				}
			}
		}
		return false
	}) {
		t.Fatalf("expected card:updated event; got %v", r.snapshot())
	}
}

func TestIgnoresBruvInternalDir(t *testing.T) {
	root := setupRepo(t)
	r := &recorder{}
	w, err := Start(root, r)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// Write inside .bruv/ — must NOT fire.
	if err := os.WriteFile(filepath.Join(root, ".bruv", "index.db"), []byte(`junk`), 0o644); err != nil {
		t.Fatalf("write bruv file: %v", err)
	}

	// Wait past the debounce window and assert silence.
	time.Sleep(debounce + 100*time.Millisecond)
	if len(r.snapshot()) != 0 {
		t.Errorf(".bruv/ writes should not fire events; got %v", r.snapshot())
	}
}

func TestIgnoresTempFiles(t *testing.T) {
	root := setupRepo(t)
	r := &recorder{}
	w, err := Start(root, r)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// internal/repo/io.go writes via "<path>.tmp" + rename. The
	// .tmp write must not fire an event — only the rename (which
	// surfaces as Create on <path>) should.
	if err := os.WriteFile(filepath.Join(root, "cards", "xyz.json.tmp"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write tmp: %v", err)
	}

	time.Sleep(debounce + 100*time.Millisecond)
	for _, ev := range r.snapshot() {
		if ev.topic == "card:updated" {
			t.Errorf(".tmp files should be ignored; got %+v", ev)
		}
	}
}

func TestDebounceCollapsesBursts(t *testing.T) {
	root := setupRepo(t)
	r := &recorder{}
	w, err := Start(root, r)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	cardPath := filepath.Join(root, "cards", "burst.json")
	// Five rapid writes in quick succession should collapse to one event.
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(cardPath, []byte(`{}`), 0o644); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		time.Sleep(20 * time.Millisecond) // well inside debounce window
	}
	// Let the debounce fire and then settle.
	time.Sleep(debounce + 100*time.Millisecond)

	count := 0
	for _, ev := range r.snapshot() {
		if ev.topic == "card:updated" {
			if p, ok := ev.payload.(map[string]any); ok && p["cardID"] == "burst" {
				count++
			}
		}
	}
	if count != 1 {
		t.Errorf("burst of 5 writes = %d events, want 1", count)
	}
}

func TestBrandFileFiresBrandUpdated(t *testing.T) {
	root := setupRepo(t)
	r := &recorder{}
	w, err := Start(root, r)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	if err := os.WriteFile(
		filepath.Join(root, "brands", "acme", "brand.json"),
		[]byte(`{}`), 0o644,
	); err != nil {
		t.Fatalf("write brand: %v", err)
	}

	if !waitFor(time.Second, func() bool {
		for _, ev := range r.snapshot() {
			if ev.topic == "brand:updated" {
				if p, ok := ev.payload.(map[string]any); ok && p["slug"] == "acme" {
					return true
				}
			}
		}
		return false
	}) {
		t.Fatalf("expected brand:updated for acme; got %v", r.snapshot())
	}
}
