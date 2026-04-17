package mcp

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// TestIntegrationServerProcessFilesystem exercises the ServerProcess
// lifecycle layer against the real filesystem server. Where
// integration_test.go wires the subprocess pipes directly to a
// Client to isolate the protocol layer, this one drives the whole
// ServerProcess abstraction so we know the spawn/supervise/shutdown
// path we actually use in production is correct.
//
// Skipped when npx is unavailable, same as the other integration
// test.
func TestIntegrationServerProcessFilesystem(t *testing.T) {
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available; skipping filesystem-server integration test")
	}
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	// Canonicalize so Windows 8.3 short-name expansion doesn't trip
	// the server's path-containment check. See canonical_path_windows.go.
	tmpDir := canonicalizeTempPath(t.TempDir())

	spec := ServerSpec{
		Name:        "filesystem-test",
		Description: "Integration test target",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-filesystem", tmpDir},
		Enabled:     true,
		// Cold CI runners need to npm-install the server package
		// before it can handshake — 90s covers that with headroom.
		// The outer context below is sized to match.
		InitTimeout: 90 * time.Second,
	}

	sp := NewServerProcess(spec, "test-repo-id", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := sp.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		if err := sp.Stop(); err != nil {
			t.Logf("stop: %v", err)
		}
	})

	health := sp.Health()
	if health.Status != HealthReady {
		t.Fatalf("health status = %q, want %q (lastError=%s)", health.Status, HealthReady, health.LastError)
	}
	if health.ToolCount == 0 {
		t.Errorf("expected ToolCount > 0, got 0")
	}
	if health.ProtocolVersion == "" {
		t.Errorf("expected ProtocolVersion to be populated")
	}
	if health.ServerName == "" {
		t.Errorf("expected ServerName to be populated")
	}
	t.Logf("health: %+v", health)

	// Call list_allowed_directories through the ServerProcess API
	// (which routes through the embedded Client). This is the
	// exact code path the agent dispatch uses.
	result, err := sp.CallTool(ctx, "list_allowed_directories", nil)
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Errorf("tool returned isError: %s", FlattenContent(result.Content))
	}
	flat := FlattenContent(result.Content)
	if flat == "" {
		t.Error("expected non-empty tool result")
	}
	t.Logf("tool result: %s", flat)
}
