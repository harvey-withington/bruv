package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFingerprintStable(t *testing.T) {
	a := ServerSpec{Name: "fs", Command: "npx", Args: []string{"-y", "pkg"}, EnvNames: []string{"B", "A"}, Enabled: true}
	b := a
	b.Description = "changed label"
	b.Enabled = false
	b.EnvNames = []string{"A", "B"}
	if a.Fingerprint() != b.Fingerprint() {
		t.Error("description, enabled and env-name order must not change the fingerprint")
	}
	if (ServerSpec{Command: "x"}).Fingerprint() != (ServerSpec{Command: "x", Args: []string{}, EnvNames: []string{}}).Fingerprint() {
		t.Error("nil and empty lists must fingerprint the same")
	}
}

func TestFingerprintDetectsChanges(t *testing.T) {
	base := ServerSpec{Name: "fs", Command: "npx", Args: []string{"-y", "pkg"}, EnvNames: []string{"TOKEN"}}
	changes := map[string]ServerSpec{
		"command":   {Name: "fs", Command: "powershell", Args: base.Args, EnvNames: base.EnvNames},
		"args":      {Name: "fs", Command: "npx", Args: []string{"-y", "evil-pkg"}, EnvNames: base.EnvNames},
		"arg order": {Name: "fs", Command: "npx", Args: []string{"pkg", "-y"}, EnvNames: base.EnvNames},
		"env names": {Name: "fs", Command: "npx", Args: base.Args, EnvNames: []string{"TOKEN", "NODE_OPTIONS"}},
		// Joining args differently must not collide.
		"arg split": {Name: "fs", Command: "npx", Args: []string{"-y pkg"}, EnvNames: base.EnvNames},
	}
	for what, spec := range changes {
		if spec.Fingerprint() == base.Fingerprint() {
			t.Errorf("changing %s must change the fingerprint", what)
		}
	}
}

// TestRegistryUnapprovedServerNotStarted: an enabled server the machine
// hasn't approved is tracked as unapproved, never spawned, and offers no
// tools. The spec launches the fake server, so if the gate failed the
// process would really start and report ready.
func TestRegistryUnapprovedServerNotStarted(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "spawned")
	spec := fakeSpec(t, "shared")
	r := NewRegistry("repo", fakeResolver{tool: "alpha", crashMarker: marker})
	defer r.Shutdown()

	for name, approved := range map[string]ApprovalFunc{
		"nil approval":  nil,
		"rejected":      func(ServerSpec) bool { return false },
		"other servers": func(s ServerSpec) bool { return s.Name != "shared" },
	} {
		if errs := r.LoadAndStart(testContext(t), []ServerSpec{spec}, approved); len(errs) != 0 {
			t.Fatalf("%s: unexpected errors %v", name, errs)
		}
		h := r.Health()
		if len(h) != 1 || h[0].Status != HealthUnapproved {
			t.Fatalf("%s: health = %+v, want one %q entry", name, h, HealthUnapproved)
		}
		if len(r.Tools()) != 0 || r.OwnsTool(NamespaceTool("shared", "alpha")) {
			t.Errorf("%s: unapproved server must offer no tools", name)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("%s: unapproved server was spawned", name)
		}
	}

	// Approving it starts it.
	if errs := r.LoadAndStart(testContext(t), []ServerSpec{spec}, approveAll); len(errs) != 0 {
		t.Fatalf("approved: unexpected errors %v", errs)
	}
	if !r.OwnsTool(NamespaceTool("shared", "alpha")) && !r.OwnsTool(NamespaceTool("shared", "alpha_v2")) {
		t.Error("approved server should be started and offer its tool")
	}
}

func TestRegistryDisabledServerStaysDisabledWhenUnapproved(t *testing.T) {
	r := NewRegistry("repo", nil)
	defer r.Shutdown()
	r.LoadAndStart(testContext(t), []ServerSpec{{Name: "off", Command: "x", Enabled: false}}, nil)
	if h := r.Health(); len(h) != 1 || h[0].Status != HealthDisabled {
		t.Errorf("disabled server health = %+v, want disabled", h)
	}
}
