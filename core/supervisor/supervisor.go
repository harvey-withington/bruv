package supervisor

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"bruv/core/events"
	"bruv/internal/config"
)

// Errors Load returns when it refuses to build a runtime.
var (
	// ErrRepoDisabled: the entry is disabled. Only SetEnabled(true)
	// (or RegisterAndLoad, which re-enables) brings it back.
	ErrRepoDisabled = errors.New("supervisor: repo is disabled")
	// ErrClosed: the supervisor has been closed (app / service exit).
	ErrClosed = errors.New("supervisor: closed")
	// errBuildDiscarded: the repo was unloaded, disabled or removed
	// while its runtime was being built, so the build was thrown away.
	errBuildDiscarded = errors.New("supervisor: repo unloaded while loading")
)

// buildRuntimeFn is buildRuntime, swappable so lifecycle tests can
// hold a build open.
var buildRuntimeFn = buildRuntime

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
//
// Lifecycle invariant: at most one Runtime per repo exists at a time,
// counting runtimes being built and runtimes still closing. A second
// runtime on the same repo means double schedulers (double agent runs),
// two watchers and two index handles, so Load waits for an in-flight
// build or close instead of overlapping it.
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

	// closing counts runtimes per repo ID whose Close is still running.
	// Load waits (on changed) until it drops to zero, so a reload right
	// after an unload never overlaps the old runtime's shutdown.
	closing map[string]int

	// changed is broadcast whenever a build finishes or a close
	// completes. Waiters hold mu.
	changed *sync.Cond

	// closed is set by Close; Load refuses from then on.
	closed bool
}

// buildFlight is one in-progress buildRuntime shared by concurrent Loads.
type buildFlight struct {
	done chan struct{}
	rt   *Runtime
	err  error
	// cancelled is set (under Supervisor.mu) by Unload / disable /
	// Close while the build runs; the builder then closes the fresh
	// runtime instead of caching it.
	cancelled bool
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
		closing:   make(map[string]int),
	}
	s.changed = sync.NewCond(&s.mu)
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
// isn't in the registry, the entry is disabled (ErrRepoDisabled), the
// supervisor is closed (ErrClosed), or buildRuntime fails.
//
// Concurrent Loads for the same ID share ONE buildRuntime (see the
// `building` field doc) — a build must never run twice for a repo, or
// the two runtimes contend on the repo's index.db and duplicate every
// background worker. For the same reason Load waits for a previous
// runtime of the repo that is still closing.
func (s *Supervisor) Load(id string) (*Runtime, error) {
	s.mu.Lock()
	for {
		if s.closed {
			s.mu.Unlock()
			return nil, ErrClosed
		}
		if rt, ok := s.runtimes[id]; ok {
			s.mu.Unlock()
			return rt, nil
		}
		if f, ok := s.building[id]; ok {
			// Another goroutine is mid-build — wait for its result.
			cancelled := f.cancelled
			s.mu.Unlock()
			<-f.done
			if !cancelled {
				return f.rt, f.err
			}
			// That build was abandoned before we joined; start over
			// (it may have left a runtime closing, handled below).
			s.mu.Lock()
			continue
		}
		if s.closing[id] > 0 {
			s.changed.Wait()
			continue
		}
		break
	}
	entry, ok := s.entries[id]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("supervisor: repo %q not in registry", id)
	}
	if entry.Disabled {
		s.mu.Unlock()
		return nil, fmt.Errorf("%w: %q", ErrRepoDisabled, id)
	}
	f := &buildFlight{done: make(chan struct{})}
	s.building[id] = f
	s.mu.Unlock()

	rt, err := buildRuntimeFn(entry.Path, s.configDir, s.secret)
	if err != nil {
		err = fmt.Errorf("supervisor: build %q: %w", id, err)
	}

	var discard *Runtime
	s.mu.Lock()
	delete(s.building, id)
	if err == nil {
		// Re-check: the repo may have been unloaded, disabled or
		// removed — or the supervisor closed — while we were building.
		cur, stillThere := s.entries[id]
		if f.cancelled || s.closed || !stillThere || cur.Disabled {
			discard, rt, err = rt, nil, errBuildDiscarded
			s.closing[id]++
		} else {
			s.runtimes[id] = rt
			s.startBusFanIn(id, rt)
		}
	}
	f.rt, f.err = rt, err
	s.changed.Broadcast()
	s.mu.Unlock()
	close(f.done)
	if discard != nil {
		s.finishClose(id, discard)
	}
	return rt, err
}

