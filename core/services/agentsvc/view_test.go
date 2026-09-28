package agentsvc

import (
	"slices"
	"strings"
	"testing"

	"bruv/internal/model"
)

// Agents are granted native board tools as well as their built-ins, and
// a legacy tool id from before the native set reads and saves as its
// replacement.
func TestConfigureAcceptsNativeToolsAndCanonicalisesLegacyIDs(t *testing.T) {
	svc, deps := newTestService(t)
	c, err := deps.r.CreateCard("", "Watcher")
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Configure(c.ID, map[string]any{"goal": "g", "allowed_tools": []any{"read_card", "update_card", "notify"}})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if got := res.Config.AllowedTools; !slices.Equal(got, []string{"get_card", "update_card", "notify"}) {
		t.Errorf("allowed_tools = %v, want read_card stored as get_card", got)
	}
	if _, err := svc.Configure(c.ID, map[string]any{"allowed_tools": []any{"launch_rockets"}}); err == nil || !strings.Contains(err.Error(), "launch_rockets") {
		t.Errorf("an unknown tool id should be rejected, got %v", err)
	}
}

func TestConfigureRefusesAgentManagementTools(t *testing.T) {
	svc, deps := newTestService(t)
	c, _ := deps.r.CreateCard("", "Watcher")
	for _, id := range []string{"run_card_agent", "configure_card_agent"} {
		if _, err := svc.Configure(c.ID, map[string]any{"goal": "g", "allowed_tools": []any{id}}); err == nil || !strings.Contains(err.Error(), "never start or reconfigure agents") {
			t.Errorf("granting %s should be refused, got %v", id, err)
		}
	}
}

func TestConfigureWarnsWhenNoToolsAreGranted(t *testing.T) {
	svc, deps := newTestService(t)
	c, _ := deps.r.CreateCard("", "Watcher")
	res, err := svc.Configure(c.ID, map[string]any{"enabled": true, "goal": "g"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(res.Warnings, func(w string) bool { return strings.Contains(w, "no tools are allowed") }) {
		t.Errorf("want a no-tools warning, got %v", res.Warnings)
	}
	if !strings.Contains(res.Summary, "tools: none") {
		t.Errorf("summary should say no tools, got %q", res.Summary)
	}
}

func TestGetConfigReadsLegacyToolIDsAsCurrent(t *testing.T) {
	svc, deps := newTestService(t)
	c, _ := deps.r.CreateCard("", "Old agent")
	if err := deps.r.SaveAgentConfig(c.ID, model.AgentConfig{Goal: "g", AllowedTools: []string{"read_card", "web_search"}}); err != nil {
		t.Fatal(err)
	}
	af, err := svc.GetConfig(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(af.Config.AllowedTools, []string{"get_card", "web_search"}) {
		t.Errorf("allowed_tools = %v", af.Config.AllowedTools)
	}
}

func TestDescribeReturnsTheFullGoal(t *testing.T) {
	svc, deps := newTestService(t)
	c, _ := deps.r.CreateCard("", "Watcher")
	long := strings.Repeat("Check the race card. ", 50)
	if _, err := svc.Configure(c.ID, map[string]any{"goal": long}); err != nil {
		t.Fatal(err)
	}
	view, err := svc.Describe(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Config.Goal != strings.TrimSpace(long) {
		t.Errorf("goal truncated: %d chars, want %d", len(view.Config.Goal), len(strings.TrimSpace(long)))
	}
	if !slices.ContainsFunc(view.Options.Tools, func(o ToolOption) bool { return o.ID == "update_card" }) {
		t.Error("options.tools should list the native tools")
	}
}
