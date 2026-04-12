package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// Client-level integration tests using an in-memory fake MCP server.
//
// These tests exercise the full Client code path — initialize
// handshake, notifications/initialized, tools/list pagination,
// tools/call with both success and tool-error results — without
// spawning a real subprocess. The fake server runs in-process on
// an io.Pipe pair and speaks just enough of the protocol to
// satisfy the client.
//
// If these tests pass, the real filesystem-server integration
// test (in a separate file, gated on Node being available) has a
// very high prior probability of passing too. The subprocess
// layer in server.go has only one job — wire the right pipes to
// the Client — and that's straightforward once the protocol is
// known correct.

// fakeServer is a trivial in-process MCP server implementation
// driven by a test scenario. It reads line-delimited JSON from the
// "client-to-server" pipe and writes responses to the
// "server-to-client" pipe, one message per client request.
type fakeServer struct {
	rx *io.PipeReader // client → server (fake server reads here)
	tx *io.PipeWriter // server → client (fake server writes here)

	// handler receives each decoded request and returns the
	// response (result or error) the test wants to simulate.
	// Notifications (no id) are passed in but the handler's
	// return value is ignored.
	handler func(req *Response) (result interface{}, errObj *ErrorObject)

	done chan struct{}
}

func newFakeServer(handler func(req *Response) (interface{}, *ErrorObject)) (*fakeServer, io.WriteCloser, io.ReadCloser, io.ReadCloser) {
	// Client-to-server: client writes, fake reads.
	c2sR, c2sW := io.Pipe()
	// Server-to-client: fake writes, client reads.
	s2cR, s2cW := io.Pipe()

	fs := &fakeServer{
		rx:      c2sR,
		tx:      s2cW,
		handler: handler,
		done:    make(chan struct{}),
	}
	go fs.run()

	// Return the pipes from the client's perspective, plus a
	// nil stderr (fake server doesn't emit any).
	return fs, c2sW, s2cR, nil
}

func (f *fakeServer) run() {
	defer close(f.done)
	scanner := bufio.NewScanner(f.rx)
	scanner.Buffer(make([]byte, 64*1024), maxMessageSize)
	for scanner.Scan() {
		var req Response
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		// Notification — handler may inspect it but we don't
		// write anything back.
		if req.ID == nil {
			f.handler(&req)
			continue
		}
		result, errObj := f.handler(&req)
		resp := outgoingResponse{
			JSONRPC: "2.0",
			ID:      *req.ID,
		}
		if errObj != nil {
			resp.Error = errObj
		} else {
			raw, _ := json.Marshal(result)
			resp.Result = raw
		}
		data, _ := json.Marshal(&resp)
		data = append(data, '\n')
		f.tx.Write(data)
	}
}

func (f *fakeServer) close() {
	f.tx.Close()
	f.rx.Close()
}

// TestClientInitializeAndList is the happy-path handshake plus a
// one-page tools/list. Exercises the most common client code path.
func TestClientInitializeAndList(t *testing.T) {
	initializedSeen := false
	var initializedMu sync.Mutex

	handler := func(req *Response) (interface{}, *ErrorObject) {
		switch req.Method {
		case "initialize":
			return InitializeResult{
				ProtocolVersion: ProtocolVersion,
				Capabilities:    map[string]interface{}{},
				ServerInfo:      ServerInfo{Name: "fake", Version: "0.1"},
			}, nil
		case "notifications/initialized":
			// Notification — request has no ID in the real flow.
			// This handler return value is ignored per run().
			initializedMu.Lock()
			initializedSeen = true
			initializedMu.Unlock()
			return nil, nil
		case "tools/list":
			return ListToolsResult{
				Tools: []Tool{
					{
						Name:        "echo",
						Description: "Echoes input back.",
						InputSchema: map[string]interface{}{"type": "object"},
					},
				},
			}, nil
		}
		return nil, &ErrorObject{Code: ErrCodeMethodNotFound, Message: "unknown method: " + req.Method}
	}

	fs, stdin, stdout, stderr := newFakeServer(handler)
	defer fs.close()

	tr := NewTransport("test", stdin, stdout, stderr, nil)
	tr.Start()
	client := NewClient(tr)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if client.ProtocolVersion() != ProtocolVersion {
		t.Errorf("protocol version = %q, want %q", client.ProtocolVersion(), ProtocolVersion)
	}
	if info := client.ServerInfo(); info.Name != "fake" {
		t.Errorf("server name = %q, want %q", info.Name, "fake")
	}

	// Give the notification a moment to land — it's written
	// asynchronously from Initialize and the fake server reads
	// on its own goroutine.
	time.Sleep(50 * time.Millisecond)
	initializedMu.Lock()
	got := initializedSeen
	initializedMu.Unlock()
	if !got {
		t.Error("expected notifications/initialized to reach fake server")
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Errorf("got %+v, want one tool named 'echo'", tools)
	}
	_ = client.Close()
}

