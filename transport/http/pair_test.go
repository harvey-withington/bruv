package http

import (
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pairTestSetup creates a temp config dir with a bootstrap token file
// and returns the dir + the token. Mirrors the auth_test setup.
func pairTestSetup(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	if _, err := NewDeviceStore(dir); err != nil {
		t.Fatalf("NewDeviceStore: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "bootstrap-token.txt"))
	if err != nil {
		t.Fatalf("read bootstrap-token.txt: %v", err)
	}
	return dir, strings.TrimSpace(string(data))
}

func TestPairHandlerRejectsMissingToken(t *testing.T) {
	dir, _ := pairTestSetup(t)
	handler := pairHandler(dir)

	req := httptest.NewRequest(nethttp.MethodGet, "/pair", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != nethttp.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestPairHandlerRejectsWrongToken(t *testing.T) {
	dir, _ := pairTestSetup(t)
	handler := pairHandler(dir)

	req := httptest.NewRequest(nethttp.MethodGet, "/pair?token=not-the-right-token", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != nethttp.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "does not match") {
		t.Fatalf("expected mismatch message, got body: %s", w.Body.String())
	}
}

func TestPairHandlerRejectsNonGet(t *testing.T) {
	dir, token := pairTestSetup(t)
	handler := pairHandler(dir)

	req := httptest.NewRequest(nethttp.MethodPost, "/pair?token="+token, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != nethttp.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestPairHandlerServiceUnavailableWhenNoBootstrap(t *testing.T) {
	dir := t.TempDir() // no NewDeviceStore call → no bootstrap-token.txt
	handler := pairHandler(dir)

	req := httptest.NewRequest(nethttp.MethodGet, "/pair?token=anything", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != nethttp.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestPairHandlerRendersQRForCorrectToken(t *testing.T) {
	dir, token := pairTestSetup(t)
	handler := pairHandler(dir)

	req := httptest.NewRequest(nethttp.MethodGet, "/pair?token="+token, nil)
	req.Host = "deviant.tail2ebd58.ts.net"
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	body := w.Body.String()

	// QR is embedded as a base64 PNG data URL.
	if !strings.Contains(body, "data:image/png;base64,") {
		t.Errorf("expected embedded PNG QR, body did not contain data URL")
	}

	// The page must show the EnrolURL with the right scheme + host
	// derived from X-Forwarded-Proto + r.Host.
	wantURL := "https://deviant.tail2ebd58.ts.net/m/enrol?token=" + token
	if !strings.Contains(body, wantURL) {
		t.Errorf("expected enrol URL %q in body, got: %s", wantURL, body)
	}

	// Ensure cache + referrer headers are set so the token doesn't
	// leak via either mechanism.
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("expected Cache-Control: no-store, got %q", got)
	}
	if got := w.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("expected Referrer-Policy: no-referrer, got %q", got)
	}
}

func TestPairHandlerHonoursXForwardedHost(t *testing.T) {
	dir, token := pairTestSetup(t)
	handler := pairHandler(dir)

	req := httptest.NewRequest(nethttp.MethodGet, "/pair?token="+token, nil)
	req.Host = "127.0.0.1:9870"
	req.Header.Set("X-Forwarded-Host", "machine.ts.net")
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	wantURL := "https://machine.ts.net/m/enrol?token=" + token
	if !strings.Contains(w.Body.String(), wantURL) {
		t.Errorf("expected forwarded host in URL %q, body: %s", wantURL, w.Body.String())
	}
}

func TestPairHandlerFlagsLoopbackHost(t *testing.T) {
	dir, token := pairTestSetup(t)
	handler := pairHandler(dir)

	// Hit /pair via loopback. The page should still render, but with
	// the warning class on the host form so the operator notices the
	// QR is unscannable from a phone.
	req := httptest.NewRequest(nethttp.MethodGet, "/pair?token="+token, nil)
	req.Host = "127.0.0.1:9870"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "host-form warn") {
		t.Errorf("expected warn class on host-form for loopback, got: %s", body)
	}
	if !strings.Contains(body, "your phone can't reach") {
		t.Errorf("expected loopback explanation in hint")
	}
}

func TestPairHandlerNoWarnForTailnetHost(t *testing.T) {
	dir, token := pairTestSetup(t)
	handler := pairHandler(dir)

	req := httptest.NewRequest(nethttp.MethodGet, "/pair?token="+token, nil)
	req.Host = "deviant.tail2ebd58.ts.net"
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "host-form warn") {
		t.Errorf("expected no warn class for non-loopback host")
	}
}

func TestIsLoopbackHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.1:9870", true},
		{"localhost", true},
		{"localhost:8080", true},
		{"::1", true},
		{"[::1]:9870", true},
		{"deviant.tail2ebd58.ts.net", false},
		{"machine.ts.net:443", false},
		{"192.168.1.10:80", false},
		{"100.66.105.59", false},
	}
	for _, c := range cases {
		if got := isLoopbackHost(c.host); got != c.want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestPairHandlerHonoursQueryHostAndSchemeOverrides(t *testing.T) {
	dir, token := pairTestSetup(t)
	handler := pairHandler(dir)

	// Operator hits /pair on local loopback but wants the QR to encode
	// the phone-reachable tailnet URL. The query overrides win over
	// both r.Host and X-Forwarded-Host.
	req := httptest.NewRequest(nethttp.MethodGet,
		"/pair?token="+token+"&host=deviant.ts.net&scheme=https", nil)
	req.Host = "127.0.0.1:9870"
	req.Header.Set("X-Forwarded-Host", "wrong.example")
	req.Header.Set("X-Forwarded-Proto", "http")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	wantURL := "https://deviant.ts.net/m/enrol?token=" + token
	if !strings.Contains(w.Body.String(), wantURL) {
		t.Errorf("expected query overrides to win, want %q in body", wantURL)
	}
}

func TestPairHandlerDefaultsToHTTPWithoutForwardedProto(t *testing.T) {
	dir, token := pairTestSetup(t)
	handler := pairHandler(dir)

	req := httptest.NewRequest(nethttp.MethodGet, "/pair?token="+token, nil)
	req.Host = "127.0.0.1:9870"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	wantURL := "http://127.0.0.1:9870/m/enrol?token=" + token
	if !strings.Contains(w.Body.String(), wantURL) {
		t.Errorf("expected http scheme in URL %q, body: %s", wantURL, w.Body.String())
	}
}
