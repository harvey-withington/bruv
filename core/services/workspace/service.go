package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"bruv/core/services/card"
	wsengine "bruv/core/workspace"
	"bruv/internal/model"
	"bruv/internal/repo"
	pathsafe "bruv/internal/workspace"

	"github.com/google/uuid"
)

// MaxReadBytes caps ReadFile — the Tier 2 editor targets markdown/plain
// text; anything bigger opens externally.
const MaxReadBytes = 10 << 20

// Deps is the narrow host contract for WorkspaceService.
type Deps interface {
	Repo() *repo.Repository
	// Publish announces a domain event: "workspace:updated" on every
	// mutation (attach, refresh, config, file write), "workspace:deleted"
	// on detach. Payload is a Ref.
	Publish(topic string, payload any)
	// Card routes card-folder bindings through the card service so its
	// activity-log + event instrumentation is inherited.
	Card() *card.Service
}

// Ref is the event payload locating the workspace's project.
type Ref struct {
	BrandSlug   string `json:"brand_slug"`
	StreamSlug  string `json:"stream_slug"`
	ProjectSlug string `json:"project_slug"`
}

// Service exposes vault-side workspace operations.
type Service struct{ deps Deps }

// New constructs a WorkspaceService.
func New(deps Deps) *Service { return &Service{deps: deps} }

func (s *Service) repo() (*repo.Repository, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r, nil
}

func (s *Service) emit(topic, b, st, p string) {
	s.deps.Publish(topic, Ref{BrandSlug: b, StreamSlug: st, ProjectSlug: p})
}

// Get returns the project's workspace config.
func (s *Service) Get(brandSlug, streamSlug, projectSlug string) (*model.Workspace, error) {
	r, err := s.repo()
	if err != nil {
		return nil, err
	}
	return r.GetWorkspace(brandSlug, streamSlug, projectSlug)
}

// GetIndex returns the cached adapter index.
func (s *Service) GetIndex(brandSlug, streamSlug, projectSlug string) (*model.WorkspaceIndex, error) {
	r, err := s.repo()
	if err != nil {
		return nil, err
	}
	return r.GetWorkspaceIndex(brandSlug, streamSlug, projectSlug)
}

// Attach connects an existing local folder to the project (0 or 1 workspace
// per project): detects the best adapter, indexes, persists, announces.
// The path must be visible to this backend — the UI gates attach on
// hasLocalFilesystem for exactly that reason.
func (s *Service) Attach(ctx context.Context, brandSlug, streamSlug, projectSlug, dirPath string) (*model.Workspace, error) {
	r, err := s.repo()
	if err != nil {
		return nil, err
	}
	if r.HasWorkspace(brandSlug, streamSlug, projectSlug) {
		return nil, fmt.Errorf("project %q already has a workspace — detach it first", projectSlug)
	}
	project, err := r.GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	fs, err := wsengine.NewLocalFS(dirPath)
	if err != nil {
		return nil, fmt.Errorf("open workspace folder: %w", err)
	}
	tree, _, err := fs.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("scan workspace folder: %w", err)
	}
	adapter := detectAdapter(tree)

	dir, _ := fs.LocalDir()
	ws := &model.Workspace{
		ID:        uuid.New().String(),
		ProjectID: project.ID,
		Origin:    model.WorkspaceOrigin{Kind: model.OriginLocal, URL: dir},
		Adapter:   adapter.Name(),
	}
	if err := r.SaveWorkspace(brandSlug, streamSlug, projectSlug, ws); err != nil {
		return nil, err
	}
	if _, err := s.RefreshIndex(ctx, brandSlug, streamSlug, projectSlug); err != nil {
		// The workspace is attached; a failed first index is a warning-grade
		// problem the panel surfaces via refresh, not an attach failure.
		s.emit("workspace:updated", brandSlug, streamSlug, projectSlug)
		return ws, nil
	}
	s.emit("workspace:updated", brandSlug, streamSlug, projectSlug)
	return ws, nil
}

// Detach removes the workspace config + index from the vault. Never touches
// the origin folder or any checkout.
func (s *Service) Detach(brandSlug, streamSlug, projectSlug string) error {
	r, err := s.repo()
	if err != nil {
		return err
	}
	if err := r.DeleteWorkspace(brandSlug, streamSlug, projectSlug); err != nil {
		return err
	}
	s.emit("workspace:deleted", brandSlug, streamSlug, projectSlug)
	return nil
}

