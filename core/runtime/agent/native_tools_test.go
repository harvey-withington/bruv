package agent

// Agents reach BRUV's native board tools through the registry — board-
// wide (no chat scope) and filtered by allowed_tools, with legacy ids
// (read_card) still granting their replacement (get_card).

import (
	"context"
	"testing"

	"bruv/core/runtime/tools"
	"bruv/internal/llm"
)

// fakeNative stands in for core/boardtools and records each call.
type fakeNative struct {
	calls  []string
	scopes []*tools.ProjectChatScope
}

func (f *fakeNative) Defs(bool) []llm.ToolDef {
	return []llm.ToolDef{{Name: "get_card"}, {Name: "create_card"}, {Name: "update_card"}, {Name: "run_card_agent"}, {Name: "configure_card_agent"}}
}
func (f *fakeNative) Has(name string) bool {
	return name == "get_card" || name == "create_card" || name == "update_card"
}
func (f *fakeNative) IsWrite(name string) bool { return name != "get_card" }
func (f *fakeNative) Check(*tools.ProjectChatScope, string, map[string]any) error {
	return nil
}
func (f *fakeNative) Call(scope *tools.ProjectChatScope, name string, _ map[string]any) (string, bool) {
	f.calls = append(f.calls, name)
	f.scopes = append(f.scopes, scope)
	return `{"card_id":"c9","title":"Race 5"}`, false
}
func (f *fakeNative) Summary(name string, _ map[string]any, _ string) string { return "did " + name }

func TestNativeToolsRouteBoardWide(t *testing.T) {
	a, r := testRuntime(t)
	native := a.deps.(*toolsTestDeps).native.(*fakeNative)
	agentID := testCard(t, r, "Watcher", nil)
	agentCard, _ := r.GetCard(agentID)

	result, action := a.executeAgentToolCall(context.Background(), agentID, agentCard, call("create_card", map[string]any{"title": "Race 5"}))
	if len(native.calls) != 1 || native.calls[0] != "create_card" {
		t.Fatalf("native calls = %v, want create_card routed to the registry", native.calls)
	}
	if native.scopes[0] != nil {
		t.Error("agents call native tools board-wide (nil scope)")
	}
	if result != `{"card_id":"c9","title":"Race 5"}` || action == nil || action.Result != "did create_card" {
		t.Errorf("result=%q action=%+v", result, action)
	}
}

func TestNativeToolDefsFollowAllowedTools(t *testing.T) {
	a, _ := testRuntime(t)
	names := func(allowed []string) map[string]bool {
		out := map[string]bool{}
		for _, d := range a.nativeToolDefs(allowed) {
			out[d.Name] = true
		}
		return out
	}
	// No permissions means no tools — the Agent tab shows nothing ticked,
	// and that must be what the agent gets (field report 2026-09-29: an
	// "unticked" agent really had every tool and ran up a 1M-token run).
	if got := names(nil); len(got) != 0 {
		t.Errorf("an empty allowed list must grant no native tools, got %v", got)
	}
	if got := a.nativeToolDefs(nil); got != nil {
		t.Errorf("want nil defs for no permissions, got %v", got)
	}
	// An agent can never be granted the agent-management tools, even by name.
	if got := names([]string{"run_card_agent", "configure_card_agent", "get_card"}); got["run_card_agent"] || got["configure_card_agent"] || !got["get_card"] {
		t.Errorf("agents must not get run/configure_card_agent, got %v", got)
	}
	// A saved agent from before the native set: read_card now means get_card.
	if got := names([]string{"read_card", "update_self"}); !got["get_card"] || got["create_card"] {
		t.Errorf("legacy read_card should grant get_card only, got %v", got)
	}

}
