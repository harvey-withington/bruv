package agent

// Agents use BRUV's native board tools — the same registry the MCP
// server and chat call — but only the ones granted in allowed_tools.
// An empty list grants nothing, and the agent-management tools
// (configure_card_agent, run_card_agent) are never granted. Legacy ids
// from before the native set (read_card) still grant their replacement
// (get_card).

import "bruv/internal/llm"

// nativeToolDefs returns the native tools this agent was granted.
func (rt *Runtime) nativeToolDefs(allowed []string) []llm.ToolDef {
	n := rt.deps.Native()
	if n == nil || len(allowed) == 0 {
		return nil
	}
	ok := allowedSet(allowed)
	var out []llm.ToolDef
	for _, d := range n.Defs(false) {
		if ok[d.Name] && llm.AgentMayUse(d.Name) {
			out = append(out, d)
		}
	}
	return out
}

func allowedSet(allowed []string) map[string]bool {
	set := make(map[string]bool, len(allowed))
	for _, id := range allowed {
		set[llm.CanonicalAgentToolID(id)] = true
	}
	return set
}

// offeredSet names the tools a run actually offered the model. Execution
// is limited to it: a model can name a tool it wasn't given, and an agent
// must never run one it wasn't granted.
func offeredSet(defs []llm.ToolDef) map[string]bool {
	set := make(map[string]bool, len(defs))
	for _, d := range defs {
		set[d.Name] = true
	}
	return set
}