// detachLocked removes id's cached runtime (if any) from the
// supervisor, cancels an in-flight build of it, and marks the removed
// runtime as closing. The caller must pass the returned runtime to
// finishClose after releasing s.mu. Caller holds s.mu.
func (s *Supervisor) detachLocked(id string) *Runtime {
	if f, ok := s.building[id]; ok {
		f.cancelled = true
	}
	rt := s.runtimes[id]
	delete(s.runtimes, id)
	s.stopBusFanIn(id)
	if rt != nil {
		s.closing[id]++
	}
	return rt
}

// finishClose closes a detached runtime outside the lock, then clears
// its closing mark and wakes any Load waiting to rebuild the repo.
func (s *Supervisor) finishClose(id string, rt *Runtime) {
	if rt == nil {
		return
	}
	rt.Close()
	s.mu.Lock()
	if s.closing[id]--; s.closing[id] <= 0 {
		delete(s.closing, id)
	}
	s.changed.Broadcast()
	s.mu.Unlock()
}

// Secret returns the HMAC secret used for signed attachment URLs.
// Exposed so the wiring layer can plug it into transport's
// AttachmentConfig without reaching into supervisor internals.
func (s *Supervisor) Secret() []byte { return s.secret }

// Resolve returns the loaded Runtime for the given repo ID, or nil if
// the repo is unknown / disabled.
func (s *Supervisor) Resolve(id string) *Runtime {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runtimes[id]
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
// starts its Runtime; disabling shuts the existing Runtime down (and
// abandons a build in flight). Idempotent. Persists the change to
// repos.json so it survives restart.
func (s *Supervisor) SetEnabled(id string, enabled bool) error {
	s.mu.Lock()
	entry, ok := s.entries[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("supervisor: repo %q not in registry", id)
	}
	// Flip the in-memory flag first: Load reads it, so an enable lets
	// the Load below build, and a disable stops any concurrent Load
	// from caching a fresh runtime.
	entry.Disabled = !enabled
	s.entries[id] = entry
	var rt *Runtime
	if !enabled {
		rt = s.detachLocked(id)
	}
	s.mu.Unlock()

	if enabled {
		if _, err := s.Load(id); err != nil {
			return fmt.Errorf("supervisor: enable %q: %w", id, err)
		}
	} else {
		s.finishClose(id, rt)
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
	if entry.Disabled {
		// Opening a disabled repo explicitly is a request to use it.
		if err := s.SetEnabled(entry.ID, true); err != nil {
			return nil, err
		}
	}
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

// Unload shuts a runtime down WITHOUT removing it from the registry,
// abandoning a build in flight. Returns once the runtime has closed.
// The next Resolve will lazy-rebuild it. Used by the desktop when the
// user closes the active repo.
func (s *Supervisor) Unload(id string) {
	s.mu.Lock()
	rt := s.detachLocked(id)
	s.mu.Unlock()
	s.finishClose(id, rt)
}

// SetName renames a registry entry and (if loaded) propagates the
// rename into the live Repository's manifest. Persists to repos.json
// AND updates the supervisor's in-memory entries map so subsequent
// List() / Resolve() calls reflect the new name without waiting for
// a reload. Returns the path of the renamed entry so callers needing
// the disk-only manifest rewrite (no live runtime) can do it.
func (s *Supervisor) SetName(id, name string) (string, error) {
	s.mu.Lock()
	_, ok := s.entries[id]
	s.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("supervisor: repo %q not in registry", id)
	}
	if err := config.SetRepoName(id, name); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Re-read: the entry may have changed (e.g. disabled) meanwhile.
	entry, ok := s.entries[id]
	if !ok {
		return "", fmt.Errorf("supervisor: repo %q not in registry", id)
	}
	entry.Name = name
	s.entries[id] = entry
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

// Close shuts down every loaded Runtime and waits for in-flight builds
// and closes to finish, so nothing keeps running after it returns.
// Later Loads fail with ErrClosed. Safe to call from a defer and more
// than once.
func (s *Supervisor) Close() {
	s.mu.Lock()
	s.closed = true
	ids := make([]string, 0, len(s.runtimes))
	for id := range s.runtimes {
		ids = append(ids, id)
	}
	for id := range s.building {
		ids = append(ids, id)
	}
	detached := make(map[string]*Runtime, len(ids))
	for _, id := range ids {
		if rt := s.detachLocked(id); rt != nil {
			detached[id] = rt
		}
	}
	s.mu.Unlock()

	var wg sync.WaitGroup
	for id, rt := range detached {
		wg.Add(1)
		go func(id string, rt *Runtime) {
			defer wg.Done()
			s.finishClose(id, rt)
		}(id, rt)
	}
	wg.Wait()

	// Abandoned builds close their runtimes when they finish.
	s.mu.Lock()
	for len(s.building) > 0 || len(s.closing) > 0 {
		s.changed.Wait()
	}
	s.mu.Unlock()
}
