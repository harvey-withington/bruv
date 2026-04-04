package config

import (
	"testing"
)

func TestLoadRecentEmpty(t *testing.T) {
	redirectConfig(t)
	repos, err := LoadRecent()
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if repos != nil {
		t.Errorf("expected nil for no file, got %v", repos)
	}
}

func TestAddRecentSingle(t *testing.T) {
	redirectConfig(t)
	if err := AddRecent("/path/to/repo", "My Repo"); err != nil {
		t.Fatalf("AddRecent: %v", err)
	}

	repos, err := LoadRecent()
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 recent, got %d", len(repos))
	}
	if repos[0].Path != "/path/to/repo" {
		t.Errorf("Path = %q, want %q", repos[0].Path, "/path/to/repo")
	}
	if repos[0].Name != "My Repo" {
		t.Errorf("Name = %q, want %q", repos[0].Name, "My Repo")
	}
	if repos[0].LastOpened.IsZero() {
		t.Error("expected non-zero LastOpened")
	}
}

func TestAddRecentBumpsExisting(t *testing.T) {
	redirectConfig(t)
	AddRecent("/path/a", "Repo A")
	AddRecent("/path/b", "Repo B")

	// Re-add A — should move to top
	AddRecent("/path/a", "Repo A")

	repos, _ := LoadRecent()
	if len(repos) != 2 {
		t.Fatalf("expected 2 recent, got %d", len(repos))
	}
	if repos[0].Path != "/path/a" {
		t.Errorf("first repo Path = %q, want %q", repos[0].Path, "/path/a")
	}
	if repos[1].Path != "/path/b" {
		t.Errorf("second repo Path = %q, want %q", repos[1].Path, "/path/b")
	}
}

func TestAddRecentTrimsToMax(t *testing.T) {
	redirectConfig(t)

	// Add 12 repos (max is 10)
	for i := 0; i < 12; i++ {
		AddRecent("/path/"+string(rune('a'+i)), "Repo")
	}

	repos, _ := LoadRecent()
	if len(repos) != maxRecent {
		t.Errorf("expected %d recent, got %d", maxRecent, len(repos))
	}
}

func TestRemoveRecent(t *testing.T) {
	redirectConfig(t)
	AddRecent("/path/a", "A")
	AddRecent("/path/b", "B")
	AddRecent("/path/c", "C")

	if err := RemoveRecent("/path/b"); err != nil {
		t.Fatalf("RemoveRecent: %v", err)
	}

	repos, _ := LoadRecent()
	if len(repos) != 2 {
		t.Fatalf("expected 2 recent after remove, got %d", len(repos))
	}
	for _, r := range repos {
		if r.Path == "/path/b" {
			t.Error("removed repo should not be in list")
		}
	}
}

func TestRemoveRecentIdempotent(t *testing.T) {
	redirectConfig(t)
	AddRecent("/path/a", "A")

	// Remove nonexistent — should not error
	if err := RemoveRecent("/path/nonexistent"); err != nil {
		t.Fatalf("RemoveRecent nonexistent: %v", err)
	}

	repos, _ := LoadRecent()
	if len(repos) != 1 {
		t.Errorf("expected 1 recent, got %d", len(repos))
	}
}

func TestAddRecentUpdatesName(t *testing.T) {
	redirectConfig(t)
	AddRecent("/path/a", "Old Name")
	AddRecent("/path/a", "New Name")

	repos, _ := LoadRecent()
	if len(repos) != 1 {
		t.Fatalf("expected 1 recent, got %d", len(repos))
	}
	if repos[0].Name != "New Name" {
		t.Errorf("Name = %q, want %q", repos[0].Name, "New Name")
	}
}
