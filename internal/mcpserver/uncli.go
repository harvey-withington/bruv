package mcpserver

// BRUV's own UNCLI chats (internal/uncliengine) call the board tools here
// as a BRUV chat: their calls carry config.UncliChatHeader, and the Chat /
// Suggest / Edit mode (ai_mode, the same setting as BRUV's own chat)
// decides what their writes do, at call time, so switching mid-chat
// works. Reads always run.
//
//   - Chat: writes are refused, with a reason the model can act on.
//   - Suggest: writes are staged for the user's review on the chat's own
//     history (supervisor.StageUncliEdit), within its project like BRUV's
//     own chat in Suggest mode.
//   - Edit: writes run, as for any other MCP client.

import (
	"strings"

	"bruv/core/boardtools"
	"bruv/core/supervisor"
	"bruv/internal/config"
	"bruv/internal/mcp"
)

const (
	refusedInChatMode = "This chat is in Chat mode, so it can't change the board. " +
		"Describe the change instead, or ask the user to switch the chat to Suggest or Edit."
	refusedUnlinked = "This chat is in Suggest mode but is no longer one of BRUV's card or project chats, " +
		"so there's nowhere to stage the change. Describe it instead, or ask the user to switch to Edit."
)

func callAsUncliChat(rt *supervisor.Runtime, session, name string, args map[string]any) mcp.CallToolResult {
	if !boardtools.IsWrite(name) {
		return boardtools.Call(rt, name, args)
	}
	cfg, err := config.LoadLLMConfig()
	if err != nil {
		return toolError("couldn't read BRUV's chat mode: " + err.Error())
	}
	switch cfg.AIMode {
	case "chat":
		return toolError(refusedInChatMode)
	case "suggest":
		chat, ok := config.UncliSessionChat(session, 0)
		if !ok {
			return toolError(refusedUnlinked)
		}
		result, err := rt.StageUncliEdit(chat.ChatID, name, args)
		if err != nil {
			return toolError("couldn't stage the change: " + err.Error())
		}
		// A call the staging bounced (out of scope, bad arguments) reads
		// "error: …", as it does for BRUV's own chat.
		return mcp.CallToolResult{Content: []mcp.Content{{Type: "text", Text: result}}, IsError: strings.HasPrefix(result, "error:")}
	default:
		return boardtools.Call(rt, name, args)
	}
}
