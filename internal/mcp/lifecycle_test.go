package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeServerEnv switches the test binary into a minimal stdio MCP
// server (see TestMain), so lifecycle tests can drive real
// subprocesses without Node.
const fakeServerEnv = "BRUV_FAKE_MCP_SERVER"

// fakeToolEnv names the one tool the fake server advertises.
const fakeToolEnv = "BRUV_FAKE_MCP_TOOL"

// fakeCrashOnceEnv names a marker file: the first launch (marker
// absent) creates it and exits right after tools/list; later launches
// advertise "<tool>_v2" and stay up.
const fakeCrashOnceEnv = "BRUV_FAKE_MCP_CRASH_ONCE"

func TestMain(m *testing.M) {
	if os.Getenv(fakeServerEnv) == "1" {
		runFakeServer()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runFakeServer answers initialize + tools/list and exits when stdin
// closes (MCP's shutdown signal).
func runFakeServer() {
	tool := os.Getenv(fakeToolEnv)
	if tool == "" {
		tool = "t1"
	}
	crashAfterList := false
	if marker := os.Getenv(fakeCrashOnceEnv); marker != "" {
		if _, err := os.Stat(marker); err == nil {
			tool += "_v2"
		} else {
			_ = os.WriteFile(marker, nil, 0o644)
			crashAfterList = true
		}
	}
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var req struct {
			ID     *json.RawMessage `json:"id"`
			Method string           `json:"method"`
		}
		if json.Unmarshal(sc.Bytes(), &req) != nil || req.ID == nil {
			continue
		}
		var result string
		switch req.Method {
		case "initialize":
			result = fmt.Sprintf(`{"protocolVersion":%q,"capabilities":{},"serverInfo":{"name":"fake","version":"1"}}`, ProtocolVersion)
		case "tools/list":
			result = fmt.Sprintf(`{"tools":[{"name":%q,"inputSchema":{"type":"object"}}]}`, tool)
		default:
			result = `{}`
		}
		fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":%s}`+"\n", string(*req.ID), result)
		if crashAfterList && req.Method == "tools/list" {
			time.Sleep(200 * time.Millisecond)
			os.Exit(1)
		}
	}
}

// fakeSpec returns a spec that launches the fake server. EnvNames +
// the resolver pass the switch through buildEnv's allowlist.
func fakeSpec(t *testing.T, name string) ServerSpec {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return ServerSpec{
		Name:        name,
		Command:     exe,
		EnvNames:    []string{fakeServerEnv, fakeToolEnv, fakeCrashOnceEnv},
		Enabled:     true,
		InitTimeout: 10 * time.Second,
	}
}

type fakeResolver struct{ tool, crashMarker string }

func (f fakeResolver) Lookup(_, _, name string) (string, bool) {
	switch name {
	case fakeServerEnv:
		return "1", true
	case fakeToolEnv:
		return f.tool, f.tool != ""
	case fakeCrashOnceEnv:
		return f.crashMarker, f.crashMarker != ""
	}
	return "", false
}

// TestServerStopAfterSuperviseWaits: Stop used to call cmd.Wait while
// supervise was already blocked in cmd.Wait on the same process. Stop
// must return promptly (clean exit via stdin close) and leave the
// server disabled, not restarting.
func TestServerStopAfterSuperviseWaits(t *testing.T) {
	sp := NewServerProcess(fakeSpec(t, "fake"), "repo", fakeResolver{})
	if err := sp.Start(testContext(t)); err != nil {
		t.Fatalf("start: %v", err)
	}
	if h := sp.Health(); h.Status != HealthReady || h.ToolCount != 1 {
		t.Fatalf("health = %+v, want ready with 1 tool", h)
	}
	start := time.Now()
	if err := sp.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("Stop took %v, want a clean exit", d)
	}
	deadline := time.Now().Add(2 * time.Second)
	for sp.Health().Status != HealthDisabled {
		if time.Now().After(deadline) {
			t.Fatalf("status = %s after Stop, want disabled", sp.Health().Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestRegistryReloadSwapsWithoutGap: a reload must keep the old set
// answering until the new one is ready, then replace it — and a
// Shutdown landing mid-reload must not leave the new set running.
func TestRegistryReloadSwapsWithoutGap(t *testing.T) {
	r := NewRegistry("repo", fakeResolver{tool: "alpha"})
	if errs := r.LoadAndStart(testContext(t), []ServerSpec{fakeSpec(t, "fake")}); len(errs) != 0 {
		t.Fatalf("load: %v", errs)
	}
	if !r.OwnsTool("fake__alpha") {
		t.Fatal("tool not indexed after load")
	}

	r.resolver = fakeResolver{tool: "beta"}
	if errs := r.LoadAndStart(testContext(t), []ServerSpec{fakeSpec(t, "fake")}); len(errs) != 0 {
		t.Fatalf("reload: %v", errs)
	}
	if r.OwnsTool("fake__alpha") || !r.OwnsTool("fake__beta") {
		t.Fatal("tool index not rebuilt from the new set")
	}

	r.Shutdown()
	// A reload after Shutdown stops what it starts.
	r.LoadAndStart(testContext(t), []ServerSpec{fakeSpec(t, "fake")})
	if n := len(r.Health()); n != 0 {
		t.Fatalf("registry has %d servers after Shutdown, want 0", n)
	}
}

// TestRegistryReindexesAfterSupervisedRestart: a server that crashes
// and is restarted by supervise may come back with a different tool
// list; the registry's tool index must follow.
func TestRegistryReindexesAfterSupervisedRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the 2 s restart delay")
	}
	marker := filepath.Join(t.TempDir(), "crashed")
	r := NewRegistry("repo", fakeResolver{tool: "alpha", crashMarker: marker})
	defer r.Shutdown()
	if errs := r.LoadAndStart(testContext(t), []ServerSpec{fakeSpec(t, "fake")}); len(errs) != 0 {
		t.Fatalf("load: %v", errs)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !r.OwnsTool("fake__alpha_v2") {
		if time.Now().After(deadline) {
			t.Fatalf("restarted server's tools never indexed; health=%+v", r.Health())
		}
		time.Sleep(50 * time.Millisecond)
	}
	if r.OwnsTool("fake__alpha") {
		t.Fatal("stale pre-restart tool still indexed")
	}
}
