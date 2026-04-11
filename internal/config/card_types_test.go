package config

import (
	"bruv/internal/model"
	"os"
	"testing"
)

// redirectConfig points configDir() at a temp directory for test isolation.
//
// os.UserConfigDir() resolves differently on each OS:
//   - Windows uses %APPDATA%
//   - Linux uses $XDG_CONFIG_HOME, falling back to $HOME/.config
//   - macOS uses $HOME/Library/Application Support (XDG is ignored)
//
// To cover all three we set APPDATA, unset XDG_CONFIG_HOME (so Linux
// falls through to HOME instead of using a pre-existing CI value), and
// set HOME. On macOS the HOME override is the only thing that works; on
// Windows the APPDATA override dominates; on Linux the XDG clear + HOME
// set combination lands in a predictable temp directory.
func redirectConfig(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("APPDATA", tmp)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", tmp)
}

func TestLoadUserTypeStoreEmpty(t *testing.T) {
	redirectConfig(t)
	store, err := LoadUserTypeStore()
	if err != nil {
		t.Fatalf("LoadUserTypeStore: %v", err)
	}
	if len(store.Types) != 0 {
		t.Errorf("expected 0 types, got %d", len(store.Types))
	}
	if len(store.Templates) != 0 {
		t.Errorf("expected 0 templates, got %d", len(store.Templates))
	}
}

func TestSaveLoadUserTypeStoreRoundTrip(t *testing.T) {
	redirectConfig(t)

	store := UserTypeStore{
		Types: []UserCardType{
			{ID: "bug", Label: "Bug", Color: "#ff0000", Description: "A bug report"},
			{ID: "feature", Label: "Feature", Color: "#00ff00", Description: "A feature request", AIHint: "classify as feature"},
		},
		Templates: []CardTemplate{
			{
				ID:   "tpl-1",
				Name: "Basic Template",
				Blocks: []model.Block{
					{ID: "b1", Type: model.BlockText, Label: "Description", Key: "description"},
				},
			},
		},
	}

	if err := SaveUserTypeStore(store); err != nil {
		t.Fatalf("SaveUserTypeStore: %v", err)
	}

	loaded, err := LoadUserTypeStore()
	if err != nil {
		t.Fatalf("LoadUserTypeStore: %v", err)
	}

	if len(loaded.Types) != 2 {
		t.Fatalf("expected 2 types, got %d", len(loaded.Types))
	}
	if loaded.Types[0].ID != "bug" {
		t.Errorf("type[0].ID = %q, want %q", loaded.Types[0].ID, "bug")
	}
	if loaded.Types[1].AIHint != "classify as feature" {
		t.Errorf("type[1].AIHint = %q, want %q", loaded.Types[1].AIHint, "classify as feature")
	}

	if len(loaded.Templates) != 1 {
		t.Fatalf("expected 1 template, got %d", len(loaded.Templates))
	}
	if loaded.Templates[0].Name != "Basic Template" {
		t.Errorf("template name = %q, want %q", loaded.Templates[0].Name, "Basic Template")
	}
	if len(loaded.Templates[0].Blocks) != 1 {
		t.Fatalf("expected 1 block in template, got %d", len(loaded.Templates[0].Blocks))
	}
}

func TestLoadUserTypeStoreCorruptedJSON(t *testing.T) {
	redirectConfig(t)

	// Write garbage to the file
	path, err := userTypesPath()
	if err != nil {
		t.Fatalf("userTypesPath: %v", err)
	}
	os.WriteFile(path, []byte("{invalid json"), 0o644)

	_, err = LoadUserTypeStore()
	if err == nil {
		t.Fatal("expected error for corrupted JSON, got nil")
	}
}

func TestSaveUserTypeStoreOverwrites(t *testing.T) {
	redirectConfig(t)

	// Save initial store
	store1 := UserTypeStore{
		Types: []UserCardType{{ID: "a", Label: "A", Color: "#aaa", Description: "first"}},
	}
	SaveUserTypeStore(store1)

	// Overwrite with different data
	store2 := UserTypeStore{
		Types: []UserCardType{
			{ID: "b", Label: "B", Color: "#bbb", Description: "second"},
			{ID: "c", Label: "C", Color: "#ccc", Description: "third"},
		},
	}
	SaveUserTypeStore(store2)

	loaded, _ := LoadUserTypeStore()
	if len(loaded.Types) != 2 {
		t.Fatalf("expected 2 types after overwrite, got %d", len(loaded.Types))
	}
	if loaded.Types[0].ID != "b" {
		t.Errorf("type[0].ID = %q, want %q", loaded.Types[0].ID, "b")
	}
}

func TestUserTypeStoreTemplateIDReference(t *testing.T) {
	redirectConfig(t)

	store := UserTypeStore{
		Types: []UserCardType{
			{ID: "bug", Label: "Bug", Color: "#f00", Description: "Bug", TemplateID: "tpl-1"},
		},
		Templates: []CardTemplate{
			{ID: "tpl-1", Name: "Bug Template", Blocks: []model.Block{}},
		},
	}

	SaveUserTypeStore(store)
	loaded, _ := LoadUserTypeStore()

	if loaded.Types[0].TemplateID != "tpl-1" {
		t.Errorf("TemplateID = %q, want %q", loaded.Types[0].TemplateID, "tpl-1")
	}
}
