package http

import (
	"io"
	nethttp "net/http"
	"strings"
	"testing"
	"testing/fstest"

	"bruv/core/events"
)

// buildFakeBundle constructs an in-memory fs.FS that mimics the
// `frontend/dist` layout produced by vite build, so the static
// handler can be tested without running the real frontend build.
func buildFakeBundle() fstest.MapFS {
	return fstest.MapFS{
		"frontend/dist/index.html": &fstest.MapFile{
			Data: []byte(`<html><body>BRUV shell</body></html>`),
		},
		"frontend/dist/assets/index-hash.js": &fstest.MapFile{
			Data: []byte(`console.log("bundle")`),
		},
	}
}

// buildServerWithAssets boots a real server with the fake bundle so
// HTTP routing + static handler + caching are exercised together.
func buildServerWithAssets(t *testing.T) string {
	t.Helper()
	bus := events.NewMemBus(16)
	srv, err := New(Config{
		Addr:         "127.0.0.1:0",
		ConfigDir:    t.TempDir(),
		Version:      "test",
		StaticAssets: buildFakeBundle(),
	}, &mockApp{}, bus)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return "http://" + srv.Addr()
}

func TestAppServesIndex(t *testing.T) {
	base := buildServerWithAssets(t)
	resp, err := nethttp.Get(base + "/app/")
	if err != nil {
		t.Fatalf("get /app/: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != nethttp.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "BRUV shell") {
		t.Errorf("body missing shell marker; got %q", string(body))
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("content-type = %q, want text/html", ct)
	}
}

func TestAppSPAFallback(t *testing.T) {
	base := buildServerWithAssets(t)
	// Request an unknown route — should fall back to index.html so
	// the Svelte router can handle it client-side.
	resp, err := nethttp.Get(base + "/app/some/nested/route")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != nethttp.StatusOK {
		t.Errorf("status = %d, want 200 (SPA fallback)", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "BRUV shell") {
		t.Errorf("SPA fallback didn't serve index.html; got %q", string(body))
	}
}

func TestAppAssetCaching(t *testing.T) {
	base := buildServerWithAssets(t)
	resp, err := nethttp.Get(base + "/app/assets/index-hash.js")
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != nethttp.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	cc := resp.Header.Get("Cache-Control")
	if !strings.Contains(cc, "immutable") {
		t.Errorf("hashed asset missing immutable caching; got Cache-Control=%q", cc)
	}
}

func TestAppUnauthenticated(t *testing.T) {
	// The bundle itself doesn't require auth — that's the UI shell,
	// not data. /rpc + /events still do (covered by other tests).
	base := buildServerWithAssets(t)
	resp, err := nethttp.Get(base + "/app/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == nethttp.StatusUnauthorized {
		t.Error("/app/ should be reachable without auth (it's the login UI + shell)")
	}
}

func TestAppBareRedirects(t *testing.T) {
	base := buildServerWithAssets(t)
	client := &nethttp.Client{
		CheckRedirect: func(*nethttp.Request, []*nethttp.Request) error { return nethttp.ErrUseLastResponse },
	}
	resp, err := client.Get(base + "/app")
	if err != nil {
		t.Fatalf("get /app: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusMovedPermanently {
		t.Errorf("status = %d, want 301", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/app/" {
		t.Errorf("redirect location = %q, want /app/", loc)
	}
}
