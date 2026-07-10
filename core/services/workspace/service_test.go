package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bruv/core/services/card"
	"bruv/internal/index"
	"bruv/internal/model"
	"bruv/internal/repo"

	ft "github.com/harvey-withington/foldertemplate"
)

type testDeps struct {
	r       *repo.Repository
	cardSvc *card.Service
	topics  []string
}

func (d *testDeps) Repo() *repo.Repository      { return d.r }
func (d *testDeps) Publish(topic string, _ any) { d.topics = append(d.topics, topic) }
func (d *testDeps) Card() *card.Service         { return d.cardSvc }

// cardTestDeps satisfies card.Deps with no-op instrumentation, sharing the
// repo + topic sink with the workspace testDeps.
type cardTestDeps struct{ d *testDeps }

func (c cardTestDeps) Repo() *repo.Repository                                          { return c.d.r }
func (c cardTestDeps) Index() *index.Index                                             { return nil }
func (c cardTestDeps) ApplyTypeBlocks(_, _ string)                                     {}
func (c cardTestDeps) LogActivity(_, _, _ string)                                      {}
func (c cardTestDeps) LogActivityWithContext(_, _, _, _ string, _ []card.CategoryPath) {}
func (c cardTestDeps) Publish(topic string, _ any)                                     { c.d.topics = append(c.d.topics, topic) }
func (d *testDeps) emitted(topic string) bool {
	for _, t := range d.topics {
		if t == topic {
			return true
		}
	}
	return false
}

func newTestService(t *testing.T) (*Service, *testDeps, string, string, string) {
	t.Helper()
	r, err := repo.InitAt(filepath.Join(t.TempDir(), "vault"), "Vault")
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.CreateBrand("Acme")
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.CreateStream(b.Slug, "Films")
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.CreateProject(b.Slug, st.Slug, "Big Movie")
	if err != nil {
		t.Fatal(err)
	}
	deps := &testDeps{r: r}
	deps.cardSvc = card.New(cardTestDeps{d: deps})
	return New(deps), deps, b.Slug, st.Slug, p.Slug
}

