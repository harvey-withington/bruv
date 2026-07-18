// Package reposync watches an open repo directory for external
// changes and publishes the corresponding domain events, so a git
// pull / Syncthing / hand-edit surfaces to the UI via the same
// `card:updated`, `project:updated`, etc. events that user-driven
// mutations fire. This is the file-sync-based collaboration path
// from the architecture plan — BRUV is a well-behaved citizen
// inside whichever sync tool the user picks.
//
// Semantics:
//
//   - Watches every subdirectory of the repo except `.bruv/` (index
//     DB, caches, lock files).
//   - Per-file debounce (200ms) collapses bursts into one event.
//     Our own writes also trigger events — clients tolerate the
//     harmless extra fire rather than engage in cross-process
//     coordination.
//   - Atomic-write temp files (`<name>.tmp`) are ignored.
//   - Adds newly-created directories to the watch set at runtime so
//     a brand created mid-session still propagates card changes.
package reposync

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Publisher is the narrow bus contract. Passed in rather than
// importing core/events to keep this package transport-free.
type Publisher interface {
	Publish(topic string, payload any)
}

// Watcher runs one fsnotify goroutine against a repo root and
// translates raw FS events into domain events.
type Watcher struct {
	root      string
	publisher Publisher
	watcher   *fsnotify.Watcher

	// Per-file debounce timers. Keyed by absolute path so siblings
	// don't collapse into each other's timers.
	mu     sync.Mutex
	timers map[string]*time.Timer

	// watched tracks the absolute paths currently registered with the
	// underlying fsnotify watcher, so DetachSubtree can find every
	// descendant of a given root and Remove them. fsnotify itself
	// doesn't expose its registered set, hence the parallel map.
	// Required on Windows: ReadDirectoryChangesW uses overlapped I/O,
	// which leaves a pending IRP on the directory handle — and Windows
	// refuses to rename a directory with pending IRPs even when the
	// handle has FILE_SHARE_DELETE. Callers about to rename / delete a
	// subtree must DetachSubtree first, then AttachSubtree on the new
	// (or parent) path afterwards.
	watched map[string]bool

	stopCh  chan struct{}
	stopped bool
	// done is closed by run() when the event loop exits, so Stop() can
	// block until the goroutine is truly gone. Closing stopCh only signals
	// it; without waiting, run() may still be mid-handleEvent (or a
	// debounced publish may still fire) after the caller has torn down the
	// repo — racing t.TempDir cleanup in tests and repo deletion in prod.
	done chan struct{}
}

// debounce is intentionally brief — long enough to collapse bursts
// from atomic writes (3-4 events in quick succession), short enough
// that a git pull feels "instant" to the user.
const debounce = 200 * time.Millisecond

// Start launches the watcher. Returns immediately; the watcher runs
// until Stop is called. Errors adding initial directories are logged
// but non-fatal — the watcher still runs against any directories it
// did successfully add.
func Start(root string, publisher Publisher) (*Watcher, error) {
	fs, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		root:      root,
		publisher: publisher,
		watcher:   fs,
		timers:    make(map[string]*time.Timer),
		watched:   make(map[string]bool),
		stopCh:    make(chan struct{}),
		done:      make(chan struct{}),
	}
	if err := w.walkAndAdd(root); err != nil {
		slog.Warn("reposync: initial walk had errors", "err", err)
	}
	go w.run()
	return w, nil
}

// DetachSubtree removes the given root path and every watched
// descendant from the underlying fsnotify watcher. Callers must
// invoke this BEFORE renaming or deleting a directory subtree on
// Windows — see the Watcher type doc for the IRP / ACCESS_DENIED
// reason. Pair with AttachSubtree afterwards (typically against the
// new path, or against the parent if the rename moves within the
// same parent and the parent itself is already watched).
func (w *Watcher) DetachSubtree(rootPath string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	prefix := rootPath + string(filepath.Separator)
	for path := range w.watched {
		if path == rootPath || strings.HasPrefix(path, prefix) {
			_ = w.watcher.Remove(path)
			delete(w.watched, path)
		}
	}
}

