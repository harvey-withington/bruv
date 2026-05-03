// Package push manages Web Push notifications for the BRUV server.
//
// The package owns three concerns:
//
//   - VAPID keypair generation + persistence (vapid.go).
//     Keys are generated once on first server boot and stored beside
//     the bootstrap token in the configDir. The public key is shared
//     with mobile clients; the private key signs JWT claims for each
//     push request. Rotation is manual (delete vapid.json, restart).
//
//   - Subscription registry (registry.go). One subscription per
//     device, keyed by device ID from the existing device store.
//     File-backed JSON, mutex-protected, written through (no in-
//     memory drift from disk).
//
//   - Push sender (sender.go). Wraps webpush-go's Send with the
//     server's VAPID claim. Cleans up subscriptions on 404/410
//     responses — the push service tells us the subscription is
//     dead and we should stop sending.
//
// Trust model is the same as the rest of the server: file-system
// access to configDir is the credential. Push subscriptions are
// not secret per se (the endpoint URL has its own per-client auth)
// but treating them as device-scoped data keeps the API tidy.
package push

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// vapidFile is the on-disk filename within configDir.
const vapidFile = "vapid.json"

// defaultSubject is the VAPID `sub` claim sent to push services. The
// spec recommends a mailto: or https URL identifying the application
// server; push services use it as a contact for abuse reports. We
// don't have a real address, so a synthetic mailto suffices — push
// services accept it. Operators can override by setting the subject
// in vapid.json directly.
const defaultSubject = "mailto:bruv-push@local.invalid"

// VAPID holds the keypair + subject for the application server.
// Loaded once at boot via LoadOrCreate; treat as immutable.
type VAPID struct {
	mu         sync.RWMutex
	privateKey string
	publicKey  string
	subject    string
}

// vapidJSON is the on-disk shape. Kept small because it's read once
// at boot and never written back (rotation = manual file delete).
type vapidJSON struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
	Subject    string `json:"subject,omitempty"`
}

// LoadOrCreate reads the VAPID keypair from <configDir>/vapid.json,
// generating + persisting a fresh one if the file doesn't exist.
// Returns a usable *VAPID either way. Errors only on truly broken
// state (unwriteable directory, malformed existing file we can't
// recover from).
func LoadOrCreate(configDir string) (*VAPID, error) {
	if configDir == "" {
		return nil, fmt.Errorf("push: configDir is required")
	}
	path := filepath.Join(configDir, vapidFile)
	if data, err := os.ReadFile(path); err == nil {
		var vj vapidJSON
		if err := json.Unmarshal(data, &vj); err != nil {
			return nil, fmt.Errorf("push: parse %s: %w", path, err)
		}
		if vj.PrivateKey == "" || vj.PublicKey == "" {
			return nil, fmt.Errorf("push: %s missing keys", path)
		}
		v := &VAPID{
			privateKey: vj.PrivateKey,
			publicKey:  vj.PublicKey,
			subject:    vj.Subject,
		}
		if v.subject == "" {
			v.subject = defaultSubject
		}
		return v, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("push: read %s: %w", path, err)
	}

	// Fresh keypair.
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return nil, fmt.Errorf("push: generate VAPID keys: %w", err)
	}

	vj := vapidJSON{PrivateKey: priv, PublicKey: pub, Subject: defaultSubject}
	if err := writeJSONAtomically(path, vj); err != nil {
		return nil, fmt.Errorf("push: persist %s: %w", path, err)
	}

	return &VAPID{
		privateKey: priv,
		publicKey:  pub,
		subject:    defaultSubject,
	}, nil
}

// Public returns the base64url-encoded public key. Mobile clients
// pass this to navigator.pushManager.subscribe()'s applicationServerKey.
func (v *VAPID) Public() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.publicKey
}

// Private returns the base64url-encoded private key. Used by the
// sender when signing per-request JWTs; not exposed to clients.
func (v *VAPID) Private() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.privateKey
}

// Subject returns the VAPID `sub` claim — typically a mailto: or
// https URL. The default is good enough for personal use; operators
// who want to set their own can edit vapid.json.
func (v *VAPID) Subject() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.subject
}

// writeJSONAtomically writes v to path via a tmp+rename so a crash
// mid-write doesn't leave a half-written file. Same pattern the rest
// of the BRUV config loaders use.
func writeJSONAtomically(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Use crypto/rand for the tmp suffix so concurrent writers don't
	// collide. 8 bytes is plenty for the no-collision-in-practice case.
	suffixBytes := make([]byte, 8)
	if _, err := rand.Read(suffixBytes); err != nil {
		return err
	}
	suffix := strings.TrimRight(base64.URLEncoding.EncodeToString(suffixBytes), "=")
	tmp := path + ".tmp." + suffix
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
