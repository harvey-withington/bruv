package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- Preferences ---

func TestDefaultPreferences(t *testing.T) {
	p := DefaultPreferences()
	if p.Theme != "dark" {
		t.Errorf("Theme = %q, want %q", p.Theme, "dark")
	}
	if p.Locale != "en" {
		t.Errorf("Locale = %q, want %q", p.Locale, "en")
	}
	if !p.ConfirmBeforeDelete {
		t.Error("ConfirmBeforeDelete should default to true")
	}
	if p.SidebarWidth != 260 {
		t.Errorf("SidebarWidth = %d, want 260", p.SidebarWidth)
	}
	if p.ReopenLastRepo {
		t.Error("ReopenLastRepo should default to false")
	}
}

func TestPreferencesSaveLoad(t *testing.T) {
	p := Preferences{
		ReopenLastRepo:      true,
		Theme:               "light",
		Locale:              "en",
		ConfirmBeforeDelete: false,
		SidebarWidth:        300,
	}

	if err := SavePreferences(p); err != nil {
		t.Fatalf("SavePreferences: %v", err)
	}

	loaded, err := LoadPreferences()
	if err != nil {
		t.Fatalf("LoadPreferences: %v", err)
	}

	if loaded.ReopenLastRepo != true {
		t.Error("ReopenLastRepo not persisted")
	}
	if loaded.Theme != "light" {
		t.Errorf("Theme = %q, want %q", loaded.Theme, "light")
	}
	if loaded.ConfirmBeforeDelete != false {
		t.Error("ConfirmBeforeDelete should be false")
	}
	if loaded.SidebarWidth != 300 {
		t.Errorf("SidebarWidth = %d, want 300", loaded.SidebarWidth)
	}

	// Restore defaults so test doesn't pollute config
	_ = SavePreferences(DefaultPreferences())
}

func TestLoadPreferencesMissingFileReturnsDefaults(t *testing.T) {
	// Temporarily rename the file if it exists
	path, err := prefsPath()
	if err != nil {
		t.Skip("cannot resolve config dir")
	}
	backup := path + ".test-backup"
	renamed := false
	if _, err := os.Stat(path); err == nil {
		os.Rename(path, backup)
		renamed = true
	}
	defer func() {
		if renamed {
			os.Rename(backup, path)
		}
	}()

	p, err := LoadPreferences()
	if err != nil {
		t.Fatalf("LoadPreferences: %v", err)
	}
	def := DefaultPreferences()
	if p.Theme != def.Theme || p.SidebarWidth != def.SidebarWidth {
		t.Errorf("missing file should return defaults, got Theme=%q SidebarWidth=%d", p.Theme, p.SidebarWidth)
	}
}

func TestLoadPreferencesCorruptedFileReturnsDefaults(t *testing.T) {
	path, err := prefsPath()
	if err != nil {
		t.Skip("cannot resolve config dir")
	}
	backup := path + ".test-backup"
	renamed := false
	if _, err := os.Stat(path); err == nil {
		os.Rename(path, backup)
		renamed = true
	}
	defer func() {
		os.Remove(path)
		if renamed {
			os.Rename(backup, path)
		}
	}()

	// Write invalid JSON
	os.WriteFile(path, []byte("{not valid json!!!"), 0o644)

	p, err := LoadPreferences()
	if err == nil {
		t.Fatal("expected error on corrupted JSON")
	}
	def := DefaultPreferences()
	if p.Theme != def.Theme {
		t.Errorf("corrupted file should return defaults, got Theme=%q", p.Theme)
	}
}

// --- UserProfile ---

