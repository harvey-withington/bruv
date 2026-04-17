package mcp

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// End-to-end integration test against the reference filesystem MCP
// server (@modelcontextprotocol/server-filesystem). Unlike the pure
// client tests in client_test.go, this one actually spawns a Node
// subprocess via npx and exercises the full ServerProcess lifecycle:
// spawn, handshake, tools/list, tools/call, shutdown.
//
// Skipped when npx isn't available in PATH (Node not installed, or
// the test is running somewhere we can't reach it) — this keeps the
// test suite green in environments where we can't exercise the real
// path, while still running the check locally and in CI where Node
// is available for the frontend build anyway.
//
// This is the highest-confidence test we have that the whole MCP
// stack actually works end-to-end. If the pure-Go tests all pass
// but this one fails, the bug is in the subprocess/pipe wiring
// layer, not the protocol implementation.

func TestIntegrationFilesystemServer(t *testing.T) {
	// Skip if npx isn't available — MCP servers typically require
	// Node, and we don't want our own test suite to fail on
	// machines without it.
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available; skipping filesystem-server integration test")
	}
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	// Create a temp directory for the filesystem server to operate
	// on — this is the server's "sandbox root" which it enforces
	// as the outer boundary for all file operations.
	//
	// canonicalizeTempPath handles a Windows CI quirk: GitHub Actions'
	// runneradmin user (>8 chars) triggers 8.3 short-name expansion,
	// causing the MCP filesystem server to compare long-form allowed
	// dir against short-form target paths and reject its own sandbox.
	// See canonical_path_windows.go for the full explanation.
	tmpDir := canonicalizeTempPath(t.TempDir())
	if err := os.WriteFile(tmpDir+"/hello.txt", []byte("world"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	command := "npx"
	args := []string{"-y", "@modelcontextprotocol/server-filesystem", tmpDir}
	if runtime.GOOS == "windows" {
		// Windows npx is actually npx.cmd — our server.go handles
		// this by wrapping in cmd /c but the test needs the same
		// treatment since we're bypassing ServerProcess here for
		// tighter lifecycle control in the test.
		args = append([]string{"/c", "npx"}, args[1:]...)
		command = "cmd"
	}

	// Spawn the subprocess with a fresh pipe per stream.
	cmd := exec.Command(command, args...)
	cmd.Env = os.Environ() // filesystem server doesn't need secrets

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	// Guaranteed cleanup even if the test fails mid-flight.
	t.Cleanup(func() {
		stdin.Close()
		done := make(chan struct{})
		go func() {
			cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			cmd.Process.Kill()
		}
	})

	transport := NewTransport("filesystem", stdin, stdout, stderr, nil)
	transport.Start()
	client := NewClient(transport)

	// Generous timeout — npx cold start can take 5-15 seconds on
	// first invocation when it has to download the package.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	t.Logf("protocol version: %s", client.ProtocolVersion())
	t.Logf("server info: %+v", client.ServerInfo())

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected at least one tool from filesystem server")
	}
	t.Logf("discovered %d tools", len(tools))

	// Verify we see one of the tools we expect from the filesystem
	// server. `list_allowed_directories` is a safe bet because it
	// takes no arguments and returns a text block listing the
	// directories we passed on argv.
	var hasListDirs bool
	for _, tool := range tools {
		if tool.Name == "list_allowed_directories" {
			hasListDirs = true
			break
		}
	}
	if !hasListDirs {
		t.Errorf("expected tool 'list_allowed_directories' not found in tool list")
	}

	// Call list_allowed_directories as a round-trip smoke test of
	// the full request → response → content-flatten pipeline.
	result, err := client.CallTool(ctx, "list_allowed_directories", map[string]interface{}{})
	if err != nil {
		t.Fatalf("call list_allowed_directories: %v", err)
	}
	if result.IsError {
		t.Errorf("tool returned isError=true: %s", FlattenContent(result.Content))
	}
	flat := FlattenContent(result.Content)
	if flat == "" {
		t.Error("expected non-empty result from list_allowed_directories")
	}
	t.Logf("list_allowed_directories returned: %s", flat)

	// Call a real file-reading tool to prove we can pass arguments
	// and get non-trivial content back. list_directory returns the
	// entries in a directory; we should see our hello.txt file.
	result, err = client.CallTool(ctx, "list_directory", map[string]interface{}{
		"path": tmpDir,
	})
	if err != nil {
		t.Fatalf("call list_directory: %v", err)
	}
	if result.IsError {
		t.Errorf("list_directory returned isError=true: %s", FlattenContent(result.Content))
	}
	flat = FlattenContent(result.Content)
	if flat == "" || !containsHelper(flat, "hello.txt") {
		t.Errorf("expected list_directory to show hello.txt, got: %q", flat)
	}

	// Clean shutdown.
	if err := client.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}
