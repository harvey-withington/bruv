package llm

// OpenAI Chat Completions adapter. The load-bearing quirk is that tool call
// arguments cross the wire as a JSON *string* in both directions — a parse
// miss there silently drops every tool call's arguments while leaving the
// call itself looking valid, which is the hardest failure mode to notice.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// --- request shape ----------------------------------------------------------

func TestOpenAIRequestShape(t *testing.T) {
	s := okStub(t, "openai_text.json")
	p := NewOpenAI("sk-test-key", s.URL)

	_, err := p.ChatCompletion(context.Background(), ChatRequest{
		SystemPrompt: "You are a card assistant.",
		Model:        "gpt-4o",
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
	if req.path != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", req.path)
	}
	if got := req.header.Get("Authorization"); got != "Bearer sk-test-key" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer sk-test-key")
	}
	if got := req.header.Get("x-api-key"); got != "" {
		t.Errorf("x-api-key = %q, want empty (that's Anthropic's scheme)", got)
	}

	body := s.lastBody(t)
	if got := digStr(t, body, "model"); got != "gpt-4o" {
		t.Errorf("model = %q", got)
	}
	if got := digNum(t, body, "max_tokens"); got != 1024 {
		t.Errorf("max_tokens = %v, want 1024", got)
	}

	// Unlike Anthropic, the system prompt is PREPENDED as messages[0] with
	// role "system"; there is no top-level system field.
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
	if got := digStr(t, body, "messages", 1, "role"); got != "user" {
		t.Errorf("messages[1].role = %q", got)
	}
	if got := digStr(t, body, "messages", 2, "role"); got != "assistant" {
		t.Errorf("messages[2].role = %q", got)
	}

	// Tools wrap in {"type":"function","function":{...}} with the schema
	// under "parameters" — the mirror image of Anthropic's "input_schema".
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
	if got := digStr(t, body, "tools", 0, "function", "description"); got != "Set the card title." {
		t.Errorf("tools[0].function.description = %q", got)
	}
	if got := digStr(t, body, "tools", 0, "function", "parameters", "type"); got != "object" {
		t.Errorf("parameters.type = %q", got)
	}
	if got := digStr(t, body, "tools", 0, "function", "parameters", "properties", "meta", "properties", "confidence", "type"); got != "string" {
		t.Errorf("nested schema property lost: %q", got)
	}
	wantAbsent(t, digMap(t, body, "tools", 0), "input_schema")

	// No temperature knob exists on ChatRequest (provider.go:67); locked so
	// adding one is deliberate. Streaming is likewise not implemented.
	wantAbsent(t, body, "temperature")
	wantAbsent(t, body, "stream")
}

func TestOpenAIDefaultMaxTokens(t *testing.T) {
	s := okStub(t, "openai_text.json")
	_, err := NewOpenAI("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := s.lastBody(t)
	if got := digNum(t, body, "max_tokens"); got != 4096 {
		t.Errorf("default max_tokens = %v, want 4096", got)
	}
	// FINDING (openai.go:69): the key is always "max_tokens". OpenAI's
	// reasoning models (o1/o3/o4-mini and newer) reject max_tokens outright
	// and require "max_completion_tokens", so those models 400 on every call
	// through this adapter. Locked to current behaviour.
	wantAbsent(t, body, "max_completion_tokens")
}

// A local OpenAI-compatible endpoint (LM Studio, llama.cpp, vLLM) needs no
// key; sending "Bearer " with an empty key is rejected by some of them.
func TestOpenAIOmitsAuthHeaderWithoutKey(t *testing.T) {
	s := okStub(t, "openai_text.json")
	_, err := NewOpenAI("", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "local-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := s.lastRequest(t).header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want no header when the key is empty", got)
	}
}

