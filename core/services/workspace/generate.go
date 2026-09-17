package workspace

// Structure lives in the workspace (plan/2026-09-17 workspace files
// block.md): new files, new folders and template generation all act on the
// workspace tree, and a card then names the results through its Workspace
// Files block. Nothing here binds a card.

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"bruv/internal/model"
	pathsafe "bruv/internal/workspace"

	ft "github.com/harvey-withington/foldertemplate"
)

// ListProjectTemplates returns every template usable from this project:
// templates living INSIDE the project's workspace first (they travel with
// the work — the Bad Therapist pattern), then the vault/brand registries.
// Workspace-scoped entries use the template folder's absolute path as ID.
func (s *Service) ListProjectTemplates(brandSlug, streamSlug, projectSlug string) ([]TemplateEntry, error) {
	out := []TemplateEntry{}
	if ws, err := s.Get(brandSlug, streamSlug, projectSlug); err == nil && ws.Origin.Kind == model.OriginLocal {
		out = append(out, scanWorkspaceTemplates(ws.Origin.URL)...)
	}
	vault, err := s.ListTemplates()
	if err != nil {
		return nil, err
	}
	return append(out, vault...), nil
}

// scanWorkspaceTemplates walks a workspace folder for template roots
// (subfolders containing .ft/template.json), stopping the descent at each
// template and at VCS/app state dirs.
func scanWorkspaceTemplates(root string) []TemplateEntry {
	out := []TemplateEntry{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil //nolint:nilerr — unreadable subtrees are skipped, not fatal
		}
		switch d.Name() {
		case ".git", ".obsidian":
			return filepath.SkipDir
		}
		tpl, lerr := ft.Load(path)
		if lerr != nil {
			return nil // not a template root; keep descending
		}
		out = append(out, TemplateEntry{
			ID:                path, // absolute — resolvable by loadTemplateRef's IsAbs branch
			Name:              tpl.Name,
			Description:       tpl.Description,
			Scope:             "workspace",
			Parameters:        nonNilParams(tpl.Parameters),
			DefaultTargetPath: tpl.DefaultTargetPath,
		})
		return filepath.SkipDir // a template's own subtree is opaque
	})
	return out
}

// GenerateTemplate generates a folder template into the project's
// workspace and returns the generated root, workspace-relative. targetRel
// is the parent to generate under (blank → the template's own
// DefaultTargetPath); cardTitle, when set, feeds the {{$bruvCard}} built-in
// so a template generated from a card can name its folder after it.
// User-confirmed only — never AI-initiated.
func (s *Service) GenerateTemplate(ctx context.Context, brandSlug, streamSlug, projectSlug, ref, targetRel, cardTitle string, values map[string]string) (string, error) {
	ws, root, err := s.localRoot(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return "", err
	}
	tpl, err := s.loadTemplateRef(ref)
	if err != nil {
		return "", err
	}
	parentAbs, err := resolveTemplateTarget(root, tpl, targetRel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(parentAbs, 0755); err != nil {
		return "", err
	}

	extra := s.builtinParams(brandSlug, streamSlug, projectSlug)
	if cardTitle != "" {
		extra["bruvCard"] = cardTitle
	}
	res, err := ft.Generate(tpl, parentAbs, values, extra, nil)
	if err != nil {
		return "", err
	}
	rel := relOf(root, res.RootPath)

	s.commitIfEnabled(ctx, ws, root, []string{rel}, "Generate "+rel+" from template (BRUV)")
	// Index freshness is best-effort — the folder is the result. RefreshIndex
	// emits workspace:updated on success; emit explicitly on failure too so
	// open panels re-fetch rather than showing a stale tree.
	if _, err := s.RefreshIndex(ctx, brandSlug, streamSlug, projectSlug); err != nil {
		slog.Warn("generate template: index refresh failed", "err", err)
		s.emit("workspace:updated", brandSlug, streamSlug, projectSlug)
	}
	return rel, nil
}

// CreateDir makes a new folder (and any missing parents) inside the
// workspace. Returns the cleaned workspace-relative path. Git tracks no
// empty folders, so there is nothing to commit until a file lands in it.
func (s *Service) CreateDir(ctx context.Context, brandSlug, streamSlug, projectSlug, rel string) (string, error) {
	_, root, err := s.localRoot(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return "", err
	}
	abs, err := pathsafe.Resolve(root, rel)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(abs); err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("%s already exists", rel)
		}
		return "", fmt.Errorf("%s already exists as a file", rel)
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return "", err
	}
	s.emit("workspace:updated", brandSlug, streamSlug, projectSlug)
	return relOf(root, abs), nil
}

// CreateFile makes a new, empty text file (parents created as needed).
// Refuses to overwrite: an existing path is an error, never a truncation.
func (s *Service) CreateFile(ctx context.Context, brandSlug, streamSlug, projectSlug, rel string) (string, error) {
	ws, root, err := s.localRoot(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return "", err
	}
	abs, err := pathsafe.Resolve(root, rel)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err == nil {
		return "", fmt.Errorf("%s already exists", rel)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return "", err
	}
	f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	clean := relOf(root, abs)
	s.commitIfEnabled(ctx, ws, root, []string{clean}, "Create "+clean+" (BRUV)")
	s.emit("workspace:updated", brandSlug, streamSlug, projectSlug)
	return clean, nil
}

// relOf is the slash-form workspace-relative path of abs (already
// chokepoint-resolved, so Rel cannot fail in practice).
func relOf(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}

// resolveTemplateTarget picks the generation parent:
//   - targetRel set (user override in the dialog): workspace-root-relative,
//     through the chokepoint.
//   - targetRel blank: the template's own DefaultTargetPath — a relative
//     path (../ allowed) resolved against the template folder's PARENT
//     (Bad Therapist: template `<show>/_Template - …` + "Episodes" →
//     `<show>/Episodes`), or an absolute path taken as-is.
//
// Either way the result must stay INSIDE the workspace root — generated
// folders are workspace content by definition.
func resolveTemplateTarget(wsRoot string, tpl *ft.Template, targetRel string) (string, error) {
	if targetRel != "" {
		return pathsafe.Resolve(wsRoot, targetRel)
	}
	dtp := tpl.DefaultTargetPath
	if dtp == "" {
		return pathsafe.Resolve(wsRoot, "")
	}
	abs := filepath.FromSlash(dtp)
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(filepath.Dir(tpl.Dir()), abs)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(wsRoot, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("template target %q resolves outside the workspace — set a target inside it", dtp)
	}
	// Re-run through the chokepoint for symlink safety.
	return pathsafe.Resolve(wsRoot, filepath.ToSlash(rel))
}