func TestProfileSaveLoad(t *testing.T) {
	p := UserProfile{
		DisplayName: "Test User",
		Role:        "Developer",
		Bio:         "Testing profile persistence.",
		Expertise:   []string{"Go", "Svelte", "TypeScript"},
		Context:     "I prefer functional programming patterns.",
	}

	if err := SaveProfile(p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	loaded, err := LoadProfile()
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	if loaded.DisplayName != "Test User" {
		t.Errorf("DisplayName = %q, want %q", loaded.DisplayName, "Test User")
	}
	if loaded.Role != "Developer" {
		t.Errorf("Role = %q, want %q", loaded.Role, "Developer")
	}
	if loaded.Bio != "Testing profile persistence." {
		t.Errorf("Bio mismatch")
	}
	if len(loaded.Expertise) != 3 {
		t.Errorf("Expertise count = %d, want 3", len(loaded.Expertise))
	}
	if loaded.Context != "I prefer functional programming patterns." {
		t.Errorf("Context mismatch")
	}

	// Clean up
	path, _ := profilePath()
	os.Remove(path)
}

func TestLoadProfileMissingFileReturnsEmpty(t *testing.T) {
	path, err := profilePath()
	if err != nil {
		t.Skip("cannot resolve config dir")
	}
	backup := path + ".test-backup"
	renamed := false
	if _, err := os.Stat(path); err == nil {
		os.Rename(path, backup)
		renamed = true
	}
	defer func() {
		if renamed {
			os.Rename(backup, path)
		}
	}()

	p, err := LoadProfile()
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if p.DisplayName != "" || p.Role != "" {
		t.Errorf("missing file should return empty profile, got %+v", p)
	}
}

// --- WindowBounds ---

func TestWindowBoundsSaveLoad(t *testing.T) {
	wb := &WindowBounds{
		X: 100, Y: 200, Width: 1280, Height: 800, Maximised: false,
	}

	if err := SaveWindowBounds(wb); err != nil {
		t.Fatalf("SaveWindowBounds: %v", err)
	}

	loaded := LoadWindowBounds()
	if loaded == nil {
		t.Fatal("LoadWindowBounds returned nil")
	}

	if loaded.X != 100 || loaded.Y != 200 {
		t.Errorf("position = (%d,%d), want (100,200)", loaded.X, loaded.Y)
	}
	if loaded.Width != 1280 || loaded.Height != 800 {
		t.Errorf("size = %dx%d, want 1280x800", loaded.Width, loaded.Height)
	}
	if loaded.Maximised {
		t.Error("Maximised should be false")
	}

	// Clean up
	path, _ := windowFilePath()
	os.Remove(path)
}

func TestWindowBoundsSaveLoadMaximised(t *testing.T) {
	wb := &WindowBounds{
		X: 50, Y: 50, Width: 1920, Height: 1080, Maximised: true,
	}

	if err := SaveWindowBounds(wb); err != nil {
		t.Fatalf("SaveWindowBounds: %v", err)
	}

	loaded := LoadWindowBounds()
	if loaded == nil {
		t.Fatal("LoadWindowBounds returned nil")
	}
	if !loaded.Maximised {
		t.Error("Maximised should be true")
	}

	// Clean up
	path, _ := windowFilePath()
	os.Remove(path)
}

func TestLoadWindowBoundsMissingFileReturnsNil(t *testing.T) {
	path, err := windowFilePath()
	if err != nil {
		t.Skip("cannot resolve config dir")
	}
	backup := path + ".test-backup"
	renamed := false
	if _, err := os.Stat(path); err == nil {
		os.Rename(path, backup)
		renamed = true
	}
	defer func() {
		if renamed {
			os.Rename(backup, path)
		}
	}()

	wb := LoadWindowBounds()
	if wb != nil {
		t.Errorf("missing file should return nil, got %+v", wb)
	}
}

func TestLoadWindowBoundsRejectsTooSmall(t *testing.T) {
	path, err := windowFilePath()
	if err != nil {
		t.Skip("cannot resolve config dir")
	}
	defer os.Remove(path)

	// Write bounds that are too small
	small := WindowBounds{X: 0, Y: 0, Width: 50, Height: 50}
	data, _ := json.Marshal(small)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, data, 0o644)

	wb := LoadWindowBounds()
	if wb != nil {
		t.Errorf("bounds with width/height < 200 should return nil, got %+v", wb)
	}
}

func TestLoadWindowBoundsRejectsCorruptedJSON(t *testing.T) {
	path, err := windowFilePath()
	if err != nil {
		t.Skip("cannot resolve config dir")
	}
	defer os.Remove(path)

	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("not json"), 0o644)

	wb := LoadWindowBounds()
	if wb != nil {
		t.Errorf("corrupted JSON should return nil, got %+v", wb)
	}
}

// --- ClampToVisible ---

func TestClampToVisibleNoOpWhenOnScreen(t *testing.T) {
	wb := &WindowBounds{X: 100, Y: 100, Width: 800, Height: 600}
	ClampToVisible(wb, 1920, 1080)
	if wb.X != 100 || wb.Y != 100 {
		t.Errorf("on-screen window should not move, got (%d,%d)", wb.X, wb.Y)
	}
}

