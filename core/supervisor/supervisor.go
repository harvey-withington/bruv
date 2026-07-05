package supervisor

import (
	"fmt"
	"log/slog"
	"sync"

	"bruv/core/events"
	"bruv/internal/config"
)

// Supervisor holds N Runtimes indexed by repo ID and resolves them at
// request time. Disabled entries are present in the registry but have
// no Runtime — Resolve returns nil so the per-repo dispatcher 404s.
// Re-enabling lazy-builds the Runtime.
//
// The Supervisor exposes an aggregated event bus via Bus(): every
// loaded runtime's events fan in to it, so a host (the desktop tray,
// future cross-repo digest views) can subscribe once and receive
// from any open repo. Per-repo subscribers (the HTTP SSE handler at
// /repos/<id>/events) keep talking to their runtime's own bus
// directly — the aggregated bus is opt-in for hosts that want
// cross-repo visibility.
type Supervisor struct {
	mu        sync.Mutex
	configDir string
	runtimes  map[string]*Runtime
	entries   map[string]config.RepoEntry
	secret    []byte

	// mux is the aggregated event bus. Every loaded runtime's bus
	// fans in here via a goroutine started in Load(); Unload() and
	// SetEnabled(false) cancel that goroutine.
	mux       *events.MemBus
	muxUnsubs map[string]func() // per-runtime cancel handles for the fan-in

	// building single-flights buildRuntime per repo ID. On a repo
	// switch the frontend fires a burst of parallel RPCs, each lazily
	// calling Load(id) — without this, several buildRuntimes race on
	// the same repo: duplicate watchers/schedulers/MCP subprocesses,
	// and concurrent index.db opens where the loser-with-a-nil-index
	// finishes FIRST (it skipped the refresh) and wins the cache slot.
	// That was the "repo loads slowly then renders cardless until app
	// restart" bug.
	building map[string]*buildFlight
}

// buildFlight is one in-progress buildRuntime shared by concurrent Loads.
type buildFlight struct {
	done chan struct{}
	rt   *Runtime
	err  error
}

// New constructs a Supervisor from a slice of registry entries. Does
// NOT build any runtimes — callers explicitly load what they need.
// LoadAll for the headless server (warms every non-disabled entry at
// startup); RegisterAndLoad / Load(id) for the desktop (lazy, only
// the active repo gets a runtime).
func New(entries []config.RepoEntry, configDir string) (*Supervisor, error) {
	s := &Supervisor{
		configDir: configDir,
		runtimes:  make(map[string]*Runtime, len(entries)),
		entries:   make(map[string]config.RepoEntry, len(entries)),
		secret:    config.LoadServerSecret(),
		mux:       events.NewMemBus(256),
		muxUnsubs: make(map[string]func(), len(entries)),
		building:  make(map[string]*buildFlight),
	}
	for _, e := range entries {
		s.entries[e.ID] = e
	}
	return s, nil
}

// Bus returns the supervisor's aggregated event bus. Subscribers
// receive events published by every loaded runtime — useful for
// cross-repo concerns like the desktop tray's unread-count tooltip.
// Per-repo subscribers should use rt.Bus() instead, both because
// it's lower latency (no fan-in goroutine hop) and because the
// per-runtime bus carries event IDs in publish order without the
// cross-repo interleaving the aggregated bus has.
func (s *Supervisor) Bus() *events.MemBus { return s.mux }

// startBusFanIn subscribes to the runtime's bus and re-publishes
// every event onto the supervisor's aggregated bus. The cancel
// handle is stashed in muxUnsubs[id] so Unload / SetEnabled(false)
// can stop the goroutine cleanly. Caller holds s.mu.
func (s *Supervisor) startBusFanIn(id string, rt *Runtime) {
	ch, unsub := rt.Bus().Subscribe()
	s.muxUnsubs[id] = unsub
	mux := s.mux
	go func() {
		for ev := range ch {
			mux.Publish(ev.Topic, ev.Payload)
		}
	}()
}

// stopBusFanIn cancels the per-runtime fan-in goroutine, if any.
// Caller holds s.mu.
func (s *Supervisor) stopBusFanIn(id string) {
	if unsub, ok := s.muxUnsubs[id]; ok {
		unsub()
		delete(s.muxUnsubs, id)
	}
}

