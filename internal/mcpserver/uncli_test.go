package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bruv/core/supervisor"
	"bruv/internal/config"
)

// callAs drives a tools/call as one of BRUV's UNCLI chats (session "" =
// any other MCP client).
func callAs(t *testing.T, h *Handler, session, name string, args map[string]any) (string, bool) {
	t.Helper()
	params, _ := json.Marshal(map[string]any{"name": name, "arguments": args})
	payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": json.RawMessage(params)})
	req := httptest.NewRequest(http.MethodPost, "/repos/"+testRepoID+"/mcp", strings.NewReader(string(payload)))
	if session != "" {
		req.Header.Set(config.UncliChatHeader, session)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || len(resp.Result.Content) == 0 {
		t.Fatalf("tools/call %s: %q (%v)", name, rec.Body.String(), err)
	}
	return resp.Result.Content[0].Text, resp.Result.IsError
}

func setAIMode(t *testing.T, mode string) {
	t.Helper()
	if err := config.SaveLLMConfig(config.LLMConfig{AIMode: mode}); err != nil {
		t.Fatal(err)
	}
}

// status decodes the explicit status a UNCLI chat's write answers with.
func status(t *testing.T, text string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("not a status object: %q", text)
	}
	return out
}

// TestUncliChatModes: a UNCLI chat's writes follow BRUV's Chat / Suggest
// / Edit mode and always say what happened: refused in Chat, staged on
// the chat's history in Suggest (applied only once accepted), applied in
// Edit. Reads run, and other MCP clients are unaffected.
func TestUncliChatModes(t *testing.T) {
	h, sup := newTestHandler(t)
	config.SetConfigDir(t.TempDir())
	t.Cleanup(func() { config.SetConfigDir("") })
	rt := sup.Resolve(testRepoID)
	card, err := rt.CreateCard("task", "Before")
	if err != nil {
		t.Fatal(err)
	}
	if err := config.SetChatUncliSession(testRepoID, card.ID, "s1"); err != nil {
		t.Fatal(err)
	}
	title := func() string {
		c, err := rt.GetCard(card.ID)
		if err != nil {
			t.Fatal(err)
		}
		return c.Title
	}
	retitle := func(session, to string) (string, bool) {
		return callAs(t, h, session, "set_card_title", map[string]any{"card_id": card.ID, "title": to})
	}

	setAIMode(t, "chat")
	if text, isErr := retitle("s1", "Chat"); !isErr || status(t, text)["status"] != "refused" || !strings.Contains(text, "Chat mode") || title() != "Before" {
		t.Errorf("chat mode write: %q, isErr=%v, title=%q", text, isErr, title())
	}
	if text, isErr := callAs(t, h, "s1", "get_card", map[string]any{"card_id": card.ID}); isErr {
		t.Errorf("chat mode read refused: %s", text)
	}
	if text, isErr := retitle("", "Other client"); isErr || title() != "Other client" {
		t.Errorf("another MCP client was held to chat mode: %q, title=%q", text, title())
	}

	// Suggest: staged, every time, and said so; applied once accepted.
	setAIMode(t, "suggest")
	for range 2 {
		text, isErr := retitle("s1", "Suggested")
		if s := status(t, text); isErr || s["status"] != "staged" || s["note"] == nil || title() != "Other client" {
			t.Fatalf("suggest mode write: %q, isErr=%v, title=%q", text, isErr, title())
		}
	}
	cf, err := rt.LoadChatHistory(card.ID)
	if err != nil || len(cf.Messages) != 2 || cf.Messages[0].Content != supervisor.UncliStagedContent || len(cf.Messages[0].PendingEdits) != 1 {
		t.Fatalf("staged history = %+v, %v", cf, err)
	}
	staged := cf.Messages[0].PendingEdits[0]
	if !strings.HasPrefix(staged.Label, "Set title → Suggested on “") {
		t.Errorf("staged label = %q", staged.Label)
	}
	if _, err := rt.ApplyPendingEdits(card.ID, cf.Messages[0].ID, []string{staged.ID}); err != nil {
		t.Fatal(err)
	}
	if title() != "Suggested" {
		t.Errorf("accepted edit not applied: title=%q", title())
	}
	if text, isErr := retitle("gone", "Nowhere"); !isErr || status(t, text)["status"] != "refused" {
		t.Errorf("unlinked suggest: %q, isErr=%v", text, isErr)
	}

	// Edit: applied, and said so.
	setAIMode(t, "edit")
	if text, isErr := retitle("s1", "Edited"); isErr || status(t, text)["status"] != "applied" || title() != "Edited" {
		t.Errorf("edit mode write: %q, isErr=%v, title=%q", text, isErr, title())
	}
}
