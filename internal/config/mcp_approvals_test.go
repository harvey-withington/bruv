package config

import (
	"os"
	"path/filepath"
	"testing"
)

func useTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	SetConfigDir(dir)
	t.Cleanup(func() { SetConfigDir("") })
	return dir
}

func TestMCPApprovalPersistsPerMachine(t *testing.T) {
	dir := useTempConfigDir(t)
	if LoadMCPApprovals("repo-a").Approved("fs", "fp1") {
		t.Fatal("nothing is approved before ApproveMCPServer")
	}
	if err := ApproveMCPServer("repo-a", "fs", "fp1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, mcpApprovalsFileName)); err != nil {
		t.Fatalf("approval must land in the machine config dir: %v", err)
	}
	a := LoadMCPApprovals("repo-a")
	if !a.Approved("fs", "fp1") {
		t.Error("approved server should read back as approved")
	}
	if a.Approved("fs", "fp2") {
		t.Error("a changed fingerprint must need re-approval")
	}
	if LoadMCPApprovals("repo-b").Approved("fs", "fp1") {
		t.Error("approvals are per repo")
	}

	if err := RevokeMCPServerApproval("repo-a", "fs"); err != nil {
		t.Fatal(err)
	}
	if LoadMCPApprovals("repo-a").Approved("fs", "fp1") {
		t.Error("revoked approval should be gone")
	}
	if err := RevokeMCPServerApproval("repo-a", "missing"); err != nil {
		t.Errorf("revoke is idempotent, got %v", err)
	}
}

func TestMCPApprovalsSeedRunsOnce(t *testing.T) {
	useTempConfigDir(t)
	calls := 0
	seed := func() map[string]map[string]string {
		calls++
		return map[string]map[string]string{"repo-a": {"fs": "fp1"}, "": {"x": "y"}}
	}
	migrated, err := SeedMCPApprovalsOnce(seed)
	if err != nil || !migrated {
		t.Fatalf("first run should seed: migrated=%v err=%v", migrated, err)
	}
	if !LoadMCPApprovals("repo-a").Approved("fs", "fp1") {
		t.Error("seeded server should be approved")
	}
	migrated, err = SeedMCPApprovalsOnce(seed)
	if err != nil || migrated || calls != 1 {
		t.Errorf("second run must not seed: migrated=%v err=%v calls=%d", migrated, err, calls)
	}
}

func TestMCPApprovalsSeedSkippedWhenFileExists(t *testing.T) {
	useTempConfigDir(t)
	// An install that recorded an approval (e.g. a fresh install where
	// the user added a server) must never be bulk-seeded later.
	if err := ApproveMCPServer("repo-a", "mine", "fp"); err != nil {
		t.Fatal(err)
	}
	migrated, err := SeedMCPApprovalsOnce(func() map[string]map[string]string {
		return map[string]map[string]string{"repo-b": {"shared": "fp"}}
	})
	if err != nil || migrated {
		t.Fatalf("seed must not run once the file exists: migrated=%v err=%v", migrated, err)
	}
	if LoadMCPApprovals("repo-b").Approved("shared", "fp") {
		t.Error("seed contents must not be applied")
	}
}

func TestMCPApprovalsCorruptFileFailsClosed(t *testing.T) {
	dir := useTempConfigDir(t)
	if err := os.WriteFile(filepath.Join(dir, mcpApprovalsFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if LoadMCPApprovals("repo-a").Approved("fs", "fp") {
		t.Error("corrupt file approves nothing")
	}
	if migrated, _ := SeedMCPApprovalsOnce(func() map[string]map[string]string {
		return map[string]map[string]string{"repo-a": {"fs": "fp"}}
	}); migrated {
		t.Error("corrupt file must not be replaced by a seed")
	}
	if err := ApproveMCPServer("repo-a", "fs", "fp"); err == nil {
		t.Error("approving over an unreadable file should fail rather than overwrite it")
	}
}
