package main

// Integration test: stands up a real *App behind the HTTP transport
// on a random loopback port and drives it with a real HTTP client.
// This is the "does the transport actually see the App's methods" test
// the transport package's own unit tests can't prove — they use a mock.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	transporthttp "bruv/transport/http"

	"bruv/core/events"
)

func TestHTTPTransportAgainstRealApp(t *testing.T) {
	app := NewApp()

	cfgDir := t.TempDir()
	srv, err := transporthttp.New(transporthttp.Config{
		Addr:      "127.0.0.1:0",
		ConfigDir: cfgDir,
		Version:   "integration-test",
		BuildDate: "integration-test",
	}, app, events.NewMemBus(16))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	// Enrol a device so tests have a valid bearer for /rpc.
	bootstrap, err := os.ReadFile(filepath.Join(cfgDir, "bootstrap-token.txt"))
	if err != nil {
		t.Fatalf("read bootstrap: %v", err)
	}
	token, _, err := srv.Devices().Enrol(strings.TrimSpace(string(bootstrap)), "integration-test")
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}

	base := "http://" + srv.Addr()

	// /healthz — unauthed.
	resp, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz status = %d", resp.StatusCode)
	}

	// /rpc Version — authed, zero-arg method with a single string return.
	body := strings.NewReader(`{"jsonrpc":"2.0","method":"Version","params":[],"id":1}`)
	req, _ := http.NewRequest(http.MethodPost, base+"/rpc", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("rpc Version: %v", err)
	}
	defer resp.Body.Close()

	var rpcOut struct {
		Result any            `json:"result"`
		Error  map[string]any `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&rpcOut)
	if rpcOut.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcOut.Error)
	}
	// AppVersion is a package-level var; whatever it's set to, the
	// result should be a non-empty string.
	s, ok := rpcOut.Result.(string)
	if !ok || s == "" {
		t.Errorf("Version result = %v (%T), want non-empty string", rpcOut.Result, rpcOut.Result)
	}

	// /rpc HasRepository — authed, zero-arg, returns bool. No repo is
	// open, so result must be false — proves the method actually ran
	// on our real App and not a shadow.
	body = strings.NewReader(`{"jsonrpc":"2.0","method":"HasRepository","params":[],"id":2}`)
	req, _ = http.NewRequest(http.MethodPost, base+"/rpc", body)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("rpc HasRepository: %v", err)
	}
	defer resp.Body.Close()
	_ = json.NewDecoder(resp.Body).Decode(&rpcOut)
	if rpcOut.Error != nil {
		t.Fatalf("rpc error: %+v", rpcOut.Error)
	}
	if got, _ := rpcOut.Result.(bool); got != false {
		t.Errorf("HasRepository = %v, want false (no repo open)", rpcOut.Result)
	}

	// /rpc PickFolder — denied by policy. This proves the denylist
	// actually blocks the real method even though it exists on App.
	body = strings.NewReader(`{"jsonrpc":"2.0","method":"PickFolder","params":[],"id":3}`)
	req, _ = http.NewRequest(http.MethodPost, base+"/rpc", body)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("rpc PickFolder: %v", err)
	}
	defer resp.Body.Close()
	_ = json.NewDecoder(resp.Body).Decode(&rpcOut)
	if rpcOut.Error == nil {
		t.Fatal("expected denial error on PickFolder, got success")
	}
}
