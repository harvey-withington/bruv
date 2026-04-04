package repo

import (
	"testing"
)

func TestMoveProjectBasic(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("Brand A")
	r.CreateStream("brand-a", "Stream 1")
	r.CreateStream("brand-a", "Stream 2")
	r.CreateProject("brand-a", "stream-1", "My Project")

	moved, err := r.MoveProject("brand-a", "stream-1", "my-project", "brand-a", "stream-2")
	if err != nil {
		t.Fatalf("MoveProject: %v", err)
	}

	// Verify moved project references updated
	stream2, _ := r.GetStream("brand-a", "stream-2")
	if moved.StreamID != stream2.ID {
		t.Errorf("StreamID = %q, want %q", moved.StreamID, stream2.ID)
	}

	// Verify project no longer in source
	projects1, _ := r.ListProjects("brand-a", "stream-1")
	if len(projects1) != 0 {
		t.Errorf("expected 0 projects in stream-1, got %d", len(projects1))
	}

	// Verify project exists in destination
	projects2, _ := r.ListProjects("brand-a", "stream-2")
	if len(projects2) != 1 {
		t.Fatalf("expected 1 project in stream-2, got %d", len(projects2))
	}
}

func TestMoveProjectCrossBrand(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("Brand A")
	r.CreateBrand("Brand B")
	r.CreateStream("brand-a", "Stream 1")
	r.CreateStream("brand-b", "Stream 1")
	r.CreateProject("brand-a", "stream-1", "My Project")

	moved, err := r.MoveProject("brand-a", "stream-1", "my-project", "brand-b", "stream-1")
	if err != nil {
		t.Fatalf("MoveProject cross-brand: %v", err)
	}

	brandB, _ := r.GetBrand("brand-b")
	if moved.BrandID != brandB.ID {
		t.Errorf("BrandID = %q, want %q", moved.BrandID, brandB.ID)
	}
}

func TestMoveProjectSourceNotFound(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("Brand A")
	r.CreateStream("brand-a", "Stream 1")
	r.CreateStream("brand-a", "Stream 2")

	_, err := r.MoveProject("brand-a", "stream-1", "nonexistent", "brand-a", "stream-2")
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
}

func TestMoveProjectDestinationConflict(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("Brand A")
	r.CreateStream("brand-a", "Stream 1")
	r.CreateStream("brand-a", "Stream 2")
	r.CreateProject("brand-a", "stream-1", "My Project")
	r.CreateProject("brand-a", "stream-2", "My Project")

	_, err := r.MoveProject("brand-a", "stream-1", "my-project", "brand-a", "stream-2")
	if err == nil {
		t.Fatal("expected error for destination conflict")
	}
}

func TestMoveStreamBasic(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("Brand A")
	r.CreateBrand("Brand B")
	r.CreateStream("brand-a", "Stream 1")
	r.CreateProject("brand-a", "stream-1", "Project X")

	moved, err := r.MoveStream("brand-a", "stream-1", "brand-b")
	if err != nil {
		t.Fatalf("MoveStream: %v", err)
	}

	brandB, _ := r.GetBrand("brand-b")
	if moved.BrandID != brandB.ID {
		t.Errorf("BrandID = %q, want %q", moved.BrandID, brandB.ID)
	}

	// Verify stream gone from source
	streams1, _ := r.ListStreams("brand-a")
	if len(streams1) != 0 {
		t.Errorf("expected 0 streams in brand-a, got %d", len(streams1))
	}

	// Verify stream exists in destination
	streams2, _ := r.ListStreams("brand-b")
	if len(streams2) != 1 {
		t.Fatalf("expected 1 stream in brand-b, got %d", len(streams2))
	}

	// Verify child projects have updated BrandID
	projects, _ := r.ListProjects("brand-b", "stream-1")
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].BrandID != brandB.ID {
		t.Errorf("child project BrandID = %q, want %q", projects[0].BrandID, brandB.ID)
	}
}

func TestMoveStreamSourceNotFound(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("Brand A")
	r.CreateBrand("Brand B")

	_, err := r.MoveStream("brand-a", "nonexistent", "brand-b")
	if err == nil {
		t.Fatal("expected error for nonexistent source stream")
	}
}

func TestMoveStreamDestinationConflict(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("Brand A")
	r.CreateBrand("Brand B")
	r.CreateStream("brand-a", "Stream 1")
	r.CreateStream("brand-b", "Stream 1")

	_, err := r.MoveStream("brand-a", "stream-1", "brand-b")
	if err == nil {
		t.Fatal("expected error for destination conflict")
	}
}
