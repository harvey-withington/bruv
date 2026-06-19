package mcpserver

// End-to-end tests for the MCP server: stand up a real Supervisor over a
// freshly-initialised repo and drive the handler through the JSON-RPC
// handshake and the capture tools, asserting the data actually lands.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bruv/core/supervisor"
	"bruv/internal/config"
	"bruv/internal/repo"
)

const testRepoID = "repo1"

func newTestHandler(t *testing.T) (*Handler, *supervisor.Supervisor) {
	t.Helper()
	cfgDir := t.TempDir()
	r, err := repo.InitAt(t.TempDir(), "Test Repo")
	if err != nil {
		t.Fatalf("repo.InitAt: %v", err)
	}
	sup, err := supervisor.New([]config.RepoEntry{{
		ID:   testRepoID,
		Name: "Test Repo",
		Path: r.Root,
	}}, cfgDir)
	if err != nil {
		t.Fatalf("supervisor.New: %v", err)
	}
	if _, err := sup.Load(testRepoID); err != nil {
		t.Fatalf("supervisor.Load: %v", err)
	}
	t.Cleanup(sup.Close)
	return New(sup, "test"), sup
}

// rpc sends one JSON-RPC request and returns the decoded response plus
// the raw recorder for status/header assertions.
func rpc(t *testing.T, h *Handler, payload string) (*httptest.ResponseRecorder, rpcResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/repos/"+testRepoID+"/mcp", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var resp rpcResponse
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response %q: %v", rec.Body.String(), err)
		}
	}
	return rec, resp
}

// callTool drives a tools/call and returns the decoded CallToolResult.
func callToolRPC(t *testing.T, h *Handler, name string, args map[string]any) (text string, isErr bool) {
	t.Helper()
	params, _ := json.Marshal(map[string]any{"name": name, "arguments": args})
	payload, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": json.RawMessage(params),
	})
	_, resp := rpc(t, h, string(payload))
	if resp.Error != nil {
		t.Fatalf("tools/call %s returned JSON-RPC error: %+v", name, resp.Error)
	}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	raw, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode CallToolResult: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatalf("tools/call %s returned no content", name)
	}
	return result.Content[0].Text, result.IsError
}

func TestInitialize(t *testing.T) {
	h, _ := newTestHandler(t)
	rec, resp := rpc(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)

	if resp.Error != nil {
		t.Fatalf("initialize error: %+v", resp.Error)
	}
	if rec.Header().Get("Mcp-Session-Id") == "" {
		t.Error("initialize did not set Mcp-Session-Id header")
	}
	result, _ := resp.Result.(map[string]any)
	if result["protocolVersion"] != "2025-06-18" {
		t.Errorf("protocolVersion = %v, want 2025-06-18", result["protocolVersion"])
	}
	caps, _ := result["capabilities"].(map[string]any)
	if _, ok := caps["tools"]; !ok {
		t.Errorf("capabilities missing tools: %v", caps)
	}
	info, _ := result["serverInfo"].(map[string]any)
	if name, _ := info["name"].(string); !strings.Contains(name, "Test Repo") {
		t.Errorf("serverInfo.name = %q, want it to contain the repo name", name)
	}
}

