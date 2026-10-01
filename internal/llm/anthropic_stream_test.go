package llm

// Streamed Messages API responses. Every Anthropic call streams; these pin
// the assembly from events back into one response — text, tool input
// arriving in fragments, hidden thinking, and the ways a stream can end.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// sse renders events (raw JSON objects) as a server-sent-event body, each
// under its own "event:" line the way the API sends them.
func sse(t *testing.T, events ...string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(": keep-alive comment\n\n")
	for _, e := range events {
		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(e), &head); err != nil {
			t.Fatalf("bad test event %s: %v", e, err)
		}
		fmt.Fprintf(&b, "event: %s\ndata: %s\n\n", head.Type, e)
	}
	return b.String()
}

func streamStub(t *testing.T, body string) *stub {
	t.Helper()
	return newStub(t, http.StatusOK, body, map[string]string{"Content-Type": "text/event-stream; charset=utf-8"})
}

const (
	evStart = `{"type":"message_start","message":{"model":"claude-opus-5-5","content":[],"usage":{"input_tokens":10,"output_tokens":1}}}`
	evStop  = `{"type":"message_stop"}`
)

func streamCall(t *testing.T, s *stub, onOutput func(int)) (*ChatResponse, error) {
	t.Helper()
	return NewAnthropic("k", s.URL).ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-opus-5-5",
		Messages: []Message{{Role: "user", Content: "hi"}},
		OnOutput: onOutput,
	})
}

func TestAnthropicStreamText(t *testing.T) {
	thinking := strings.Repeat("hidden reasoning ", 50)
	s := streamStub(t, sse(t,
		evStart,
		`{"type":"ping"}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"`+thinking+`"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"abc"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Hello, "}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"world, again."}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":812}}`,
		evStop,
	))
	var estimates []int
	resp, err := streamCall(t, s, func(n int) { estimates = append(estimates, n) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello, world, again." {
		t.Errorf("Content = %q", resp.Content)
	}
	if strings.Contains(resp.Content, "hidden reasoning") {
		t.Error("thinking leaked into Content")
	}
	if resp.StopReason != StopEnd || resp.Model != "claude-opus-5-5" {
		t.Errorf("StopReason = %q, Model = %q", resp.StopReason, resp.Model)
	}
	// Output tokens come from the final message_delta (cumulative, and the
	// only count that includes thinking); input from message_start.
	if resp.Usage == nil || resp.Usage.PromptTokens != 10 || resp.Usage.CompletionTokens != 812 || resp.Usage.TotalTokens != 822 {
		t.Errorf("Usage = %+v, want 10 + 812", resp.Usage)
	}
	// One estimate per visible delta, ~4 characters a token; thinking
	// deltas are hidden and must not count.
	if len(estimates) != 2 || estimates[1] != len("Hello, world, again.")/4 {
		t.Errorf("OnOutput estimates = %v, want 2 calls ending at %d", estimates, len("Hello, world, again.")/4)
	}
}

func TestAnthropicStreamToolInputInFragments(t *testing.T) {
	s := streamStub(t, sse(t,
		evStart,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Retitling."}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"set_title","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"title\": \"Ship"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":" it\", \"meta\": {\"n\": 2}}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_2","name":"list_cards","input":{}}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":40}}`,
		evStop,
	))
	resp, err := streamCall(t, s, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Retitling." || resp.StopReason != StopToolUse {
		t.Errorf("Content = %q, StopReason = %q", resp.Content, resp.StopReason)
	}
	if len(resp.ToolCalls) != 2 {
		t.Fatalf("got %d tool calls, want 2", len(resp.ToolCalls))
	}
	first := resp.ToolCalls[0]
	if first.ID != "toolu_1" || first.Name != "set_title" || first.Arguments["title"] != "Ship it" {
		t.Errorf("first call = %+v", first)
	}
	if meta, ok := first.Arguments["meta"].(map[string]any); !ok || meta["n"] != float64(2) {
		t.Errorf("nested input lost: %v", first.Arguments["meta"])
	}
	// A call with no arguments streams no input at all — it still gets an
	// empty object, never nil.
	if args := resp.ToolCalls[1].Arguments; args == nil || len(args) != 0 {
		t.Errorf("argument-less call Arguments = %v, want {}", args)
	}
}

