package supervisor

import (
	"testing"

	"bruv/internal/config"
	"bruv/internal/mcp"
)

// TestMCPApprovalMigrationThenGate: the first load after the upgrade
// grandfathers servers already enabled; anything that arrives or changes
// afterwards (a shared repo's new server, an edited command) is not
// approved until the user approves it on this machine.
func TestMCPApprovalMigrationThenGate(t *testing.T) {
	config.SetConfigDir(t.TempDir())
	t.Cleanup(func() { config.SetConfigDir("") })

	mine := mcp.ServerSpec{Name: "mine", Command: "npx", Args: []string{"-y", "server"}, Enabled: true}
	off := mcp.ServerSpec{Name: "off", Command: "npx", Enabled: false}

	approved := mcpApprovalFunc("repo-1", []mcp.ServerSpec{mine, off})
	if !approved(mine) {
		t.Fatal("migration should approve a server already enabled before the upgrade")
	}
	off.Enabled = true
	if approved(off) {
		t.Error("migration only approves servers that were enabled")
	}

	// Later loads: the migration never runs again.
	shared := mcp.ServerSpec{Name: "shared", Command: "powershell", Args: []string{"-c", "evil"}, Enabled: true}
	approved = mcpApprovalFunc("repo-2", []mcp.ServerSpec{shared})
	if approved(shared) {
		t.Fatal("a pre-enabled server in a repo opened after the upgrade must not be approved")
	}

	changed := mine
	changed.Args = []string{"-y", "other-server"}
	approved = mcpApprovalFunc("repo-1", []mcp.ServerSpec{changed})
	if approved(changed) {
		t.Error("a changed command needs re-approval")
	}
	if !approved(mine) {
		t.Error("the unchanged spec stays approved")
	}

	if err := config.ApproveMCPServer("repo-2", shared.Name, shared.Fingerprint()); err != nil {
		t.Fatal(err)
	}
	if !mcpApprovalFunc("repo-2", nil)(shared) {
		t.Error("an explicit approval should take effect on the next load")
	}
}
