package llm

// Shared plumbing for the three provider adapter tests, plus the
// cross-provider contracts that must hold for all of them.
//
// The maintainer does not hold keys for all three providers, so these
// adapters are otherwise unverifiable by hand. Every constructor takes a
// baseURL, which is the only seam needed: point it at an httptest server and
// both halves of the adapter — request building and response parsing — run
// for real, against fixtures, with no key and no network.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// --- recording stub server --------------------------------------------------

// capturedRequest is what an adapter actually put on the wire.
type capturedRequest struct {
	method string
	path   string
	header http.Header
	body   []byte
}

// stub is an httptest server that records every request and answers all of
// them with one canned status/body.
type stub struct {
	URL string

	mu       sync.Mutex
	requests []capturedRequest
}

// newStub starts a recording server. respHeader may be nil; it is applied to
// every response (used for Retry-After).
func newStub(t *testing.T, status int, body string, respHeader map[string]string) *stub {
	t.Helper()
	s := &stub{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never assert in here — a failed assertion on a server goroutine is
		// not attributable to the test. Record only; assert on the way out.
		raw, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.requests = append(s.requests, capturedRequest{
			method: r.Method,
			path:   r.URL.Path,
			header: r.Header.Clone(),
			body:   raw,
		})
		s.mu.Unlock()
		for k, v := range respHeader {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	s.URL = srv.URL
	return s
}

// okStub answers 200 with the named fixture.
func okStub(t *testing.T, fixtureName string) *stub {
	t.Helper()
	return newStub(t, http.StatusOK, fixture(t, fixtureName), nil)
}

// lastRequest returns the most recent captured request, failing if none.
func (s *stub) lastRequest(t *testing.T) capturedRequest {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		t.Fatal("adapter made no HTTP request")
	}
	return s.requests[len(s.requests)-1]
}

// lastBody decodes the most recent request body as a JSON object.
func (s *stub) lastBody(t *testing.T) map[string]any {
	t.Helper()
	req := s.lastRequest(t)
	var m map[string]any
	if err := json.Unmarshal(req.body, &m); err != nil {
		t.Fatalf("request body is not a JSON object: %v\nbody: %s", err, req.body)
	}
	return m
}

func (s *stub) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

// --- fixtures ---------------------------------------------------------------

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return string(data)
}

// --- JSON navigation --------------------------------------------------------

// dig walks a decoded JSON tree by object key (string) or array index (int),
// failing with the path travelled so far when a step is missing or mistyped.
// Keeps the request-shape assertions readable at the call site.
func dig(t *testing.T, root any, path ...any) any {
	t.Helper()
	cur := root
	for i, step := range path {
		switch s := step.(type) {
		case string:
			m, ok := cur.(map[string]any)
			if !ok {
				t.Fatalf("dig %v: expected object at %v, got %T", path, path[:i], cur)
			}
			v, ok := m[s]
			if !ok {
				t.Fatalf("dig %v: missing key %q at %v", path, s, path[:i])
			}
			cur = v
		case int:
			a, ok := cur.([]any)
			if !ok {
				t.Fatalf("dig %v: expected array at %v, got %T", path, path[:i], cur)
			}
			if s < 0 || s >= len(a) {
				t.Fatalf("dig %v: index %d out of range (len %d) at %v", path, s, len(a), path[:i])
			}
			cur = a[s]
		default:
			t.Fatalf("dig: bad path step %v (%T)", step, step)
		}
	}
	return cur
}

func digStr(t *testing.T, root any, path ...any) string {
	t.Helper()
	v := dig(t, root, path...)
	s, ok := v.(string)
	if !ok {
		t.Fatalf("dig %v: expected string, got %T (%v)", path, v, v)
	}
	return s
}

func digNum(t *testing.T, root any, path ...any) float64 {
	t.Helper()
	v := dig(t, root, path...)
	n, ok := v.(float64)
	if !ok {
		t.Fatalf("dig %v: expected number, got %T (%v)", path, v, v)
	}
	return n
}

func digBool(t *testing.T, root any, path ...any) bool {
	t.Helper()
	v := dig(t, root, path...)
	b, ok := v.(bool)
	if !ok {
		t.Fatalf("dig %v: expected bool, got %T (%v)", path, v, v)
	}
	return b
}

func digSlice(t *testing.T, root any, path ...any) []any {
	t.Helper()
	v := dig(t, root, path...)
	a, ok := v.([]any)
	if !ok {
		t.Fatalf("dig %v: expected array, got %T (%v)", path, v, v)
	}
	return a
}

func digMap(t *testing.T, root any, path ...any) map[string]any {
	t.Helper()
	v := dig(t, root, path...)
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("dig %v: expected object, got %T (%v)", path, v, v)
	}
	return m
}

// wantAbsent asserts a key was NOT serialised. Used for the "omit when empty"
// contracts (no system prompt, no tools, no auth header).
func wantAbsent(t *testing.T, m map[string]any, key string) {
	t.Helper()
	if v, ok := m[key]; ok {
		t.Errorf("expected key %q to be omitted, got %v", key, v)
	}
}

// --- shared test data -------------------------------------------------------

// sampleTool is a realistic BRUV tool definition with a nested-object property,
// so schema nesting is exercised rather than a flat string bag.
func sampleTool() ToolDef {
	return ToolDef{
		Name:        "set_title",
		Description: "Set the card title.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{"type": "string", "description": "New title"},
				"meta": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"confidence": map[string]any{"type": "string", "enum": []any{"low", "high"}},
					},
				},
			},
			"required": []any{"title"},
		},
	}
}

