package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockApp is a reflection target that exercises every code path the
// dispatcher cares about: no-arg, positional args, ctx injection,
// multiple returns, error return, no-return.
type mockApp struct{}

func (m *mockApp) NoArgs() string                           { return "hello" }
func (m *mockApp) Echo(s string) string                     { return s }
func (m *mockApp) Add(a, b int) int                         { return a + b }
func (m *mockApp) DivideByZero(n int) (int, error) {
	if n == 0 {
		return 0, fmt.Errorf("n must be non-zero")
	}
	return 10 / n, nil
}
func (m *mockApp) WithCtx(ctx context.Context, s string) string {
	if ctx == nil {
		return "nil ctx"
	}
	return "got ctx: " + s
}
func (m *mockApp) MultipleReturns() (string, int) { return "x", 42 }
func (m *mockApp) Panics()                        { panic("boom") }
func (m *mockApp) PickFolder() string             { return "should be denied" }

func callRPC(t *testing.T, d *Dispatcher, method string, params []any) rpcResponse {
	t.Helper()
	rawParams := make([]json.RawMessage, len(params))
	for i, p := range params {
		b, _ := json.Marshal(p)
		rawParams[i] = b
	}
	body := rpcRequest{JSONRPC: "2.0", Method: method, Params: rawParams, ID: json.RawMessage(`1`)}
	buf, _ := json.Marshal(body)

	req := httptest.NewRequest(nethttp.MethodPost, "/rpc", bytes.NewReader(buf))
	rec := httptest.NewRecorder()
	d.Handler()(rec, req)

	var resp rpcResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func TestDispatchNoArgs(t *testing.T) {
	d := NewDispatcher(&mockApp{}, nil)
	resp := callRPC(t, d, "NoArgs", nil)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if resp.Result != "hello" {
		t.Errorf("result = %v, want hello", resp.Result)
	}
}

func TestDispatchPositional(t *testing.T) {
	d := NewDispatcher(&mockApp{}, nil)
	resp := callRPC(t, d, "Echo", []any{"abc"})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if resp.Result != "abc" {
		t.Errorf("result = %v, want abc", resp.Result)
	}

	resp = callRPC(t, d, "Add", []any{2, 3})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	// JSON numbers decode as float64.
	if f, ok := resp.Result.(float64); !ok || f != 5 {
		t.Errorf("Add result = %v (%T), want 5", resp.Result, resp.Result)
	}
}

func TestDispatchErrorReturn(t *testing.T) {
	d := NewDispatcher(&mockApp{}, nil)

	// Happy path: no error.
	resp := callRPC(t, d, "DivideByZero", []any{5})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if f, _ := resp.Result.(float64); f != 2 {
		t.Errorf("result = %v, want 2", resp.Result)
	}

	// Error path: the method's error is reported as an RPC error.
	resp = callRPC(t, d, "DivideByZero", []any{0})
	if resp.Error == nil {
		t.Fatal("expected error, got success")
	}
	if !strings.Contains(resp.Error.Message, "non-zero") {
		t.Errorf("error message = %q, want contains 'non-zero'", resp.Error.Message)
	}
}

func TestDispatchContextInjection(t *testing.T) {
	d := NewDispatcher(&mockApp{}, nil)
	resp := callRPC(t, d, "WithCtx", []any{"hi"})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if resp.Result != "got ctx: hi" {
		t.Errorf("result = %v, want 'got ctx: hi'", resp.Result)
	}
}

func TestDispatchMultipleReturns(t *testing.T) {
	d := NewDispatcher(&mockApp{}, nil)
	resp := callRPC(t, d, "MultipleReturns", nil)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	arr, ok := resp.Result.([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("result = %v, want 2-element array", resp.Result)
	}
	if arr[0] != "x" || arr[1].(float64) != 42 {
		t.Errorf("result = %v, want [x, 42]", arr)
	}
}

func TestDispatchPanicRecovery(t *testing.T) {
	d := NewDispatcher(&mockApp{}, nil)
	resp := callRPC(t, d, "Panics", nil)
	if resp.Error == nil {
		t.Fatal("expected RPC error, got success — panic wasn't caught")
	}
	if !strings.Contains(resp.Error.Message, "boom") {
		t.Errorf("error message = %q, want contains 'boom'", resp.Error.Message)
	}
}

func TestDispatchMethodNotFound(t *testing.T) {
	d := NewDispatcher(&mockApp{}, nil)
	resp := callRPC(t, d, "DoesNotExist", nil)
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != ErrMethodNotFound {
		t.Errorf("code = %d, want %d", resp.Error.Code, ErrMethodNotFound)
	}
}

func TestDispatchParamCountMismatch(t *testing.T) {
	d := NewDispatcher(&mockApp{}, nil)
	resp := callRPC(t, d, "Echo", []any{"one", "two"}) // Echo takes 1
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != ErrInvalidParams {
		t.Errorf("code = %d, want %d", resp.Error.Code, ErrInvalidParams)
	}
}

func TestDispatchDenyList(t *testing.T) {
	d := NewDispatcher(&mockApp{}, []string{"PickFolder"})
	resp := callRPC(t, d, "PickFolder", nil)
	if resp.Error == nil {
		t.Fatal("expected denial error")
	}
	if resp.Error.Code != ErrForbidden {
		t.Errorf("code = %d, want %d", resp.Error.Code, ErrForbidden)
	}
}

func TestRejectsNonPOST(t *testing.T) {
	d := NewDispatcher(&mockApp{}, nil)
	req := httptest.NewRequest(nethttp.MethodGet, "/rpc", nil)
	rec := httptest.NewRecorder()
	d.Handler()(rec, req)

	var resp rpcResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error == nil || resp.Error.Code != ErrInvalidRequest {
		t.Errorf("want ErrInvalidRequest, got %+v", resp.Error)
	}
}