func TestOpenAIOmitsToolsWhenNone(t *testing.T) {
	s := okStub(t, "openai_text.json")
	_, err := NewOpenAI("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "tools": null is a 400 on the real API.
	wantAbsent(t, s.lastBody(t), "tools")
}

func TestOpenAIToolResultRoundTrip(t *testing.T) {
	s := okStub(t, "openai_text.json")
	_, err := NewOpenAI("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o",
		Messages: toolRoundTripMessages(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := s.lastBody(t)
	msgs := digSlice(t, body, "messages")
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3 (no system prompt set)", len(msgs))
	}

	// [1] assistant carries tool_calls as a sibling field of content.
	if got := digStr(t, body, "messages", 1, "role"); got != "assistant" {
		t.Errorf("messages[1].role = %q", got)
	}
	calls := digSlice(t, body, "messages", 1, "tool_calls")
	if len(calls) != 1 {
		t.Fatalf("got %d tool_calls, want 1", len(calls))
	}
	if got := digStr(t, body, "messages", 1, "tool_calls", 0, "id"); got != "call_abc123" {
		t.Errorf("tool_calls[0].id = %q — must match the tool message's tool_call_id", got)
	}
	if got := digStr(t, body, "messages", 1, "tool_calls", 0, "type"); got != "function" {
		t.Errorf("tool_calls[0].type = %q, want function", got)
	}
	if got := digStr(t, body, "messages", 1, "tool_calls", 0, "function", "name"); got != "set_title" {
		t.Errorf("tool_calls[0].function.name = %q", got)
	}

	// Arguments go OUT as a JSON string, not an object. Sending an object
	// here is a 400 from OpenAI.
	rawArgs := digStr(t, body, "messages", 1, "tool_calls", 0, "function", "arguments")
	var args map[string]any
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		t.Fatalf("arguments is not valid JSON-in-a-string: %v (%q)", err, rawArgs)
	}
	if args["title"] != "Ship the web clipper" {
		t.Errorf("arguments.title = %v", args["title"])
	}
	meta, ok := args["meta"].(map[string]any)
	if !ok || meta["confidence"] != "high" {
		t.Errorf("nested argument object lost in encoding: %v", args["meta"])
	}

	// [2] the result is a role="tool" message keyed by tool_call_id.
	if got := digStr(t, body, "messages", 2, "role"); got != "tool" {
		t.Errorf("messages[2].role = %q, want tool", got)
	}
	if got := digStr(t, body, "messages", 2, "tool_call_id"); got != "call_abc123" {
		t.Errorf("tool_call_id = %q", got)
	}
	if got := digStr(t, body, "messages", 2, "content"); got != "Title updated." {
		t.Errorf("tool result content = %q", got)
	}
}

// --- response parsing -------------------------------------------------------

func TestOpenAIParseTextResponse(t *testing.T) {
	s := okStub(t, "openai_text.json")
	resp, err := NewOpenAI("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "The card looks complete. Nothing is out of place." {
		t.Errorf("Content = %q", resp.Content)
	}
	// The resolved model id (with date suffix) is what gets recorded, not the
	// alias that was requested.
	if resp.Model != "gpt-4o-2024-08-06" {
		t.Errorf("Model = %q", resp.Model)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(resp.ToolCalls))
	}
	if resp.Usage == nil {
		t.Fatal("Usage is nil — cost tracking depends on it")
	}
	if resp.Usage.PromptTokens != 1117 || resp.Usage.CompletionTokens != 46 || resp.Usage.TotalTokens != 1163 {
		t.Errorf("Usage = %+v, want 1117/46/1163", resp.Usage)
	}
}

