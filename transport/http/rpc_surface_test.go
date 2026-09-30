package http

import "testing"

// surfaceApp declares an explicit RPC surface; Close and Internal are
// exported but not in it.
type surfaceApp struct{ closed bool }

func (s *surfaceApp) RPCMethods() []string { return []string{"Hello", "PickFolder"} }
func (s *surfaceApp) Hello() string        { return "hi" }
func (s *surfaceApp) Close()               { s.closed = true }
func (s *surfaceApp) Internal() string     { return "secret" }
func (s *surfaceApp) PickFolder() string   { return "native" }

// TestDispatcherHonoursRPCSurface: only declared methods are callable.
// Before, reflection exposed every exported method, so any device
// token could call e.g. Runtime.Close.
func TestDispatcherHonoursRPCSurface(t *testing.T) {
	app := &surfaceApp{}
	d := NewDispatcher(app, DefaultDeniedMethods())

	if resp := callRPC(t, d, "Hello", nil); resp.Error != nil || resp.Result != "hi" {
		t.Fatalf("Hello = %+v, want hi", resp)
	}
	for _, m := range []string{"Close", "Internal", "RPCMethods"} {
		resp := callRPC(t, d, m, nil)
		if resp.Error == nil || resp.Error.Code != ErrMethodNotFound {
			t.Errorf("%s: got %+v, want method not found", m, resp)
		}
	}
	if app.closed {
		t.Fatal("Close ran over RPC")
	}
	// The deny list still applies on top of the surface.
	if resp := callRPC(t, d, "PickFolder", nil); resp.Error == nil || resp.Error.Code != ErrForbidden {
		t.Errorf("PickFolder: got %+v, want forbidden", resp)
	}
}
