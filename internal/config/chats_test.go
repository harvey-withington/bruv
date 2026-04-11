package config

import (
	"bruv/internal/model"
	"testing"
)

// redirectConfig is defined in card_types_test.go — reused here so each
// test gets an isolated temp config directory.

func TestLoadChatForEmpty(t *testing.T) {
	redirectConfig(t)

	cf, err := LoadChatFor("repo-1", "card-1")
	if err != nil {
		t.Fatalf("LoadChatFor: %v", err)
	}
	if cf.CardID != "card-1" {
		t.Errorf("CardID = %q, want %q", cf.CardID, "card-1")
	}
	if len(cf.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(cf.Messages))
	}
}

func TestSaveLoadChatRoundTrip(t *testing.T) {
	redirectConfig(t)

	cf := &model.ChatFile{
		CardID: "card-abc",
		Messages: []model.ChatMessage{
			{ID: "m1", Role: "user", Content: "hi"},
			{ID: "m2", Role: "assistant", Content: "hello"},
		},
	}

	if err := SaveChatFor("repo-1", cf); err != nil {
		t.Fatalf("SaveChatFor: %v", err)
	}

	loaded, err := LoadChatFor("repo-1", "card-abc")
	if err != nil {
		t.Fatalf("LoadChatFor: %v", err)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Content != "hi" {
		t.Errorf("messages[0].Content = %q, want %q", loaded.Messages[0].Content, "hi")
	}
}

// Chats are keyed by repo ID so two repos on the same machine keep
// independent chat histories for cards that happen to share an ID.
func TestChatFilesAreIsolatedPerRepo(t *testing.T) {
	redirectConfig(t)

	cf1 := &model.ChatFile{
		CardID:   "shared-card-id",
		Messages: []model.ChatMessage{{ID: "m1", Role: "user", Content: "repo-1"}},
	}
	cf2 := &model.ChatFile{
		CardID:   "shared-card-id",
		Messages: []model.ChatMessage{{ID: "m1", Role: "user", Content: "repo-2"}},
	}
	if err := SaveChatFor("repo-1", cf1); err != nil {
		t.Fatalf("SaveChatFor repo-1: %v", err)
	}
	if err := SaveChatFor("repo-2", cf2); err != nil {
		t.Fatalf("SaveChatFor repo-2: %v", err)
	}

	loaded1, _ := LoadChatFor("repo-1", "shared-card-id")
	loaded2, _ := LoadChatFor("repo-2", "shared-card-id")
	if loaded1.Messages[0].Content != "repo-1" {
		t.Errorf("repo-1 leaked: got %q", loaded1.Messages[0].Content)
	}
	if loaded2.Messages[0].Content != "repo-2" {
		t.Errorf("repo-2 leaked: got %q", loaded2.Messages[0].Content)
	}
}

func TestAppendChatMessage(t *testing.T) {
	redirectConfig(t)

	_, err := AppendChatMessage("repo-1", "card-1", model.ChatMessage{ID: "m1", Role: "user", Content: "first"})
	if err != nil {
		t.Fatalf("AppendChatMessage: %v", err)
	}
	cf, err := AppendChatMessage("repo-1", "card-1", model.ChatMessage{ID: "m2", Role: "assistant", Content: "second"})
	if err != nil {
		t.Fatalf("AppendChatMessage: %v", err)
	}
	if len(cf.Messages) != 2 {
		t.Fatalf("expected 2 messages after two appends, got %d", len(cf.Messages))
	}
}

// DeleteChatFor is idempotent — missing files are not an error so
// cleanup hooks can call it unconditionally when a card is deleted.
func TestDeleteChatForIdempotent(t *testing.T) {
	redirectConfig(t)

	// Deleting a non-existent chat file should not error.
	if err := DeleteChatFor("repo-1", "never-existed"); err != nil {
		t.Errorf("DeleteChatFor on missing file: %v", err)
	}

	// Create, delete, delete again — all three should succeed.
	SaveChatFor("repo-1", &model.ChatFile{CardID: "to-delete"})
	if err := DeleteChatFor("repo-1", "to-delete"); err != nil {
		t.Errorf("DeleteChatFor: %v", err)
	}
	if err := DeleteChatFor("repo-1", "to-delete"); err != nil {
		t.Errorf("DeleteChatFor (second): %v", err)
	}
}

// Synthetic project chat IDs with the __project__ prefix are treated
// identically to card chat IDs — the storage layer doesn't care.
func TestProjectChatIDsAreSupported(t *testing.T) {
	redirectConfig(t)

	cf := &model.ChatFile{
		CardID:   "__project__my-project-id",
		Messages: []model.ChatMessage{{ID: "m1", Role: "user", Content: "project scope"}},
	}
	if err := SaveChatFor("repo-1", cf); err != nil {
		t.Fatalf("SaveChatFor: %v", err)
	}
	loaded, err := LoadChatFor("repo-1", "__project__my-project-id")
	if err != nil {
		t.Fatalf("LoadChatFor: %v", err)
	}
	if len(loaded.Messages) != 1 {
		t.Errorf("project chat lost messages: got %d", len(loaded.Messages))
	}
}
