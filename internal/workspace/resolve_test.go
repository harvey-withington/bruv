package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveAcceptsSafePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"", ".", "a", "a/b", "a/b/c.txt", "./a", "a/../a/b",
		"a/b/not-yet-created/deep.txt", // creation targets resolve too
		"weird name with spaces.md",
	} {
		abs, err := Resolve(root, rel)
		if err != nil {
			t.Errorf("Resolve(%q) unexpected error: %v", rel, err)
			continue
		}
		if !strings.HasPrefix(strings.ToLower(abs), strings.ToLower(root)) {
			t.Errorf("Resolve(%q) = %q, outside root %q", rel, abs, root)
		}
	}
}

func TestResolveRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	cases := []string{
		"..",
		"../x",
		"a/../../x",
		"a/../..",
		"/etc/passwd",
		`\windows\system32`, // backslash-rooted is rejected on every platform
		"../" + filepath.Base(root),
	}
	if runtime.GOOS == "windows" {
		// Drive-qualified forms are absolute-intent ONLY on Windows; on
		// Linux/macOS "C:evil.txt" is a legal relative filename and must
		// be accepted — do not assert rejection there.
		cases = append(cases, `C:\Windows`, "C:evil.txt")
	}
	for _, rel := range cases {
		if _, err := Resolve(root, rel); !errors.Is(err, ErrPathEscapes) {
			t.Errorf("Resolve(%q) = %v, want ErrPathEscapes", rel, err)
		}
	}
}

func TestResolveRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "sneaky")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err) // Windows without developer mode
	}

	if _, err := Resolve(root, "sneaky"); !errors.Is(err, ErrPathEscapes) {
		t.Errorf("symlink dir itself: got %v, want ErrPathEscapes", err)
	}
	if _, err := Resolve(root, "sneaky/file.txt"); !errors.Is(err, ErrPathEscapes) {
		t.Errorf("through symlink dir: got %v, want ErrPathEscapes", err)
	}
	if _, err := Resolve(root, "sneaky/new-dir/new.txt"); !errors.Is(err, ErrPathEscapes) {
		t.Errorf("creation target through symlink: got %v, want ErrPathEscapes", err)
	}
}

func TestResolveAllowsInternalSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "alias")
	if err := os.Symlink(filepath.Join(root, "real"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Resolve(root, "alias"); err != nil {
		t.Errorf("symlink inside the root should be allowed: %v", err)
	}
}
