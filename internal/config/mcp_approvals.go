package config

// Per-machine approvals for repo-defined MCP servers.
//
// A repo's mcp_servers.json travels with the repo and can arrive with
// servers already marked "enabled". Enabled alone never runs anything:
// a server is spawned only when THIS machine has approved it, and the
// approval is bound to the spec's fingerprint (mcp.ServerSpec.Fingerprint
// — command, args, env names). Change any of those and the server needs
// approving again.
//
// Storage: <configDir>/mcp_approvals.json — machine-owned, never in the
// repo. Shape:
//
//	{
//	  "version": 1,
//	  "migrated_at": "2026-10-01T12:00:00Z",   // one-time upgrade seeding, see SeedMCPApprovalsOnce
//	  "repos": { "<repo manifest ID>": { "<server name>": "<sha256 fingerprint>" } }
//	}

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const mcpApprovalsFileName = "mcp_approvals.json"

// mcpApprovalsMu serialises every read-modify-write of the approvals
// file (several repo runtimes share one machine config).
var mcpApprovalsMu sync.Mutex

type mcpApprovalsFile struct {
	Version int `json:"version"`
	// MigratedAt is set when SeedMCPApprovalsOnce seeded the file on the
	// first run of a version with approvals. Informational: the file's
	// mere existence is what stops seeding from ever running again.
	MigratedAt string                       `json:"migrated_at,omitempty"`
	Repos      map[string]map[string]string `json:"repos"`
}

func mcpApprovalsPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, mcpApprovalsFileName), nil
}

// loadMCPApprovalsLocked reads the file. exists reports whether a file is
// on disk at all — true even when it fails to parse. Assumes the mutex.
func loadMCPApprovalsLocked() (f mcpApprovalsFile, exists bool, err error) {
	path, err := mcpApprovalsPath()
	if err != nil {
		return f, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return mcpApprovalsFile{Version: 1, Repos: map[string]map[string]string{}}, false, nil
	}
	if err != nil {
		return f, true, err
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return mcpApprovalsFile{}, true, fmt.Errorf("parse %s: %w", mcpApprovalsFileName, err)
	}
	if f.Repos == nil {
		f.Repos = map[string]map[string]string{}
	}
	return f, true, nil
}

func saveMCPApprovalsLocked(f mcpApprovalsFile) error {
	path, err := mcpApprovalsPath()
	if err != nil {
		return err
	}
	if f.Version == 0 {
		f.Version = 1
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data, 0o600)
}

// MCPApprovals is a read-only snapshot of one repo's approvals.
type MCPApprovals struct{ servers map[string]string }

// Approved reports whether server name is approved with exactly this
// fingerprint.
func (a MCPApprovals) Approved(name, fingerprint string) bool {
	fp, ok := a.servers[name]
	return ok && fingerprint != "" && fp == fingerprint
}

// LoadMCPApprovals returns repoID's approvals. Fails closed: an
// unreadable or corrupt file approves nothing (logged), so a damaged
// file can only stop servers, never start one.
func LoadMCPApprovals(repoID string) MCPApprovals {
	mcpApprovalsMu.Lock()
	defer mcpApprovalsMu.Unlock()
	f, _, err := loadMCPApprovalsLocked()
	if err != nil {
		slog.Warn("mcp approvals unreadable; no servers approved", "err", err)
		return MCPApprovals{}
	}
	return MCPApprovals{servers: f.Repos[repoID]}
}

// ApproveMCPServer records that server name in repoID may run on this
// machine while its spec has this fingerprint.
func ApproveMCPServer(repoID, name, fingerprint string) error {
	if repoID == "" || name == "" || fingerprint == "" {
		return fmt.Errorf("repo ID, server name and fingerprint are required")
	}
	mcpApprovalsMu.Lock()
	defer mcpApprovalsMu.Unlock()
	f, _, err := loadMCPApprovalsLocked()
	if err != nil {
		return err // never overwrite a file we couldn't read
	}
	if f.Repos[repoID] == nil {
		f.Repos[repoID] = map[string]string{}
	}
	f.Repos[repoID][name] = fingerprint
	return saveMCPApprovalsLocked(f)
}

// RevokeMCPServerApproval drops server name's approval in repoID.
// Idempotent.
func RevokeMCPServerApproval(repoID, name string) error {
	mcpApprovalsMu.Lock()
	defer mcpApprovalsMu.Unlock()
	f, exists, err := loadMCPApprovalsLocked()
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if _, ok := f.Repos[repoID][name]; !ok {
		return nil
	}
	delete(f.Repos[repoID], name)
	if len(f.Repos[repoID]) == 0 {
		delete(f.Repos, repoID)
	}
	return saveMCPApprovalsLocked(f)
}

// SeedMCPApprovalsOnce is the one-time upgrade migration. If the
// approvals file does not exist at all on this machine (first run of a
// version with approvals), it writes one containing seed() — the
// servers that were already enabled before the upgrade — and returns
// true. Once the file exists it never seeds again, whatever happens
// later; a corrupt file is left alone (fail closed).
//
// Trade-off: the seed can't tell a server the user enabled themselves
// from one that arrived pre-enabled in a shared repo — before this
// version both ran on open, so it keeps today's setup working exactly as
// it was. Every repo opened, and every server added or changed by a
// repo file, from then on needs explicit approval on this machine.
func SeedMCPApprovalsOnce(seed func() map[string]map[string]string) (bool, error) {
	mcpApprovalsMu.Lock()
	defer mcpApprovalsMu.Unlock()
	_, exists, err := loadMCPApprovalsLocked()
	if exists || err != nil {
		return false, err
	}
	f := mcpApprovalsFile{Version: 1, MigratedAt: time.Now().UTC().Format(time.RFC3339), Repos: map[string]map[string]string{}}
	for repoID, servers := range seed() {
		if repoID == "" || len(servers) == 0 {
			continue
		}
		f.Repos[repoID] = servers
	}
	if err := saveMCPApprovalsLocked(f); err != nil {
		return false, err
	}
	return true, nil
}
