package mcp

import "testing"

// Registry tests that don't need real subprocesses. The live
// initialize/tools/list/tools/call paths are covered by
// transport_test.go and client_test.go via in-memory pipes.

func TestNamespaceToolRoundTrip(t *testing.T) {
	cases := []struct {
		server, tool, id string
	}{
		{"filesystem", "read_text_file", "filesystem__read_text_file"},
		{"github", "list_issues", "github__list_issues"},
		// Tool names can contain underscores themselves — only the
		// double-underscore separator should be treated as a split.
		{"my_server", "my_tool", "my_server__my_tool"},
	}
	for _, tc := range cases {
		got := NamespaceTool(tc.server, tc.tool)
		if got != tc.id {
			t.Errorf("NamespaceTool(%q, %q) = %q, want %q", tc.server, tc.tool, got, tc.id)
		}
		srv, tool := SplitNamespacedTool(got)
		if srv != tc.server || tool != tc.tool {
			t.Errorf("SplitNamespacedTool(%q) = (%q, %q), want (%q, %q)", got, srv, tool, tc.server, tc.tool)
		}
	}
}

func TestSplitNamespacedToolFallback(t *testing.T) {
	// IDs without the separator are treated as plain tool names
	// with an empty server — the fallback for built-in tools that
	// never went through namespacing.
	srv, tool := SplitNamespacedTool("web_search")
	if srv != "" || tool != "web_search" {
		t.Errorf("unqualified tool: got (%q, %q), want ('', 'web_search')", srv, tool)
	}
}

func TestRegistryEmptyStartup(t *testing.T) {
	// A registry with no servers should come up cleanly and return
	// empty everything. This is the "fresh repo" case.
	r := NewRegistry("test-repo", nil)
	errs := r.LoadAndStart(testContext(t), nil)
	if len(errs) != 0 {
		t.Errorf("empty specs should not produce errors, got %v", errs)
	}
	if len(r.Tools()) != 0 {
		t.Errorf("empty registry should have no tools")
	}
	if len(r.Health()) != 0 {
		t.Errorf("empty registry should have no health entries")
	}
	r.Shutdown()
}

func TestRegistryDisabledServer(t *testing.T) {
	// A disabled server is tracked (so the UI can show it and
	// toggle it back on) but not spawned. Trying to start one
	// should not fail and should not contribute any tools.
	r := NewRegistry("test-repo", nil)
	specs := []ServerSpec{
		{Name: "disabled-one", Command: "nonexistent", Enabled: false},
	}
	errs := r.LoadAndStart(testContext(t), specs)
	if len(errs) != 0 {
		t.Errorf("disabled server should not produce errors, got %v", errs)
	}
	if len(r.Tools()) != 0 {
		t.Errorf("disabled server contributes no tools")
	}
	if len(r.Health()) != 1 {
		t.Errorf("disabled server should still appear in health list, got %d entries", len(r.Health()))
	}
	if r.Health()[0].Status != HealthDisabled {
		t.Errorf("disabled server status: got %q, want %q", r.Health()[0].Status, HealthDisabled)
	}
	r.Shutdown()
}

func TestRegistryDuplicateName(t *testing.T) {
	// Two specs with the same name — the second should be
	// rejected with an error. This prevents ambiguous tool
	// dispatch and is enforced at load time rather than save
	// time so we catch it even if the config file was edited by
	// hand.
	r := NewRegistry("test-repo", nil)
	specs := []ServerSpec{
		{Name: "dup", Command: "x", Enabled: false},
		{Name: "dup", Command: "y", Enabled: false},
	}
	errs := r.LoadAndStart(testContext(t), specs)
	if _, ok := errs["dup"]; !ok {
		t.Errorf("expected duplicate name error, got errs=%v", errs)
	}
	r.Shutdown()
}

func TestRegistryStartFailure(t *testing.T) {
	// A server with a nonexistent command should land in
	// HealthFailed, and its error should appear in the startup
	// error map, but it should not block other servers from
	// starting.
	r := NewRegistry("test-repo", nil)
	specs := []ServerSpec{
		{Name: "broken", Command: "this-command-definitely-does-not-exist-xyz123", Enabled: true},
	}
	errs := r.LoadAndStart(testContext(t), specs)
	if _, ok := errs["broken"]; !ok {
		t.Errorf("expected startup error for broken server, got errs=%v", errs)
	}
	health := r.Health()
	if len(health) != 1 {
		t.Fatalf("expected 1 health entry, got %d", len(health))
	}
	if health[0].Status != HealthFailed {
		t.Errorf("expected HealthFailed, got %q", health[0].Status)
	}
	if health[0].LastError == "" {
		t.Errorf("expected LastError to be populated on failure")
	}
	r.Shutdown()
}

func TestRegistryOwnsToolUnknown(t *testing.T) {
	r := NewRegistry("test-repo", nil)
	if r.OwnsTool("nonexistent__tool") {
		t.Error("empty registry should not claim any tool")
	}
}
