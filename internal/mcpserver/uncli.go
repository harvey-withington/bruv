package mcpserver

// BRUV's own UNCLI chats (internal/uncliengine) call the board tools here
// as a BRUV chat: their calls carry config.UncliChatHeader. UNCLI applies
// BRUV's Chat / Suggest / Edit mode before a call gets here (the engine's
// HostToolPolicy: Chat refuses writes, Suggest asks the user on UNCLI's
// approval card, Edit runs them), so a call that arrives was allowed.
// This is BRUV's own check behind it: Chat-mode writes are refused here
// too, whatever UNCLI decided.

import (
	"bruv/core/boardtools"
	"bruv/core/supervisor"
	"bruv/internal/config"
	"bruv/internal/mcp"
)

func callAsUncliChat(rt *supervisor.Runtime, name string, args map[string]any) mcp.CallToolResult {
	if boardtools.IsWrite(name) {
		cfg, err := config.LoadLLMConfig()
		if err != nil {
			return toolError("couldn't read BRUV's chat mode: " + err.Error())
		}
		if cfg.AIMode == "chat" {
			return toolError(config.UncliChatModeRefusal)
		}
	}
	return boardtools.Call(rt, name, args)
}