// RefreshIndex re-runs the workspace's adapter and stores the result.
func (s *Service) RefreshIndex(ctx context.Context, brandSlug, streamSlug, projectSlug string) (*model.WorkspaceIndex, error) {
	r, err := s.repo()
	if err != nil {
		return nil, err
	}
	ws, err := r.GetWorkspace(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	fs, err := s.originFS(ws)
	if err != nil {
		return nil, err
	}
	idx, err := adapterByName(ws.Adapter).Index(ctx, fs)
	if err != nil {
		return nil, fmt.Errorf("index workspace: %w", err)
	}
	idx.WorkspaceID = ws.ID
	idx.GeneratedAt = time.Now().UTC()
	if err := r.SaveWorkspaceIndex(brandSlug, streamSlug, projectSlug, idx); err != nil {
		return nil, err
	}
	s.emit("workspace:updated", brandSlug, streamSlug, projectSlug)
	return idx, nil
}

// SetLaunchCommand stores the per-workspace "Open workspace in…" launcher.
func (s *Service) SetLaunchCommand(brandSlug, streamSlug, projectSlug, command string) (*model.Workspace, error) {
	r, err := s.repo()
	if err != nil {
		return nil, err
	}
	ws, err := r.GetWorkspace(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	ws.LaunchCommand = command
	if err := r.SaveWorkspace(brandSlug, streamSlug, projectSlug, ws); err != nil {
		return nil, err
	}
	s.emit("workspace:updated", brandSlug, streamSlug, projectSlug)
	return ws, nil
}

// ReadFile returns one text file's content (Tier 1/2 read). Every path goes
// through the internal/workspace chokepoint; binary content is refused —
// binaries open externally per Tier 1 rules.
func (s *Service) ReadFile(ctx context.Context, brandSlug, streamSlug, projectSlug, rel string) (string, error) {
	ws, root, err := s.localRoot(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return "", err
	}
	_ = ws
	abs, err := pathsafe.Resolve(root, rel)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", rel)
	}
	if info.Size() > MaxReadBytes {
		return "", fmt.Errorf("%s is too large to open in BRUV (%s) — use Open in default app", rel, humanBytes(info.Size()))
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(raw) {
		return "", fmt.Errorf("%s is not a text file — use Open in default app", rel)
	}
	return string(raw), nil
}

// WriteFile saves one text file (Tier 2 editor). User-initiated writes only —
// no AI tool calls this (AI write access is out of scope by spec). Atomic
// tmp+rename; the parent directory must already exist.
func (s *Service) WriteFile(ctx context.Context, brandSlug, streamSlug, projectSlug, rel, content string) error {
	_, root, err := s.localRoot(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return err
	}
	abs, err := pathsafe.Resolve(root, rel)
	if err != nil {
		return err
	}
	if info, err := os.Stat(abs); err == nil && info.IsDir() {
		return fmt.Errorf("%s is a directory", rel)
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".bruv-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, abs); err != nil {
		os.Remove(tmpName)
		return err
	}
	s.emit("workspace:updated", brandSlug, streamSlug, projectSlug)
	return nil
}

// originFS builds the FS for a workspace's origin. M1: local origins only;
// M2 adds the remote listing/fetch implementation over transports.
func (s *Service) originFS(ws *model.Workspace) (wsengine.FS, error) {
	switch ws.Origin.Kind {
	case model.OriginLocal:
		return wsengine.NewLocalFS(ws.Origin.URL)
	default:
		return nil, fmt.Errorf("origin kind %q is not supported yet on this device", ws.Origin.Kind)
	}
}

// localRoot returns the workspace's on-disk root for file operations,
// erroring when the files aren't on this device (Tier 0 here).
func (s *Service) localRoot(brandSlug, streamSlug, projectSlug string) (*model.Workspace, string, error) {
	r, err := s.repo()
	if err != nil {
		return nil, "", err
	}
	ws, err := r.GetWorkspace(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, "", err
	}
	if ws.Origin.Kind != model.OriginLocal {
		return nil, "", fmt.Errorf("workspace files are not on this device (origin %q) — not materialized", ws.Origin.Kind)
	}
	return ws, ws.Origin.URL, nil
}
