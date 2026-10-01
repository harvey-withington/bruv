package supervisor

import (
	"log/slog"

	"bruv/internal/config"
	"bruv/internal/mcp"
	"bruv/internal/repo"
)

// mcpApprovalFunc returns this machine's approval check for repoID's MCP
// servers (see internal/mcp/approval.go). Runs the one-time upgrade
// migration first so an existing install's servers keep running.
func mcpApprovalFunc(repoID string, specs []mcp.ServerSpec) mcp.ApprovalFunc {
	migrateMCPApprovals(repoID, specs)
	approvals := config.LoadMCPApprovals(repoID)
	return func(spec mcp.ServerSpec) bool {
		return approvals.Approved(spec.Name, spec.Fingerprint())
	}
}

// migrateMCPApprovals seeds the approvals file on the first run of a
// version with per-machine approval: every server currently enabled in a
// repo registered on this machine (repos.json) plus the repo being
// loaded is approved as it stands. Before this version all of them
// already ran on open, so this keeps an existing setup working; after
// it, the file exists and nothing is ever auto-approved again. The
// trade-off — a pre-enabled server from a shared repo that was already
// registered here is grandfathered in — is documented on
// config.SeedMCPApprovalsOnce.
func migrateMCPApprovals(repoID string, specs []mcp.ServerSpec) {
	count := 0
	migrated, err := config.SeedMCPApprovalsOnce(func() map[string]map[string]string {
		seed := map[string]map[string]string{}
		add := func(id string, servers []mcp.ServerSpec) {
			for _, spec := range servers {
				if !spec.Enabled || spec.Name == "" || id == "" {
					continue
				}
				if seed[id] == nil {
					seed[id] = map[string]string{}
				}
				if _, dup := seed[id][spec.Name]; !dup {
					count++
				}
				seed[id][spec.Name] = spec.Fingerprint()
			}
		}
		if store, err := config.LoadRepos(); err == nil {
			for _, e := range store.Repos {
				if s, err := repo.LoadMCPServerStoreAt(e.Path); err == nil {
					add(e.ID, s.Servers)
				}
			}
		}
		add(repoID, specs)
		return seed
	})
	if err != nil {
		slog.Warn("mcp approvals migration failed; enabled servers need approving", "err", err)
		return
	}
	if migrated {
		slog.Info("mcp approvals: one-time upgrade approved servers already enabled on this machine", "servers", count)
	}
}
