package llm

// Ollama /api/chat adapter. It is OpenAI-shaped on the way out (system prompt
// prepended, tools wrapped in {"type":"function"}) but Ollama-shaped on the
// way back: tool arguments arrive as a real JSON object, tool calls carry no
// id, and token counts are named prompt_eval_count / eval_count.

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// --- request shape ----------------------------------------------------------

func TestOllamaRequestShape(t *testing.T) {
	s := okStub(t, "ollama_text.json")
	p := NewOllama(s.URL)

	_, err := p.ChatCompletion(context.Background(), ChatRequest{
		SystemPrompt: "You are a card assistant.",
		Model:        "llama3.1:8b",
		MaxTokens:    1024,
		Messages: []Message{
			{Role: "user", Content: "Summarise this card."},
			{Role: "assistant", Content: "Sure."},
		},
		Tools: []ToolDef{sampleTool()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := s.lastRequest(t)
	if req.path != "/api/chat" {
		t.Errorf("path = %q, want /api/chat", req.path)
	}
	// Ollama is unauthenticated — leaking a key header to a local daemon
	// would be wrong, and there is no key to leak.
	if got := req.header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want none", got)
	}
	if got := req.header.Get("x-api-key"); got != "" {
		t.Errorf("x-api-key = %q, want none", got)
	}

	body := s.lastBody(t)
	if got := digStr(t, body, "model"); got != "llama3.1:8b" {
		t.Errorf("model = %q", got)
	}
	// stream MUST be false — the adapter reads one whole JSON document, and
	// a streamed reply would arrive as NDJSON and fail to parse.
	if digBool(t, body, "stream") {
		t.Error("stream = true; the adapter does not handle NDJSON")
	}

	// System prompt is prepended as a message, OpenAI-style.
	wantAbsent(t, body, "system")
	msgs := digSlice(t, body, "messages")
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3 (system + 2)", len(msgs))
	}
	if got := digStr(t, body, "messages", 0, "role"); got != "system" {
		t.Errorf("messages[0].role = %q, want system", got)
	}
	if got := digStr(t, body, "messages", 0, "content"); got != "You are a card assistant." {
		t.Errorf("messages[0].content = %q", got)
	}

	tools := digSlice(t, body, "tools")
	if len(tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(tools))
	}
	if got := digStr(t, body, "tools", 0, "type"); got != "function" {
		t.Errorf("tools[0].type = %q, want function", got)
	}
	if got := digStr(t, body, "tools", 0, "function", "name"); got != "set_title" {
		t.Errorf("tools[0].function.name = %q", got)
	}
	if got := digStr(t, body, "tools", 0, "function", "parameters", "properties", "meta", "properties", "confidence", "type"); got != "string" {
		t.Errorf("nested schema property lost: %q", got)
	}

	// FINDING (ollama.go:42-46): ChatRequest.MaxTokens is silently ignored —
	// Ollama takes it as options.num_predict, which the adapter never sets.
	// A caller capping output at 1024 tokens gets the model default instead.
	wantAbsent(t, body, "max_tokens")
	wantAbsent(t, body, "options")
	wantAbsent(t, body, "temperature")
}

