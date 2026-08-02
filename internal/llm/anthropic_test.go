package llm

// Anthropic Messages API adapter. Two things are distinctly Anthropic-shaped
// and easy to get silently wrong:
//   - the system prompt is a TOP-LEVEL "system" field, not a message with
//     role "system" (OpenAI/Ollama do the opposite);
//   - tool calls and tool results are CONTENT BLOCKS, not sibling fields —
//     tool_use on an assistant message, tool_result inside a *user* message.

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// --- request shape ----------------------------------------------------------

func TestAnthropicRequestShape(t *testing.T) {
	s := okStub(t, "anthropic_text.json")
	p := NewAnthropic("sk-ant-test-key", s.URL)

	_, err := p.ChatCompletion(context.Background(), ChatRequest{
		SystemPrompt: "You are a card assistant.",
		Model:        "claude-sonnet-4-5-20250929",
		MaxTokens:    1024,
		Messages: []Message{
			{Role: "user", Content: "Summarise this card."},
			{Role: "assistant", Content: "Sure."},
			{Role: "user", Content: "Thanks."},
		},
		Tools: []ToolDef{sampleTool()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := s.lastRequest(t)
	if req.path != "/v1/messages" {
		t.Errorf("path = %q, want /v1/messages", req.path)
	}
	// Anthropic authenticates with x-api-key, NOT a bearer token, and rejects
	// requests without an explicit API version.
	if got := req.header.Get("x-api-key"); got != "sk-ant-test-key" {
		t.Errorf("x-api-key = %q, want sk-ant-test-key", got)
	}
	if got := req.header.Get("anthropic-version"); got != "2023-06-01" {
		t.Errorf("anthropic-version = %q, want 2023-06-01", got)
	}
	if got := req.header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want empty (Anthropic uses x-api-key)", got)
	}

	body := s.lastBody(t)
	if got := digStr(t, body, "model"); got != "claude-sonnet-4-5-20250929" {
		t.Errorf("model = %q", got)
	}
	if got := digNum(t, body, "max_tokens"); got != 1024 {
		t.Errorf("max_tokens = %v, want 1024", got)
	}
	// System prompt is hoisted out of the message list entirely.
	if got := digStr(t, body, "system"); got != "You are a card assistant." {
		t.Errorf("system = %q", got)
	}
	msgs := digSlice(t, body, "messages")
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3 (system must NOT be a message)", len(msgs))
	}
	for i, want := range []struct{ role, content string }{
		{"user", "Summarise this card."},
		{"assistant", "Sure."},
		{"user", "Thanks."},
	} {
		if got := digStr(t, body, "messages", i, "role"); got != want.role {
			t.Errorf("messages[%d].role = %q, want %q", i, got, want.role)
		}
		if got := digStr(t, body, "messages", i, "content"); got != want.content {
			t.Errorf("messages[%d].content = %q, want %q", i, got, want.content)
		}
	}

	// Tools use "input_schema" — OpenAI's "parameters" key here would make
	// Anthropic 400 on every tool-enabled call.
	tools := digSlice(t, body, "tools")
	if len(tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(tools))
	}
	if got := digStr(t, body, "tools", 0, "name"); got != "set_title" {
		t.Errorf("tools[0].name = %q", got)
	}
	if got := digStr(t, body, "tools", 0, "description"); got != "Set the card title." {
		t.Errorf("tools[0].description = %q", got)
	}
	schema := digMap(t, body, "tools", 0, "input_schema")
	if schema["type"] != "object" {
		t.Errorf("input_schema.type = %v, want object", schema["type"])
	}
	wantAbsent(t, digMap(t, body, "tools", 0), "parameters")
	// Nested properties must survive marshalling intact.
	if got := digStr(t, body, "tools", 0, "input_schema", "properties", "meta", "properties", "confidence", "type"); got != "string" {
		t.Errorf("nested schema property lost: %q", got)
	}

	// FINDING: ChatRequest has no Temperature field, so no adapter can send
	// one (provider.go:67). Locked here so adding the knob is a deliberate,
	// test-visible change rather than an accident.
	wantAbsent(t, body, "temperature")
	wantAbsent(t, body, "stream")
}