// LoadAll builds + loads a Runtime for every non-disabled entry in
// the registry. Used by the headless server at startup to warm all
// runtimes up-front. Failures are logged-and-skipped — the supervisor
// still serves the entries it could load.
func (s *Supervisor) LoadAll() {
	s.mu.Lock()
	work := make([]config.RepoEntry, 0, len(s.entries))
	for _, e := range s.entries {
		if !e.Disabled {
			work = append(work, e)
		}
	}
	s.mu.Unlock()
	for _, e := range work {
		if _, err := s.Load(e.ID); err != nil {
			slog.Warn("supervisor: skip repo (build failed)", "id", e.ID, "path", e.Path, "err", err)
		}
	}
}

// Load builds a Runtime for the given registered ID and caches it.
// Returns the existing Runtime if already loaded. Errors when the ID
// isn't in the registry or when buildRuntime fails.
//
// Concurrent Loads for the same ID share ONE buildRuntime (see the
// `building` field doc) — a build must never run twice for a repo, or
// the two runtimes contend on the repo's index.db and duplicate every
// background worker.
func (s *Supervisor) Load(id string) (*Runtime, error) {
	s.mu.Lock()
	if rt, ok := s.runtimes[id]; ok {
		s.mu.Unlock()
		return rt, nil
	}
	if f, ok := s.building[id]; ok {
		// Another goroutine is mid-build — wait for its result.
		s.mu.Unlock()
		<-f.done
		if f.err != nil {
			return nil, f.err
		}
		return f.rt, nil
	}
	entry, ok := s.entries[id]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("supervisor: repo %q not in registry", id)
	}
	f := &buildFlight{done: make(chan struct{})}
	s.building[id] = f
	s.mu.Unlock()

	rt, err := buildRuntime(entry.Path, s.configDir, s.secret)
	if err != nil {
		err = fmt.Errorf("supervisor: build %q: %w", id, err)
	}

	s.mu.Lock()
	delete(s.building, id)
	if err == nil {
		s.runtimes[id] = rt
		s.startBusFanIn(id, rt)
	}
	f.rt, f.err = rt, err
	s.mu.Unlock()
	close(f.done)
	return rt, err
}

// Secret returns the HMAC secret used for signed attachment URLs.
// Exposed so the wiring layer can plug it into transport's
// AttachmentConfig without reaching into supervisor internals.
func (s *Supervisor) Secret() []byte { return s.secret }

// Resolve returns the loaded Runtime for the given repo ID, or nil if
// the repo is unknown / disabled.
func (s *Supervisor) Resolve(id string) *Runtime {
	s.mu.Lock()
	rt := s.runtimes[id]
	s.mu.Unlock()
	return rt
}

// LoadedRuntimes returns a snapshot of every Runtime currently loaded
// (regardless of whether the underlying entry is enabled / disabled
// in the registry — disabling unloads, so this list reflects the
// actual in-memory set). Caller gets its own slice; safe to iterate
// without the lock. Used for cross-runtime fan-out from the desktop
// tray (pause-all / resume-all across every loaded repo).
func (s *Supervisor) LoadedRuntimes() []*Runtime {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Runtime, 0, len(s.runtimes))
	for _, rt := range s.runtimes {
		out = append(out, rt)
	}
	return out
}

// List returns a snapshot of every registered entry (loaded or not).
// Caller gets its own slice — safe to iterate without the lock.
func (s *Supervisor) List() []config.RepoEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]config.RepoEntry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e)
	}
	return out
}

