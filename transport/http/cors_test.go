package http

import (
	nethttp "net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSPreflightAllowed(t *testing.T) {
	h := withCORS(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		t.Fatal("inner handler should not run for OPTIONS preflight")
	}))

	req := httptest.NewRequest(nethttp.MethodOptions, "/rpc", nil)
	req.Header.Set("Origin", "http://wails.localhost:34115")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://wails.localhost:34115" {
		t.Errorf("Allow-Origin = %q, want echoed origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("Allow-Headers missing")
	}
}

func TestCORSActualRequestGetsHeaders(t *testing.T) {
	called := false
	h := withCORS(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		called = true
		w.WriteHeader(nethttp.StatusOK)
	}))

	req := httptest.NewRequest(nethttp.MethodPost, "/rpc", nil)
	req.Header.Set("Origin", "http://wails.localhost:34115")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !called {
		t.Fatal("inner handler not called for non-OPTIONS request")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://wails.localhost:34115" {
		t.Errorf("Allow-Origin = %q, want echoed origin", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}
}

func TestCORSNoOriginHeaderSkipsEcho(t *testing.T) {
	// Direct curl/browser-to-localhost requests often omit Origin.
	// The middleware should not set Allow-Origin in that case —
	// doing so would be noise, not functionality.
	h := withCORS(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.WriteHeader(nethttp.StatusOK)
	}))
	req := httptest.NewRequest(nethttp.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin set without Origin request header: %q", got)
	}
}
