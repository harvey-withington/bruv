package repo

import (
	"bruv/internal/model"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Vault-side workspace persistence: workspace.json (config) and index.json
// (adapter output) under the owning project's workspace/ directory. The
// device-side pieces (localPath, snapshot.json, the working copy) are NOT
// stored here — see internal/model/workspace.go for the split.

const (
	workspaceDir       = "workspace"
	workspaceFile      = "workspace.json"
	workspaceIndexFile = "index.json"
)

func (r *Repository) workspacePath(brandSlug, streamSlug, projectSlug string) string {
	return filepath.Join(r.projectPath(brandSlug, streamSlug, projectSlug), workspaceDir)
}

func (r *Repository) workspaceFilePath(brandSlug, streamSlug, projectSlug string) string {
	return filepath.Join(r.workspacePath(brandSlug, streamSlug, projectSlug), workspaceFile)
}

func (r *Repository) workspaceIndexFilePath(brandSlug, streamSlug, projectSlug string) string {
	return filepath.Join(r.workspacePath(brandSlug, streamSlug, projectSlug), workspaceIndexFile)
}

// HasWorkspace reports whether the project has a workspace attached.
func (r *Repository) HasWorkspace(brandSlug, streamSlug, projectSlug string) bool {
	return fileExists(r.workspaceFilePath(brandSlug, streamSlug, projectSlug))
}

// GetWorkspace reads the project's workspace config.
func (r *Repository) GetWorkspace(brandSlug, streamSlug, projectSlug string) (*model.Workspace, error) {
	path := r.workspaceFilePath(brandSlug, streamSlug, projectSlug)
	if !fileExists(path) {
		return nil, fmt.Errorf("project %q has no workspace", projectSlug)
	}
	var ws model.Workspace
	if err := readJSON(path, &ws); err != nil {
		return nil, err
	}
	return &ws, nil
}

// SaveWorkspace writes the workspace config, stamping UpdatedAt. The owning
// project must exist.
func (r *Repository) SaveWorkspace(brandSlug, streamSlug, projectSlug string, ws *model.Workspace) error {
	if _, err := r.GetProject(brandSlug, streamSlug, projectSlug); err != nil {
		return err
	}
	if err := os.MkdirAll(r.workspacePath(brandSlug, streamSlug, projectSlug), 0755); err != nil {
		return fmt.Errorf("create workspace directory: %w", err)
	}
	ws.UpdatedAt = time.Now().UTC()
	if ws.CreatedAt.IsZero() {
		ws.CreatedAt = ws.UpdatedAt
	}
	if err := writeJSON(r.workspaceFilePath(brandSlug, streamSlug, projectSlug), ws); err != nil {
		return fmt.Errorf("write workspace: %w", err)
	}
	return nil
}

// GetWorkspaceIndex reads the cached adapter index, if one has been generated.
func (r *Repository) GetWorkspaceIndex(brandSlug, streamSlug, projectSlug string) (*model.WorkspaceIndex, error) {
	path := r.workspaceIndexFilePath(brandSlug, streamSlug, projectSlug)
	if !fileExists(path) {
		return nil, fmt.Errorf("project %q has no workspace index", projectSlug)
	}
	var idx model.WorkspaceIndex
	if err := readJSON(path, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// SaveWorkspaceIndex writes the adapter's index output.
func (r *Repository) SaveWorkspaceIndex(brandSlug, streamSlug, projectSlug string, idx *model.WorkspaceIndex) error {
	if !r.HasWorkspace(brandSlug, streamSlug, projectSlug) {
		return fmt.Errorf("project %q has no workspace", projectSlug)
	}
	if err := writeJSON(r.workspaceIndexFilePath(brandSlug, streamSlug, projectSlug), idx); err != nil {
		return fmt.Errorf("write workspace index: %w", err)
	}
	return nil
}

// DeleteWorkspace detaches the workspace: removes workspace.json, index.json
// and the workspace/ directory. It never touches the origin or any checkout —
// detaching is a vault-side operation only.
func (r *Repository) DeleteWorkspace(brandSlug, streamSlug, projectSlug string) error {
	dir := r.workspacePath(brandSlug, streamSlug, projectSlug)
	if !fileExists(dir) {
		return fmt.Errorf("project %q has no workspace", projectSlug)
	}
	cleanup := r.withDirOp(dir)
	defer cleanup()
	return os.RemoveAll(dir)
}

// WorkspaceRef locates a workspace within the vault hierarchy.
type WorkspaceRef struct {
	BrandSlug   string          `json:"brand_slug"`
	StreamSlug  string          `json:"stream_slug"`
	ProjectSlug string          `json:"project_slug"`
	Workspace   model.Workspace `json:"workspace"`
}

// ListWorkspaces walks every project and returns the attached workspaces.
// N is small (0–1 per project); no index table needed for v1.
func (r *Repository) ListWorkspaces() ([]WorkspaceRef, error) {
	brands, err := r.ListBrands()
	if err != nil {
		return nil, err
	}
	var refs []WorkspaceRef
	for _, b := range brands {
		streams, err := r.ListStreams(b.Slug)
		if err != nil {
			continue
		}
		for _, s := range streams {
			projects, err := r.ListProjects(b.Slug, s.Slug)
			if err != nil {
				continue
			}
			for _, p := range projects {
				ws, err := r.GetWorkspace(b.Slug, s.Slug, p.Slug)
				if err != nil {
					continue
				}
				refs = append(refs, WorkspaceRef{
					BrandSlug:   b.Slug,
					StreamSlug:  s.Slug,
					ProjectSlug: p.Slug,
					Workspace:   *ws,
				})
			}
		}
	}
	return refs, nil
}