// SetEnabled flips a repo on or off at runtime. Enabling builds and
// starts its Runtime; disabling shuts the existing Runtime down.
// Idempotent. Persists the change to repos.json so it survives restart.
func (s *Supervisor) SetEnabled(id string, enabled bool) error {
	s.mu.Lock()
	entry, ok := s.entries[id]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("supervisor: repo %q not in registry", id)
	}
	currentlyLoaded := s.Resolve(id) != nil
	if enabled == currentlyLoaded {
		entry.Disabled = !enabled
		s.mu.Lock()
		s.entries[id] = entry
		s.mu.Unlock()
		return config.SetRepoDisabled(id, !enabled)
	}
	if enabled {
		// Persist Disabled=false BEFORE Load so RegistrationByID
		// reflects the new state if buildRuntime queries it.
		entry.Disabled = false
		s.mu.Lock()
		s.entries[id] = entry
		s.mu.Unlock()
		if _, err := s.Load(id); err != nil {
			return fmt.Errorf("supervisor: enable %q: %w", id, err)
		}
	} else {
		s.mu.Lock()
		rt := s.runtimes[id]
		delete(s.runtimes, id)
		s.stopBusFanIn(id)
		entry.Disabled = true
		s.entries[id] = entry
		s.mu.Unlock()
		if rt != nil {
			rt.Close()
		}
	}
	return config.SetRepoDisabled(id, !enabled)
}

// RegisterAndLoad ensures a repo at the given path is in the registry
// and has a loaded Runtime, returning the Runtime. Idempotent: if the
// path is already registered and loaded, returns the existing Runtime.
// If registered-but-disabled, re-enables it. If not registered, adds
// to repos.json (via config.AppendRepo) then builds + loads.
//
// Used by the desktop App's OpenRepository flow — picks an existing
// folder, registers it (no-op if already there), and brings up the
// runtime so per-repo RPCs work.
func (s *Supervisor) RegisterAndLoad(path string) (*Runtime, error) {
	entry, err := config.AppendRepo(path, "")
	if err != nil {
		return nil, fmt.Errorf("supervisor: append repo: %w", err)
	}
	s.mu.Lock()
	s.entries[entry.ID] = entry
	s.mu.Unlock()
	// Delegate to Load so registration shares the same single-flight
	// as every other build path.
	return s.Load(entry.ID)
}

// EntryByPath returns the registry entry whose Path matches (after
// abs/normalisation), or zero + false if not registered. Useful for
// the desktop after a freshly-loaded runtime needs its ID.
func (s *Supervisor) EntryByPath(path string) (config.RepoEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.Path == path {
			return e, true
		}
	}
	return config.RepoEntry{}, false
}

// Unload shuts a runtime down WITHOUT removing it from the registry.
// The next Resolve will lazy-rebuild it. Used by the desktop when the
// user closes the active repo.
func (s *Supervisor) Unload(id string) {
	s.mu.Lock()
	rt := s.runtimes[id]
	delete(s.runtimes, id)
	s.stopBusFanIn(id)
	s.mu.Unlock()
	if rt != nil {
		rt.Close()
	}
}

// SetName renames a registry entry and (if loaded) propagates the
// rename into the live Repository's manifest. Persists to repos.json
// AND updates the supervisor's in-memory entries map so subsequent
// List() / Resolve() calls reflect the new name without waiting for
// a reload. Returns the path of the renamed entry so callers needing
// the disk-only manifest rewrite (no live runtime) can do it.
func (s *Supervisor) SetName(id, name string) (string, error) {
	s.mu.Lock()
	entry, ok := s.entries[id]
	s.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("supervisor: repo %q not in registry", id)
	}
	if err := config.SetRepoName(id, name); err != nil {
		return "", err
	}
	entry.Name = name
	s.mu.Lock()
	s.entries[id] = entry
	s.mu.Unlock()
	return entry.Path, nil
}

// Remove drops an entry from the registry, unloading its runtime
// first. Persists to repos.json AND prunes the in-memory entries
// map so subsequent List() doesn't keep returning the gone repo.
func (s *Supervisor) Remove(id string) error {
	s.Unload(id)
	if err := config.RemoveRepo(id); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.entries, id)
	s.mu.Unlock()
	return nil
}

// Close shuts down every loaded Runtime. Safe to call from a defer.
func (s *Supervisor) Close() {
	s.mu.Lock()
	rts := make([]*Runtime, 0, len(s.runtimes))
	for _, rt := range s.runtimes {
		rts = append(rts, rt)
	}
	for id := range s.muxUnsubs {
		s.stopBusFanIn(id)
	}
	s.runtimes = nil
	s.mu.Unlock()
	for _, rt := range rts {
		rt.Close()
	}
}
