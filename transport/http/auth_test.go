package http

import (
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// enrolHelper issues a fresh device token against a temp device
// store. Separates test setup from assertion logic.
func enrolHelper(t *testing.T, dir string) (*DeviceStore, string) {
	t.Helper()
	store, err := NewDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewDeviceStore: %v", err)
	}
	bootstrap, err := os.ReadFile(filepath.Join(dir, "bootstrap-token.txt"))
	if err != nil {
		t.Fatalf("read bootstrap token: %v", err)
	}
	token, _, err := store.Enrol(strings.TrimSpace(string(bootstrap)), "test-device")
	if err != nil {
		t.Fatalf("Enrol: %v", err)
	}
	return store, token
}

func TestDeviceStoreGeneratesBootstrapOnFirstBoot(t *testing.T) {
	dir := t.TempDir()
	_, err := NewDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewDeviceStore: %v", err)
	}
	// bootstrap-token.txt must exist and be non-empty.
	data, err := os.ReadFile(filepath.Join(dir, "bootstrap-token.txt"))
	if err != nil {
		t.Fatalf("read bootstrap: %v", err)
	}
	if len(strings.TrimSpace(string(data))) != 64 {
		t.Errorf("bootstrap token length = %d, want 64 hex chars", len(data))
	}
}

func TestDeviceStoreReusesExisting(t *testing.T) {
	dir := t.TempDir()
	first, err := NewDeviceStore(dir)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := NewDeviceStore(dir)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	// Same bootstrap token survives across loads.
	if len(first.devices) != len(second.devices) {
		t.Errorf("device count changed on reload: %d → %d", len(first.devices), len(second.devices))
	}
}

func TestLegacyTokenMigration(t *testing.T) {
	dir := t.TempDir()
	// Simulate a pre-phase-5 install with auth-token.txt present.
	legacy := "legacy" + strings.Repeat("0", 58)
	if err := os.WriteFile(filepath.Join(dir, "auth-token.txt"), []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	store, err := NewDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewDeviceStore: %v", err)
	}
	// The legacy token must authenticate as a device (not just bootstrap).
	dev := store.LookupDevice(legacy)
	if dev == nil {
		t.Fatal("legacy token did not migrate into devices")
	}
	if dev.Scope != "device" {
		t.Errorf("legacy scope = %q, want device", dev.Scope)
	}
}

func TestRequireAuthRejectsMissing(t *testing.T) {
	store, _ := enrolHelper(t, t.TempDir())
	h := requireAuth(store, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		t.Fatal("handler should not be called without auth")
	}))
	req := httptest.NewRequest(nethttp.MethodPost, "/rpc", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != nethttp.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestRequireAuthAcceptsValidDeviceToken(t *testing.T) {
	store, token := enrolHelper(t, t.TempDir())
	called := false
	h := requireAuth(store, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		called = true
	}))
	req := httptest.NewRequest(nethttp.MethodPost, "/rpc", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatalf("handler not called; body=%q", rec.Body.String())
	}
}

func TestRequireAuthAcceptsQueryParam(t *testing.T) {
	store, token := enrolHelper(t, t.TempDir())
	called := false
	h := requireAuth(store, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		called = true
	}))
	req := httptest.NewRequest(nethttp.MethodGet, "/events?token="+token, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called {
		t.Errorf("handler not called via query param; body=%q", rec.Body.String())
	}
}

func TestRequireAuthRejectsBootstrapScope(t *testing.T) {
	// Bootstrap tokens must not authenticate /rpc — they're enrol-only.
	dir := t.TempDir()
	store, err := NewDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewDeviceStore: %v", err)
	}
	bootstrapBytes, _ := os.ReadFile(filepath.Join(dir, "bootstrap-token.txt"))
	bootstrap := strings.TrimSpace(string(bootstrapBytes))

	h := requireAuth(store, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		t.Fatal("handler should not be called with bootstrap token")
	}))
	req := httptest.NewRequest(nethttp.MethodPost, "/rpc", nil)
	req.Header.Set("Authorization", "Bearer "+bootstrap)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != nethttp.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestEnrolIssuesUniqueTokens(t *testing.T) {
	dir := t.TempDir()
	store, err := NewDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewDeviceStore: %v", err)
	}
	bootstrapBytes, _ := os.ReadFile(filepath.Join(dir, "bootstrap-token.txt"))
	bootstrap := strings.TrimSpace(string(bootstrapBytes))

	t1, _, err := store.Enrol(bootstrap, "laptop")
	if err != nil {
		t.Fatalf("first enrol: %v", err)
	}
	t2, _, err := store.Enrol(bootstrap, "phone")
	if err != nil {
		t.Fatalf("second enrol: %v", err)
	}
	if t1 == t2 {
		t.Error("consecutive enrolments returned the same token")
	}
	if store.LookupDevice(t1) == nil || store.LookupDevice(t2) == nil {
		t.Error("issued tokens did not resolve back to devices")
	}
}

func TestEnrolRejectsBadBootstrap(t *testing.T) {
	dir := t.TempDir()
	store, err := NewDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewDeviceStore: %v", err)
	}
	if _, _, err := store.Enrol("wrongtoken"+strings.Repeat("0", 53), "phone"); err == nil {
		t.Fatal("expected rejection on bad bootstrap")
	}
}
