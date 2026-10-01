package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
)

// Per-machine approval of MCP servers.
//
// mcp_servers.json travels with a repo, so a shared repo can arrive with
// servers already marked enabled. Enabled alone therefore never starts a
// subprocess: the server must also be approved on THIS machine, and the
// approval is bound to a fingerprint of everything that decides what gets
// executed. If a shared repo's author later changes the command, the
// fingerprint changes and the server stops until it is approved again.
// Approvals themselves live in the per-machine config dir (see
// internal/config/mcp_approvals.go), never in the repo.

// ApprovalFunc reports whether spec is approved to run on this machine.
// A nil ApprovalFunc approves nothing (fail closed).
type ApprovalFunc func(spec ServerSpec) bool

// Fingerprint returns a stable sha256 (hex) over the spec fields that
// determine what runs: Command, Args (order matters — they are
// positional) and EnvNames (sorted — order doesn't change the child's
// environment). Env *values* are not part of the spec: they are
// per-machine keychain entries the local user set, resolved at spawn
// time, so they never come from the repo file. There is no working-dir
// field; the child inherits BRUV's own. Name and Description are
// excluded: approvals are already keyed by name, and the description is
// display-only.
func (s ServerSpec) Fingerprint() string {
	envNames := slices.Clone(s.EnvNames)
	slices.Sort(envNames)
	payload := struct {
		Command  string   `json:"command"`
		Args     []string `json:"args"`
		EnvNames []string `json:"env_names"`
	}{
		Command:  s.Command,
		Args:     nonNilStrings(s.Args),
		EnvNames: nonNilStrings(envNames),
	}
	data, _ := json.Marshal(payload) // cannot fail: strings only
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// nonNilStrings normalises nil to empty so a missing "args" key and an
// empty "args" list fingerprint the same.
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
