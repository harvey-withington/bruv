package supervisor

import (
	"fmt"
	"log/slog"
	"sync"

	"bruv/internal/config"
)

// Supervisor holds N Runtimes indexed by repo ID and resolves them at
// request time. Disabled entries are present in the registry but have
// no Runtime — Resolve returns nil so the per-repo dispatcher 404s.
// Re-enabling lazy-builds the Runtime.
type Supervisor struct {
	mu        sync.Mutex
	configDir string
	runtimes  map[string]*Runtime
	entries   map[string]config.RepoEntry
	secret    []byte
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
	}
	for _, e := range entries {
		s.entries[e.ID] = e
	}
	return s, nil
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
func (s *Supervisor) Load(id string) (*Runtime, error) {
	s.mu.Lock()
	if rt, ok := s.runtimes[id]; ok {
		s.mu.Unlock()
		return rt, nil
	}
	entry, ok := s.entries[id]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("supervisor: repo %q not in registry", id)
	}
	rt, err := buildRuntime(entry.Path, s.configDir)
	if err != nil {
		return nil, fmt.Errorf("supervisor: build %q: %w", id, err)
	}
	s.mu.Lock()
	s.runtimes[id] = rt
	s.mu.Unlock()
	return rt, nil
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
	existing := s.runtimes[entry.ID]
	s.mu.Unlock()
	if existing != nil {
		return existing, nil
	}
	rt, err := buildRuntime(entry.Path, s.configDir)
	if err != nil {
		return nil, fmt.Errorf("supervisor: build runtime: %w", err)
	}
	s.mu.Lock()
	s.runtimes[entry.ID] = rt
	s.mu.Unlock()
	return rt, nil
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
	s.mu.Unlock()
	if rt != nil {
		rt.Close()
	}
}

// Close shuts down every loaded Runtime. Safe to call from a defer.
func (s *Supervisor) Close() {
	s.mu.Lock()
	rts := make([]*Runtime, 0, len(s.runtimes))
	for _, rt := range s.runtimes {
		rts = append(rts, rt)
	}
	s.runtimes = nil
	s.mu.Unlock()
	for _, rt := range rts {
		rt.Close()
	}
}
