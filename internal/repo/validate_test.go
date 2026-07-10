package repo

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIsSafeSeg(t *testing.T) {
	safe := []string{
		"abc",
		"a1b2c3-d4e5",
		"att-9b2f7c1e",
		"__project__42",
		"card.with.dots",
		"UPPER-case_mix",
	}
	for _, s := range safe {
		if !isSafeSeg(s) {
			t.Errorf("isSafeSeg(%q) = false, want true", s)
		}
	}

	unsafe := []string{
		"",
		".",
		"..",
		"../x",
		"..\\x",
		"../../etc/passwd",
		"..\\..\\windows\\system32",
		"a/b",
		"a\\b",
		"/abs",
		"\\abs",
		"x\x00y",
	}
	for _, s := range unsafe {
		if isSafeSeg(s) {
			t.Errorf("isSafeSeg(%q) = true, want false", s)
		}
	}

	if runtime.GOOS == "windows" {
		// Drive-relative forms and reserved device names are rejected
		// by filepath.IsLocal on Windows only — on Linux they are
		// ordinary (if ugly) filenames, which is correct per-OS.
		for _, s := range []string{"c:evil", "C:\\evil", "CON", "NUL", "aux"} {
			if isSafeSeg(s) {
				t.Errorf("isSafeSeg(%q) = true on windows, want false", s)
			}
		}
	}
}

// TestTraversalIDsCannotEscapeRepo drives the public API with hostile
// IDs and asserts nothing outside the repo root is read or written.
func TestTraversalIDsCannotEscapeRepo(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := Init(dir, "Traversal Test")
	if err != nil {
		t.Fatal(err)
	}

	// Plant a JSON file OUTSIDE the repo that a traversal read would hit.
	secret := filepath.Join(parent, "secret.json")
	if err := os.WriteFile(secret, []byte(`{"id":"secret","title":"leaked"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	hostile := []string{
		"../secret",
		"..\\secret",
		"../../secret",
		"..",
		"a/../../secret",
	}

	for _, id := range hostile {
		if card, err := r.GetCard(id); err == nil {
			t.Errorf("GetCard(%q) succeeded (title %q), want error", id, card.Title)
		}
		if err := r.DeleteCard(id); err == nil {
			t.Errorf("DeleteCard(%q) succeeded, want error", id)
		}
		if _, err := r.AddCardAttachment(id, "x.txt", "aGVsbG8="); err == nil {
			t.Errorf("AddCardAttachment(%q) succeeded, want error", id)
		}
		if p := r.AttachmentPath(id, "att-1"); !strings.Contains(p, "\x00") {
			t.Errorf("AttachmentPath(%q) = %q, want sentinel path", id, p)
		}
		if p := r.AttachmentPath("card-1", id); !strings.Contains(p, "\x00") {
			t.Errorf("AttachmentPath(_, %q) = %q, want sentinel path", id, p)
		}
	}

	// The planted file must be untouched (DeleteCard on "../secret"
	// must not have removed it).
	if _, err := os.Stat(secret); err != nil {
		t.Fatalf("secret file outside repo was touched: %v", err)
	}

	// AddCardAttachment on a hostile ID must not have created any
	// attachment directories.
	if entries, err := os.ReadDir(filepath.Join(dir, "attachments")); err == nil && len(entries) > 0 {
		t.Errorf("attachments dir has %d entries after hostile calls, want none", len(entries))
	}
	// And nothing new may appear in the parent dir (an escaped MkdirAll
	// would land here).
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "repo" && e.Name() != "secret.json" {
			t.Errorf("unexpected entry %q escaped into parent dir", e.Name())
		}
	}
}

// TestAddCardAttachmentRequiresCard: the write must not happen before
// the card-existence check (a bogus ID used to create the directory
// and write the bytes anyway).
func TestAddCardAttachmentRequiresCard(t *testing.T) {
	r := setupTestRepo(t)
	if _, err := r.AddCardAttachment("no-such-card", "x.txt", "aGVsbG8="); err == nil {
		t.Fatal("AddCardAttachment on missing card succeeded, want error")
	}
	if fileExists(filepath.Join(r.Root, "attachments", "no-such-card")) {
		t.Fatal("attachment dir was created for a nonexistent card")
	}
}
