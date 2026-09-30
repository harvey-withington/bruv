package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bruv/internal/model"
)

// A card_types.json that can't be read must never be "repaired" by
// seeding: that overwrote the file with the two seed types and wiped
// every user type and template (pre-release sweep 2026-09-29). Listing
// degrades to the built-ins, nothing is written, and user types fail
// closed.
func TestUnreadableTypeStoreIsNeverOverwritten(t *testing.T) {
	s, r := newMergeService(t)
	if _, err := s.CreateNamedType("Recipe", "", "", ""); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(r.Root, "card_types.json")
	corrupt := []byte(`{"types": [ {"id": "recipe"`) // a torn write
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatal(err)
	}

	types := s.ListCardTypes()
	if len(types) != len(BuiltinTypes) {
		t.Errorf("listed %d types, want the %d built-ins only", len(types), len(BuiltinTypes))
	}
	if _, err := s.LoadCardTypes(); err == nil {
		t.Error("LoadCardTypes must report the load error")
	}
	if s.CardTypeExists("recipe") {
		t.Error("CardTypeExists must fail closed while the store is unreadable")
	}
	if !s.CardTypeExists("task") {
		t.Error("built-in types must still exist")
	}
	if _, err := s.ResolveType("Recipe"); err == nil {
		t.Error("ResolveType must not resolve (or pretend-unknown) a type it couldn't load")
	} else if errors.As(err, new(*UnknownTypeError)) {
		t.Errorf("a load failure is not an unknown type: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(corrupt) {
		t.Errorf("card_types.json was rewritten after a failed read:\n%s", got)
	}
}

func TestResolveTypeNeverCreates(t *testing.T) {
	s, _ := newMergeService(t)

	for in, want := range map[string]string{"TASK": "task", "Feature": "feature", "episode": "episode", "  ": ""} {
		got, err := s.ResolveType(in)
		if err != nil || got != want {
			t.Errorf("ResolveType(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	_, err := s.ResolveType("Field Note")
	var unknown *UnknownTypeError
	if !errors.As(err, &unknown) {
		t.Fatalf("unknown type: err = %v, want *UnknownTypeError", err)
	}
	if !strings.Contains(err.Error(), "brainstorm") || !strings.Contains(err.Error(), "feature") {
		t.Errorf("refusal should list the available types: %v", err)
	}
	if s.CardTypeExists("field-note") {
		t.Error("ResolveType created a type")
	}
}

func TestCreateNamedTypeRefusesDuplicates(t *testing.T) {
	s, _ := newMergeService(t)
	created, err := s.CreateNamedType("Field Note", "", "notes", "")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "field-note" || created.Color == "" {
		t.Errorf("created %+v, want id field-note with a palette colour", created)
	}
	for _, dup := range []string{"field note", "FIELD-NOTE", "Brainstorm", "task"} {
		if _, err := s.CreateNamedType(dup, "", "", ""); err == nil {
			t.Errorf("CreateNamedType(%q) duplicated an existing type", dup)
		}
	}
	if _, err := s.CreateNamedType("Recipe", "red", "", ""); err == nil {
		t.Error("a non-hex colour must be refused")
	}
	if _, err := s.CreateNamedType("  ", "", "", ""); err == nil {
		t.Error("an empty label must be refused")
	}
}

// A failed template merge surfaces instead of vanishing (RefreshTypeBlocks
// returned the untouched card as if the merge had worked).
func TestMergeTemplateBlocksReportsMissingCard(t *testing.T) {
	s, _ := newMergeService(t)
	if err := s.mergeTemplateBlocks("no-such-card", []model.Block{{Type: model.BlockText, Key: "k"}}); err == nil {
		t.Error("merging into a missing card must return an error")
	}
}
