package repo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- Slugify ---

func TestSlugifySimpleName(t *testing.T) {
	if got := Slugify("Hello World"); got != "hello-world" {
		t.Errorf("Slugify(%q) = %q, want %q", "Hello World", got, "hello-world")
	}
}

func TestSlugifyUppercase(t *testing.T) {
	if got := Slugify("MY PROJECT"); got != "my-project" {
		t.Errorf("Slugify(%q) = %q, want %q", "MY PROJECT", got, "my-project")
	}
}

func TestSlugifySpecialChars(t *testing.T) {
	// Special chars are dropped but the space between words is preserved as a dash
	if got := Slugify("Hello! @World#"); got != "hello-world" {
		t.Errorf("Slugify(%q) = %q, want %q", "Hello! @World#", got, "hello-world")
	}
}

func TestSlugifyTrailingDash(t *testing.T) {
	if got := Slugify("Hello "); got != "hello" {
		t.Errorf("Slugify(%q) = %q, want %q", "Hello ", got, "hello")
	}
}

func TestSlugifyMultipleSpaces(t *testing.T) {
	if got := Slugify("a   b"); got != "a-b" {
		t.Errorf("Slugify(%q) = %q, want %q", "a   b", got, "a-b")
	}
}

func TestSlugifyEmptyString(t *testing.T) {
	if got := Slugify(""); got != "" {
		t.Errorf("Slugify(%q) = %q, want %q", "", got, "")
	}
}

func TestSlugifyNumbers(t *testing.T) {
	if got := Slugify("Episode 42"); got != "episode-42" {
		t.Errorf("Slugify(%q) = %q, want %q", "Episode 42", got, "episode-42")
	}
}

func TestSlugifyDashes(t *testing.T) {
	if got := Slugify("my-project"); got != "my-project" {
		t.Errorf("Slugify(%q) = %q, want %q", "my-project", got, "my-project")
	}
}

func TestSlugifyUnderscores(t *testing.T) {
	if got := Slugify("my_project"); got != "my-project" {
		t.Errorf("Slugify(%q) = %q, want %q", "my_project", got, "my-project")
	}
}

func TestSlugifyDots(t *testing.T) {
	if got := Slugify("v1.0.0"); got != "v1-0-0" {
		t.Errorf("Slugify(%q) = %q, want %q", "v1.0.0", got, "v1-0-0")
	}
}

// --- SanitizeText ---

func TestSanitizeTextReplacesDelimiter(t *testing.T) {
	if got := SanitizeText("A\u203aB"); got != "A>B" {
		t.Errorf("SanitizeText = %q, want %q", got, "A>B")
	}
}

func TestSanitizeTextLeavesNormalText(t *testing.T) {
	input := "Hello, World!"
	if got := SanitizeText(input); got != input {
		t.Errorf("SanitizeText = %q, want %q", got, input)
	}
}

func TestSanitizeTextEmptyString(t *testing.T) {
	if got := SanitizeText(""); got != "" {
		t.Errorf("SanitizeText = %q, want %q", got, "")
	}
}

// --- uniqueSlug ---

func TestUniqueSlugBaseAvailable(t *testing.T) {
	taken := func(s string) bool { return false }
	if got := uniqueSlug("my-slug", taken); got != "my-slug" {
		t.Errorf("uniqueSlug = %q, want %q", got, "my-slug")
	}
}

func TestUniqueSlugBaseTaken(t *testing.T) {
	takenSet := map[string]bool{"my-slug": true}
	taken := func(s string) bool { return takenSet[s] }
	if got := uniqueSlug("my-slug", taken); got != "my-slug-2" {
		t.Errorf("uniqueSlug = %q, want %q", got, "my-slug-2")
	}
}

func TestUniqueSlugMultipleTaken(t *testing.T) {
	takenSet := map[string]bool{"item": true, "item-2": true, "item-3": true}
	taken := func(s string) bool { return takenSet[s] }
	if got := uniqueSlug("item", taken); got != "item-4" {
		t.Errorf("uniqueSlug = %q, want %q", got, "item-4")
	}
}

// --- readJSON / writeJSON round-trip ---

func TestReadWriteJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	type testData struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	original := testData{Name: "hello", Count: 42}
	if err := writeJSON(path, original); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}

	// Verify temp file was cleaned up
	if fileExists(path + ".tmp") {
		t.Error("temp file should be cleaned up after writeJSON")
	}

	var loaded testData
	if err := readJSON(path, &loaded); err != nil {
		t.Fatalf("readJSON: %v", err)
	}

	if loaded.Name != original.Name || loaded.Count != original.Count {
		t.Errorf("round-trip mismatch: got %+v, want %+v", loaded, original)
	}
}

func TestReadJSONMissingFile(t *testing.T) {
	var dest map[string]string
	err := readJSON("/nonexistent/file.json", &dest)
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestWriteJSONCreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "test.json")

	if err := writeJSON(path, map[string]string{"key": "value"}); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}

	if !fileExists(path) {
		t.Error("expected file to exist after writeJSON with nested dirs")
	}
}

// --- marshalSorted ---

func TestMarshalSortedKeyOrder(t *testing.T) {
	input := map[string]string{
		"zebra": "z",
		"alpha": "a",
		"mid":   "m",
	}

	data, err := marshalSorted(input)
	if err != nil {
		t.Fatalf("marshalSorted: %v", err)
	}

	// Parse back and verify it's valid JSON
	var parsed map[string]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal sorted output: %v", err)
	}
	if parsed["alpha"] != "a" || parsed["zebra"] != "z" || parsed["mid"] != "m" {
		t.Errorf("unexpected parsed values: %v", parsed)
	}

	// Verify keys appear in sorted order in the raw output
	output := string(data)
	alphaIdx := len(output) // fallback
	zebraIdx := 0
	for i := range output {
		if output[i] == 'a' && i+5 < len(output) && output[i:i+5] == "alpha" {
			alphaIdx = i
		}
		if output[i] == 'z' && i+5 < len(output) && output[i:i+5] == "zebra" {
			zebraIdx = i
		}
	}
	if alphaIdx >= zebraIdx {
		t.Error("expected 'alpha' to appear before 'zebra' in sorted output")
	}
}

// --- fileExists ---

func TestFileExistsTrue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(path, []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	if !fileExists(path) {
		t.Error("expected fileExists to return true for existing file")
	}
}

func TestFileExistsFalse(t *testing.T) {
	if fileExists("/nonexistent/path/to/file") {
		t.Error("expected fileExists to return false for non-existent file")
	}
}

func TestFileExistsDirectory(t *testing.T) {
	dir := t.TempDir()
	if !fileExists(dir) {
		t.Error("expected fileExists to return true for existing directory")
	}
}