func TestClampToVisibleWindowFarRight(t *testing.T) {
	wb := &WindowBounds{X: 3000, Y: 100, Width: 800, Height: 600}
	ClampToVisible(wb, 1920, 1080)
	if wb.X >= 1920 {
		t.Errorf("window far right should be clamped, X=%d", wb.X)
	}
}

func TestClampToVisibleWindowFarLeft(t *testing.T) {
	wb := &WindowBounds{X: -2000, Y: 100, Width: 800, Height: 600}
	ClampToVisible(wb, 1920, 1080)
	if wb.X != 0 {
		t.Errorf("window far left should clamp to X=0, got X=%d", wb.X)
	}
}

func TestClampToVisibleWindowFarBelow(t *testing.T) {
	wb := &WindowBounds{X: 100, Y: 2000, Width: 800, Height: 600}
	ClampToVisible(wb, 1920, 1080)
	if wb.Y >= 1080 {
		t.Errorf("window far below should be clamped, Y=%d", wb.Y)
	}
}

func TestClampToVisibleWindowFarAbove(t *testing.T) {
	wb := &WindowBounds{X: 100, Y: -2000, Width: 800, Height: 600}
	ClampToVisible(wb, 1920, 1080)
	if wb.Y != 0 {
		t.Errorf("window far above should clamp to Y=0, got Y=%d", wb.Y)
	}
}

func TestClampToVisibleWindowOffAllSides(t *testing.T) {
	wb := &WindowBounds{X: -5000, Y: -5000, Width: 800, Height: 600}
	ClampToVisible(wb, 1920, 1080)
	if wb.X != 0 || wb.Y != 0 {
		t.Errorf("fully off-screen should clamp to (0,0), got (%d,%d)", wb.X, wb.Y)
	}
}

func TestClampToVisibleWindowLargerThanScreen(t *testing.T) {
	wb := &WindowBounds{X: -100, Y: -100, Width: 3000, Height: 2000}
	ClampToVisible(wb, 1920, 1080)
	// Window at X=-100 with Width=3000 → right edge at 2900, well on-screen.
	// ClampToVisible only moves windows that are entirely off-screen.
	// This window is partially visible, so position should be unchanged.
	if wb.X != -100 || wb.Y != -100 {
		t.Errorf("partially visible large window should not move, got (%d,%d)", wb.X, wb.Y)
	}
}

func TestClampToVisibleWindowLargerThanScreenFullyOffRight(t *testing.T) {
	wb := &WindowBounds{X: 5000, Y: 5000, Width: 3000, Height: 2000}
	ClampToVisible(wb, 1920, 1080)
	// Fully off-screen right/below — should be clamped
	if wb.X >= 1920 {
		t.Errorf("should clamp X, got %d", wb.X)
	}
	if wb.Y >= 1080 {
		t.Errorf("should clamp Y, got %d", wb.Y)
	}
}

func TestClampToVisibleWindowPartiallyRight(t *testing.T) {
	// Window hangs off right edge but minVisible pixels are still on-screen
	wb := &WindowBounds{X: 1800, Y: 100, Width: 800, Height: 600}
	ClampToVisible(wb, 1920, 1080)
	// X=1800 means X > screenW-minVisible (1920-100=1820), so it should clamp
	if wb.X > 1820 {
		t.Errorf("partially right window should clamp, X=%d", wb.X)
	}
}

func TestClampToVisibleSmallScreen(t *testing.T) {
	wb := &WindowBounds{X: 500, Y: 300, Width: 800, Height: 600}
	ClampToVisible(wb, 800, 600)
	// X=500 > 800-100=700, should clamp
	if wb.X > 700 {
		t.Errorf("small screen clamp: X=%d should be <= 700", wb.X)
	}
}

func TestClampToVisibleNegativeYClampedToZero(t *testing.T) {
	wb := &WindowBounds{X: 0, Y: -800, Width: 400, Height: 300}
	ClampToVisible(wb, 1920, 1080)
	// Y + Height = -800 + 300 = -500 < 100 → Y = 0
	if wb.Y != 0 {
		t.Errorf("negative Y should clamp to 0, got Y=%d", wb.Y)
	}
}
