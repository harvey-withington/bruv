package config

import "testing"

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