func TestOllamaOmitsToolsWhenNone(t *testing.T) {
	s := okStub(t, "ollama_text.json")
	_, err := NewOllama(s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "llama3.1:8b",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantAbsent(t, s.lastBody(t), "tools")
}

func TestOllamaOmitsSystemMessageWhenNoPrompt(t *testing.T) {
	s := okStub(t, "ollama_text.json")
	_, err := NewOllama(s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "llama3.1:8b",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs := digSlice(t, s.lastBody(t), "messages")
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1 (no empty system message)", len(msgs))
	}
}

// BUG (ollama.go:37-40): the outgoing message is built as
// `{"role": m.Role, "content": m.Content}` and nothing else, so
//   - the assistant turn's ToolCalls are DROPPED, and
//   - the tool result's ToolCallID is DROPPED.
//
// RunLoop (core/runtime/chat/chat.go:132-174) always replays a tool round as
// assistant-with-ToolCalls followed by role="tool"+ToolCallID, so on Ollama
// every multi-turn tool conversation reaches the model as a bare "tool"
// message with no record of what was called. Ollama's /api/chat documents
// tool_calls on assistant messages and (newer builds) tool_name on tool
// messages; neither is sent. Practical effect: the model sees an
// unattributed result and commonly re-issues the same tool call, burning
// iterations until MaxIter.
//
// Locked to current behaviour — do not "fix" by editing source from here.
// REGRESSION (found + fixed 2026-08-02): the adapter used to build every
// outgoing message as {role, content} only, so ToolCalls and ToolCallID
// never reached the wire. The model then saw a tool RESULT with no record
// of the call that produced it and would re-issue the same call until
// MaxIter — every Ollama tool loop was broken. Ollama's /api/chat takes
// the OpenAI shapes, except arguments are an OBJECT, not a JSON string.
func TestOllamaToolRoundTripCarriesLinkage(t *testing.T) {
	s := okStub(t, "ollama_text.json")
	_, err := NewOllama(s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "llama3.1:8b",
		Messages: toolRoundTripMessages(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := s.lastBody(t)
	if msgs := digSlice(t, body, "messages"); len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3", len(msgs))
	}

	assistant := digMap(t, body, "messages", 1)
	if assistant["role"] != "assistant" {
		t.Errorf("messages[1].role = %v", assistant["role"])
	}
	rawCalls, ok := assistant["tool_calls"].([]any)
	if !ok || len(rawCalls) != 1 {
		t.Fatalf("assistant tool_calls = %#v, want 1 entry", assistant["tool_calls"])
	}
	call, _ := rawCalls[0].(map[string]any)
	if call["type"] != "function" {
		t.Errorf("tool_calls[0].type = %v, want function", call["type"])
	}
	if call["id"] != "call_abc123" {
		t.Errorf("tool_calls[0].id = %v, want call_abc123", call["id"])
	}
	fn, _ := call["function"].(map[string]any)
	if fn["name"] != "set_title" {
		t.Errorf("tool_calls[0].function.name = %v", fn["name"])
	}
	// Object, not a JSON string — the Ollama-vs-OpenAI difference.
	args, ok := fn["arguments"].(map[string]any)
	if !ok {
		t.Fatalf("arguments = %#v, want an object", fn["arguments"])
	}
	if args["title"] != "Ship the web clipper" {
		t.Errorf("arguments.title = %v", args["title"])
	}
	// Nested argument objects must survive intact.
	meta, ok := args["meta"].(map[string]any)
	if !ok || meta["confidence"] != "high" {
		t.Errorf("nested arguments lost: %#v", args["meta"])
	}

	toolMsg := digMap(t, body, "messages", 2)
	if toolMsg["role"] != "tool" {
		t.Errorf("messages[2].role = %v, want tool", toolMsg["role"])
	}
	if toolMsg["content"] != "Title updated." {
		t.Errorf("messages[2].content = %v", toolMsg["content"])
	}
	if toolMsg["tool_call_id"] != "call_abc123" {
		t.Errorf("messages[2].tool_call_id = %v, want call_abc123 (linkage back to the call)", toolMsg["tool_call_id"])
	}
}

// --- response parsing -------------------------------------------------------

func TestOllamaParseTextResponse(t *testing.T) {
	s := okStub(t, "ollama_text.json")
	resp, err := NewOllama(s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "llama3.1:8b",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "The card looks complete. Nothing is out of place." {
		t.Errorf("Content = %q", resp.Content)
	}
	if resp.Model != "llama3.1:8b" {
		t.Errorf("Model = %q", resp.Model)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(resp.ToolCalls))
	}
	if resp.Usage == nil {
		t.Fatal("Usage is nil — cost tracking depends on it")
	}
	// Ollama names its counters prompt_eval_count / eval_count and sends no
	// total; reading the OpenAI names here would silently zero every count.
	if resp.Usage.PromptTokens != 26 {
		t.Errorf("PromptTokens = %d, want 26 (prompt_eval_count)", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 282 {
		t.Errorf("CompletionTokens = %d, want 282 (eval_count)", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 308 {
		t.Errorf("TotalTokens = %d, want 308 (synthesised sum)", resp.Usage.TotalTokens)
	}
}

func TestOllamaParseToolCallResponse(t *testing.T) {
	s := okStub(t, "ollama_tool_call.json")
	resp, err := NewOllama(s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "llama3.1:8b",
		Messages: []Message{{Role: "user", Content: "hi"}},
		Tools:    []ToolDef{sampleTool()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 2 {
		t.Fatalf("got %d tool calls, want 2", len(resp.ToolCalls))
	}

	first := resp.ToolCalls[0]
	// Ollama sends no call id, so the adapter synthesises a positional one.
	// It is unique only within a single response — see the finding about the
	// id never reaching the model on the next turn anyway.
	if first.ID != "ollama-0" {
		t.Errorf("ToolCalls[0].ID = %q, want ollama-0", first.ID)
	}
	if resp.ToolCalls[1].ID != "ollama-1" {
		t.Errorf("ToolCalls[1].ID = %q, want ollama-1", resp.ToolCalls[1].ID)
	}
	if first.Name != "set_title" {
		t.Errorf("ToolCalls[0].Name = %q", first.Name)
	}
	// Unlike OpenAI, arguments are already a JSON object — no string parse
	// step, but the nesting must still land as real maps/slices.
	if got := first.Arguments["title"]; got != "Ship the web clipper" {
		t.Errorf("Arguments[title] = %v", got)
	}
	meta, ok := first.Arguments["meta"].(map[string]any)
	if !ok {
		t.Fatalf("Arguments[meta] = %T, want map[string]any", first.Arguments["meta"])
	}
	if meta["confidence"] != "high" || meta["auto"] != true {
		t.Errorf("meta = %v", meta)
	}
	if got, ok := meta["score"].(float64); !ok || got != 0.87 {
		t.Errorf("meta.score = %v (%T), want 0.87", meta["score"], meta["score"])
	}
	if resp.ToolCalls[1].Name != "add_tags" {
		t.Errorf("ToolCalls[1].Name = %q", resp.ToolCalls[1].Name)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 353 {
		t.Errorf("Usage = %+v, want total 353", resp.Usage)
	}
}

// A model with no usage counters (some embedded/proxy builds omit them) must
// leave Usage nil rather than record a bogus zero-cost call.
func TestOllamaNoUsageCountersLeavesUsageNil(t *testing.T) {
	body := `{"model":"llama3.1:8b","message":{"role":"assistant","content":"hi"},"done":true}`
	s := newStub(t, http.StatusOK, body, nil)
	resp, err := NewOllama(s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "llama3.1:8b",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Usage != nil {
		t.Errorf("Usage = %+v, want nil when the server reports no counts", resp.Usage)
	}
}

// --- error paths ------------------------------------------------------------

func TestOllamaErrorStatuses(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		fixture string
		body    string
		want    []string
	}{
		{name: "404 model not pulled", status: http.StatusNotFound, fixture: "ollama_error_404.json",
			want: []string{"404", "not found"}},
		{name: "400 bad request", status: http.StatusBadRequest,
			body: `{"error":"invalid options: num_ctx"}`,
			want: []string{"400", "num_ctx"}},
		{name: "500 server error", status: http.StatusInternalServerError,
			body: `{"error":"llama runner process has terminated: exit status 2"}`,
			want: []string{"500", "runner"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := c.body
			if c.fixture != "" {
				body = fixture(t, c.fixture)
			}
			s := newStub(t, c.status, body, nil)
			resp, err := NewOllama(s.URL).ChatCompletion(context.Background(), ChatRequest{
				Model:    "llama3.1:8b",
				Messages: []Message{{Role: "user", Content: "hi"}},
			})
			if err == nil {
				t.Fatalf("expected error, got %+v", resp)
			}
			if resp != nil {
				t.Errorf("expected nil response with error, got %+v", resp)
			}
			for _, want := range c.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q missing %q", err, want)
				}
			}
		})
	}
}

// FINDING (ollama.go:86): unlike the other two adapters, Ollama has no 429
// branch — a rate-limited response from a proxied/hosted Ollama endpoint
// (OpenWebUI, a reverse proxy, or a cloud-hosted gateway) becomes a generic
// error, so IsRateLimitError is false and callers apply the normal retry
// schedule instead of backing off. Locked to current behaviour.
func TestOllama429IsNotClassifiedAsRateLimit(t *testing.T) {
	s := newStub(t, http.StatusTooManyRequests, `{"error":"too many requests"}`,
		map[string]string{"Retry-After": "30"})
	_, err := NewOllama(s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "llama3.1:8b",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if IsRateLimitError(err) {
		t.Error("behaviour changed — Ollama 429 is now a RateLimitError; update the finding")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error %q should at least carry the status", err)
	}
}

func TestOllamaErrorBodyTruncated(t *testing.T) {
	huge := `{"error":"` + strings.Repeat("x", 5000) + `"}`
	s := newStub(t, http.StatusInternalServerError, huge, nil)
	_, err := NewOllama(s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "llama3.1:8b",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(err.Error()) > 300 {
		t.Errorf("error message is %d chars — body not truncated", len(err.Error()))
	}
}

func TestOllamaMalformedJSONBody(t *testing.T) {
	// Streaming left on would produce exactly this: several JSON documents
	// concatenated, which json.Unmarshal rejects.
	s := newStub(t, http.StatusOK,
		`{"model":"llama3.1:8b","message":{"role":"assistant","content":"The"},"done":false}`+"\n"+
			`{"model":"llama3.1:8b","message":{"role":"assistant","content":" card"},"done":true}`, nil)
	_, err := NewOllama(s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "llama3.1:8b",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected parse error for NDJSON body, got nil")
	}
	if !strings.Contains(err.Error(), "parse response") {
		t.Errorf("error %q should identify the parse stage", err)
	}
}

// The common real-world failure: Ollama simply isn't running.
func TestOllamaUnreachableHost(t *testing.T) {
	_, err := NewOllama("http://127.0.0.1:0").ChatCompletion(context.Background(), ChatRequest{
		Model:    "llama3.1:8b",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("error %q should identify the transport stage", err)
	}
}
