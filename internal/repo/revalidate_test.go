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
	if stats.StalePinsRemoved != 0 || stats.OrphanedPinDirs != 0 || stats.OrphanedAgentFiles != 0 {
		t.Errorf("clean repo should have zero stats, got %+v", stats)
	}
}

func TestRevalidateStatsString(t *testing.T) {
	s := RevalidateStats{}
	if got := s.String(); got != "nothing to repair" {
		t.Errorf("empty stats = %q, want %q", got, "nothing to repair")
	}

	s = RevalidateStats{StalePinsRemoved: 2, OrphanedAgentFiles: 1}
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
	_, _ = r.GetProject("brand", "stream", "project")
	r.PinCard(card.ID, cat.ID)

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

	// Reported, never deleted: under Syncthing the pin can land before
	// the card file.
	if !fileExists(orphanedPinDir) {
		t.Error("pin directory without a card file must be kept")
	}
}

// A category file that fails to parse (torn write, partial sync) must
// not make its pins look stale — the whole stale-pin pass is skipped.
func TestRevalidateKeepsPinsWhenHierarchyUnreadable(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("Brand")
	r.CreateStream("brand", "Stream")
	r.CreateProject("brand", "stream", "Project")
	backlog, _ := r.CreateCategory("brand", "stream", "project", "Backlog", 0)
	r.CreateCategory("brand", "stream", "project", "Done", 1)

	card, _ := r.CreateCard("task", "Test Card")
	if err := r.PinCard(card.ID, backlog.ID); err != nil {
		t.Fatalf("PinCard: %v", err)
	}

	if err := os.WriteFile(r.categoryFilePath("brand", "stream", "project", "backlog"), []byte("{torn"), 0o644); err != nil {
		t.Fatal(err)
	}

	stats, err := r.Revalidate()
	if err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
	if stats.StalePinsRemoved != 0 || !stats.StalePinCheckSkipped {
		t.Errorf("stats = %+v, want no removals and the check skipped", stats)
	}
	if pins, _ := r.GetCardPins(card.ID); len(pins) != 1 {
		t.Errorf("expected pin kept, got %d pins", len(pins))
	}
}

// Chat files no longer live in the repo — they're stored in the OS
// config folder keyed by repo ID so they stay personal when the repo is
// shared. The revalidator therefore has nothing to clean up for chats;
// any leftover .messages.json files in an old repo are handled by the
// one-shot migration in OpenRepository, not by Revalidate.

func TestRevalidatePreservesValidPins(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("Brand")
	r.CreateStream("brand", "Stream")
	r.CreateProject("brand", "stream", "Project")
	cat, _ := r.CreateCategory("brand", "stream", "project", "Backlog", 0)

	card, _ := r.CreateCard("task", "Test Card")
	_, _ = r.GetProject("brand", "stream", "project")
	r.PinCard(card.ID, cat.ID)

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

	stats, err := r.Revalidate()
	if err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
	if stats.OrphanedPinDirs != 2 {
		t.Errorf("OrphanedPinDirs = %d, want 2", stats.OrphanedPinDirs)
	}
}

// An agent file whose card hasn't synced in yet is reported, never
// deleted (pre-release sweep 2026-09-29, §6.5).
func TestRevalidateKeepsAgentFileWithoutCard(t *testing.T) {
	r := setupTestRepo(t)
	const cardID = "11111111-2222-3333-4444-555555555555"
	if err := r.SaveAgentConfig(cardID, model.AgentConfig{Goal: "g"}); err != nil {
		t.Fatal(err)
	}
	stats, err := r.Revalidate()
	if err != nil {
		t.Fatal(err)
	}
	if stats.OrphanedAgentFiles != 1 {
		t.Errorf("OrphanedAgentFiles = %d, want 1", stats.OrphanedAgentFiles)
	}
	if !fileExists(r.agentFilePath(cardID)) {
		t.Error("agent file without a card file must be kept")
	}
}
