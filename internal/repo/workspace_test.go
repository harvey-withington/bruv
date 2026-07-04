package repo

import (
	"bruv/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func newWorkspaceTestRepo(t *testing.T) (*Repository, string, string, string) {
	t.Helper()
	r, err := InitAt(filepath.Join(t.TempDir(), "vault"), "Test Vault")
	if err != nil {
		t.Fatal(err)
	}
	brand, err := r.CreateBrand("Acme")
	if err != nil {
		t.Fatal(err)
	}
	stream, err := r.CreateStream(brand.Slug, "Videos")
	if err != nil {
		t.Fatal(err)
	}
	project, err := r.CreateProject(brand.Slug, stream.Slug, "Song Alpha")
	if err != nil {
		t.Fatal(err)
	}
	return r, brand.Slug, stream.Slug, project.Slug
}

func TestWorkspaceRoundTrip(t *testing.T) {
	r, b, s, p := newWorkspaceTestRepo(t)

	if r.HasWorkspace(b, s, p) {
		t.Fatal("fresh project must have no workspace")
	}
	if _, err := r.GetWorkspace(b, s, p); err == nil {
		t.Fatal("GetWorkspace on fresh project must error")
	}

	ws := &model.Workspace{
		ID:      "ws-1",
		Origin:  model.WorkspaceOrigin{Kind: model.OriginLocal, URL: `D:\work\song-alpha`},
		Adapter: "plain-folder",
	}
	if err := r.SaveWorkspace(b, s, p, ws); err != nil {
		t.Fatal(err)
	}
	if ws.CreatedAt.IsZero() || ws.UpdatedAt.IsZero() {
		t.Error("SaveWorkspace must stamp timestamps")
	}

	got, err := r.GetWorkspace(b, s, p)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "ws-1" || got.Origin.Kind != model.OriginLocal || got.Adapter != "plain-folder" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.Claim != nil {
		t.Error("claim must be absent, not zero-valued")
	}

	// Index round-trip requires the workspace to exist first.
	idx := &model.WorkspaceIndex{
		WorkspaceID: "ws-1",
		Adapter:     "plain-folder",
		Summary:     "3 files",
		Tree:        []model.WorkspaceEntry{{Path: "a.md", Size: 12}},
	}
	if err := r.SaveWorkspaceIndex(b, s, p, idx); err != nil {
		t.Fatal(err)
	}
	gotIdx, err := r.GetWorkspaceIndex(b, s, p)
	if err != nil {
		t.Fatal(err)
	}
	if gotIdx.Summary != "3 files" || len(gotIdx.Tree) != 1 {
		t.Errorf("index round-trip mismatch: %+v", gotIdx)
	}
}

func TestSaveWorkspaceRequiresProject(t *testing.T) {
	r, b, s, _ := newWorkspaceTestRepo(t)
	err := r.SaveWorkspace(b, s, "no-such-project", &model.Workspace{ID: "x"})
	if err == nil {
		t.Fatal("SaveWorkspace must fail for a missing project")
	}
}

func TestSaveWorkspaceIndexRequiresWorkspace(t *testing.T) {
	r, b, s, p := newWorkspaceTestRepo(t)
	err := r.SaveWorkspaceIndex(b, s, p, &model.WorkspaceIndex{WorkspaceID: "x"})
	if err == nil {
		t.Fatal("SaveWorkspaceIndex must fail when no workspace is attached")
	}
}

func TestDeleteWorkspace(t *testing.T) {
	r, b, s, p := newWorkspaceTestRepo(t)
	ws := &model.Workspace{ID: "ws-del", Origin: model.WorkspaceOrigin{Kind: model.OriginLocal}}
	if err := r.SaveWorkspace(b, s, p, ws); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteWorkspace(b, s, p); err != nil {
		t.Fatal(err)
	}
	if r.HasWorkspace(b, s, p) {
		t.Error("workspace must be gone after delete")
	}
	if _, err := os.Stat(r.workspacePath(b, s, p)); !os.IsNotExist(err) {
		t.Error("workspace/ directory must be removed")
	}
	if err := r.DeleteWorkspace(b, s, p); err == nil {
		t.Error("double delete must error")
	}
}

func TestListWorkspaces(t *testing.T) {
	r, b, s, p := newWorkspaceTestRepo(t)

	refs, err := r.ListWorkspaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("no workspaces yet, got %d", len(refs))
	}

	if err := r.SaveWorkspace(b, s, p, &model.Workspace{
		ID: "ws-list", Origin: model.WorkspaceOrigin{Kind: model.OriginLocal},
	}); err != nil {
		t.Fatal(err)
	}
	// A second project without a workspace must not appear.
	if _, err := r.CreateProject(b, s, "No Workspace Here"); err != nil {
		t.Fatal(err)
	}

	refs, err = r.ListWorkspaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("want 1 workspace, got %d", len(refs))
	}
	ref := refs[0]
	if ref.BrandSlug != b || ref.StreamSlug != s || ref.ProjectSlug != p || ref.Workspace.ID != "ws-list" {
		t.Errorf("ref mismatch: %+v", ref)
	}
}
