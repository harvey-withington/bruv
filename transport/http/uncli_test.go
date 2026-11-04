package http

import (
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUncliGuard: who reaches the server's UNCLI, which can run tools on
// the machine. Only a device token in a bearer header gets through
// (never ?token=); UNCLI's signed links pass without one.
func TestUncliGuard(t *testing.T) {
	dir := t.TempDir()
	store, token := enrolHelper(t, dir)
	bootstrap, err := os.ReadFile(filepath.Join(dir, "bootstrap-token.txt"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		path   string
		bearer string
		want   int
	}{
		{name: "no token", path: "/uncli/rpc", want: 401},
		{name: "token in the query", path: "/uncli/rpc?token=" + token, want: 401},
		{name: "bearer", path: "/uncli/rpc", bearer: token, want: 200},
		{name: "bootstrap token", path: "/uncli/rpc", bearer: strings.TrimSpace(string(bootstrap)), want: 401},
		{name: "unknown token", path: "/uncli/rpc", bearer: "nope", want: 401},
		{name: "signed link", path: "/uncli/l/artifact?sig=x", want: 200},
		{name: "not quite a signed link", path: "/uncli/rpc/l/x", want: 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := uncliGuard(store, nethttp.HandlerFunc(func(nethttp.ResponseWriter, *nethttp.Request) {}))
			req := httptest.NewRequest(nethttp.MethodPost, tc.path, nil)
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// TestUncliOriginAllowed: the pages that may call the server's UNCLI.
func TestUncliOriginAllowed(t *testing.T) {
	cases := []struct {
		name      string
		origin    string
		remote    string
		forwarded string
		want      bool
	}{
		{name: "same host", origin: "https://bruv.example", want: true},
		{name: "another site", origin: "https://evil.example", want: false},
		{name: "desktop webview", origin: "http://wails.localhost", remote: "100.64.0.9:50000", want: true},
		{name: "dev server via loopback", origin: "http://localhost:5174", remote: "127.0.0.1:50000", want: true},
		{name: "loopback page from afar", origin: "http://localhost:5174", remote: "100.64.0.9:50000", want: false},
		{name: "loopback proxy forwarding the name", origin: "https://box.tail.ts.net", remote: "127.0.0.1:50000", forwarded: "box.tail.ts.net", want: true},
		{name: "forwarded name from afar", origin: "https://box.tail.ts.net", remote: "100.64.0.9:50000", forwarded: "box.tail.ts.net", want: false},
		{name: "not an origin", origin: "null", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(nethttp.MethodPost, "https://bruv.example/uncli/rpc", nil)
			if tc.remote != "" {
				r.RemoteAddr = tc.remote
			} else {
				r.RemoteAddr = "100.64.0.9:50000"
			}
			if tc.forwarded != "" {
				r.Header.Set("X-Forwarded-Host", tc.forwarded)
			}
			if got := UncliOriginAllowed(tc.origin, r); got != tc.want {
				t.Errorf("UncliOriginAllowed(%q) = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}

// SelfEnrol enrols once and reuses the cached token after, re-enrolling
// when the cached one stops working.
func TestSelfEnrol(t *testing.T) {
	dir := t.TempDir()
	store, err := NewDeviceStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "own-token.txt")
	first, err := store.SelfEnrol(cache, dir, "own client")
	if err != nil {
		t.Fatal(err)
	}
	if dev := store.LookupDevice(first); dev == nil || dev.Scope != "device" || dev.Name != "own client" {
		t.Fatalf("device = %+v", dev)
	}
	again, err := store.SelfEnrol(cache, dir, "own client")
	if err != nil || again != first {
		t.Errorf("second call = %q, %v; want the cached token", again, err)
	}
	if err := os.WriteFile(cache, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fresh, err := store.SelfEnrol(cache, dir, "own client")
	if err != nil || fresh == "stale" || store.LookupDevice(fresh) == nil {
		t.Errorf("after a stale cache = %q, %v", fresh, err)
	}
}