func TestInitializeNegotiatesUnknownProtocol(t *testing.T) {
	h, _ := newTestHandler(t)
	_, resp := rpc(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	result, _ := resp.Result.(map[string]any)
	// Unknown version → server falls back to its own supported version.
	if result["protocolVersion"] == "1999-01-01" {
		t.Errorf("server echoed an unsupported protocol version")
	}
}

func TestNotificationGets202(t *testing.T) {
	h, _ := newTestHandler(t)
	rec, _ := rpc(t, h, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if rec.Code != http.StatusAccepted {
		t.Errorf("notification status = %d, want 202", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("notification produced a body: %q", rec.Body.String())
	}
}

func TestToolsList(t *testing.T) {
	h, _ := newTestHandler(t)
	_, resp := rpc(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %+v", resp.Error)
	}
	raw, _ := json.Marshal(resp.Result)
	var out struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"tools"`
	}
	_ = json.Unmarshal(raw, &out)
	want := map[string]bool{"create_card": false, "create_brand": false, "list_brands": false, "search_cards": false}
	for _, tool := range out.Tools {
		if _, ok := want[tool.Name]; ok {
			want[tool.Name] = true
		}
		if !strings.Contains(tool.Description, "Test Repo") {
			t.Errorf("tool %q description not templated with repo name: %q", tool.Name, tool.Description)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("tools/list missing expected tool %q", name)
		}
	}
}

func TestUnknownMethod(t *testing.T) {
	h, _ := newTestHandler(t)
	_, resp := rpc(t, h, `{"jsonrpc":"2.0","id":1,"method":"does/not/exist"}`)
	if resp.Error == nil {
		t.Fatal("expected JSON-RPC error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601", resp.Error.Code)
	}
}

func TestUnknownRepo404(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/repos/nope/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown repo status = %d, want 404", rec.Code)
	}
}

func TestGetMethodNotAllowed(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/repos/"+testRepoID+"/mcp", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want 405", rec.Code)
	}
}

// TestCreateCardEndToEnd is the headline test: capture a card with a full
// hierarchy + description + tags + blocks, then verify it all landed by
// reading back through the read tools and the runtime directly.
func TestCreateCardEndToEnd(t *testing.T) {
	h, sup := newTestHandler(t)

	text, isErr := callToolRPC(t, h, "create_card", map[string]any{
		"title":       "Launch teaser idea",
		"brand":       "Acme",
		"stream":      "Marketing",
		"project":     "Spring Campaign",
		"category":    "Ideas",
		"tags":        []any{"video", "social"},
		"description": "A 15s teaser for the spring launch.",
		"blocks": []any{
			map[string]any{"type": "checklist", "label": "Shots", "value": []any{"opening", "logo"}},
		},
	})
	if isErr {
		t.Fatalf("create_card reported error: %s", text)
	}
	var created struct {
		CardID   string `json:"card_id"`
		PinnedTo string `json:"pinned_to"`
	}
	if err := json.Unmarshal([]byte(text), &created); err != nil {
		t.Fatalf("decode create_card result %q: %v", text, err)
	}
	if created.CardID == "" {
		t.Fatal("create_card returned no card_id")
	}
	if !strings.Contains(created.PinnedTo, "Acme") || !strings.Contains(created.PinnedTo, "Ideas") {
		t.Errorf("pinned_to = %q, want the Acme…Ideas breadcrumb", created.PinnedTo)
	}

	// Verify the hierarchy was actually created.
	rt := sup.Resolve(testRepoID)
	brands, _ := rt.ListBrands()
	if len(brands) != 1 || brands[0].Name != "Acme" {
		t.Fatalf("brands = %+v, want one named Acme", brands)
	}

	// Verify card contents via the runtime.
	card, err := rt.GetCard(created.CardID)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if card.Description != "A 15s teaser for the spring launch." {
		t.Errorf("description = %q", card.Description)
	}
	if len(card.Tags) != 2 {
		t.Errorf("tags = %v, want 2", card.Tags)
	}
	var foundChecklist bool
	for _, b := range card.Blocks {
		if b.Label == "Shots" {
			foundChecklist = true
			if _, ok := b.Value.([]string); !ok {
				// coercion turns the []any into a checklist value; accept
				// either []string or []any of strings.
				if arr, ok := b.Value.([]any); !ok || len(arr) != 2 {
					t.Errorf("checklist block value = %#v, want 2 items", b.Value)
				}
			}
		}
	}
	if !foundChecklist {
		t.Errorf("checklist block 'Shots' not found on card; blocks = %+v", card.Blocks)
	}

	// search_cards should now find it.
	searchText, isErr := callToolRPC(t, h, "search_cards", map[string]any{"query": "teaser"})
	if isErr {
		t.Fatalf("search_cards error: %s", searchText)
	}
	if !strings.Contains(searchText, created.CardID) {
		t.Errorf("search_cards did not return the new card; got: %s", searchText)
	}
}

func TestCreateCardPartialHierarchyRejected(t *testing.T) {
	h, _ := newTestHandler(t)
	text, isErr := callToolRPC(t, h, "create_card", map[string]any{
		"title": "Half-filed",
		"brand": "Acme",
		// stream/project/category intentionally omitted
	})
	if !isErr {
		t.Errorf("expected error for partial hierarchy, got: %s", text)
	}
}

func TestSetCardFieldsAndTags(t *testing.T) {
	h, sup := newTestHandler(t)
	rt := sup.Resolve(testRepoID)

	// Make a typed card so it has schema field keys to set.
	created, err := rt.CreateCard("task", "Do the thing")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	// Pick a real field key from the seeded blocks, if any.
	var key string
	for _, b := range created.Blocks {
		if b.Key != "" && b.Type == "text" {
			key = b.Key
			break
		}
	}

	if key != "" {
		text, isErr := callToolRPC(t, h, "set_card_fields", map[string]any{
			"card_id": created.ID,
			"fields":  map[string]any{key: "updated value"},
		})
		if isErr {
			t.Fatalf("set_card_fields error: %s", text)
		}
		reloaded, _ := rt.GetCard(created.ID)
		var ok bool
		for _, b := range reloaded.Blocks {
			if b.Key == key && b.Value == "updated value" {
				ok = true
			}
		}
		if !ok {
			t.Errorf("set_card_fields did not update key %q", key)
		}
	}

	// add_card_tags merges, not replaces.
	if _, isErr := callToolRPC(t, h, "add_card_tags", map[string]any{
		"card_id": created.ID, "tags": []any{"alpha", "beta"},
	}); isErr {
		t.Fatal("add_card_tags reported error")
	}
	reloaded, _ := rt.GetCard(created.ID)
	if len(reloaded.Tags) != 2 {
		t.Errorf("tags after add = %v, want 2", reloaded.Tags)
	}
}