// AttachSubtree walks the filesystem at the given root and registers
// every directory with the underlying fsnotify watcher. Idempotent —
// fsnotify silently no-ops on duplicate Adds, and the tracking map
// stores presence-only. Safe to call against a path that doesn't
// exist (filepath.Walk returns immediately with no callbacks).
func (w *Watcher) AttachSubtree(rootPath string) {
	if err := w.walkAndAdd(rootPath); err != nil {
		slog.Warn("reposync: attach subtree had errors", "root", rootPath, "err", err)
	}
}

// Stop tears down the watcher. Idempotent.
func (w *Watcher) Stop() {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	w.stopped = true
	// Cancel any pending debounced fires so we don't publish events
	// after the caller has closed the repo.
	for _, t := range w.timers {
		t.Stop()
	}
	w.timers = nil
	w.mu.Unlock()

	close(w.stopCh)
	_ = w.watcher.Close()
	// Block until run() has actually returned. Closing stopCh only signals
	// it; without this wait an in-flight handleEvent or debounced publish
	// can still touch the repo after Stop() returns.
	<-w.done
}

// walkAndAdd recursively adds every directory under root to the
// watcher, skipping `.bruv/` (internal state) and `.git/` (sync tool's
// own metadata). Tracks added paths in w.watched so DetachSubtree can
// find them later.
func (w *Watcher) walkAndAdd(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info == nil || !info.IsDir() {
			return nil
		}
		// shouldIgnoreDir compares against the watcher's repo root
		// (NOT the walk root) so subtree re-attaches still respect
		// the .bruv / .git skips even when called against a deeper
		// path like brands/<slug>/.
		if shouldIgnoreDir(path, w.root) {
			return filepath.SkipDir
		}
		if err := w.watcher.Add(path); err != nil {
			slog.Warn("reposync: add dir failed", "path", path, "err", err)
			return nil
		}
		w.mu.Lock()
		w.watched[path] = true
		w.mu.Unlock()
		return nil
	})
}

func shouldIgnoreDir(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == ".bruv" || strings.HasPrefix(rel, ".bruv/") ||
		rel == ".git" || strings.HasPrefix(rel, ".git/") {
		return true
	}
	// Template stores are opaque vault content (spec §6.3): possibly
	// thousands of files of boilerplate. Watching them costs fsnotify
	// handles and — on Windows — pins IRPs that make template folder
	// renames/deletes fail. Global: templates/; brand-scoped:
	// brands/<slug>/templates/.
	parts := strings.Split(rel, "/")
	if parts[0] == "templates" {
		return true
	}
	if len(parts) >= 3 && parts[0] == "brands" && parts[2] == "templates" {
		return true
	}
	return false
}

// run is the main event loop — drain fsnotify events, classify,
// debounce, publish. Exits when the watcher is closed.
func (w *Watcher) run() {
	defer close(w.done)
	for {
		select {
		case <-w.stopCh:
			return
		case ev, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(ev)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("reposync: watcher error", "err", err)
		}
	}
}

func (w *Watcher) handleEvent(ev fsnotify.Event) {
	// Ignore the atomic-write .tmp files produced by internal/repo/io.go.
	if strings.HasSuffix(ev.Name, ".tmp") {
		return
	}

	// New directory? Add to the watch set so its future contents
	// fire too. Brand creation → streams dir → projects dir → etc.
	if ev.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			if !shouldIgnoreDir(ev.Name, w.root) {
				if err := w.watcher.Add(ev.Name); err == nil {
					w.mu.Lock()
					w.watched[ev.Name] = true
					w.mu.Unlock()
				}
			}
		}
	}

	// Only care about writes, creates, removes, renames — chmod spam
	// on Windows is safe to ignore.
	if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
		return
	}

	topic, payload, ok := classify(w.root, ev.Name)
	if !ok {
		return
	}

	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	if t, exists := w.timers[ev.Name]; exists {
		t.Stop()
	}
	w.timers[ev.Name] = time.AfterFunc(debounce, func() {
		w.mu.Lock()
		delete(w.timers, ev.Name)
		stopped := w.stopped
		w.mu.Unlock()
		// Don't emit after Stop(): a late publish can trigger downstream
		// writes into a repo the caller is already tearing down.
		if stopped {
			return
		}
		w.publisher.Publish(topic, payload)
	})
	w.mu.Unlock()
}

