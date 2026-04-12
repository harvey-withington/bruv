package config

// Keychain-backed storage for MCP server environment variable secrets.
//
// MCP servers typically need API keys and tokens via env vars (KIWI_API_KEY,
// GITHUB_TOKEN, etc.). Sprint B established the pattern of storing LLM API
// keys in the OS keychain rather than plaintext; this file does the same
// thing for MCP secrets, keyed so that each repo + server + variable has
// its own isolated slot.
//
// Key format:
//
//	mcp:<repoID>:<serverName>:<varName>
//
// Everything is user-scoped: two BRUV users on the same machine with the
// same repo open see different keychain entries because they run under
// different OS user accounts. Two repos on the same machine with the same
// server name see different entries because the repoID is part of the key.
// This is exactly the isolation the portability sprint established for
// chat history — same reasoning, same mechanism.
//
// When a repo is shared (zip, git, cloud sync), the keychain entries stay
// on the original machine. The recipient opens the repo, sees the MCP
// server config listing env var *names* with no values, and has to fill
// them in themselves. This is the desired behaviour — API keys must never
// travel with a shared repo.

import (
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"
)

// mcpSecretKey builds the keychain entry name for a given repo + server
// + env var tuple. All three components are sanitised because keyring
// backends can be fussy about special characters — colons and slashes
// are the main risk.
func mcpSecretKey(repoID, serverName, varName string) string {
	sanitize := func(s string) string {
		// Colons are the separator, so they can't appear in any
		// component. Backslashes and control chars are similarly
		// problematic on Windows Credential Manager. Replace with
		// underscores — we never need to reverse this mapping.
		r := strings.NewReplacer(":", "_", "\\", "_", "/", "_", "\n", "_", "\r", "_")
		return r.Replace(s)
	}
	return fmt.Sprintf("mcp:%s:%s:%s", sanitize(repoID), sanitize(serverName), sanitize(varName))
}

// SetMCPSecret stores a secret value in the OS keychain. Returns an
// error if the keychain is unavailable — callers must decide whether
// to surface this to the user or fall back to unset (no plaintext
// fallback for MCP secrets, unlike LLM keys, because per-repo
// plaintext would leak on repo share).
func SetMCPSecret(repoID, serverName, varName, value string) error {
	if !KeychainAvailable() {
		return fmt.Errorf("OS keychain is not available on this system")
	}
	return keyring.Set(keychainService, mcpSecretKey(repoID, serverName, varName), value)
}

// GetMCPSecret fetches a previously-stored secret. Returns ("", false)
// if the entry doesn't exist or the keychain is unavailable. Missing
// entries are not an error — they're the normal state for a
// just-imported repo where the user hasn't set their own keys yet.
func GetMCPSecret(repoID, serverName, varName string) (string, bool) {
	if !KeychainAvailable() {
		return "", false
	}
	val, err := keyring.Get(keychainService, mcpSecretKey(repoID, serverName, varName))
	if err != nil {
		return "", false
	}
	return val, true
}

// DeleteMCPSecret removes a stored secret. Idempotent — missing
// entries are not an error, so the Settings UI can call this
// unconditionally on server removal without checking first.
func DeleteMCPSecret(repoID, serverName, varName string) error {
	if !KeychainAvailable() {
		return nil
	}
	err := keyring.Delete(keychainService, mcpSecretKey(repoID, serverName, varName))
	if err != nil && err != keyring.ErrNotFound {
		return err
	}
	return nil
}

// MCPSecretResolver adapts the package-level secret helpers to the
// mcp.SecretResolver interface so the mcp package can look up values
// without importing internal/config directly (which would create a
// cycle — internal/config already imports internal/mcp through the
// repo package).
//
// Used by the Registry wiring in app.go: construct one of these and
// pass it to mcp.NewRegistry.
type MCPSecretResolver struct{}

// Lookup implements mcp.SecretResolver.
func (MCPSecretResolver) Lookup(repoID, serverName, envVarName string) (string, bool) {
	return GetMCPSecret(repoID, serverName, envVarName)
}
