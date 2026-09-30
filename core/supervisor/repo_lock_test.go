package supervisor

import (
	"path/filepath"
	"strings"
	"testing"

	"bruv/internal/repo"
)

// Two runtimes on one repo folder stand in for two BRUV processes (the
// lock is per open file handle, so a second handle in this process is
// refused exactly like another process would be). The second must fail
// with a message that says who holds the folder, and the folder must be
// openable again once the first closes.
func TestSecondRuntimeOnSameRepoFolderIsRefused(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	if _, err := repo.InitAt(root, "Locked"); err != nil {
		t.Fatal(err)
	}
	cfg := t.TempDir()

	first, err := buildRuntime(root, cfg, []byte("secret"))
	if err != nil {
		t.Fatalf("first open: %v", err)
	}

	if second, err := buildRuntime(root, cfg, []byte("secret")); err == nil {
		second.close()
		first.close()
		t.Fatal("a second runtime opened a folder another one holds")
	} else if !strings.Contains(err.Error(), "already open in another BRUV process") || !strings.Contains(err.Error(), "pid") {
		t.Errorf("refusal doesn't say who holds the folder: %v", err)
	}

	first.close()
	again, err := buildRuntime(root, cfg, []byte("secret"))
	if err != nil {
		t.Fatalf("reopen after close: %v", err)
	}
	again.close()
}
