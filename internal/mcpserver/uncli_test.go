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
	return callAsWithMeta(t, h, session, "", name, args)
}

// asUncliChat marks req as a UNCLI chat's: its session and, if any, meta.
func asUncliChat(req *http.Request, session, meta string) {
	if session != "" {
		req.Header.Set(config.UncliChatHeader, session)
	}
	if meta != "" {
		req.Header.Set(config.UncliChatMetaHeader, meta)
	}
}

func callAsWithMeta(t *testing.T, h *Handler, session, meta, name string, args map[string]any) (string, bool) {
	t.Helper()
	params, _ := json.Marshal(map[string]any{"name": name, "arguments": args})
	payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": json.RawMessage(params)})
	req := httptest.NewRequest(http.MethodPost, "/repos/"+testRepoID+"/mcp", strings.NewReader(string(payload)))
	asUncliChat(req, session, meta)
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

// instructionsAs is what a client is told on connecting, as a UNCLI chat
// with session and meta (both "" = any other client).
func instructionsAs(t *testing.T, h *Handler, session, meta string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/repos/"+testRepoID+"/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`))
	asUncliChat(req, session, meta)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var resp struct {
		Result struct {
			Instructions string `json:"instructions"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("initialize: %q (%v)", rec.Body.String(), err)
	}
	return resp.Result.Instructions
}

// A UNCLI chat is told what it's about when it connects, from the meta
// it was made with: the card, the user's own context. Nothing else is.
func TestUncliChatBrief(t *testing.T) {
	h, sup := newTestHandler(t)
	config.SetConfigDir(t.TempDir())
	t.Cleanup(func() { config.SetConfigDir("") })
	if err := config.SaveLLMConfig(config.LLMConfig{Context: "I plan trips with my kids."}); err != nil {
		t.Fatal(err)
	}
	rt := sup.Resolve(testRepoID)
	card, err := rt.CreateCard("task", "Melbourne trip")
	if err != nil {
		t.Fatal(err)
	}
	meta := "bruv/" + testRepoID + "/card/" + card.ID

	brief := instructionsAs(t, h, "s1", meta)
	for _, want := range []string{"## This chat", `"Melbourne trip"`, card.ID, "in the inbox", "I plan trips with my kids."} {
		if !strings.Contains(brief, want) {
			t.Errorf("brief lacks %q:\n%s", want, brief)
		}
	}
	for name, got := range map[string]string{
		"another client":      instructionsAs(t, h, "", ""),
		"another repo's chat": instructionsAs(t, h, "s2", "bruv/other-repo/card/"+card.ID),
	} {
		if strings.Contains(got, "## This chat") {
			t.Errorf("%s was briefed:\n%s", name, got)
		}
	}

	// A project chat: its place, and that the tools default to it.
	if _, isErr := callToolRPC(t, h, "create_card", map[string]any{"title": "Flights", "brand": "Home", "stream": "Trips", "project": "Melbourne", "category": "Ideas"}); isErr {
		t.Fatal("create_card failed")
	}
	brief = instructionsAs(t, h, "s3", "bruv/"+testRepoID+"/project/home/trips/melbourne")
	for _, want := range []string{"## This chat", "Project: Melbourne (in Home › Trips)", "leave brand, stream and project out"} {
		if !strings.Contains(brief, want) {
			t.Errorf("project brief lacks %q:\n%s", want, brief)
		}
	}

	// Suggest stages by the meta alone, before BRUV records the session.
	setAIMode(t, "suggest")
	text, isErr := callAsWithMeta(t, h, "new", meta, "set_card_title", map[string]any{"card_id": card.ID, "title": "Sydney trip"})
	if isErr || status(t, text)["status"] != "staged" {
		t.Errorf("staging by meta: %q", text)
	}
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
