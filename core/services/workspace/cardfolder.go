package workspace

// Card Folders (plan/2026-07-05 card folders design.md): a card binds to a
// subfolder of its project's Workspace, generated from a Folder Template.
// The Workspace stays 0-or-1 per project; cards get reachability into it.

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

// GenerateCardFolder generates a template into the project's workspace and
// binds the resulting folder to the card. targetRel is the workspace-
// relative parent (the create dialog pre-fills it from the template's
// relative defaultTargetPath). User-confirmed only — never AI-initiated.
func (s *Service) GenerateCardFolder(ctx context.Context, brandSlug, streamSlug, projectSlug, cardID, ref, targetRel string, values map[string]string) (*model.Card, error) {
	r, err := s.repo()
	if err != nil {
		return nil, err
	}
	ws, err := r.GetWorkspace(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	if ws.Origin.Kind != model.OriginLocal {
		return nil, fmt.Errorf("workspace files are not on this device (origin %q) — not materialized", ws.Origin.Kind)
	}
	card, err := r.GetCard(cardID)
	if err != nil {
		return nil, err
	}
	if card.Folder != nil {
		return nil, fmt.Errorf("card already has a folder (%s) — unbind it first", card.Folder.Path)
	}
	tpl, err := s.loadTemplateRef(ref)
	if err != nil {
		return nil, err
	}

	parentAbs, err := resolveCardFolderTarget(ws.Origin.URL, tpl, targetRel)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(parentAbs, 0755); err != nil {
		return nil, err
	}

	// Built-in context params gain bruvCard: the card's title, so an
	// episode template can name its folder after the episode card.
	extra := s.builtinParams(brandSlug, streamSlug, projectSlug)
	extra["bruvCard"] = card.Title

	res, err := ft.Generate(tpl, parentAbs, values, extra, nil)
	if err != nil {
		return nil, err
	}
	folderRel, err := filepath.Rel(ws.Origin.URL, res.RootPath)
	if err != nil {
		return nil, err
	}

	updated, err := s.deps.Card().SetFolder(cardID, &model.CardFolder{
		WorkspaceID: ws.ID,
		Path:        filepath.ToSlash(folderRel),
	})
	if err != nil {
		return nil, fmt.Errorf("folder generated at %s but binding failed: %w", res.RootPath, err)
	}
	// Index freshness is best-effort — the folder + binding are the result.
	// RefreshIndex emits workspace:updated on success; emit explicitly on
	// failure too so open panels re-fetch rather than showing a stale tree.
	if _, err := s.RefreshIndex(ctx, brandSlug, streamSlug, projectSlug); err != nil {
		slog.Warn("card folder: index refresh failed", "err", err)
		s.emit("workspace:updated", brandSlug, streamSlug, projectSlug)
	}
	return updated, nil
}

// LinkCardFolder binds an EXISTING workspace subfolder to the card — the
// re-link-after-unlink and made-it-by-hand paths. Files untouched.
func (s *Service) LinkCardFolder(brandSlug, streamSlug, projectSlug, cardID, rel string) (*model.Card, error) {
	r, err := s.repo()
	if err != nil {
		return nil, err
	}
	ws, err := r.GetWorkspace(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	card, err := r.GetCard(cardID)
	if err != nil {
		return nil, err
	}
	if card.Folder != nil {
		return nil, fmt.Errorf("card already has a folder (%s) — unbind it first", card.Folder.Path)
	}
	if ws.Origin.Kind == model.OriginLocal {
		abs, err := pathsafe.Resolve(ws.Origin.URL, rel)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("%s is not a folder in the workspace", rel)
		}
	}
	return s.deps.Card().SetFolder(cardID, &model.CardFolder{
		WorkspaceID: ws.ID,
		Path:        filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel))),
	})
}

// ClearCardFolder unbinds the card's folder. Files are never touched —
// unbinding is a vault-side operation only.
func (s *Service) ClearCardFolder(cardID string) (*model.Card, error) {
	return s.deps.Card().SetFolder(cardID, nil)
}

// resolveCardFolderTarget picks the generation parent:
//   - targetRel set (user override in the dialog): workspace-root-relative,
//     through the chokepoint.
//   - targetRel blank: the template's own DefaultTargetPath — a relative
//     path (../ allowed) resolved against the template folder's PARENT
//     (Bad Therapist: template `<show>/_Template - …` + "Episodes" →
//     `<show>/Episodes`), or an absolute path taken as-is.
//
// Either way the result must stay INSIDE the workspace root — card folders
// are workspace content by definition.
func resolveCardFolderTarget(wsRoot string, tpl *ft.Template, targetRel string) (string, error) {
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