// TestClientProtocolVersionDowngrade verifies that a server returning
// an older-but-accepted version is not rejected. This is the real
// world case — many existing servers only speak 2024-11-05.
func TestClientProtocolVersionDowngrade(t *testing.T) {
	handler := func(req *Response) (interface{}, *ErrorObject) {
		if req.Method == "initialize" {
			return InitializeResult{
				ProtocolVersion: "2024-11-05", // older but accepted
				ServerInfo:      ServerInfo{Name: "old-server"},
			}, nil
		}
		return nil, nil
	}
	fs, stdin, stdout, stderr := newFakeServer(handler)
	defer fs.close()
	tr := NewTransport("test", stdin, stdout, stderr, nil)
	tr.Start()
	client := NewClient(tr)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("downgrade should be accepted: %v", err)
	}
	if client.ProtocolVersion() != "2024-11-05" {
		t.Errorf("protocol version = %q, want downgraded value", client.ProtocolVersion())
	}
	_ = client.Close()
}

// TestClientProtocolVersionUnsupported verifies that a server
// returning an unknown version is rejected. This is the "we
// can't talk to future servers" case.
func TestClientProtocolVersionUnsupported(t *testing.T) {
	handler := func(req *Response) (interface{}, *ErrorObject) {
		if req.Method == "initialize" {
			return InitializeResult{
				ProtocolVersion: "2099-12-31", // from the future
				ServerInfo:      ServerInfo{Name: "future-server"},
			}, nil
		}
		return nil, nil
	}
	fs, stdin, stdout, stderr := newFakeServer(handler)
	defer fs.close()
	tr := NewTransport("test", stdin, stdout, stderr, nil)
	tr.Start()
	client := NewClient(tr)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err == nil {
		t.Fatal("expected error for unsupported protocol version")
	}
	_ = client.Close()
}

// TestClientToolsPagination verifies that tools/list chases
// nextCursor correctly. Critical because a server with many
// tools (GitHub has ~60) will paginate and a client that drops
// the second page would silently lose tools.
func TestClientToolsPagination(t *testing.T) {
	callCount := 0
	handler := func(req *Response) (interface{}, *ErrorObject) {
		switch req.Method {
		case "initialize":
			return InitializeResult{ProtocolVersion: ProtocolVersion}, nil
		case "tools/list":
			callCount++
			if callCount == 1 {
				return ListToolsResult{
					Tools:      []Tool{{Name: "a"}, {Name: "b"}},
					NextCursor: "page2",
				}, nil
			}
			return ListToolsResult{
				Tools: []Tool{{Name: "c"}},
			}, nil
		}
		return nil, nil
	}
	fs, stdin, stdout, stderr := newFakeServer(handler)
	defer fs.close()
	tr := NewTransport("test", stdin, stdout, stderr, nil)
	tr.Start()
	client := NewClient(tr)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 3 {
		t.Errorf("got %d tools, want 3 after pagination", len(tools))
	}
	if callCount != 2 {
		t.Errorf("got %d tools/list calls, want 2", callCount)
	}
	_ = client.Close()
}

