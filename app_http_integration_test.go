package main

// Integration test: stands up the real multi-repo HTTP transport
// against a MachineService and proves the dispatcher serves a
// per-machine RPC end-to-end. Transport unit tests use a mock target;
// this test guards the wiring (Server + MachineTarget + auth) the
// desktop App relies on at boot.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bruv/core/supervisor"
	transporthttp "bruv/transport/http"
)

func TestHTTPTransportAgainstMachineService(t *testing.T) {
	cfgDir := t.TempDir()
	sup, err := supervisor.New(nil, cfgDir)
	if err != nil {
		t.Fatalf("supervisor.New: %v", err)
	}
	srv, err := transporthttp.NewMulti(transporthttp.Config{
		Addr:          "127.0.0.1:0",
		ConfigDir:     cfgDir,
		Version:       "integration-test",
		BuildDate:     "integration-test",
		Repos:         supervisor.NewHTTPAdapter(sup),
		MachineTarget: supervisor.NewMachineService(),
	})
	if err != nil {
		t.Fatalf("NewMulti: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	// Enrol a device so tests have a valid bearer for /server/rpc.
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

	// /server/rpc IsLLMConfigured — authed, zero-arg, returns bool.
	// Proves the dispatcher actually reaches MachineService methods.
	body := strings.NewReader(`{"jsonrpc":"2.0","method":"IsLLMConfigured","params":[],"id":1}`)
	req, _ := http.NewRequest(http.MethodPost, base+"/server/rpc", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("rpc IsLLMConfigured: %v", err)
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
	// Fresh tempdir → no LLM config → false. The exact value is less
	// important than: dispatcher ran the method, returned a typed bool.
	if _, ok := rpcOut.Result.(bool); !ok {
		t.Errorf("IsLLMConfigured result = %v (%T), want bool", rpcOut.Result, rpcOut.Result)
	}

	// /repos — authed GET against the multi-repo collection. Empty
	// supervisor → empty list. Proves the registry endpoint wires up.
	req, _ = http.NewRequest(http.MethodGet, base+"/repos", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get /repos: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/repos status = %d", resp.StatusCode)
	}
	var repos []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&repos)
	if len(repos) != 0 {
		t.Errorf("/repos with empty supervisor = %v, want empty", repos)
	}
}
