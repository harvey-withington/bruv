package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// TestUncliChatModes: BRUV's own check behind UNCLI's (the engine's
// HostToolPolicy). A UNCLI chat's writes are refused in Chat mode; in
// Suggest and Edit a call that reaches BRUV was allowed (by the user, on
// UNCLI's approval card, in Suggest) and runs. Reads always run, and
// other MCP clients are unaffected.
func TestUncliChatModes(t *testing.T) {
	h, sup := newTestHandler(t)
	config.SetConfigDir(t.TempDir())
	t.Cleanup(func() { config.SetConfigDir("") })
	rt := sup.Resolve(testRepoID)
	card, err := rt.CreateCard("task", "Before")
	if err != nil {
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
	if text, isErr := retitle("s1", "Chat"); !isErr || !strings.Contains(text, "Chat mode") || title() != "Before" {
		t.Errorf("chat mode write: %q, isErr=%v, title=%q", text, isErr, title())
	}
	if text, isErr := callAs(t, h, "s1", "get_card", map[string]any{"card_id": card.ID}); isErr {
		t.Errorf("chat mode read refused: %s", text)
	}
	if text, isErr := retitle("", "Other client"); isErr || title() != "Other client" {
		t.Errorf("another MCP client was held to chat mode: %q, title=%q", text, title())
	}

	for _, mode := range []string{"suggest", "edit"} {
		setAIMode(t, mode)
		if text, isErr := retitle("s1", mode); isErr || title() != mode {
			t.Errorf("%s mode write: %q, isErr=%v, title=%q", mode, text, isErr, title())
		}
	}
}