// toolRoundTripMessages is the exact shape core/runtime/chat.RunLoop feeds back
// after executing a tool: the assistant turn carrying the tool call, then a
// role="tool" message carrying the result keyed by ToolCallID.
// (see core/runtime/chat/chat.go RunLoop)
func toolRoundTripMessages() []Message {
	return []Message{
		{Role: "user", Content: "Retitle this card."},
		{
			Role:    "assistant",
			Content: "I'll retitle it.",
			ToolCalls: []ToolCall{{
				ID:   "call_abc123",
				Name: "set_title",
				Arguments: map[string]any{
					"title": "Ship the web clipper",
					"meta":  map[string]any{"confidence": "high"},
				},
			}},
		},
		{Role: "tool", Content: "Title updated.", ToolCallID: "call_abc123"},
	}
}

// allProviders builds each adapter against a given base URL. Used by the
// cross-provider contract tests below.
var allProviders = []struct {
	name string
	make func(baseURL string) Provider
}{
	{"anthropic", func(u string) Provider { return NewAnthropic("test-key", u) }},
	{"openai", func(u string) Provider { return NewOpenAI("test-key", u) }},
	{"ollama", func(u string) Provider { return NewOllama(u) }},
}

// --- cross-provider contracts -----------------------------------------------

// A truncated body must surface as an error, never as an empty success — an
// empty ChatResponse would look to RunLoop like a legitimate final answer and
// silently end the conversation.
func TestAllProvidersRejectTruncatedJSON(t *testing.T) {
	for _, p := range allProviders {
		t.Run(p.name, func(t *testing.T) {
			s := newStub(t, http.StatusOK, `{"content":[{"type":"text","text":"hel`, nil)
			resp, err := p.make(s.URL).ChatCompletion(context.Background(), ChatRequest{
				Model:    "test-model",
				Messages: []Message{{Role: "user", Content: "hi"}},
			})
			if err == nil {
				t.Fatalf("expected parse error, got response %+v", resp)
			}
			if resp != nil {
				t.Errorf("expected nil response alongside error, got %+v", resp)
			}
		})
	}
}

// An entirely empty 200 body (proxy hiccup, dropped connection) must error too.
func TestAllProvidersRejectEmptyBody(t *testing.T) {
	for _, p := range allProviders {
		t.Run(p.name, func(t *testing.T) {
			s := newStub(t, http.StatusOK, "", nil)
			if _, err := p.make(s.URL).ChatCompletion(context.Background(), ChatRequest{
				Model:    "test-model",
				Messages: []Message{{Role: "user", Content: "hi"}},
			}); err == nil {
				t.Fatal("expected error for empty body, got nil")
			}
		})
	}
}

// A non-JSON body (HTML error page from a reverse proxy in front of a
// self-hosted endpoint) must not be mistaken for a valid response.
func TestAllProvidersRejectHTMLBody(t *testing.T) {
	for _, p := range allProviders {
		t.Run(p.name, func(t *testing.T) {
			s := newStub(t, http.StatusOK, "<html><body>502 Bad Gateway</body></html>", nil)
			if _, err := p.make(s.URL).ChatCompletion(context.Background(), ChatRequest{
				Model:    "test-model",
				Messages: []Message{{Role: "user", Content: "hi"}},
			}); err == nil {
				t.Fatal("expected error for HTML body, got nil")
			}
		})
	}
}

// Every adapter must honour context cancellation — the agent scheduler relies
// on it to abort in-flight runs.
func TestAllProvidersHonourContextCancellation(t *testing.T) {
	for _, p := range allProviders {
		t.Run(p.name, func(t *testing.T) {
			s := okStub(t, "anthropic_text.json") // body irrelevant; never reached
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := p.make(s.URL).ChatCompletion(ctx, ChatRequest{
				Model:    "test-model",
				Messages: []Message{{Role: "user", Content: "hi"}},
			}); err == nil {
				t.Fatal("expected error from cancelled context, got nil")
			}
			if s.count() != 0 {
				t.Errorf("cancelled request still reached the server (%d requests)", s.count())
			}
		})
	}
}

// One ChatCompletion call is exactly one HTTP request — no retry loop hides
// inside an adapter (retry/backoff is the caller's job, driven by
// RateLimitError).
func TestAllProvidersMakeExactlyOneRequest(t *testing.T) {
	fixtures := map[string]string{
		"anthropic": "anthropic_text.json",
		"openai":    "openai_text.json",
		"ollama":    "ollama_text.json",
	}
	for _, p := range allProviders {
		t.Run(p.name, func(t *testing.T) {
			s := okStub(t, fixtures[p.name])
			if _, err := p.make(s.URL).ChatCompletion(context.Background(), ChatRequest{
				Model:    "test-model",
				Messages: []Message{{Role: "user", Content: "hi"}},
			}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := s.count(); got != 1 {
				t.Errorf("made %d HTTP requests, want 1", got)
			}
		})
	}
}

// Every adapter POSTs JSON. Cheap contract, catches a copy-paste GET.
func TestAllProvidersPostJSON(t *testing.T) {
	fixtures := map[string]string{
		"anthropic": "anthropic_text.json",
		"openai":    "openai_text.json",
		"ollama":    "ollama_text.json",
	}
	for _, p := range allProviders {
		t.Run(p.name, func(t *testing.T) {
			s := okStub(t, fixtures[p.name])
			if _, err := p.make(s.URL).ChatCompletion(context.Background(), ChatRequest{
				Model:    "test-model",
				Messages: []Message{{Role: "user", Content: "hi"}},
			}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			req := s.lastRequest(t)
			if req.method != http.MethodPost {
				t.Errorf("method = %s, want POST", req.method)
			}
			if ct := req.header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
		})
	}
}