// TestClientCallToolSuccess verifies the happy tools/call path with
// a text-content result, including the flattening step.
func TestClientCallToolSuccess(t *testing.T) {
	handler := func(req *Response) (interface{}, *ErrorObject) {
		switch req.Method {
		case "initialize":
			return InitializeResult{ProtocolVersion: ProtocolVersion}, nil
		case "tools/call":
			return CallToolResult{
				Content: []Content{{Type: "text", Text: "result-of-the-call"}},
			}, nil
		}
		return nil, nil
	}
	fs, stdin, stdout, stderr := newFakeServer(handler)
	defer fs.close()
	tr := NewTransport("test", stdin, stdout, stderr, nil)
	tr.Start()
	client := NewClient(tr)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	result, err := client.CallTool(ctx, "echo", map[string]interface{}{"msg": "hi"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Errorf("IsError = true, want false")
	}
	flat := FlattenContent(result.Content)
	if flat != "result-of-the-call" {
		t.Errorf("flattened = %q, want %q", flat, "result-of-the-call")
	}
	_ = client.Close()
}

// TestClientCallToolToolError verifies that a tool-level error
// (isError=true in the result body) comes back as a successful
// Go call with IsError set — not as a Go error. This is the
// critical distinction from the spec research: tool errors
// should flow to the LLM, not abort the run.
func TestClientCallToolToolError(t *testing.T) {
	handler := func(req *Response) (interface{}, *ErrorObject) {
		switch req.Method {
		case "initialize":
			return InitializeResult{ProtocolVersion: ProtocolVersion}, nil
		case "tools/call":
			return CallToolResult{
				IsError: true,
				Content: []Content{{Type: "text", Text: "rate limit exceeded"}},
			}, nil
		}
		return nil, nil
	}
	fs, stdin, stdout, stderr := newFakeServer(handler)
	defer fs.close()
	tr := NewTransport("test", stdin, stdout, stderr, nil)
	tr.Start()
	client := NewClient(tr)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	result, err := client.CallTool(ctx, "anything", nil)
	if err != nil {
		t.Fatalf("tool error should not be a Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("IsError = false, want true")
	}
	if FlattenContent(result.Content) != "rate limit exceeded" {
		t.Errorf("content flatten = %q", FlattenContent(result.Content))
	}
	_ = client.Close()
}

// TestClientCallToolProtocolError verifies that a JSON-RPC error
// response (structural failure — bad tool name, malformed args)
// comes back as a Go error and NOT as a successful result. This
// is the other side of the distinction in the previous test.
func TestClientCallToolProtocolError(t *testing.T) {
	handler := func(req *Response) (interface{}, *ErrorObject) {
		switch req.Method {
		case "initialize":
			return InitializeResult{ProtocolVersion: ProtocolVersion}, nil
		case "tools/call":
			return nil, &ErrorObject{
				Code:    ErrCodeInvalidParams,
				Message: "Unknown tool: bogus",
			}
		}
		return nil, nil
	}
	fs, stdin, stdout, stderr := newFakeServer(handler)
	defer fs.close()
	tr := NewTransport("test", stdin, stdout, stderr, nil)
	tr.Start()
	client := NewClient(tr)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	_, err := client.CallTool(ctx, "bogus", nil)
	if err == nil {
		t.Fatal("expected Go error for JSON-RPC protocol error")
	}
	// The error message should contain the server's original
	// message somewhere — we don't care about exact wrapping.
	if !containsHelper(err.Error(), "Unknown tool") {
		t.Errorf("error %q should reference the server's error message", err.Error())
	}
	_ = client.Close()
}

// TestClientDoubleInitializeRejected confirms we reject a second
// Initialize call. The state machine should only allow one.
func TestClientDoubleInitializeRejected(t *testing.T) {
	handler := func(req *Response) (interface{}, *ErrorObject) {
		if req.Method == "initialize" {
			return InitializeResult{ProtocolVersion: ProtocolVersion}, nil
		}
		return nil, nil
	}
	fs, stdin, stdout, stderr := newFakeServer(handler)
	defer fs.close()
	tr := NewTransport("test", stdin, stdout, stderr, nil)
	tr.Start()
	client := NewClient(tr)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("first Initialize: %v", err)
	}
	if err := client.Initialize(ctx); err == nil {
		t.Error("second Initialize should be rejected")
	}
	_ = client.Close()
}

// containsHelper is a test-only substring check avoiding a clash
// with flatten_test.go's contains — go test compiles both files
// into the same package.
func containsHelper(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Suppress unused-var warnings for helpers that might not be used
// in every test run (errors import used for completeness).
var _ = errors.New
