package repo

import (
	"bruv/internal/model"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRevalidateCleanRepo(t *testing.T) {
	r := setupTestRepo(t)
	stats, err := r.Revalidate()
	if err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
	if stats.StalePinsRemoved != 0 || stats.OrphanedPinDirs != 0 || stats.OrphanedChatFiles != 0 {
		t.Errorf("clean repo should have zero stats, got %+v", stats)
	}
}

func TestRevalidateStatsString(t *testing.T) {
	s := RevalidateStats{}
	if got := s.String(); got != "nothing to repair" {
		t.Errorf("empty stats = %q, want %q", got, "nothing to repair")
	}

	s = RevalidateStats{StalePinsRemoved: 2, OrphanedChatFiles: 1}
	got := s.String()
	if got == "nothing to repair" {
		t.Error("non-empty stats should not say 'nothing to repair'")
	}
}

func TestRevalidateRemovesStalePins(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("Brand")
	r.CreateStream("brand", "Stream")
	r.CreateProject("brand", "stream", "Project")
	cat, _ := r.CreateCategory("brand", "stream", "project", "Backlog", 0)
	// Need a second category so collectAllCategoryIDs is non-empty after deleting "Backlog"
	r.CreateCategory("brand", "stream", "project", "Done", 1)

	card, _ := r.CreateCard("task", "Test Card")
	project, _ := r.GetProject("brand", "stream", "project")
	r.PinCard(card.ID, project.ID, cat.ID)

	// Verify pin exists
	pins, _ := r.GetCardPins(card.ID)
	if len(pins) != 1 {
		t.Fatalf("expected 1 pin, got %d", len(pins))
	}

	// Delete the category so the pin becomes stale
	r.DeleteCategory("brand", "stream", "project", "backlog")

	stats, err := r.Revalidate()
	if err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
	if stats.StalePinsRemoved != 1 {
		t.Errorf("StalePinsRemoved = %d, want 1", stats.StalePinsRemoved)
	}

	// Pin should be gone
	pins, _ = r.GetCardPins(card.ID)
	if len(pins) != 0 {
		t.Errorf("expected 0 pins after revalidate, got %d", len(pins))
	}
}

func TestRevalidateRemovesOrphanedPinDirs(t *testing.T) {
	r := setupTestRepo(t)

	// Create a pin directory for a card that doesn't exist
	orphanedPinDir := filepath.Join(r.Root, "pins", "nonexistent-card-id")
	os.MkdirAll(orphanedPinDir, 0755)

	// Write a dummy pin file so the dir isn't empty
	pinFile := &model.PinFile{
		CardID: "nonexistent-card-id",
		Pins: []model.Pin{
			{CardID: "nonexistent-card-id", ProjectID: "p1", CategoryID: "c1", PinnedAt: time.Now()},
		},
	}
	writeJSON(filepath.Join(orphanedPinDir, "pins.json"), pinFile)

	stats, err := r.Revalidate()
	if err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
	if stats.OrphanedPinDirs != 1 {
		t.Errorf("OrphanedPinDirs = %d, want 1", stats.OrphanedPinDirs)
	}

	// Directory should be cleaned up
	if fileExists(orphanedPinDir) {
		t.Error("orphaned pin directory should be removed")
	}
}

func TestRevalidateRemovesOrphanedChatFiles(t *testing.T) {
	r := setupTestRepo(t)

	// Create a chat file for a card that doesn't exist
	chatFile := filepath.Join(r.Root, "cards", "nonexistent-card.messages.json")
	writeJSON(chatFile, &model.ChatFile{
		CardID:   "nonexistent-card",
		Messages: []model.ChatMessage{{ID: "m1", Role: "user", Content: "hi"}},
	})

	stats, err := r.Revalidate()
	if err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
	if stats.OrphanedChatFiles != 1 {
		t.Errorf("OrphanedChatFiles = %d, want 1", stats.OrphanedChatFiles)
	}

	// Chat file should be cleaned up
	if fileExists(chatFile) {
		t.Error("orphaned chat file should be removed")
	}
}

// Project-level chat files use the synthetic prefix `__project__` and have no
// backing card on disk by design. The revalidator must NOT delete them.
func TestRevalidatePreservesProjectChatFiles(t *testing.T) {
	r := setupTestRepo(t)

	projectChatFile := filepath.Join(r.Root, "cards", "__project__some-project-id.messages.json")
	writeJSON(projectChatFile, &model.ChatFile{
		CardID:   "__project__some-project-id",
		Messages: []model.ChatMessage{{ID: "m1", Role: "user", Content: "test"}},
	})

	stats, err := r.Revalidate()
	if err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
	if stats.OrphanedChatFiles != 0 {
		t.Errorf("OrphanedChatFiles = %d, want 0 (project chat files must be preserved)", stats.OrphanedChatFiles)
	}
	if !fileExists(projectChatFile) {
		t.Error("project chat file must NOT be removed by revalidate")
	}
}

func TestRevalidatePreservesValidPins(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("Brand")
	r.CreateStream("brand", "Stream")
	r.CreateProject("brand", "stream", "Project")
	cat, _ := r.CreateCategory("brand", "stream", "project", "Backlog", 0)

	card, _ := r.CreateCard("task", "Test Card")
	project, _ := r.GetProject("brand", "stream", "project")
	r.PinCard(card.ID, project.ID, cat.ID)

	stats, _ := r.Revalidate()
	if stats.StalePinsRemoved != 0 {
		t.Errorf("should not remove valid pins, removed %d", stats.StalePinsRemoved)
	}

	// Pin should still exist
	pins, _ := r.GetCardPins(card.ID)
	if len(pins) != 1 {
		t.Errorf("expected 1 pin preserved, got %d", len(pins))
	}
}

func TestRevalidateMultipleIssues(t *testing.T) {
	r := setupTestRepo(t)

	// Create orphaned pin dir
	orphanedPinDir := filepath.Join(r.Root, "pins", "ghost-card-1")
	os.MkdirAll(orphanedPinDir, 0755)
	writeJSON(filepath.Join(orphanedPinDir, "pins.json"), &model.PinFile{
		CardID: "ghost-card-1",
		Pins:   []model.Pin{{CardID: "ghost-card-1", ProjectID: "p1", CategoryID: "c1"}},
	})

	// Create another orphaned pin dir
	orphanedPinDir2 := filepath.Join(r.Root, "pins", "ghost-card-2")
	os.MkdirAll(orphanedPinDir2, 0755)
	writeJSON(filepath.Join(orphanedPinDir2, "pins.json"), &model.PinFile{
		CardID: "ghost-card-2",
		Pins:   []model.Pin{{CardID: "ghost-card-2", ProjectID: "p2", CategoryID: "c2"}},
	})

	// Create orphaned chat file
	writeJSON(filepath.Join(r.Root, "cards", "ghost-card-3.messages.json"), &model.ChatFile{
		CardID: "ghost-card-3", Messages: []model.ChatMessage{},
	})

	stats, err := r.Revalidate()
	if err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
	if stats.OrphanedPinDirs != 2 {
		t.Errorf("OrphanedPinDirs = %d, want 2", stats.OrphanedPinDirs)
	}
	if stats.OrphanedChatFiles != 1 {
		t.Errorf("OrphanedChatFiles = %d, want 1", stats.OrphanedChatFiles)
	}
}
