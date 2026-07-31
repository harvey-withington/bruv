package http

import (
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func localPairSetup(t *testing.T) nethttp.HandlerFunc {
	t.Helper()
	dir := t.TempDir()
	store, err := NewDeviceStore(dir)
	if err != nil {
		t.Fatalf("device store: %v", err)
	}
	return localPairHandler(dir, store)
}

func localPairReq(remoteAddr, host, origin string) *nethttp.Request {
	req := httptest.NewRequest(nethttp.MethodPost, "/auth/local-pair", strings.NewReader(`{"device_name":"Test"}`))
	req.RemoteAddr = remoteAddr
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return req
}

func TestLocalPairHappyPaths(t *testing.T) {
	h := localPairSetup(t)
	for _, origin := range []string{
		"chrome-extension://abcdefghijklmnop",
		"moz-extension://uuid-here",
		"http://localhost:5174",
		"http://127.0.0.1:5174",
	} {
		rec := httptest.NewRecorder()
		h(rec, localPairReq("127.0.0.1:54321", "127.0.0.1:9876", origin))
		if rec.Code != nethttp.StatusOK {
			t.Errorf("origin %s: status %d body %s", origin, rec.Code, rec.Body.String())
			continue
		}
		var res enrolResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.DeviceToken == "" || res.DeviceID == "" {
			t.Errorf("origin %s: bad response %s (err %v)", origin, rec.Body.String(), err)
		}
	}
}

func TestLocalPairRejections(t *testing.T) {
	h := localPairSetup(t)
	ext := "chrome-extension://abcdefghijklmnop"
	cases := []struct {
		name string
		req  *nethttp.Request
	}{
		{"non-loopback source", localPairReq("192.168.1.50:44000", "127.0.0.1:9876", ext)},
		{"tailnet source", localPairReq("100.72.207.107:44000", "127.0.0.1:9876", ext)},
		{"rebinding host", localPairReq("127.0.0.1:54321", "evil.example.com:9876", ext)},
		{"web origin", localPairReq("127.0.0.1:54321", "127.0.0.1:9876", "https://evil.example.com")},
		{"missing origin", localPairReq("127.0.0.1:54321", "127.0.0.1:9876", "")},
		{"https localhost-lookalike origin", localPairReq("127.0.0.1:54321", "127.0.0.1:9876", "https://localhost.evil.example.com")},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h(rec, c.req)
		if rec.Code != nethttp.StatusForbidden {
			t.Errorf("%s: status %d, want 403 (body %s)", c.name, rec.Code, rec.Body.String())
		}
	}

	// Proxied request (tailscale serve arrives loopback-sourced but
	// stamps forwarding headers) — must be refused.
	proxied := localPairReq("127.0.0.1:54321", "127.0.0.1:9876", ext)
	proxied.Header.Set("X-Forwarded-For", "100.72.207.107")
	rec := httptest.NewRecorder()
	h(rec, proxied)
	if rec.Code != nethttp.StatusForbidden {
		t.Errorf("proxied: status %d, want 403", rec.Code)
	}

	// Wrong method.
	get := httptest.NewRequest(nethttp.MethodGet, "/auth/local-pair", nil)
	get.RemoteAddr = "127.0.0.1:54321"
	rec = httptest.NewRecorder()
	h(rec, get)
	if rec.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("GET: status %d, want 405", rec.Code)
	}
}

func TestLocalPairTokenActuallyWorks(t *testing.T) {
	dir := t.TempDir()
	store, err := NewDeviceStore(dir)
	if err != nil {
		t.Fatalf("device store: %v", err)
	}
	h := localPairHandler(dir, store)
	rec := httptest.NewRecorder()
	h(rec, localPairReq("127.0.0.1:54321", "localhost:9876", "chrome-extension://abc"))
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("pair failed: %d %s", rec.Code, rec.Body.String())
	}
	var res enrolResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	dev := store.LookupDevice(res.DeviceToken)
	if dev == nil || dev.ID != res.DeviceID {
		t.Fatalf("minted token doesn't resolve to the enrolled device: %+v", dev)
	}
}
