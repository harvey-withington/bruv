package config

import (
	"bytes"
	"os"
	"testing"
)

// redirectConfig is defined in card_types_test.go — reused here so each
// test gets an isolated temp config directory.

func TestDeleteNotificationRemovesOnlyMatchingID(t *testing.T) {
	redirectConfig(t)

	if err := AppendNotification(Notification{ID: "a", Title: "first"}); err != nil {
		t.Fatalf("AppendNotification a: %v", err)
	}
	if err := AppendNotification(Notification{ID: "b", Title: "second"}); err != nil {
		t.Fatalf("AppendNotification b: %v", err)
	}

	if err := DeleteNotification("a"); err != nil {
		t.Fatalf("DeleteNotification: %v", err)
	}

	list, err := LoadNotifications()
	if err != nil {
		t.Fatalf("LoadNotifications: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 notification remaining, got %d", len(list))
	}
	if list[0].ID != "b" {
		t.Errorf("remaining notification ID = %q, want %q", list[0].ID, "b")
	}
}

// DeleteNotification is idempotent — deleting a missing/already-removed
// ID is not an error, matching DeleteChatFor's contract.
func TestDeleteNotificationIdempotent(t *testing.T) {
	redirectConfig(t)

	if err := DeleteNotification("never-existed"); err != nil {
		t.Errorf("DeleteNotification on missing ID: %v", err)
	}

	if err := AppendNotification(Notification{ID: "x", Title: "one"}); err != nil {
		t.Fatalf("AppendNotification: %v", err)
	}
	if err := DeleteNotification("x"); err != nil {
		t.Errorf("DeleteNotification: %v", err)
	}
	if err := DeleteNotification("x"); err != nil {
		t.Errorf("DeleteNotification (second): %v", err)
	}

	list, err := LoadNotifications()
	if err != nil {
		t.Fatalf("LoadNotifications: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 notifications remaining, got %d", len(list))
	}
}

// A corrupt history is reported, never overwritten by the next append
// (pre-release sweep 2026-09-29, §6.5).
func TestAppendNotificationKeepsCorruptHistory(t *testing.T) {
	redirectConfig(t)
	path, err := notificationsPath()
	if err != nil {
		t.Fatal(err)
	}
	corrupt := []byte(`[{"id":"old","title":"kept"`)
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadNotifications(); err == nil {
		t.Error("LoadNotifications of a corrupt file must fail")
	}
	if err := AppendNotification(Notification{ID: "new"}); err == nil {
		t.Error("AppendNotification over a corrupt history must fail")
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, corrupt) {
		t.Errorf("history overwritten: %s", got)
	}
}
