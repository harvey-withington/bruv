package supervisor

// Workspace RPC surface (M1: local origins). Thin forwarders to the
// WorkspaceService — auto-exposed to /repos/<id>/rpc by the reflection
// dispatcher, so adding a method here is the entire backend registration.
// Mirror every signature into shared/types.ts (positional params, Go
// declaration order — mobile calls by position).

import (
	"context"

	workspacesvc "bruv/core/services/workspace"
	"bruv/internal/model"

	ft "github.com/harvey-withington/foldertemplate"
)

// WorkspaceState is the panel's single-fetch view of a project's workspace.
type WorkspaceState struct {
	Attached  bool                  `json:"attached"`
	Workspace *model.Workspace      `json:"workspace,omitempty"`
	Index     *model.WorkspaceIndex `json:"index,omitempty"`
}

// GetWorkspaceState returns the workspace + cached index, or Attached=false —
// never an error for the everyday "no workspace here" case.
func (r *Runtime) GetWorkspaceState(brandSlug, streamSlug, projectSlug string) (*WorkspaceState, error) {
	ws, err := r.Workspace.Get(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return &WorkspaceState{Attached: false}, nil
	}
	state := &WorkspaceState{Attached: true, Workspace: ws}
	if idx, err := r.Workspace.GetIndex(brandSlug, streamSlug, projectSlug); err == nil {
		state.Index = idx
	}
	return state, nil
}

// AttachWorkspace connects an existing local folder to the project.
func (r *Runtime) AttachWorkspace(ctx context.Context, brandSlug, streamSlug, projectSlug, dirPath string) (*model.Workspace, error) {
	return r.Workspace.Attach(ctx, brandSlug, streamSlug, projectSlug, dirPath)
}

// DetachWorkspace removes the workspace config from the vault (files untouched).
func (r *Runtime) DetachWorkspace(brandSlug, streamSlug, projectSlug string) error {
	return r.Workspace.Detach(brandSlug, streamSlug, projectSlug)
}

// RefreshWorkspaceIndex re-runs the adapter and returns the fresh index.
func (r *Runtime) RefreshWorkspaceIndex(ctx context.Context, brandSlug, streamSlug, projectSlug string) (*model.WorkspaceIndex, error) {
	return r.Workspace.RefreshIndex(ctx, brandSlug, streamSlug, projectSlug)
}

// SetWorkspaceLaunchCommand stores the "Open workspace in…" launcher.
func (r *Runtime) SetWorkspaceLaunchCommand(brandSlug, streamSlug, projectSlug, command string) (*model.Workspace, error) {
	return r.Workspace.SetLaunchCommand(brandSlug, streamSlug, projectSlug, command)
}

// ReadWorkspaceFile returns one text file's content (Tier 1/2 read).
func (r *Runtime) ReadWorkspaceFile(ctx context.Context, brandSlug, streamSlug, projectSlug, rel string) (string, error) {
	return r.Workspace.ReadFile(ctx, brandSlug, streamSlug, projectSlug, rel)
}

// WriteWorkspaceFile saves one text file (Tier 2 editor; user-initiated only).
func (r *Runtime) WriteWorkspaceFile(ctx context.Context, brandSlug, streamSlug, projectSlug, rel, content string) error {
	return r.Workspace.WriteFile(ctx, brandSlug, streamSlug, projectSlug, rel, content)
}

// --- Templates -------------------------------------------------------------

// ListWorkspaceTemplates merges global + Brand-scoped vault templates.
func (r *Runtime) ListWorkspaceTemplates() ([]workspacesvc.TemplateEntry, error) {
	return r.Workspace.ListTemplates()
}

// GetWorkspaceTemplateParams returns a template's parameter list for the form.
func (r *Runtime) GetWorkspaceTemplateParams(ref string) ([]ft.Parameter, error) {
	return r.Workspace.GetTemplateParams(ref)
}

// WorkspaceTemplatePreview bundles a dry run's tree + warnings (the RPC
// dispatcher supports one non-error return value).
type WorkspaceTemplatePreview struct {
	Entries  []ft.PreviewEntry `json:"entries"`
	Warnings []string          `json:"warnings,omitempty"`
}

// PreviewWorkspaceTemplate dry-runs generation with the given values.
func (r *Runtime) PreviewWorkspaceTemplate(ref string, values map[string]string) (*WorkspaceTemplatePreview, error) {
	entries, warnings, err := r.Workspace.PreviewTemplate(ref, values)
	if err != nil {
		return nil, err
	}
	return &WorkspaceTemplatePreview{Entries: entries, Warnings: warnings}, nil
}

// GenerateWorkspaceFromTemplate generates into targetParent and attaches the
// result to the project. User-confirmed only — never AI-initiated.
func (r *Runtime) GenerateWorkspaceFromTemplate(ctx context.Context, brandSlug, streamSlug, projectSlug, ref, targetParent string, values map[string]string) (*model.Workspace, error) {
	return r.Workspace.GenerateFromTemplate(ctx, brandSlug, streamSlug, projectSlug, ref, targetParent, values)
}

// InspectWorkspaceTemplateFolder validates a folder before import.
func (r *Runtime) InspectWorkspaceTemplateFolder(dir string) (*workspacesvc.TemplateInspection, error) {
	return r.Workspace.InspectTemplateFolder(dir)
}

// ImportWorkspaceTemplate copies a template folder into the vault
// (brandSlug == "" → global scope).
func (r *Runtime) ImportWorkspaceTemplate(srcDir, brandSlug string) (*workspacesvc.TemplateEntry, error) {
	return r.Workspace.ImportTemplateFromFolder(srcDir, brandSlug)
}

// SaveWorkspaceTemplate writes a template descriptor (template editor save).
func (r *Runtime) SaveWorkspaceTemplate(ref string, tpl ft.Template) error {
	return r.Workspace.SaveTemplate(ref, tpl)
}
