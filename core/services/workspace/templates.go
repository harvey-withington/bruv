package workspace

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"bruv/internal/model"
	pathsafe "bruv/internal/workspace"

	ft "github.com/harvey-withington/foldertemplate"
)

// Templates live in the vault (spec §6.3): <vault>/templates/ for global,
// brands/<brand>/templates/ for Brand-scoped. Every subfolder containing
// .ft/template.json is a template. The vault indexer and watcher treat these
// directories as opaque; only the entries below are exposed.

const templatesDirName = "templates"

// ImportSizeWarnBytes — vault-resident templates ride the user's vault
// backup/sync, so heavyweight placeholder media should be a conscious choice.
const ImportSizeWarnBytes = 50 << 20

// TemplateEntry is what the picker and the AI list. ID is the vault-relative
// slash path — stable, human-readable, and resolvable server-side only.
type TemplateEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Scope is "global" or the owning brand's slug.
	Scope      string         `json:"scope"`
	Parameters []ft.Parameter `json:"parameters"`
}

// ListTemplates merges vault-level and Brand-scoped templates.
func (s *Service) ListTemplates() ([]TemplateEntry, error) {
	r, err := s.repo()
	if err != nil {
		return nil, err
	}
	var out []TemplateEntry
	scan := func(dir, scope string) {
		root := filepath.Join(r.Root, filepath.FromSlash(dir))
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil //nolint:nilerr — missing templates dirs are normal
			}
			tpl, lerr := ft.Load(path)
			if lerr != nil {
				return nil // not a template root; keep descending
			}
			rel, rerr := filepath.Rel(r.Root, path)
			if rerr != nil {
				return nil
			}
			out = append(out, TemplateEntry{
				ID:          filepath.ToSlash(rel),
				Name:        tpl.Name,
				Description: tpl.Description,
				Scope:       scope,
				Parameters:  tpl.Parameters,
			})
			return filepath.SkipDir // a template's own subtree is opaque
		})
	}
	scan(templatesDirName, "global")
	brands, err := r.ListBrands()
	if err == nil {
		for _, b := range brands {
			scan("brands/"+b.Slug+"/"+templatesDirName, b.Slug)
		}
	}
	return out, nil
}

// loadTemplateRef resolves a template reference: an absolute folder path (the
// editor's edit-in-place workflow) or a vault-relative template ID. Vault IDs
// go through the path chokepoint so an ID can never escape the vault.
func (s *Service) loadTemplateRef(ref string) (*ft.Template, error) {
	if filepath.IsAbs(ref) {
		return ft.Load(ref)
	}
	r, err := s.repo()
	if err != nil {
		return nil, err
	}
	abs, err := pathsafe.Resolve(r.Root, ref)
	if err != nil {
		return nil, err
	}
	return ft.Load(abs)
}

// GetTemplateParams returns a template's full parameter list for the form.
func (s *Service) GetTemplateParams(ref string) ([]ft.Parameter, error) {
	tpl, err := s.loadTemplateRef(ref)
	if err != nil {
		return nil, err
	}
	return tpl.Parameters, nil
}

// PreviewTemplate dry-runs generation for the editor/creation dialog.
func (s *Service) PreviewTemplate(ref string, values map[string]string) ([]ft.PreviewEntry, []string, error) {
	tpl, err := s.loadTemplateRef(ref)
	if err != nil {
		return nil, nil, err
	}
	return ft.Preview(tpl, values, s.builtinParams("", "", ""), nil)
}

// GenerateFromTemplate creates a new workspace folder from a template and
// attaches it to the project (Tier 1, local origin). The user confirmed the
// parameter form; generation itself is never AI-initiated (spec §6.3).
func (s *Service) GenerateFromTemplate(ctx context.Context, brandSlug, streamSlug, projectSlug, ref, targetParent string, values map[string]string) (*model.Workspace, error) {
	r, err := s.repo()
	if err != nil {
		return nil, err
	}
	if r.HasWorkspace(brandSlug, streamSlug, projectSlug) {
		return nil, fmt.Errorf("project %q already has a workspace — detach it first", projectSlug)
	}
	tpl, err := s.loadTemplateRef(ref)
	if err != nil {
		return nil, err
	}
	res, err := ft.Generate(tpl, targetParent, values, s.builtinParams(brandSlug, streamSlug, projectSlug), nil)
	if err != nil {
		return nil, err
	}
	return s.Attach(ctx, brandSlug, streamSlug, projectSlug, res.RootPath)
}

