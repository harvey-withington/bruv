package repo

import (
	"testing"
)

// helper to create a full hierarchy: brand > stream > project > category
func setupCopyTestRepo(t *testing.T) *Repository {
	t.Helper()
	r := setupTestRepo(t)
	r.CreateBrand("Brand A")
	r.CreateStream("brand-a", "Stream 1")
	r.CreateProject("brand-a", "stream-1", "Project X")
	r.CreateCategory("brand-a", "stream-1", "project-x", "Backlog", 0)
	r.CreateCategory("brand-a", "stream-1", "project-x", "Done", 1)
	return r
}

func TestCopyProjectBasic(t *testing.T) {
	r := setupCopyTestRepo(t)
	copied, err := r.CopyProject("brand-a", "stream-1", "project-x", "brand-a", "stream-1", -1)
	if err != nil {
		t.Fatalf("CopyProject: %v", err)
	}
	if copied.Name != "Project X Copy" {
		t.Errorf("name = %q, want %q", copied.Name, "Project X Copy")
	}
	if copied.Slug != "project-x-copy" {
		t.Errorf("slug = %q, want %q", copied.Slug, "project-x-copy")
	}

	// Verify copy has new ID
	original, _ := r.GetProject("brand-a", "stream-1", "project-x")
	if copied.ID == original.ID {
		t.Error("copied project should have a new ID")
	}

	// Verify categories were copied with new IDs
	copiedCats, _ := r.ListCategories("brand-a", "stream-1", "project-x-copy")
	originalCats, _ := r.ListCategories("brand-a", "stream-1", "project-x")
	if len(copiedCats) != len(originalCats) {
		t.Fatalf("expected %d categories, got %d", len(originalCats), len(copiedCats))
	}
	for i := range copiedCats {
		if copiedCats[i].ID == originalCats[i].ID {
			t.Errorf("category %d should have new ID", i)
		}
	}
}

func TestCopyProjectSourceUnmodified(t *testing.T) {
	r := setupCopyTestRepo(t)
	originalProject, _ := r.GetProject("brand-a", "stream-1", "project-x")
	originalID := originalProject.ID
	originalName := originalProject.Name

	r.CopyProject("brand-a", "stream-1", "project-x", "brand-a", "stream-1", -1)

	// Verify source is unchanged
	after, _ := r.GetProject("brand-a", "stream-1", "project-x")
	if after.ID != originalID {
		t.Errorf("source ID changed: %q -> %q", originalID, after.ID)
	}
	if after.Name != originalName {
		t.Errorf("source name changed: %q -> %q", originalName, after.Name)
	}
}

func TestCopyProjectNameCollision(t *testing.T) {
	r := setupCopyTestRepo(t)
	// First copy
	first, _ := r.CopyProject("brand-a", "stream-1", "project-x", "brand-a", "stream-1", -1)
	if first.Name != "Project X Copy" {
		t.Errorf("first copy name = %q, want %q", first.Name, "Project X Copy")
	}
	// Second copy — should get "Copy 2"
	second, err := r.CopyProject("brand-a", "stream-1", "project-x", "brand-a", "stream-1", -1)
	if err != nil {
		t.Fatalf("CopyProject second: %v", err)
	}
	if second.Name != "Project X Copy 2" {
		t.Errorf("second copy name = %q, want %q", second.Name, "Project X Copy 2")
	}
}

func TestCopyProjectCrossStream(t *testing.T) {
	r := setupCopyTestRepo(t)
	r.CreateStream("brand-a", "Stream 2")

	copied, err := r.CopyProject("brand-a", "stream-1", "project-x", "brand-a", "stream-2", 0)
	if err != nil {
		t.Fatalf("CopyProject cross-stream: %v", err)
	}

	// Verify it's in the destination stream
	projects, _ := r.ListProjects("brand-a", "stream-2")
	if len(projects) != 1 {
		t.Fatalf("expected 1 project in stream-2, got %d", len(projects))
	}
	if projects[0].ID != copied.ID {
		t.Error("project in destination stream should match copy")
	}
}

func TestCopyStreamBasic(t *testing.T) {
	r := setupCopyTestRepo(t)
	r.CreateBrand("Brand B")

	copied, err := r.CopyStream("brand-a", "stream-1", "brand-b")
	if err != nil {
		t.Fatalf("CopyStream: %v", err)
	}
	if copied.Name != "Stream 1 Copy" {
		t.Errorf("name = %q, want %q", copied.Name, "Stream 1 Copy")
	}

	// Verify child projects were copied with new IDs
	projects, _ := r.ListProjects("brand-b", "stream-1-copy")
	originalProjects, _ := r.ListProjects("brand-a", "stream-1")
	if len(projects) != len(originalProjects) {
		t.Fatalf("expected %d projects, got %d", len(originalProjects), len(projects))
	}
	for i := range projects {
		if projects[i].ID == originalProjects[i].ID {
			t.Errorf("child project %d should have new ID", i)
		}
	}
}

func TestCopyBrandBasic(t *testing.T) {
	r := setupCopyTestRepo(t)

	copied, err := r.CopyBrand("brand-a")
	if err != nil {
		t.Fatalf("CopyBrand: %v", err)
	}
	if copied.Name != "Brand A Copy" {
		t.Errorf("name = %q, want %q", copied.Name, "Brand A Copy")
	}

	// Verify deep hierarchy copied
	streams, _ := r.ListStreams("brand-a-copy")
	if len(streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(streams))
	}

	projects, _ := r.ListProjects("brand-a-copy", streams[0].Slug)
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}

	cats, _ := r.ListCategories("brand-a-copy", streams[0].Slug, projects[0].Slug)
	if len(cats) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(cats))
	}

	// Verify all IDs are new
	originalBrand, _ := r.GetBrand("brand-a")
	if copied.ID == originalBrand.ID {
		t.Error("copied brand should have new ID")
	}
}

func TestCopyProjectNonexistentSource(t *testing.T) {
	r := setupCopyTestRepo(t)
	_, err := r.CopyProject("brand-a", "stream-1", "nonexistent", "brand-a", "stream-1", -1)
	if err == nil {
		t.Fatal("expected error copying nonexistent project")
	}
}
