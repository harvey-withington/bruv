package repo

import (
	"testing"
)

func TestListAllCategoriesFlat_EmptyRepo(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir, "test")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	flat, err := r.ListAllCategoriesFlat()
	if err != nil {
		t.Fatalf("ListAllCategoriesFlat: %v", err)
	}
	if len(flat) != 0 {
		t.Errorf("expected 0 flat entries on empty repo, got %d", len(flat))
	}
}

func TestListAllCategoriesFlat_PopulatedRepo(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir, "test")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Build a small tree: 2 brands, 2 streams each, 2 projects each.
	// Each project auto-gets a default "Ideas" category in production,
	// but repo.CreateProject does not — we have to create categories
	// explicitly here.
	wantCount := 0
	for _, brandName := range []string{"Brand A", "Brand B"} {
		brand, err := r.CreateBrand(brandName)
		if err != nil {
			t.Fatalf("CreateBrand %q: %v", brandName, err)
		}
		for _, streamName := range []string{"Stream 1", "Stream 2"} {
			stream, err := r.CreateStream(brand.Slug, streamName)
			if err != nil {
				t.Fatalf("CreateStream: %v", err)
			}
			for _, projName := range []string{"Project X", "Project Y"} {
				proj, err := r.CreateProject(brand.Slug, stream.Slug, projName)
				if err != nil {
					t.Fatalf("CreateProject: %v", err)
				}
				// Three categories per project.
				for _, catName := range []string{"Inbox", "Doing", "Done"} {
					if _, err := r.CreateCategory(brand.Slug, stream.Slug, proj.Slug, catName, 0); err != nil {
						t.Fatalf("CreateCategory: %v", err)
					}
					wantCount++
				}
			}
		}
	}

	flat, err := r.ListAllCategoriesFlat()
	if err != nil {
		t.Fatalf("ListAllCategoriesFlat: %v", err)
	}
	if len(flat) != wantCount {
		t.Errorf("expected %d flat entries, got %d", wantCount, len(flat))
	}

	// Each entry should carry its full parent chain populated.
	for _, f := range flat {
		if f.Brand.Slug == "" {
			t.Errorf("entry missing Brand.Slug: %+v", f)
		}
		if f.Stream.Slug == "" {
			t.Errorf("entry missing Stream.Slug: %+v", f)
		}
		if f.Project.Slug == "" {
			t.Errorf("entry missing Project.Slug: %+v", f)
		}
		if f.Category.ID == "" {
			t.Errorf("entry missing Category.ID: %+v", f)
		}
	}
}