// classify maps a filesystem path to a (topic, payload) pair based
// on BRUV's repo layout. Returns ok=false for files that don't
// represent a user-visible entity (index, lock, pins, etc.).
//
// Layout reference:
//
//	brands/<slug>/brand.json
//	brands/<slug>/streams/<slug>/stream.json
//	brands/<slug>/streams/<slug>/projects/<slug>/project.json
//	brands/<slug>/streams/<slug>/projects/<slug>/categories/<slug>.json
//	cards/<id>.json
//	cards/<id>.agent.json      (not watched — agent runs are ephemeral)
//	cards/<id>.comments.json   (watched as card:updated — comments render with card)
func classify(root, path string) (topic string, payload any, ok bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", nil, false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) == 0 {
		return "", nil, false
	}

	switch parts[0] {
	case "cards":
		if len(parts) != 2 {
			return "", nil, false
		}
		name := parts[1]
		if !strings.HasSuffix(name, ".json") {
			return "", nil, false
		}
		// Skip .agent.json run-state updates (noisy).
		if strings.HasSuffix(name, ".agent.json") {
			return "", nil, false
		}
		// <cardID>.json or <cardID>.comments.json both fire card:updated.
		cardID := strings.TrimSuffix(strings.TrimSuffix(name, ".json"), ".comments")
		return "card:updated", map[string]any{"cardID": cardID, "external": true}, true

	case "brands":
		// brands/<slug>/brand.json
		if len(parts) == 3 && parts[2] == "brand.json" {
			return "brand:updated", map[string]any{"slug": parts[1], "external": true}, true
		}
		// brands/<bSlug>/streams/<sSlug>/stream.json
		if len(parts) == 5 && parts[2] == "streams" && parts[4] == "stream.json" {
			return "stream:updated", map[string]any{
				"brandSlug":  parts[1],
				"streamSlug": parts[3],
				"external":   true,
			}, true
		}
		// brands/<b>/streams/<s>/projects/<p>/project.json
		if len(parts) == 7 && parts[2] == "streams" && parts[4] == "projects" && parts[6] == "project.json" {
			return "project:updated", map[string]any{
				"brandSlug":   parts[1],
				"streamSlug":  parts[3],
				"projectSlug": parts[5],
				"external":    true,
			}, true
		}
		// brands/<b>/streams/<s>/projects/<p>/workspace/{workspace,index}.json
		// — vault-side workspace state changed externally (synced vault /
		// hand edit). Local mutations also land here; clients tolerate the
		// duplicate fire like every other topic.
		if len(parts) == 8 && parts[2] == "streams" && parts[4] == "projects" && parts[6] == "workspace" &&
			strings.HasSuffix(parts[7], ".json") {
			return "workspace:updated", map[string]any{
				"brand_slug":   parts[1],
				"stream_slug":  parts[3],
				"project_slug": parts[5],
				"external":     true,
			}, true
		}
		// brands/<b>/streams/<s>/projects/<p>/categories/<c>.json
		// (8 segments — the old len==7 && parts[5]=="categories" condition
		// was unreachable, so external category edits refreshed nothing.)
		if len(parts) == 8 && parts[2] == "streams" && parts[4] == "projects" && parts[6] == "categories" {
			catFile := parts[7]
			if strings.HasSuffix(catFile, ".json") {
				return "category:updated", map[string]any{
					"brandSlug":    parts[1],
					"streamSlug":   parts[3],
					"projectSlug":  parts[5],
					"categorySlug": strings.TrimSuffix(catFile, ".json"),
					"external":     true,
				}, true
			}
		}
	}

	return "", nil, false
}