// builtinParams are BRUV's injected context parameters, resolvable in names
// and content without being declared (declared parameters win).
func (s *Service) builtinParams(brandSlug, streamSlug, projectSlug string) map[string]string {
	extra := map[string]string{"bruvDate": time.Now().Format("2006-01-02")}
	r := s.deps.Repo()
	if r == nil {
		return extra
	}
	if b, err := r.GetBrand(brandSlug); err == nil {
		extra["bruvBrand"] = b.Name
	}
	if st, err := r.GetStream(brandSlug, streamSlug); err == nil {
		extra["bruvStream"] = st.Name
	}
	if p, err := r.GetProject(brandSlug, streamSlug, projectSlug); err == nil {
		extra["bruvProject"] = p.Name
	}
	return extra
}

// TemplateInspection is the pre-import review of a candidate folder.
type TemplateInspection struct {
	IsTemplate  bool           `json:"is_template"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  []ft.Parameter `json:"parameters"`
	SizeBytes   int64          `json:"size_bytes"`
	// LargeWarning flags folders above ImportSizeWarnBytes.
	LargeWarning bool     `json:"large_warning"`
	Issues       []string `json:"issues,omitempty"`
}

// InspectTemplateFolder validates a folder before import: is it a template,
// what does it declare, how big is it. Folders without .ft/ report
// IsTemplate=false — the UI offers to templatize them after import.
func (s *Service) InspectTemplateFolder(dir string) (*TemplateInspection, error) {
	insp := &TemplateInspection{}
	var size int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info, e := d.Info(); e == nil && !d.IsDir() {
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	insp.SizeBytes = size
	insp.LargeWarning = size > ImportSizeWarnBytes

	tpl, err := ft.Load(dir)
	if err != nil {
		return insp, nil // not a template — still importable via templatize
	}
	insp.IsTemplate = true
	insp.Name = tpl.Name
	insp.Description = tpl.Description
	insp.Parameters = tpl.Parameters
	for _, issue := range tpl.Validate() {
		insp.Issues = append(insp.Issues, issue.Error())
	}
	return insp, nil
}

// ImportTemplateFromFolder copies a template folder into the vault:
// <vault>/templates/ (brandSlug == "") or the brand's templates/. The .ft/
// directory travels with it; nothing is excluded. Refuses invalid templates
// and name collisions.
func (s *Service) ImportTemplateFromFolder(srcDir, brandSlug string) (*TemplateEntry, error) {
	r, err := s.repo()
	if err != nil {
		return nil, err
	}
	tpl, err := ft.Load(srcDir)
	if err != nil {
		return nil, fmt.Errorf("not a template: %w", err)
	}
	if issues := tpl.Validate(); len(issues) > 0 {
		return nil, fmt.Errorf("template is invalid: %v", issues[0])
	}

	relDir := templatesDirName
	scope := "global"
	if brandSlug != "" {
		if _, err := r.GetBrand(brandSlug); err != nil {
			return nil, err
		}
		relDir = "brands/" + brandSlug + "/" + templatesDirName
		scope = brandSlug
	}
	destParent := filepath.Join(r.Root, filepath.FromSlash(relDir))
	dest := filepath.Join(destParent, filepath.Base(srcDir))
	if _, err := os.Stat(dest); err == nil {
		return nil, fmt.Errorf("a template folder named %q already exists", filepath.Base(srcDir))
	}
	if err := os.MkdirAll(destParent, 0755); err != nil {
		return nil, err
	}
	if err := copyDir(srcDir, dest); err != nil {
		os.RemoveAll(dest)
		return nil, err
	}
	id := filepath.ToSlash(filepath.Join(relDir, filepath.Base(srcDir)))
	s.deps.Publish("workspace:templates", Ref{})
	return &TemplateEntry{
		ID: id, Name: tpl.Name, Description: tpl.Description,
		Scope: scope, Parameters: tpl.Parameters,
	}, nil
}

// SaveTemplate writes a template descriptor (the template editor's save).
// ref is an absolute folder path or a vault-relative template ID; saving to a
// folder without .ft/ creates it (templatize).
func (s *Service) SaveTemplate(ref string, tpl ft.Template) error {
	dir := ref
	if !filepath.IsAbs(ref) {
		r, err := s.repo()
		if err != nil {
			return err
		}
		abs, err := pathsafe.Resolve(r.Root, ref)
		if err != nil {
			return err
		}
		dir = abs
	}
	if err := ft.Save(&tpl, dir); err != nil {
		return err
	}
	s.deps.Publish("workspace:templates", Ref{})
	return nil
}

// copyDir copies a directory tree byte-for-byte. Symlinks are skipped —
// consistent with the engine's never-follow rule.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}
