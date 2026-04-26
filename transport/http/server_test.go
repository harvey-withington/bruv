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

// stubBackend is a minimal RepoBackend that resolves a single repo
// ID to a fixed RepoTarget. Lifecycle ops (Inspect/InitOrOpen/Rename/
// Remove) return errors — tests don't exercise them.
type stubBackend struct {
	target *RepoTarget
}

func (b *stubBackend) Resolve(id string) *RepoTarget {
	if b.target != nil && id == "stub" {
		return b.target
	}
	return nil
}
func (b *stubBackend) List() []RepoSummary {
	return []RepoSummary{{ID: "stub", Name: "stub repo"}}
}
func (b *stubBackend) SetEnabled(string, bool) error { return nil }
func (b *stubBackend) Inspect(string) (RepoInspect, error) {
	return RepoInspect{}, nil
}
func (b *stubBackend) InitOrOpen(string, string) (RepoSummary, error) {
	return RepoSummary{}, nil
}
func (b *stubBackend) Rename(string, string) error { return nil }
func (b *stubBackend) Remove(string) error         { return nil }

// buildTestServer spins up a real multi-repo transport with a stub
// backend + a MachineService-style mockApp behind /server/rpc, and
// issues a fresh device token so tests can authenticate without
// ceremony.
func buildTestServer(t *testing.T) (*Server, string, string, *events.MemBus) {
	t.Helper()
	cfgDir := t.TempDir()
	bus := events.NewMemBus(64)
	backend := &stubBackend{
		target: &RepoTarget{
			Target: &mockApp{},
			Bus:    bus,
		},
	}
	srv, err := NewMulti(Config{
		Addr:          "127.0.0.1:0",
		ConfigDir:     cfgDir,
		Version:       "test",
		BuildDate:     "test",
		Repos:         backend,
		MachineTarget: &mockApp{},
	})
	if err != nil {
		t.Fatalf("NewMulti: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	bootstrap, err := os.ReadFile(filepath.Join(cfgDir, "bootstrap-token.txt"))
	if err != nil {
		t.Fatalf("read bootstrap: %v", err)
	}
	token, _, err := srv.Devices().Enrol(strings.TrimSpace(string(bootstrap)), "test")
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}

	return srv, "http://" + srv.Addr(), token, bus
}

func TestHealthzUnauthenticated(t *testing.T) {
	_, base, _, _ := buildTestServer(t)
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
	_, base, _, _ := buildTestServer(t)
	resp, _ := nethttp.Get(base + "/version")
	defer resp.Body.Close()
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["version"] != "test" {
		t.Errorf("version = %q, want 'test'", body["version"])
	}
}

func TestRPCRejectsWithoutAuth(t *testing.T) {
	_, base, _, _ := buildTestServer(t)
	resp, _ := nethttp.Post(base+"/server/rpc", "application/json", strings.NewReader(`{}`))
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestRPCHappyPathOnRepo(t *testing.T) {
	_, base, token, _ := buildTestServer(t)

	body := strings.NewReader(`{"jsonrpc":"2.0","method":"Add","params":[3,4],"id":1}`)
	req, _ := nethttp.NewRequest(nethttp.MethodPost, base+"/repos/stub/rpc", body)
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

func TestRPCHappyPathOnMachine(t *testing.T) {
	_, base, token, _ := buildTestServer(t)

	body := strings.NewReader(`{"jsonrpc":"2.0","method":"Add","params":[10,20],"id":1}`)
	req, _ := nethttp.NewRequest(nethttp.MethodPost, base+"/server/rpc", body)
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
	if out.Result.(float64) != 30 {
		t.Errorf("result = %v, want 30", out.Result)
	}
}

func TestSSEStreamsBusEvents(t *testing.T) {
	_, base, token, bus := buildTestServer(t)

	req, _ := nethttp.NewRequest(nethttp.MethodGet, base+"/repos/stub/events?token="+token, nil)
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
