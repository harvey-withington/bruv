package http

import (
	"encoding/json"
	nethttp "net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bruv/core/events"
)

// buildTestServer spins up a real transport server + issues a fresh
// device token so tests can authenticate without ceremony.
func buildTestServer(t *testing.T) (*Server, string, string) {
	t.Helper()
	cfgDir := t.TempDir()
	bus := events.NewMemBus(64)
	srv, err := New(Config{
		Addr:      "127.0.0.1:0",
		ConfigDir: cfgDir,
		Version:   "test",
		BuildDate: "test",
	}, &mockApp{}, bus)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	// Issue a device token so tests can call /rpc + /events.
	bootstrap, err := os.ReadFile(filepath.Join(cfgDir, "bootstrap-token.txt"))
	if err != nil {
		t.Fatalf("read bootstrap: %v", err)
	}
	token, _, err := srv.Devices().Enrol(strings.TrimSpace(string(bootstrap)), "test")
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}

	return srv, "http://" + srv.Addr(), token
}

func TestHealthzUnauthenticated(t *testing.T) {
	_, base, _ := buildTestServer(t)
	resp, err := nethttp.Get(base + "/healthz")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestVersionUnauthenticated(t *testing.T) {
	_, base, _ := buildTestServer(t)
	resp, _ := nethttp.Get(base + "/version")
	defer resp.Body.Close()
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["version"] != "test" {
		t.Errorf("version = %q, want 'test'", body["version"])
	}
}

func TestRPCRejectsWithoutAuth(t *testing.T) {
	_, base, _ := buildTestServer(t)
	resp, _ := nethttp.Post(base+"/rpc", "application/json", strings.NewReader(`{}`))
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestRPCHappyPath(t *testing.T) {
	_, base, token := buildTestServer(t)

	body := strings.NewReader(`{"jsonrpc":"2.0","method":"Add","params":[3,4],"id":1}`)
	req, _ := nethttp.NewRequest(nethttp.MethodPost, base+"/rpc", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	var out rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error != nil {
		t.Fatalf("rpc error: %+v", out.Error)
	}
	if out.Result.(float64) != 7 {
		t.Errorf("result = %v, want 7", out.Result)
	}
}

func TestSSEStreamsBusEvents(t *testing.T) {
	// Use our own bus so we can publish into it while reading.
	cfgDir := t.TempDir()
	bus := events.NewMemBus(16)
	srv, err := New(Config{
		Addr:      "127.0.0.1:0",
		ConfigDir: cfgDir,
		Version:   "test",
	}, &mockApp{}, bus)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	bootstrap, _ := os.ReadFile(filepath.Join(cfgDir, "bootstrap-token.txt"))
	token, _, err := srv.Devices().Enrol(strings.TrimSpace(string(bootstrap)), "sse-test")
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}

	base := "http://" + srv.Addr()
	req, _ := nethttp.NewRequest(nethttp.MethodGet, base+"/events?token="+token, nil)
	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get /events: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type = %q, want text/event-stream", ct)
	}

	// Read the stream in a goroutine so the test can time-bound the
	// wait with a channel select. http.Response.Body doesn't expose
	// read deadlines, so the goroutine just reads until closed and
	// the test closes the body on timeout.
	gotEvent := make(chan string, 1)
	readErr := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		var collected string
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				collected += string(buf[:n])
				if strings.Contains(collected, "event: card:updated") &&
					strings.Contains(collected, `"cardID":"abc"`) {
					gotEvent <- collected
					return
				}
			}
			if err != nil {
				readErr <- err
				return
			}
		}
	}()

	// Publish after briefly letting the subscription establish.
	time.Sleep(50 * time.Millisecond)
	bus.Publish("card:updated", map[string]string{"cardID": "abc"})

	select {
	case <-gotEvent:
		// success
	case err := <-readErr:
		t.Fatalf("read loop ended before event: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for SSE event")
	}
}