func writeFiles(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	for rel, content := range files {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if strings.HasSuffix(rel, "/") {
			if err := os.MkdirAll(abs, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAttachDetectsPlainAndHonoursBruvignore(t *testing.T) {
	svc, deps, b, st, p := newTestService(t)
	dir := writeFiles(t, t.TempDir(), map[string]string{
		"README.md":      "# Song Alpha\nA concept album.",
		"mix/track1.wav": "xxx",
		"render/big.tmp": "ignored",
		".bruvignore":    "render/\n",
	})

	ws, err := svc.Attach(context.Background(), b, st, p, dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Adapter != "plain-folder" {
		t.Errorf("adapter = %q", ws.Adapter)
	}
	if ws.Origin.Kind != model.OriginLocal {
		t.Errorf("origin kind = %q", ws.Origin.Kind)
	}
	if !deps.emitted("workspace:updated") {
		t.Error("attach must publish workspace:updated")
	}

	idx, err := svc.GetIndex(b, st, p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(idx.Summary, "A concept album.") {
		t.Errorf("summary should quote README: %q", idx.Summary)
	}
	for _, e := range idx.Tree {
		if strings.HasPrefix(e.Path, "render/") {
			t.Errorf(".bruvignore'd path indexed: %s", e.Path)
		}
	}
}

func TestAdapterDetectionPrecedence(t *testing.T) {
	svc, _, b, st, p := newTestService(t)

	gitDir := writeFiles(t, t.TempDir(), map[string]string{".git/": "", "main.go": "package x"})
	ws, err := svc.Attach(context.Background(), b, st, p, gitDir)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Adapter != "git-repo" {
		t.Errorf(".git dir should detect git-repo, got %q", ws.Adapter)
	}
	if err := svc.Detach(b, st, p); err != nil {
		t.Fatal(err)
	}

	// A vault inside a repo is still primarily a vault.
	bothDir := writeFiles(t, t.TempDir(), map[string]string{
		".git/": "", ".obsidian/": "", "note.md": "#idea and #film/noir",
	})
	ws, err = svc.Attach(context.Background(), b, st, p, bothDir)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Adapter != "obsidian-vault" {
		t.Errorf(".obsidian must outrank .git, got %q", ws.Adapter)
	}
	idx, err := svc.GetIndex(b, st, p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(idx.Details["tags"], "film/noir") {
		t.Errorf("tags = %q, want inline tags found", idx.Details["tags"])
	}
	for _, e := range idx.Tree {
		if strings.HasPrefix(e.Path, ".obsidian") || strings.HasPrefix(e.Path, ".git") {
			t.Errorf("state dir leaked into index tree: %s", e.Path)
		}
	}
}

func TestAttachTwiceFails(t *testing.T) {
	svc, _, b, st, p := newTestService(t)
	dir := writeFiles(t, t.TempDir(), map[string]string{"a.md": "x"})
	if _, err := svc.Attach(context.Background(), b, st, p, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Attach(context.Background(), b, st, p, dir); err == nil {
		t.Fatal("0-or-1 workspaces per project: second attach must fail")
	}
}

func TestReadWriteFile(t *testing.T) {
	svc, deps, b, st, p := newTestService(t)
	dir := writeFiles(t, t.TempDir(), map[string]string{"notes/draft.md": "v1"})
	if _, err := svc.Attach(context.Background(), b, st, p, dir); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	got, err := svc.ReadFile(ctx, b, st, p, "notes/draft.md")
	if err != nil || got != "v1" {
		t.Fatalf("ReadFile = %q, %v", got, err)
	}
	if err := svc.WriteFile(ctx, b, st, p, "notes/draft.md", "v2 — edited in BRUV"); err != nil {
		t.Fatal(err)
	}
	if got, _ = svc.ReadFile(ctx, b, st, p, "notes/draft.md"); got != "v2 — edited in BRUV" {
		t.Errorf("after write: %q", got)
	}
	if !deps.emitted("workspace:updated") {
		t.Error("write must publish workspace:updated")
	}

	if _, err := svc.ReadFile(ctx, b, st, p, "../outside.txt"); err == nil {
		t.Error("escape must be rejected")
	}
	if err := svc.WriteFile(ctx, b, st, p, "../evil.txt", "x"); err == nil {
		t.Error("escape write must be rejected")
	}

	// Binary content is refused — binaries open externally.
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), []byte{0xFF, 0xFE, 0x00, 0x01}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReadFile(ctx, b, st, p, "blob.bin"); err == nil {
		t.Error("binary read must be refused")
	}
}

// Regression: with zero templates the list must be an EMPTY slice, not nil —
// nil marshals to JSON null, which the dialog treats as "still loading"
// (permanent spinner). Same for parameter lists on paramless templates.
func TestListTemplatesEmptyIsNotNil(t *testing.T) {
	svc, _, _, _, _ := newTestService(t)
	entries, err := svc.ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if entries == nil {
		t.Fatal("ListTemplates must return an empty slice, not nil (JSON null)")
	}
	if raw, _ := json.Marshal(entries); string(raw) != "[]" {
		t.Fatalf("empty template list marshals as %s, want []", raw)
	}
}

func TestGenerateFromTemplateAttaches(t *testing.T) {
	svc, deps, b, st, p := newTestService(t)
	r := deps.r

	// Author a template directly into the vault's global templates dir.
	tplDir := filepath.Join(r.Root, "templates", "{title} film")
	prompt := "Title?"
	if err := ft.Save(&ft.Template{
		Name: "Film Project",
		Parameters: []ft.Parameter{{
			Name: "title", Type: "text", Prompt: &prompt,
			ReplaceInFileNames: true, ReplaceInFiles: true,
		}},
	}, tplDir); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, tplDir, map[string]string{
		"brief.md.ft$": "# {{$title}}\nProject: {{$bruvProject}}\nDate: {{$bruvDate}}",
	})

	entries, err := svc.ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "Film Project" || entries[0].Scope != "global" {
		t.Fatalf("ListTemplates = %+v", entries)
	}

	target := t.TempDir()
	ws, err := svc.GenerateFromTemplate(context.Background(), b, st, p,
		entries[0].ID, target, map[string]string{"title": "Neon Nights"})
	if err != nil {
		t.Fatal(err)
	}
	if ws.Origin.Kind != model.OriginLocal {
		t.Errorf("generated workspace origin = %q", ws.Origin.Kind)
	}
	if filepath.Base(ws.Origin.URL) != "Neon Nights film" {
		t.Errorf("root = %q", ws.Origin.URL)
	}
	brief, err := svc.ReadFile(context.Background(), b, st, p, "brief.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(brief, "# Neon Nights") || !strings.Contains(brief, "Project: Big Movie") {
		t.Errorf("built-in params not applied:\n%s", brief)
	}
}

// The Bad Therapist flow: a template living INSIDE the workspace generates
// an episode folder bound to a card (plan/2026-07-05 card folders design.md).
func TestCardFolderLifecycle(t *testing.T) {
	svc, deps, b, st, p := newTestService(t)

	// Workspace = the "show" folder, with the episode template inside it.
	showDir := writeFiles(t, t.TempDir(), map[string]string{"Planning/notes.md": "x", "Episodes/": ""})
	prompt := "Episode Number"
	if err := ft.Save(&ft.Template{
		Name:              "Episode",
		DefaultTargetPath: "Episodes", // relative → resolves against the workspace root
		Parameters: []ft.Parameter{{
			Name: "epNum", Type: "text", Prompt: &prompt,
			ReplaceInFileNames: true, ReplaceInFiles: true,
		}},
	}, filepath.Join(showDir, "_Template - EP{epNum} - {bruvCard}")); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, filepath.Join(showDir, "_Template - EP{epNum} - {bruvCard}"), map[string]string{
		"Drafts/EP{epNum}.fountain.ft$": "Title: {{$bruvCard}}\nEp: {{$epNum}}",
	})
	// Strip the template prefix on generation, as the real template does.
	stripMatch := "^_Template - "
	tplDir := filepath.Join(showDir, "_Template - EP{epNum} - {bruvCard}")
	tpl, err := ft.Load(tplDir)
	if err != nil {
		t.Fatal(err)
	}
	tpl.Parameters = append(tpl.Parameters, ft.Parameter{Match: &stripMatch, ReplaceInFileNames: true})
	if err := ft.Save(tpl, tplDir); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Attach(context.Background(), b, st, p, showDir); err != nil {
		t.Fatal(err)
	}
	epCard, err := deps.cardSvc.Create("", "Patient Zero")
	if err != nil {
		t.Fatal(err)
	}

	// Workspace-resident template is discovered, listed before vault scopes.
	entries, err := svc.ListProjectTemplates(b, st, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 || entries[0].Scope != "workspace" || entries[0].Name != "Episode" {
		t.Fatalf("workspace template not first: %+v", entries)
	}

	// Blank target → the template's own DefaultTargetPath ("Episodes",
	// resolved against the template folder's parent — the show root).
	card, err := svc.GenerateCardFolder(context.Background(), b, st, p, epCard.ID,
		entries[0].ID, "", map[string]string{"epNum": "002"})
	if err != nil {
		t.Fatal(err)
	}
	if card.Folder == nil || card.Folder.Path != "Episodes/EP002 - Patient Zero" {
		t.Fatalf("folder binding = %+v", card.Folder)
	}
	script, err := os.ReadFile(filepath.Join(showDir, "Episodes", "EP002 - Patient Zero", "Drafts", "EP002.fountain"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), "Title: Patient Zero") {
		t.Errorf("bruvCard not applied in content:\n%s", script)
	}

	// Escape attempts and double-binding are refused.
	if _, err := svc.GenerateCardFolder(context.Background(), b, st, p, epCard.ID, entries[0].ID, "Episodes", nil); err == nil {
		t.Error("second generate on a bound card must fail")
	}
	unbound, err := svc.ClearCardFolder(epCard.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unbound.Folder != nil {
		t.Error("unbind must clear the binding")
	}
	if _, err := os.Stat(filepath.Join(showDir, "Episodes", "EP002 - Patient Zero")); err != nil {
		t.Error("unbind must never delete the folder on disk")
	}
	if _, err := svc.GenerateCardFolder(context.Background(), b, st, p, epCard.ID, entries[0].ID, "../outside", nil); err == nil {
		t.Error("target escape must be rejected")
	}

	// Re-link the existing folder (the unlink-then-relink path).
	relinked, err := svc.LinkCardFolder(b, st, p, epCard.ID, "Episodes/EP002 - Patient Zero")
	if err != nil {
		t.Fatal(err)
	}
	if relinked.Folder == nil || relinked.Folder.Path != "Episodes/EP002 - Patient Zero" {
		t.Fatalf("relink = %+v", relinked.Folder)
	}
	if _, err := svc.LinkCardFolder(b, st, p, epCard.ID, "Episodes"); err == nil {
		t.Error("linking while bound must fail")
	}
	if _, err := svc.ClearCardFolder(epCard.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LinkCardFolder(b, st, p, epCard.ID, "Episodes/nope"); err == nil {
		t.Error("linking a nonexistent folder must fail")
	}
	if _, err := svc.LinkCardFolder(b, st, p, epCard.ID, "../escape"); err == nil {
		t.Error("link escape must be rejected")
	}
}

func TestImportTemplateFromFolder(t *testing.T) {
	svc, deps, b, _, _ := newTestService(t)

	src := t.TempDir()
	tplDir := filepath.Join(src, "yt-video")
	if err := ft.Save(&ft.Template{Name: "YT"}, tplDir); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, tplDir, map[string]string{"plan.md": "x"})

	insp, err := svc.InspectTemplateFolder(tplDir)
	if err != nil {
		t.Fatal(err)
	}
	if !insp.IsTemplate || insp.Name != "YT" || insp.LargeWarning {
		t.Errorf("inspection = %+v", insp)
	}

	entry, err := svc.ImportTemplateFromFolder(tplDir, b)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Scope != b {
		t.Errorf("scope = %q", entry.Scope)
	}
	// The .ft/ directory must travel with the import.
	if _, err := os.Stat(filepath.Join(deps.r.Root, "brands", b, "templates", "yt-video", ".ft", "template.json")); err != nil {
		t.Errorf(".ft did not travel: %v", err)
	}
	if _, err := svc.ImportTemplateFromFolder(tplDir, b); err == nil {
		t.Error("name collision must be refused")
	}

	entries, err := svc.ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Scope != b {
		t.Fatalf("ListTemplates after import = %+v", entries)
	}
}

func TestDeleteTemplate(t *testing.T) {
	svc, deps, b, _, _ := newTestService(t)

	src := t.TempDir()
	tplDir := filepath.Join(src, "yt-video")
	if err := ft.Save(&ft.Template{Name: "YT"}, tplDir); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, tplDir, map[string]string{"plan.md": "x"})
	entry, err := svc.ImportTemplateFromFolder(tplDir, b)
	if err != nil {
		t.Fatal(err)
	}

	// Guards: absolute refs, escapes, non-template roots, and plain
	// vault folders outside a templates root must all be refused.
	if err := svc.DeleteTemplate(tplDir); err == nil {
		t.Error("absolute ref must be refused")
	}
	if err := svc.DeleteTemplate("templates/../cards"); err == nil {
		t.Error("escape ref must be refused")
	}
	if err := svc.DeleteTemplate("brands/" + b); err == nil {
		t.Error("non-template vault folder must be refused")
	}
	if err := svc.DeleteTemplate("brands/" + b + "/templates/missing"); err == nil {
		t.Error("nonexistent template must be refused")
	}

	deps.topics = nil
	if err := svc.DeleteTemplate(entry.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(deps.r.Root, "brands", b, "templates", "yt-video")); !os.IsNotExist(err) {
		t.Errorf("template folder still on disk: %v", err)
	}
	if !deps.emitted("workspace:templates") {
		t.Error("delete must publish workspace:templates")
	}
	entries, err := svc.ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("ListTemplates after delete = %+v", entries)
	}
}
