package http

import (
	"context"
	"encoding/json"
	"fmt"
	nethttp "net/http"
	"reflect"
	"strings"
	"sync"
)

// JSON-RPC 2.0 wire format. Positional params only (simpler reflection
// path, matches Wails binding convention). Named params could be added
// later by inspecting the method's argument names via go/types, but
// positional is fine for v1.

type rpcRequest struct {
	JSONRPC string            `json:"jsonrpc"`
	Method  string            `json:"method"`
	Params  []json.RawMessage `json:"params,omitempty"`
	ID      json.RawMessage   `json:"id,omitempty"` // string | number | null
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Standard JSON-RPC error codes plus BRUV extensions.
const (
	ErrParse          = -32700
	ErrInvalidRequest = -32600
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternal       = -32603
	ErrForbidden      = -32000 // BRUV extension: method denied by policy
)

// Dispatcher invokes methods on a target struct by name via reflection.
// Target is typically *App but can be any struct whose exported methods
// form the RPC surface.
type Dispatcher struct {
	target  any
	methods sync.Map // name → reflect.Value
	denied  map[string]bool
}

// NewDispatcher builds a dispatcher rooted at target. Method resolution
// is case-sensitive and happens at construction time for determinism
// and fast lookup; adding new methods requires restart.
func NewDispatcher(target any, deniedMethods []string) *Dispatcher {
	d := &Dispatcher{target: target, denied: make(map[string]bool, len(deniedMethods))}
	for _, n := range deniedMethods {
		d.denied[n] = true
	}

	v := reflect.ValueOf(target)
	t := v.Type()
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		// Only exported methods (Go reflection's NumMethod already
		// filters these, but double-checking is cheap and explicit).
		if !m.IsExported() {
			continue
		}
		d.methods.Store(m.Name, v.Method(i))
	}
	return d
}

// Handler returns the HTTP handler. Expects POST with a JSON-RPC body.
// The handler is intentionally auth-gated by the caller (server.go
// wraps it in requireAuth) so dispatch logic stays focused.
func (d *Dispatcher) Handler() nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != nethttp.MethodPost {
			writeRPCError(w, nil, ErrInvalidRequest, "method must be POST", nil)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeRPCError(w, nil, ErrParse, "invalid JSON: "+err.Error(), nil)
			return
		}
		if req.JSONRPC != "2.0" {
			writeRPCError(w, req.ID, ErrInvalidRequest, "jsonrpc field must be \"2.0\"", nil)
			return
		}
		if req.Method == "" {
			writeRPCError(w, req.ID, ErrInvalidRequest, "method is required", nil)
			return
		}

		result, rpcErr := d.Dispatch(r.Context(), req.Method, req.Params)
		if rpcErr != nil {
			writeRPCError(w, req.ID, rpcErr.Code, rpcErr.Message, rpcErr.Data)
			return
		}
		writeRPCResult(w, req.ID, result)
	}
}

// Dispatch invokes the named method with positional JSON-raw params.
// Returns the method's result (or tuple of results) on success. If the
// method returns a final error value, that's reported as an RPC error.
func (d *Dispatcher) Dispatch(ctx context.Context, method string, rawParams []json.RawMessage) (any, *rpcError) {
	if d.denied[method] {
		return nil, &rpcError{Code: ErrForbidden, Message: "method denied by policy: " + method}
	}
	val, ok := d.methods.Load(method)
	if !ok {
		return nil, &rpcError{Code: ErrMethodNotFound, Message: "method not found: " + method}
	}
	fn := val.(reflect.Value)
	fnType := fn.Type()

	// Some methods take context.Context as their first arg; detect and
	// inject the request context. Matches Go's common idiom.
	wantCtx := fnType.NumIn() > 0 && fnType.In(0) == reflect.TypeOf((*context.Context)(nil)).Elem()
	firstParamIdx := 0
	if wantCtx {
		firstParamIdx = 1
	}
	expected := fnType.NumIn() - firstParamIdx
	if len(rawParams) != expected {
		return nil, &rpcError{
			Code:    ErrInvalidParams,
			Message: fmt.Sprintf("method %s expects %d params, got %d", method, expected, len(rawParams)),
		}
	}

	args := make([]reflect.Value, fnType.NumIn())
	if wantCtx {
		args[0] = reflect.ValueOf(ctx)
	}
	for i := 0; i < expected; i++ {
		argType := fnType.In(i + firstParamIdx)
		argPtr := reflect.New(argType)
		if err := json.Unmarshal(rawParams[i], argPtr.Interface()); err != nil {
			return nil, &rpcError{
				Code:    ErrInvalidParams,
				Message: fmt.Sprintf("param %d: %s", i, err.Error()),
			}
		}
		args[i+firstParamIdx] = argPtr.Elem()
	}

	// Panic-recover so a single buggy method doesn't take the whole
	// server down. Converted into an internal error on the wire.
	var rpcErr *rpcError
	var results []reflect.Value
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				rpcErr = &rpcError{Code: ErrInternal, Message: fmt.Sprintf("panic: %v", rec)}
			}
		}()
		results = fn.Call(args)
	}()
	if rpcErr != nil {
		return nil, rpcErr
	}

	// If the last return value is of type error, split it off.
	numOut := fnType.NumOut()
	var trailingError error
	if numOut > 0 && fnType.Out(numOut-1).Name() == "error" {
		if errVal := results[numOut-1]; !errVal.IsNil() {
			trailingError = errVal.Interface().(error)
		}
		results = results[:numOut-1]
	}
	if trailingError != nil {
		return nil, &rpcError{Code: ErrInternal, Message: trailingError.Error()}
	}

	// Package remaining results. 0 returns → null, 1 → raw, >1 → array.
	switch len(results) {
	case 0:
		return nil, nil
	case 1:
		return results[0].Interface(), nil
	default:
		out := make([]any, len(results))
		for i, r := range results {
			out[i] = r.Interface()
		}
		return out, nil
	}
}

func writeRPCError(w nethttp.ResponseWriter, id json.RawMessage, code int, msg string, data any) {
	resp := rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg, Data: data}}
	_ = json.NewEncoder(w).Encode(resp)
}

func writeRPCResult(w nethttp.ResponseWriter, id json.RawMessage, result any) {
	resp := rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
	_ = json.NewEncoder(w).Encode(resp)
}

// DefaultDeniedMethods is the baseline method-denial list. Dangerous
// methods (native file pickers, force-quit, lifecycle hooks) are
// blocked from the RPC surface even though they're exported for Wails.
// Phase 6 may tighten this to an explicit allowlist.
func DefaultDeniedMethods() []string {
	return []string{
		// Native shell/OS calls that only make sense with a Wails runtime.
		"PickFolder", "PickFile", "PickSaveFile",
		"OpenConfigFolder", "OpenLogsFolder", "OpenBugReportURL",
		// Process lifecycle — never callable over RPC.
		"ForceQuit",
	}
}

// quickName is a tiny helper used in tests to produce a readable dump
// of the registered method names.
func (d *Dispatcher) methodNames() []string {
	var names []string
	d.methods.Range(func(k, _ any) bool {
		names = append(names, k.(string))
		return true
	})
	return names
}

// ensure unused-import warnings don't complain in dev when the helper
// above is temporarily commented out.
var _ = strings.TrimSpace