func TestOpenAIParseToolCallResponse(t *testing.T) {
	s := okStub(t, "openai_tool_call.json")
	resp, err := NewOpenAI("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "hi"}},
		Tools:    []ToolDef{sampleTool()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// content is null on a tool-call turn — must decode to "" not blow up.
	if resp.Content != "" {
		t.Errorf("Content = %q, want empty for a pure tool-call turn", resp.Content)
	}
	if len(resp.ToolCalls) != 2 {
		t.Fatalf("got %d tool calls, want 2", len(resp.ToolCalls))
	}

	first := resp.ToolCalls[0]
	if first.ID != "call_12345xyz" {
		t.Errorf("ToolCalls[0].ID = %q", first.ID)
	}
	if first.Name != "set_title" {
		t.Errorf("ToolCalls[0].Name = %q", first.Name)
	}
	// The whole point: arguments arrive as an escaped JSON string and must be
	// unmarshalled into a real map, nesting and types preserved.
	if got := first.Arguments["title"]; got != "Ship the web clipper" {
		t.Errorf("Arguments[title] = %v", got)
	}
	meta, ok := first.Arguments["meta"].(map[string]any)
	if !ok {
		t.Fatalf("Arguments[meta] = %T, want map[string]any (nested object not parsed)", first.Arguments["meta"])
	}
	if meta["confidence"] != "high" {
		t.Errorf("meta.confidence = %v", meta["confidence"])
	}
	if meta["auto"] != true {
		t.Errorf("meta.auto = %v, want true", meta["auto"])
	}
	if got, ok := meta["score"].(float64); !ok || got != 0.87 {
		t.Errorf("meta.score = %v (%T), want 0.87", meta["score"], meta["score"])
	}
	tags, ok := meta["tags"].([]any)
	if !ok || len(tags) != 2 || tags[1] != "clipper" {
		t.Errorf("meta.tags = %v", meta["tags"])
	}

	if resp.ToolCalls[1].Name != "add_tags" || resp.ToolCalls[1].ID != "call_67890abc" {
		t.Errorf("ToolCalls[1] = %+v", resp.ToolCalls[1])
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 2901 {
		t.Errorf("Usage = %+v, want total 2901", resp.Usage)
	}
}

// BUG (openai.go:166): the tool-argument unmarshal error is discarded with
// `_ = json.Unmarshal(...)`. When the model is cut off mid-arguments
// (finish_reason "length", which the fixture reproduces) the adapter returns
// a tool call with the right NAME and a nil Arguments map, and no error at
// all. RunLoop then executes the tool with no arguments — e.g. set_title with
// no title — instead of reporting a failure. Locked to current behaviour; a
// fix belongs in a separate change.
// REGRESSION (found + fixed 2026-08-02): the parse error was discarded
// (`_ = json.Unmarshal`), so a response truncated mid-arguments
// (finish_reason "length") produced a tool call with the right name, NIL
// arguments and no error — and the loop then ran the tool with nothing,
// e.g. set_title with no title, reporting success. A retryable error
// beats an unrepeatable wrong mutation.
func TestOpenAIMalformedToolArgumentsAreAnError(t *testing.T) {
	s := okStub(t, "openai_tool_call_bad_arguments.json")
	_, err := NewOpenAI("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "hi"}},
		Tools:    []ToolDef{sampleTool()},
	})
	if err == nil {
		t.Fatal("expected an error for unparseable tool arguments, got nil")
	}
	// The message must name the tool — otherwise the user can't tell
	// which call failed.
	if !strings.Contains(err.Error(), "set_title") {
		t.Errorf("error should name the tool, got %q", err)
	}
}

// Tools that legitimately take no arguments: providers spell an empty
// argument list as "", "null" or "{}", and none of those may be treated
// as the malformed case above.
func TestOpenAIEmptyToolArgumentsAreValid(t *testing.T) {
	for _, raw := range []string{`""`, `"null"`, `"{}"`, `"   "`} {
		body := `{"id":"c","object":"chat.completion","model":"gpt-4o",` +
			`"choices":[{"index":0,"message":{"role":"assistant","content":null,` +
			`"tool_calls":[{"id":"call_1","type":"function","function":{"name":"list_cards","arguments":` + raw + `}}]},` +
			`"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
		s := newStub(t, http.StatusOK, body, nil)
		resp, err := NewOpenAI("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
			Model:    "gpt-4o",
			Messages: []Message{{Role: "user", Content: "hi"}},
			Tools:    []ToolDef{sampleTool()},
		})
		if err != nil {
			t.Fatalf("arguments %s should be valid (no-arg tool), got %v", raw, err)
		}
		if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Arguments == nil {
			t.Fatalf("arguments %s: want one call with a non-nil empty map, got %#v", raw, resp.ToolCalls)
		}
		if len(resp.ToolCalls[0].Arguments) != 0 {
			t.Errorf("arguments %s: want empty map, got %v", raw, resp.ToolCalls[0].Arguments)
		}
	}
}

// An empty choices array is guarded (openai.go:147) — this is the check
// Anthropic's adapter lacks for empty content.
func TestOpenAINoChoicesIsAnError(t *testing.T) {
	s := okStub(t, "openai_no_choices.json")
	resp, err := NewOpenAI("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatalf("expected error for empty choices, got %+v", resp)
	}
	if !strings.Contains(err.Error(), "no choices") {
		t.Errorf("error %q should name the cause", err)
	}
}

// FINDING (openai.go:156): Usage is gated on TotalTokens > 0, so a
// compatible endpoint that reports prompt/completion counts but leaves
// total_tokens at 0 loses its usage entirely — cost tracking records nothing
// rather than the counts it was given.
func TestOpenAIUsageDroppedWhenTotalIsZero(t *testing.T) {
	body := `{"id":"c1","model":"local-model","choices":[{"index":0,"message":{"role":"assistant","content":"hi"}}],` +
		`"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":0}}`
	s := newStub(t, http.StatusOK, body, nil)
	resp, err := NewOpenAI("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "local-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Usage != nil {
		t.Errorf("behaviour changed — Usage = %+v (was nil); update the finding", resp.Usage)
	}
}

// --- error paths ------------------------------------------------------------

func TestOpenAIErrorStatuses(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		fixture string
		body    string
		want    []string
	}{
		// NOTE: the body is truncated to 200 chars (util.go:10), and OpenAI
		// puts its long human message BEFORE the machine-readable "code", so
		// the code ("invalid_api_key") is cut off. The prose survives, which
		// is what the user sees in the chat transcript.
		{name: "401 bad key", status: http.StatusUnauthorized, fixture: "openai_error_401.json",
			want: []string{"401", "Incorrect API key provided"}},
		{name: "400 bad request", status: http.StatusBadRequest,
			body: `{"error":{"message":"Unsupported parameter: 'max_tokens'","type":"invalid_request_error","code":"unsupported_parameter"}}`,
			want: []string{"400", "max_tokens"}},
		{name: "404 unknown model", status: http.StatusNotFound,
			body: `{"error":{"message":"The model 'gpt-9' does not exist","type":"invalid_request_error","code":"model_not_found"}}`,
			want: []string{"404", "model_not_found"}},
		{name: "500 server error", status: http.StatusInternalServerError,
			body: `{"error":{"message":"The server had an error","type":"server_error"}}`,
			want: []string{"500", "server_error"}},
		// 503 is transient but NOT a rate limit — it must not be classified
		// as one, or the caller applies rate-limit backoff to an outage.
		{name: "503 unavailable", status: http.StatusServiceUnavailable,
			body: `{"error":{"message":"Service temporarily unavailable"}}`,
			want: []string{"503"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := c.body
			if c.fixture != "" {
				body = fixture(t, c.fixture)
			}
			s := newStub(t, c.status, body, nil)
			resp, err := NewOpenAI("bad-key", s.URL).ChatCompletion(context.Background(), ChatRequest{
				Model:    "gpt-4o",
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
			if IsRateLimitError(err) {
				t.Errorf("HTTP %d misclassified as a rate limit", c.status)
			}
		})
	}
}

func TestOpenAIRateLimit429(t *testing.T) {
	cases := []struct {
		name       string
		retryAfter string
		want       time.Duration
	}{
		{"with Retry-After seconds", "20", 20 * time.Second},
		{"without hint", "", 0},
		{"unparseable hint", "soon", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hdr := map[string]string{}
			if c.retryAfter != "" {
				hdr["Retry-After"] = c.retryAfter
			}
			s := newStub(t, http.StatusTooManyRequests, fixture(t, "openai_error_429.json"), hdr)
			_, err := NewOpenAI("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
				Model:    "gpt-4o",
				Messages: []Message{{Role: "user", Content: "hi"}},
			})
			rle := AsRateLimitError(err)
			if rle == nil {
				t.Fatalf("expected *RateLimitError, got %T: %v", err, err)
			}
			if rle.Provider != "openai" {
				t.Errorf("Provider = %q", rle.Provider)
			}
			if rle.StatusCode != http.StatusTooManyRequests {
				t.Errorf("StatusCode = %d", rle.StatusCode)
			}
			if rle.RetryAfter != c.want {
				t.Errorf("RetryAfter = %v, want %v", rle.RetryAfter, c.want)
			}
			// Truncated at 200 chars, so only the leading prose survives —
			// enough to tell a token-per-minute cap from a request cap.
			if !strings.Contains(rle.Body, "Rate limit reached for gpt-4o") {
				t.Errorf("Body missing server detail: %q", rle.Body)
			}
		})
	}
}

func TestOpenAIMalformedJSONBody(t *testing.T) {
	s := newStub(t, http.StatusOK, `{"choices":[{"message":{"content":"hel`, nil)
	_, err := NewOpenAI("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "parse response") {
		t.Errorf("error %q should identify the parse stage", err)
	}
}

func TestOpenAIUnreachableHost(t *testing.T) {
	_, err := NewOpenAI("k", "http://127.0.0.1:0").ChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("error %q should identify the transport stage", err)
	}
}