func TestAnthropicDefaultMaxTokens(t *testing.T) {
	s := okStub(t, "anthropic_text.json")
	// Anthropic REQUIRES max_tokens; omitting it is a 400. MaxTokens=0 must
	// therefore become a concrete default, not be dropped.
	_, err := NewAnthropic("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-5-20250929",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := digNum(t, s.lastBody(t), "max_tokens"); got != 4096 {
		t.Errorf("default max_tokens = %v, want 4096", got)
	}
}

func TestAnthropicOmitsEmptyOptionals(t *testing.T) {
	s := okStub(t, "anthropic_text.json")
	_, err := NewAnthropic("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-5-20250929",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := s.lastBody(t)
	// Sending "system": "" or "tools": null is a 400 on the real API.
	wantAbsent(t, body, "system")
	wantAbsent(t, body, "tools")
}

// The round trip RunLoop actually performs: assistant tool call, then result.
func TestAnthropicToolResultRoundTrip(t *testing.T) {
	s := okStub(t, "anthropic_text.json")
	_, err := NewAnthropic("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-5-20250929",
		Messages: toolRoundTripMessages(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := s.lastBody(t)
	msgs := digSlice(t, body, "messages")
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3", len(msgs))
	}

	// [1] assistant: text block first, then tool_use with the arguments under
	// "input" (not "arguments") and the id Anthropic will echo back.
	if got := digStr(t, body, "messages", 1, "role"); got != "assistant" {
		t.Errorf("messages[1].role = %q", got)
	}
	blocks := digSlice(t, body, "messages", 1, "content")
	if len(blocks) != 2 {
		t.Fatalf("assistant content has %d blocks, want 2 (text + tool_use)", len(blocks))
	}
	if got := digStr(t, body, "messages", 1, "content", 0, "type"); got != "text" {
		t.Errorf("block 0 type = %q, want text", got)
	}
	if got := digStr(t, body, "messages", 1, "content", 0, "text"); got != "I'll retitle it." {
		t.Errorf("block 0 text = %q", got)
	}
	if got := digStr(t, body, "messages", 1, "content", 1, "type"); got != "tool_use" {
		t.Errorf("block 1 type = %q, want tool_use", got)
	}
	if got := digStr(t, body, "messages", 1, "content", 1, "id"); got != "call_abc123" {
		t.Errorf("tool_use id = %q — must match the tool_result id or Anthropic 400s", got)
	}
	if got := digStr(t, body, "messages", 1, "content", 1, "name"); got != "set_title" {
		t.Errorf("tool_use name = %q", got)
	}
	if got := digStr(t, body, "messages", 1, "content", 1, "input", "title"); got != "Ship the web clipper" {
		t.Errorf("tool_use input.title = %q", got)
	}
	// Nested argument object must not be flattened or stringified.
	if got := digStr(t, body, "messages", 1, "content", 1, "input", "meta", "confidence"); got != "high" {
		t.Errorf("tool_use nested input lost: %q", got)
	}

	// [2] the result comes back as a USER message with a tool_result block —
	// Anthropic has no "tool" role.
	if got := digStr(t, body, "messages", 2, "role"); got != "user" {
		t.Errorf("messages[2].role = %q, want user (Anthropic has no tool role)", got)
	}
	if got := digStr(t, body, "messages", 2, "content", 0, "type"); got != "tool_result" {
		t.Errorf("messages[2] block type = %q, want tool_result", got)
	}
	if got := digStr(t, body, "messages", 2, "content", 0, "tool_use_id"); got != "call_abc123" {
		t.Errorf("tool_use_id = %q", got)
	}
	if got := digStr(t, body, "messages", 2, "content", 0, "content"); got != "Title updated." {
		t.Errorf("tool_result content = %q", got)
	}
}

// An assistant turn that is pure tool call (no prose) must not emit an empty
// text block — Anthropic rejects `{"type":"text","text":""}`.
func TestAnthropicToolCallWithoutTextEmitsNoTextBlock(t *testing.T) {
	s := okStub(t, "anthropic_text.json")
	_, err := NewAnthropic("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model: "claude-sonnet-4-5-20250929",
		Messages: []Message{
			{Role: "user", Content: "Retitle."},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "t1", Name: "set_title", Arguments: map[string]any{"title": "X"}}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := digSlice(t, s.lastBody(t), "messages", 1, "content")
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1 (tool_use only, no empty text block)", len(blocks))
	}
	if got := digStr(t, s.lastBody(t), "messages", 1, "content", 0, "type"); got != "tool_use" {
		t.Errorf("block type = %q, want tool_use", got)
	}
}

// --- response parsing -------------------------------------------------------

func TestAnthropicParseTextResponse(t *testing.T) {
	s := okStub(t, "anthropic_text.json")
	resp, err := NewAnthropic("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-5-20250929",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Multiple text blocks concatenate; the thinking block is dropped rather
	// than leaking reasoning into the visible answer.
	want := "The card looks complete. Nothing is out of place."
	if resp.Content != want {
		t.Errorf("Content = %q, want %q", resp.Content, want)
	}
	if strings.Contains(resp.Content, "The user is asking") {
		t.Error("thinking block leaked into Content")
	}
	if resp.Model != "claude-sonnet-4-5-20250929" {
		t.Errorf("Model = %q", resp.Model)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(resp.ToolCalls))
	}
	if resp.Usage == nil {
		t.Fatal("Usage is nil — cost tracking depends on it")
	}
	// FINDING (anthropic.go:149-155): usage.cache_read_input_tokens (18432 in
	// the fixture) and cache_creation_input_tokens are ignored. Anthropic
	// reports cached input SEPARATELY from input_tokens, so with prompt
	// caching enabled the recorded prompt cost is only the uncached slice.
	// Locked as-is; it under-reports, it does not crash.
	if resp.Usage.PromptTokens != 2095 {
		t.Errorf("PromptTokens = %d, want 2095", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 503 {
		t.Errorf("CompletionTokens = %d, want 503", resp.Usage.CompletionTokens)
	}
	// Anthropic sends no total — the adapter must synthesise it.
	if resp.Usage.TotalTokens != 2598 {
		t.Errorf("TotalTokens = %d, want 2598 (input+output)", resp.Usage.TotalTokens)
	}
}

func TestAnthropicParseToolUseResponse(t *testing.T) {
	s := okStub(t, "anthropic_tool_use.json")
	resp, err := NewAnthropic("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-5-20250929",
		Messages: []Message{{Role: "user", Content: "hi"}},
		Tools:    []ToolDef{sampleTool()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Prose alongside tool calls must survive — RunLoop replays it as the
	// assistant turn when feeding results back.
	if resp.Content != "I'll retitle the card and tag it." {
		t.Errorf("Content = %q", resp.Content)
	}
	if len(resp.ToolCalls) != 2 {
		t.Fatalf("got %d tool calls, want 2", len(resp.ToolCalls))
	}

	first := resp.ToolCalls[0]
	if first.ID != "toolu_01A09q90qw90lq917835lq9" {
		t.Errorf("ToolCalls[0].ID = %q — the id must round-trip or the tool_result is orphaned", first.ID)
	}
	if first.Name != "set_title" {
		t.Errorf("ToolCalls[0].Name = %q", first.Name)
	}
	if got := first.Arguments["title"]; got != "Ship the web clipper" {
		t.Errorf("Arguments[title] = %v", got)
	}
	// Anthropic sends input as a real JSON object, so nesting must arrive
	// as nested maps/slices, not as a string.
	meta, ok := first.Arguments["meta"].(map[string]any)
	if !ok {
		t.Fatalf("Arguments[meta] = %T, want map[string]any", first.Arguments["meta"])
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
	if !ok || len(tags) != 2 || tags[0] != "release" {
		t.Errorf("meta.tags = %v", meta["tags"])
	}

	if resp.ToolCalls[1].Name != "add_tags" {
		t.Errorf("ToolCalls[1].Name = %q, want add_tags", resp.ToolCalls[1].Name)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 2901 {
		t.Errorf("Usage = %+v, want total 2901", resp.Usage)
	}
}

// DUBIOUS BEHAVIOUR (anthropic.go:157-168): an empty content array yields a
// successful response with empty Content and no tool calls. OpenAI's adapter
// guards the analogous case (openai.go:147 "no choices in response") but
// Anthropic's does not, so RunLoop treats it as a final answer and posts a
// blank assistant message to the chat. Locked to current behaviour; see the
// report rather than "fixing" it here.
func TestAnthropicEmptyContentIsSilentSuccess(t *testing.T) {
	s := okStub(t, "anthropic_empty_content.json")
	resp, err := NewAnthropic("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-5-20250929",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
	if resp.Content != "" || len(resp.ToolCalls) != 0 {
		t.Errorf("behaviour changed: %+v", resp)
	}
}

// --- error paths ------------------------------------------------------------

func TestAnthropicErrorStatuses(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		fixture string
		body    string
		want    []string // substrings the error must carry
	}{
		{name: "401 bad key", status: http.StatusUnauthorized, fixture: "anthropic_error_401.json",
			want: []string{"401", "authentication_error"}},
		{name: "400 bad request", status: http.StatusBadRequest,
			body: `{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens: required"}}`,
			want: []string{"400", "max_tokens"}},
		{name: "500 server error", status: http.StatusInternalServerError,
			body: `{"type":"error","error":{"type":"api_error","message":"Internal server error"}}`,
			want: []string{"500", "api_error"}},
		{name: "503 overloaded", status: http.StatusServiceUnavailable,
			body: `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
			want: []string{"503", "overloaded"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := c.body
			if c.fixture != "" {
				body = fixture(t, c.fixture)
			}
			s := newStub(t, c.status, body, nil)
			resp, err := NewAnthropic("bad-key", s.URL).ChatCompletion(context.Background(), ChatRequest{
				Model:    "claude-sonnet-4-5-20250929",
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
			// Only 429/529 are retryable; everything else must NOT be
			// classified as a rate limit or the caller backs off forever on
			// a permanent failure.
			if IsRateLimitError(err) {
				t.Errorf("HTTP %d misclassified as a rate limit", c.status)
			}
		})
	}
}

func TestAnthropicRateLimitErrors(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		retryAfter string
		want       time.Duration
	}{
		{"429 with Retry-After seconds", http.StatusTooManyRequests, "30", 30 * time.Second},
		{"429 without hint", http.StatusTooManyRequests, "", 0},
		// 529 is Anthropic's "overloaded" code — not a standard HTTP status,
		// and treated as retryable here.
		{"529 overloaded", 529, "5", 5 * time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hdr := map[string]string{}
			if c.retryAfter != "" {
				hdr["Retry-After"] = c.retryAfter
			}
			s := newStub(t, c.status, fixture(t, "anthropic_error_429.json"), hdr)
			_, err := NewAnthropic("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
				Model:    "claude-sonnet-4-5-20250929",
				Messages: []Message{{Role: "user", Content: "hi"}},
			})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			rle := AsRateLimitError(err)
			if rle == nil {
				t.Fatalf("expected *RateLimitError, got %T: %v", err, err)
			}
			if rle.Provider != "anthropic" {
				t.Errorf("Provider = %q", rle.Provider)
			}
			if rle.StatusCode != c.status {
				t.Errorf("StatusCode = %d, want %d", rle.StatusCode, c.status)
			}
			if rle.RetryAfter != c.want {
				t.Errorf("RetryAfter = %v, want %v", rle.RetryAfter, c.want)
			}
			if !strings.Contains(rle.Body, "rate_limit_error") {
				t.Errorf("Body missing server detail: %q", rle.Body)
			}
		})
	}
}

// Error bodies are truncated before they reach the user-facing chat message
// (RunLoop writes err.Error() straight into the transcript), so a giant HTML
// error page must not be pasted in whole.
func TestAnthropicErrorBodyTruncated(t *testing.T) {
	huge := `{"error":"` + strings.Repeat("x", 5000) + `"}`
	s := newStub(t, http.StatusInternalServerError, huge, nil)
	_, err := NewAnthropic("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-5-20250929",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(err.Error()) > 300 {
		t.Errorf("error message is %d chars — body not truncated", len(err.Error()))
	}
	if !strings.HasSuffix(err.Error(), "...") {
		t.Errorf("truncated error should end in ellipsis: %q", err)
	}
}

func TestAnthropicMalformedJSONBody(t *testing.T) {
	s := newStub(t, http.StatusOK, `{"content":[{"type":"text","text":"hel`, nil)
	_, err := NewAnthropic("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-5-20250929",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "parse response") {
		t.Errorf("error %q should identify the parse stage", err)
	}
}

// A dead endpoint (wrong baseURL, Ollama-style local proxy down) must produce
// a transport error, not a panic.
func TestAnthropicUnreachableHost(t *testing.T) {
	// Port 0 on the loopback is never listening.
	_, err := NewAnthropic("k", "http://127.0.0.1:0").ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-5-20250929",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("error %q should identify the transport stage", err)
	}
}
