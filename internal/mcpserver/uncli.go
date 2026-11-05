package mcpserver

// BRUV's own UNCLI chats (internal/uncliengine) call the board tools here
// as a BRUV chat: their calls carry config.UncliChatHeader. UNCLI lets
// their writes through in Suggest and Edit (the engine's HostToolPolicy;
// it refuses them in Chat), and BRUV's Chat / Suggest / Edit mode decides
// here what they do:
//
//   - Chat: refused (again, behind UNCLI).
//   - Suggest: staged on the chat's own history (supervisor.StageUncliEdit),
//     for the page to ask about once the turn ends, on one UNCLI review
//     card (shared/uncliChat.reviewStagedChanges).
//   - Edit: applied.
//
// Every write answers with an explicit status ("applied", "staged",
// "refused" or "error"), so the model never takes a staged change for a
// done one. Reads answer as for any MCP client.

import (
	"encoding/json"

	"bruv/core/boardtools"
	"bruv/core/supervisor"
	"bruv/internal/config"
	"bruv/internal/mcp"
)

const (
	refusedUnlinked = "This chat is no longer one of BRUV's card or project chats, so there's nowhere to stage the change. " +
		"Describe it instead, or ask the user to switch the chat to Edit."
	stagedNote = "Not applied yet: the user reviews this turn's suggested changes when you finish, and applies the ones they keep. " +
		"Don't make the same change again; say what you suggested."
)

// uncliCaller is one of BRUV's own UNCLI chats calling the repo repoID:
// its session (config.UncliChatHeader) and the meta naming its chat
// (config.UncliChatMetaHeader). No session for any other client.
type uncliCaller struct{ repoID, session, meta string }

// chat is the chat the caller is (supervisor.UncliChatID).
func (c uncliCaller) chat(rt *supervisor.Runtime) (string, bool) {
	if c.session == "" {
		return "", false
	}
	return rt.UncliChatID(c.repoID, c.meta, c.session)
}

// brief is what the caller's model is told about its chat on connecting,
// after the board's instructions; "" for any other client.
func (c uncliCaller) brief(rt *supervisor.Runtime) string {
	chatID, ok := c.chat(rt)
	if !ok {
		return ""
	}
	if brief := rt.UncliChatBrief(chatID); brief != "" {
		return "\n\n## This chat\n" + brief
	}
	return ""
}

func callAsUncliChat(rt *supervisor.Runtime, caller uncliCaller, name string, args map[string]any) mcp.CallToolResult {
	if !boardtools.IsWrite(name) {
		return boardtools.Call(rt, name, args)
	}
	cfg, err := config.LoadLLMConfig()
	if err != nil {
		return statusResult(true, map[string]any{"status": "error", "error": "couldn't read BRUV's chat mode: " + err.Error()})
	}
	switch cfg.AIMode {
	case "chat":
		return statusResult(true, map[string]any{"status": "refused", "reason": config.UncliChatModeRefusal})
	case "suggest":
		chatID, ok := caller.chat(rt)
		if !ok {
			return statusResult(true, map[string]any{"status": "refused", "reason": refusedUnlinked})
		}
		staged, refused, err := rt.StageUncliEdit(chatID, name, args)
		switch {
		case err != nil:
			return statusResult(true, map[string]any{"status": "error", "error": "couldn't stage the change: " + err.Error()})
		case refused != "":
			return statusResult(true, map[string]any{"status": "refused", "reason": refused})
		}
		changes := make([]string, 0, len(staged))
		for _, e := range staged {
			changes = append(changes, e.Label)
		}
		return statusResult(false, map[string]any{"status": "staged", "changes": changes, "note": stagedNote})
	default:
		return withAppliedStatus(boardtools.Call(rt, name, args))
	}
}

// withAppliedStatus puts the status on an applied write's answer: its
// JSON object gains "status": "applied" ("error" when the tool failed);
// anything else is wrapped as the result.
func withAppliedStatus(res mcp.CallToolResult) mcp.CallToolResult {
	text := ""
	if len(res.Content) > 0 {
		text = res.Content[0].Text
	}
	if res.IsError {
		return statusResult(true, map[string]any{"status": "error", "error": text})
	}
	var out map[string]any
	if json.Unmarshal([]byte(text), &out) != nil || out == nil {
		out = map[string]any{"result": text}
	}
	out["status"] = "applied"
	return statusResult(false, out)
}

func statusResult(isErr bool, v map[string]any) mcp.CallToolResult {
	b, err := json.Marshal(v)
	if err != nil {
		return toolError(err.Error())
	}
	return mcp.CallToolResult{Content: []mcp.Content{{Type: "text", Text: string(b)}}, IsError: isErr}
}
