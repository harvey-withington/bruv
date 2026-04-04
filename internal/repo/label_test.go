package repo

import (
	"bruv/internal/model"
	"testing"
)

// helper to set up a repo with a brand/stream/project for label tests
func setupLabelTestRepo(t *testing.T) (*Repository, string, string, string) {
	t.Helper()
	r := setupTestRepo(t)
	r.CreateBrand("Brand")
	r.CreateStream("brand", "Stream")
	r.CreateProject("brand", "stream", "Project")
	return r, "brand", "stream", "project"
}

func TestGetProjectLabelsEmpty(t *testing.T) {
	r, b, s, p := setupLabelTestRepo(t)
	labels, err := r.GetProjectLabels(b, s, p)
	if err != nil {
		t.Fatalf("GetProjectLabels: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("expected empty labels, got %d", len(labels))
	}
}

func TestAddProjectLabelAutoColor(t *testing.T) {
	r, b, s, p := setupLabelTestRepo(t)
	labels, err := r.AddProjectLabel(b, s, p, "Bug", "")
	if err != nil {
		t.Fatalf("AddProjectLabel: %v", err)
	}
	if len(labels) != 1 {
		t.Fatalf("expected 1 label, got %d", len(labels))
	}
	if labels[0].Name != "Bug" {
		t.Errorf("name = %q, want %q", labels[0].Name, "Bug")
	}
	if labels[0].Color == "" {
		t.Error("expected auto-assigned color, got empty")
	}
	// First auto-assigned should be first palette color
	if labels[0].Color != TagPalette[0] {
		t.Errorf("color = %q, want %q (first palette)", labels[0].Color, TagPalette[0])
	}
	if labels[0].ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestAddProjectLabelExplicitColor(t *testing.T) {
	r, b, s, p := setupLabelTestRepo(t)
	labels, err := r.AddProjectLabel(b, s, p, "Feature", "#ff0000")
	if err != nil {
		t.Fatalf("AddProjectLabel: %v", err)
	}
	if labels[0].Color != "#ff0000" {
		t.Errorf("color = %q, want %q", labels[0].Color, "#ff0000")
	}
}

func TestAddMultipleLabelsAutoColorsAreDistinct(t *testing.T) {
	r, b, s, p := setupLabelTestRepo(t)
	r.AddProjectLabel(b, s, p, "First", "")
	labels, _ := r.AddProjectLabel(b, s, p, "Second", "")

	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(labels))
	}
	if labels[0].Color == labels[1].Color {
		t.Errorf("expected different colors, both got %q", labels[0].Color)
	}
}

func TestRemoveProjectLabel(t *testing.T) {
	r, b, s, p := setupLabelTestRepo(t)
	labels, _ := r.AddProjectLabel(b, s, p, "Bug", "")
	labelID := labels[0].ID

	remaining, err := r.RemoveProjectLabel(b, s, p, labelID)
	if err != nil {
		t.Fatalf("RemoveProjectLabel: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected 0 labels after remove, got %d", len(remaining))
	}
}

func TestRemoveProjectLabelNotFound(t *testing.T) {
	r, b, s, p := setupLabelTestRepo(t)
	_, err := r.RemoveProjectLabel(b, s, p, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error removing nonexistent label")
	}
}

func TestUpdateProjectLabel(t *testing.T) {
	r, b, s, p := setupLabelTestRepo(t)
	labels, _ := r.AddProjectLabel(b, s, p, "Bug", "#ff0000")
	labelID := labels[0].ID

	updated, err := r.UpdateProjectLabel(b, s, p, labelID, "Critical Bug", "#00ff00")
	if err != nil {
		t.Fatalf("UpdateProjectLabel: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected 1 label, got %d", len(updated))
	}
	if updated[0].Name != "Critical Bug" {
		t.Errorf("name = %q, want %q", updated[0].Name, "Critical Bug")
	}
	if updated[0].Color != "#00ff00" {
		t.Errorf("color = %q, want %q", updated[0].Color, "#00ff00")
	}
}

func TestUpdateProjectLabelPartialUpdate(t *testing.T) {
	r, b, s, p := setupLabelTestRepo(t)
	labels, _ := r.AddProjectLabel(b, s, p, "Bug", "#ff0000")
	labelID := labels[0].ID

	// Update only name (empty color = keep existing)
	updated, err := r.UpdateProjectLabel(b, s, p, labelID, "Important Bug", "")
	if err != nil {
		t.Fatalf("UpdateProjectLabel: %v", err)
	}
	if updated[0].Name != "Important Bug" {
		t.Errorf("name = %q, want %q", updated[0].Name, "Important Bug")
	}
	if updated[0].Color != "#ff0000" {
		t.Errorf("color should be unchanged: %q, want %q", updated[0].Color, "#ff0000")
	}
}

func TestUpdateProjectLabelNotFound(t *testing.T) {
	r, b, s, p := setupLabelTestRepo(t)
	_, err := r.UpdateProjectLabel(b, s, p, "nonexistent-id", "New Name", "#000")
	if err == nil {
		t.Fatal("expected error updating nonexistent label")
	}
}

func TestAssignLabelColorPalette(t *testing.T) {
	// No existing labels — should return first palette color
	result := assignLabelColor(nil)
	if result != TagPalette[0] {
		t.Errorf("expected %q, got %q", TagPalette[0], result)
	}
}

func TestAssignLabelColorSkipsUsed(t *testing.T) {
	labels := []model.Label{
		{ID: "1", Name: "A", Color: TagPalette[0]},
	}
	result := assignLabelColor(labels)
	if result != TagPalette[1] {
		t.Errorf("expected %q, got %q", TagPalette[1], result)
	}
}

func TestAssignLabelColorAllUsedReturnsLeastUsed(t *testing.T) {
	// Use all palette colors, but use first one twice
	labels := make([]model.Label, 0, len(TagPalette)+1)
	for i, c := range TagPalette {
		labels = append(labels, model.Label{ID: string(rune('a' + i)), Name: "L", Color: c})
	}
	labels = append(labels, model.Label{ID: "extra", Name: "Extra", Color: TagPalette[0]})

	result := assignLabelColor(labels)
	// All used once except TagPalette[0] used twice, so any except [0] should be returned
	if result == TagPalette[0] {
		t.Error("should return a least-used color, not the most-used one")
	}
}

func TestLabelsPersistAcrossLoads(t *testing.T) {
	r, b, s, p := setupLabelTestRepo(t)
	r.AddProjectLabel(b, s, p, "Alpha", "#aaa")
	r.AddProjectLabel(b, s, p, "Beta", "#bbb")

	// Re-load from disk
	labels, err := r.GetProjectLabels(b, s, p)
	if err != nil {
		t.Fatalf("GetProjectLabels: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(labels))
	}
	if labels[0].Name != "Alpha" || labels[1].Name != "Beta" {
		t.Errorf("unexpected label names: %q, %q", labels[0].Name, labels[1].Name)
	}
}