// Cut off by the output cap mid-arguments: the text that arrived is kept,
// the half-written call is dropped instead of being run with half its input.
func TestAnthropicStreamCutOffDropsPartialToolCall(t *testing.T) {
	s := streamStub(t, sse(t,
		evStart,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Here is the start"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"set_title","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"title\": \"Sh"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":16000}}`,
		evStop,
	))
	resp, err := streamCall(t, s, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StopReason != StopMaxTokens {
		t.Errorf("StopReason = %q, want max_tokens", resp.StopReason)
	}
	if resp.Content != "Here is the start" {
		t.Errorf("Content = %q", resp.Content)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("a cut-off tool call must be dropped, got %+v", resp.ToolCalls)
	}
}

// Malformed arguments on a call that did finish are a broken response,
// not something to run.
func TestAnthropicStreamBadToolInputIsAnError(t *testing.T) {
	s := streamStub(t, sse(t,
		evStart,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"set_title","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"title\": "}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":9}}`,
		evStop,
	))
	if _, err := streamCall(t, s, nil); err == nil || !strings.Contains(err.Error(), "unparseable arguments") {
		t.Errorf("err = %v, want an unparseable-arguments error", err)
	}
}

func TestAnthropicStreamErrorEvents(t *testing.T) {
	t.Run("overloaded is retryable", func(t *testing.T) {
		s := streamStub(t, sse(t, evStart, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`))
		_, err := streamCall(t, s, nil)
		if rle := AsRateLimitError(err); rle == nil || rle.StatusCode != 529 {
			t.Errorf("err = %v, want a 529 *RateLimitError", err)
		}
	})
	t.Run("other errors are not", func(t *testing.T) {
		s := streamStub(t, sse(t, evStart, `{"type":"error","error":{"type":"api_error","message":"Internal"}}`))
		_, err := streamCall(t, s, nil)
		if err == nil || IsRateLimitError(err) || !strings.Contains(err.Error(), "api_error") {
			t.Errorf("err = %v, want a plain api_error", err)
		}
	})
}

// A stream that stops before message_stop (dropped connection) must not
// pass for a complete reply.
func TestAnthropicStreamEndingEarlyIsAnError(t *testing.T) {
	s := streamStub(t, sse(t,
		evStart,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Half a"}}`,
	))
	if _, err := streamCall(t, s, nil); err == nil || !strings.Contains(err.Error(), "ended before") {
		t.Errorf("err = %v, want a stream-ended-early error", err)
	}
}

func TestAnthropicStopReasons(t *testing.T) {
	for raw, want := range map[string]StopReason{
		"end_turn":                      StopEnd,
		"stop_sequence":                 StopEnd,
		"tool_use":                      StopToolUse,
		"max_tokens":                    StopMaxTokens,
		"model_context_window_exceeded": StopMaxTokens,
		"refusal":                       StopRefusal,
	} {
		body := strings.Replace(fixture(t, "anthropic_text.json"), `"stop_reason": "end_turn"`, `"stop_reason": "`+raw+`"`, 1)
		resp, err := NewAnthropic("k", newStub(t, http.StatusOK, body, nil).URL).ChatCompletion(context.Background(), ChatRequest{
			Model:    "claude-opus-5-5",
			Messages: []Message{{Role: "user", Content: "hi"}},
		})
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", raw, err)
		}
		if resp.StopReason != want {
			t.Errorf("stop_reason %q -> %q, want %q", raw, resp.StopReason, want)
		}
	}
}
