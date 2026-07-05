package push

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVAPID_LoadOrCreate_Generates(t *testing.T) {
	dir := t.TempDir()

	// First call generates and persists.
	v1, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatalf("first LoadOrCreate: %v", err)
	}
	if v1.Public() == "" || v1.Private() == "" {
		t.Fatal("expected non-empty keys on fresh generation")
	}
	if v1.Subject() == "" {
		t.Fatal("expected subject to default to a value")
	}

	// File on disk.
	path := filepath.Join(dir, vapidFile)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("vapid.json not persisted: %v", err)
	}

	// Second call reads from disk and returns the same keys.
	v2, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatalf("second LoadOrCreate: %v", err)
	}
	if v2.Public() != v1.Public() || v2.Private() != v1.Private() {
		t.Errorf("keys changed across LoadOrCreate calls; expected stable persistence")
	}
}

func TestVAPID_LoadOrCreate_RejectsMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, vapidFile)
	// Half-written file (private key only).
	bad := []byte(`{"private_key": "abc"}`)
	if err := os.WriteFile(path, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadOrCreate(dir)
	if err == nil {
		t.Fatal("expected error for malformed vapid.json")
	}
}

func TestRegistry_UpsertGetRemove(t *testing.T) {
	dir := t.TempDir()

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if got := reg.All(); len(got) != 0 {
		t.Errorf("fresh registry: want 0 entries, got %d", len(got))
	}

	sub := Subscription{
		DeviceID: "device-a",
		Endpoint: "https://push.example/u/abc",
		P256dh:   "p256dh-base64",
		Auth:     "auth-secret-base64",
	}
	if err := reg.Upsert(sub); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, ok := reg.Get("device-a")
	if !ok {
		t.Fatal("Get after Upsert: not found")
	}
	if got.Endpoint != sub.Endpoint || got.P256dh != sub.P256dh || got.Auth != sub.Auth {
		t.Errorf("subscription round-trip mismatch")
	}
	if got.RegisteredAt.IsZero() {
		t.Error("expected RegisteredAt to be set on Upsert")
	}

	// Persistence: second registry sees the same data.
	reg2, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("second LoadRegistry: %v", err)
	}
	got2, ok := reg2.Get("device-a")
	if !ok || got2.Endpoint != sub.Endpoint {
		t.Fatal("subscription did not survive reload")
	}

	// Re-upsert replaces.
	sub.Endpoint = "https://push.example/u/xyz"
	if err := reg.Upsert(sub); err != nil {
		t.Fatalf("re-Upsert: %v", err)
	}
	got, _ = reg.Get("device-a")
	if got.Endpoint != "https://push.example/u/xyz" {
		t.Errorf("re-Upsert didn't replace endpoint")
	}

	// Remove drops it.
	if err := reg.Remove("device-a"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := reg.Get("device-a"); ok {
		t.Error("Get after Remove: still found")
	}
	if err := reg.Remove("device-a"); err != nil {
		t.Errorf("Remove on absent device: want nil, got %v", err)
	}
}

func TestRegistry_Upsert_RejectsIncomplete(t *testing.T) {
	dir := t.TempDir()
	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	cases := []Subscription{
		{Endpoint: "https://x", P256dh: "p", Auth: "a"},     // no device id
		{DeviceID: "d", P256dh: "p", Auth: "a"},             // no endpoint
		{DeviceID: "d", Endpoint: "https://x", Auth: "a"},   // no p256dh
		{DeviceID: "d", Endpoint: "https://x", P256dh: "p"}, // no auth
	}
	for i, c := range cases {
		if err := reg.Upsert(c); err == nil {
			t.Errorf("case %d: expected error, got nil", i)
		}
	}
}

func TestRegistry_LoadRegistry_TolerantOfMissing(t *testing.T) {
	dir := t.TempDir()
	// File doesn't exist — should give an empty registry, not an error.
	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry on empty dir: %v", err)
	}
	if got := reg.All(); len(got) != 0 {
		t.Errorf("want empty, got %d entries", len(got))
	}

	// Empty file — also tolerated.
	if err := os.WriteFile(filepath.Join(dir, registryFile), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err = LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry on empty file: %v", err)
	}
	if got := reg.All(); len(got) != 0 {
		t.Errorf("want empty after empty file, got %d entries", len(got))
	}
}

func TestRegistry_LoadRegistry_RejectsMalformed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, registryFile), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadRegistry(dir)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// JSON shape sanity check: ensures the on-disk format is what we
// expect, so external tools (or future migrations) have a stable
// reference. Goldens like this one catch accidental field renames.
func TestSubscription_JSONShape(t *testing.T) {
	s := Subscription{
		DeviceID: "d",
		Endpoint: "https://x/u",
		P256dh:   "p",
		Auth:     "a",
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var asMap map[string]any
	if err := json.Unmarshal(data, &asMap); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"device_id", "endpoint", "p256dh", "auth", "registered_at"} {
		if _, ok := asMap[want]; !ok {
			t.Errorf("expected JSON field %q", want)
		}
	}
}
