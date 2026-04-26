package supervisor

// HTTPAdapter satisfies transport/http.RepoBackend by wrapping a
// Supervisor. Both the headless server (internal/server) and the
// desktop App (main package) construct one to plug their supervisor
// into the multi-repo HTTP transport — same wiring on both sides
// keeps Local and Remote routing identical.
//
// The supervisor stays free of HTTP types; this adapter is the only
// place transport concerns leak in. Lives in core/ so wiring callers
// don't need a separate type each.

import (
	"fmt"
	"log/slog"

	"bruv/internal/repo"
	transporthttp "bruv/transport/http"
)

// HTTPAdapter implements transport/http.RepoBackend.
//
// Resolve lazy-loads the per-repo Runtime via the Supervisor on first
// access and returns nil for unknown / disabled repos (the transport
// turns nil into 404). The desktop relies on this lazy-load: no
// runtime is built until the user's first RPC call against a given
// repo, so cold-start cost stays per-repo and registered-but-unused
// repos pay nothing.
type HTTPAdapter struct {
	Sup *Supervisor
}

// NewHTTPAdapter constructs an HTTPAdapter around the given Supervisor.
func NewHTTPAdapter(sup *Supervisor) *HTTPAdapter {
	return &HTTPAdapter{Sup: sup}
}

func (a *HTTPAdapter) Resolve(id string) *transporthttp.RepoTarget {
	rt, err := a.Sup.Load(id)
	if err != nil || rt == nil {
		return nil
	}
	return &transporthttp.RepoTarget{
		Target: rt,
		Bus:    rt.Bus(),
		Attachments: &transporthttp.AttachmentConfig{
			Secret:  a.Sup.Secret(),
			Resolve: rt.ResolveAttachment,
		},
	}
}

func (a *HTTPAdapter) List() []transporthttp.RepoSummary {
	entries := a.Sup.List()
	out := make([]transporthttp.RepoSummary, 0, len(entries))
	for _, e := range entries {
		out = append(out, transporthttp.RepoSummary{
			ID:       e.ID,
			Name:     e.Name,
			Disabled: e.Disabled,
		})
	}
	return out
}

func (a *HTTPAdapter) SetEnabled(id string, enabled bool) error {
	return a.Sup.SetEnabled(id, enabled)
}

// Inspect implements the read-only side of the unified repo-add flow:
// the picker calls this with a freshly-picked folder path to learn
// whether it's already a BRUV repo (UI shows "Open this repo") or a
// fresh folder (UI prompts for a name to init with). Errors only on
// I/O failures; absent manifest just returns Exists=false.
func (a *HTTPAdapter) Inspect(path string) (transporthttp.RepoInspect, error) {
	m, err := repo.InspectAt(path)
	if err != nil {
		return transporthttp.RepoInspect{}, err
	}
	if m == nil {
		return transporthttp.RepoInspect{Exists: false}, nil
	}
	return transporthttp.RepoInspect{Exists: true, Name: m.Name, ID: m.ID}, nil
}

// InitOrOpen ensures a repo is registered + loaded at the given path.
// If the path is already a BRUV repo, opens it (with revalidation).
// Otherwise inits a fresh one with the given name. Idempotent on
// already-registered paths. Returns the resulting RepoSummary so the
// client can stamp the new ID into its connection's repo-recents.
func (a *HTTPAdapter) InitOrOpen(path, name string) (transporthttp.RepoSummary, error) {
	inspect, err := repo.InspectAt(path)
	if err != nil {
		return transporthttp.RepoSummary{}, fmt.Errorf("inspect path: %w", err)
	}
	var rootPath string
	if inspect != nil {
		// Existing repo — open + revalidate. Name parameter is
		// ignored; the manifest's name wins.
		r, err := repo.Open(path)
		if err != nil {
			return transporthttp.RepoSummary{}, fmt.Errorf("open repo: %w", err)
		}
		if stats, revErr := r.Revalidate(); revErr != nil {
			slog.Warn("revalidation failed", "err", revErr)
		} else {
			slog.Info("revalidate ok", "stats", stats.String())
		}
		rootPath = r.Root
	} else {
		// Fresh folder — init.
		if name == "" {
			return transporthttp.RepoSummary{}, fmt.Errorf("name is required when initialising a new repo")
		}
		r, err := repo.InitAt(path, name)
		if err != nil {
			return transporthttp.RepoSummary{}, fmt.Errorf("init repo: %w", err)
		}
		rootPath = r.Root
	}
	rt, err := a.Sup.RegisterAndLoad(rootPath)
	if err != nil {
		return transporthttp.RepoSummary{}, fmt.Errorf("register and load: %w", err)
	}
	entry, ok := a.Sup.EntryByPath(rt.Repo().Root)
	if !ok {
		// RegisterAndLoad just appended via config.AppendRepo; an
		// EntryByPath miss right after means the in-memory entries
		// map fell out of sync with disk — defensively reload.
		return transporthttp.RepoSummary{}, fmt.Errorf("registered but not found in registry view")
	}
	return transporthttp.RepoSummary{
		ID:       entry.ID,
		Name:     entry.Name,
		Disabled: entry.Disabled,
	}, nil
}

// Rename updates BOTH the registry name (per-machine label, controls
// picker display) and the in-repo manifest name (portable identity
// that travels with the repo). When the renamed repo is loaded, the
// manifest write goes through the live Repository so its in-memory
// copy stays in sync; otherwise a disk-only rewrite suffices.
//
// SetName persists the registry change AND refreshes the supervisor's
// in-memory entries map — the latter is critical, otherwise the very
// next List() (e.g. the picker's post-rename refresh) would return
// the old name even though disk has the new one.
func (a *HTTPAdapter) Rename(id, name string) error {
	path, err := a.Sup.SetName(id, name)
	if err != nil {
		return err
	}
	if rt := a.Sup.Resolve(id); rt != nil && rt.Repo() != nil {
		if err := rt.Repo().UpdateManifestName(name); err != nil {
			return fmt.Errorf("update manifest: %w", err)
		}
		return nil
	}
	if err := repo.RewriteManifestName(path, name); err != nil {
		return fmt.Errorf("update manifest: %w", err)
	}
	return nil
}

// Remove drops a repo from the registry. The folder on disk is left
// alone. Unloads the runtime first so file handles release before the
// registry write — lets the user immediately delete the folder if
// they want — and prunes the supervisor's in-memory entries so the
// next List() doesn't show the gone repo.
func (a *HTTPAdapter) Remove(id string) error {
	return a.Sup.Remove(id)
}
