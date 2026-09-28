package catalog

import "testing"

func findType(t *testing.T, s *Service, id string) CardTypeInfo {
	t.Helper()
	for _, ct := range s.ListCardTypes() {
		if ct.ID == id {
			return ct
		}
	}
	t.Fatalf("card type %q not listed", id)
	return CardTypeInfo{}
}

// Picking an icon for a built-in type in the editor used to be dropped
// silently — the override only carried color + template, so the badge
// never showed it (Harvey, 2026-09-27).
func TestUpdateBuiltinCardTypePersistsIcon(t *testing.T) {
	s, _ := newMergeService(t)

	if err := s.UpdateBuiltinCardType("task", "#123456", "rocket", ""); err != nil {
		t.Fatal(err)
	}
	got := findType(t, s, "task")
	if got.Icon != "rocket" || got.Color != "#123456" {
		t.Fatalf("override not applied: icon=%q color=%q", got.Icon, got.Color)
	}

	// Each save replaces the whole override, so clearing the icon sticks.
	if err := s.UpdateBuiltinCardType("task", "#123456", "", ""); err != nil {
		t.Fatal(err)
	}
	if got := findType(t, s, "task"); got.Icon != "" {
		t.Fatalf("icon should be cleared, got %q", got.Icon)
	}
}

func TestUpdateBuiltinCardTypeRejectsUserType(t *testing.T) {
	s, _ := newMergeService(t)
	if err := s.UpdateBuiltinCardType("not-builtin", "#000000", "rocket", ""); err == nil {
		t.Fatal("expected an error for a non-built-in id")
	}
}
