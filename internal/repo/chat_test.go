package repo

import (
	"bruv/internal/model"
	"testing"
	"time"
)

func TestLoadChatNonExistentCard(t *testing.T) {
	r := setupTestRepo(t)
	cf, err := r.LoadChat("nonexistent-card-id")
	if err != nil {
		t.Fatalf("LoadChat: %v", err)
	}
	if cf.CardID != "nonexistent-card-id" {
		t.Errorf("CardID = %q, want %q", cf.CardID, "nonexistent-card-id")
	}
	if len(cf.Messages) != 0 {
		t.Errorf("expected empty messages, got %d", len(cf.Messages))
	}
}

func TestSaveChatThenLoad(t *testing.T) {
	r := setupTestRepo(t)
	card, err := r.CreateCard("task", "Test Card")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	cf := &model.ChatFile{
		CardID: card.ID,
		Messages: []model.ChatMessage{
			{
				ID:        "msg-1",
				Role:      model.RoleUser,
				Content:   "Hello",
				Timestamp: time.Now().UTC(),
			},
			{
				ID:        "msg-2",
				Role:      model.RoleAssistant,
				Content:   "Hi there!",
				Timestamp: time.Now().UTC(),
			},
		},
	}

	if err := r.SaveChat(cf); err != nil {
		t.Fatalf("SaveChat: %v", err)
	}

	loaded, err := r.LoadChat(card.ID)
	if err != nil {
		t.Fatalf("LoadChat: %v", err)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Content != "Hello" {
		t.Errorf("msg[0].Content = %q, want %q", loaded.Messages[0].Content, "Hello")
	}
	if loaded.Messages[1].Content != "Hi there!" {
		t.Errorf("msg[1].Content = %q, want %q", loaded.Messages[1].Content, "Hi there!")
	}
}

func TestAppendMessage(t *testing.T) {
	r := setupTestRepo(t)
	card, _ := r.CreateCard("task", "Test Card")

	// Append first message
	msg1 := model.ChatMessage{
		ID:        "msg-1",
		Role:      model.RoleUser,
		Content:   "First message",
		Timestamp: time.Now().UTC(),
	}
	cf, err := r.AppendMessage(card.ID, msg1)
	if err != nil {
		t.Fatalf("AppendMessage first: %v", err)
	}
	if len(cf.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(cf.Messages))
	}

	// Append second message — preserves existing
	msg2 := model.ChatMessage{
		ID:        "msg-2",
		Role:      model.RoleAssistant,
		Content:   "Second message",
		Timestamp: time.Now().UTC(),
	}
	cf, err = r.AppendMessage(card.ID, msg2)
	if err != nil {
		t.Fatalf("AppendMessage second: %v", err)
	}
	if len(cf.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(cf.Messages))
	}
	if cf.Messages[0].Content != "First message" {
		t.Errorf("msg[0] = %q, want %q", cf.Messages[0].Content, "First message")
	}
	if cf.Messages[1].Content != "Second message" {
		t.Errorf("msg[1] = %q, want %q", cf.Messages[1].Content, "Second message")
	}
}

func TestAppendMessageCreatesFileIfNone(t *testing.T) {
	r := setupTestRepo(t)
	card, _ := r.CreateCard("task", "Test Card")

	msg := model.ChatMessage{
		ID:        "msg-1",
		Role:      model.RoleUser,
		Content:   "Hello from nothing",
		Timestamp: time.Now().UTC(),
	}
	cf, err := r.AppendMessage(card.ID, msg)
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	if cf.CardID != card.ID {
		t.Errorf("CardID = %q, want %q", cf.CardID, card.ID)
	}
	if len(cf.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(cf.Messages))
	}
}

func TestDeleteChat(t *testing.T) {
	r := setupTestRepo(t)
	card, _ := r.CreateCard("task", "Test Card")

	// Save a chat
	cf := &model.ChatFile{
		CardID: card.ID,
		Messages: []model.ChatMessage{
			{ID: "msg-1", Role: model.RoleUser, Content: "Hi", Timestamp: time.Now().UTC()},
		},
	}
	r.SaveChat(cf)

	// Delete it
	if err := r.DeleteChat(card.ID); err != nil {
		t.Fatalf("DeleteChat: %v", err)
	}

	// Loading should return empty
	loaded, err := r.LoadChat(card.ID)
	if err != nil {
		t.Fatalf("LoadChat after delete: %v", err)
	}
	if len(loaded.Messages) != 0 {
		t.Errorf("expected 0 messages after delete, got %d", len(loaded.Messages))
	}
}

func TestDeleteChatIdempotent(t *testing.T) {
	r := setupTestRepo(t)
	// Deleting chat for a card that has no chat should not error
	if err := r.DeleteChat("no-such-card"); err != nil {
		t.Fatalf("DeleteChat on nonexistent should be idempotent: %v", err)
	}
}

func TestChatPreservesToolActions(t *testing.T) {
	r := setupTestRepo(t)
	card, _ := r.CreateCard("task", "Test Card")

	msg := model.ChatMessage{
		ID:        "msg-1",
		Role:      model.RoleAssistant,
		Content:   "Classified your card",
		Timestamp: time.Now().UTC(),
		ToolActions: []model.ToolAction{
			{Tool: "set_card_type", Input: map[string]any{"card_type": "task"}, Result: "ok"},
		},
		PinSuggestion: &model.PinSuggestion{
			CategoryID:   "cat-1",
			CategoryName: "Backlog",
			Reason:       "fits here",
			Status:       "pending",
		},
	}

	r.AppendMessage(card.ID, msg)
	loaded, _ := r.LoadChat(card.ID)

	if len(loaded.Messages[0].ToolActions) != 1 {
		t.Fatalf("expected 1 tool action, got %d", len(loaded.Messages[0].ToolActions))
	}
	if loaded.Messages[0].ToolActions[0].Tool != "set_card_type" {
		t.Errorf("tool = %q, want %q", loaded.Messages[0].ToolActions[0].Tool, "set_card_type")
	}
	if loaded.Messages[0].PinSuggestion == nil {
		t.Fatal("expected pin suggestion to be preserved")
	}
	if loaded.Messages[0].PinSuggestion.CategoryID != "cat-1" {
		t.Errorf("pin suggestion category = %q, want %q", loaded.Messages[0].PinSuggestion.CategoryID, "cat-1")
	}
}
