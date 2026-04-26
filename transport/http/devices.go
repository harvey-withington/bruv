package http

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Phase 5 groundwork: single-token auth is superseded by a device
// table. Incoming bearer tokens are SHA-256 hashed and looked up in
// a devices.json keyed by hash. The bootstrap token is a special
// entry with scope="bootstrap" — it's only valid for POST /auth/enrol,
// not for the domain surface. Per-device tokens have scope="device"
// and are what desktop/browser clients use for day-to-day traffic.
//
// Tokens are generated once, shown plaintext once, and hashed before
// storage so a compromised devices.json doesn't leak active tokens.

// Device is a single registered client. Token is never stored in
// plaintext — only the hash is on disk.
type Device struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	TokenHash  string    `json:"token_hash"` // hex-encoded SHA-256
	Scope      string    `json:"scope"`      // "bootstrap" | "device"
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// DeviceStore holds registered devices + the bootstrap token. The
// in-memory index is a hash→Device map so every auth-middleware
// lookup is O(1) and constant-time safe.
//
// Mutations (enrolment, last-seen touch) persist to disk immediately.
// Reads are lock-free (copy-on-write) so SSE hot paths aren't stalled
// by writers.
type DeviceStore struct {
	path string

	mu       sync.RWMutex
	devices  []Device
	byHash   map[string]*Device
	touchMu  sync.Mutex // serialises last-seen writes so bursts coalesce
	lastWrit map[string]time.Time
}

// NewDeviceStore loads or generates a device store. On first boot it
// creates a bootstrap token and writes it plaintext to
// `bootstrap-token.txt` beside the store; subsequent boots reuse
// whatever is on disk. Backwards-compat: if a legacy `auth-token.txt`
// exists and `devices.json` doesn't, the legacy token is folded in as
// a device entry so existing installs keep working.
func NewDeviceStore(configDir string) (*DeviceStore, error) {
	path := filepath.Join(configDir, "devices.json")
	s := &DeviceStore{
		path:     path,
		byHash:   make(map[string]*Device),
		lastWrit: make(map[string]time.Time),
	}

	if err := s.load(); err != nil {
		return nil, fmt.Errorf("load devices.json: %w", err)
	}

	if len(s.devices) == 0 {
		if err := s.bootstrapFromLegacyOrFresh(configDir); err != nil {
			return nil, fmt.Errorf("bootstrap: %w", err)
		}
	} else if _, err := os.Stat(filepath.Join(configDir, "bootstrap-token.txt")); os.IsNotExist(err) {
		// devices.json carried over from a prior install / partial
		// uninstall — but the bootstrap token file is missing. Without
		// it the user has no way to enrol new devices. Regenerate:
		// mint a fresh token, replace any existing bootstrap-scope
		// entry in devices.json with the new hash, leave non-bootstrap
		// device entries alone (so already-enrolled clients keep
		// working).
		if err := s.regenerateBootstrap(configDir); err != nil {
			return nil, fmt.Errorf("regenerate bootstrap: %w", err)
		}
	}

	return s, nil
}

// regenerateBootstrap replaces the bootstrap-scope device entry (if
// any) with a fresh token and writes bootstrap-token.txt. Used when
// devices.json survived but the on-disk token file didn't — common
// after a reinstall or a manual cleanup that nuked one but not the
// other. Non-bootstrap device entries are preserved.
func (s *DeviceStore) regenerateBootstrap(configDir string) error {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return err
	}
	bootstrap := hex.EncodeToString(buf[:])
	if err := os.WriteFile(
		filepath.Join(configDir, "bootstrap-token.txt"),
		[]byte(bootstrap+"\n"), 0o600,
	); err != nil {
		return err
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	// Drop any old bootstrap-scope entries.
	kept := make([]Device, 0, len(s.devices))
	for _, d := range s.devices {
		if d.Scope != "bootstrap" {
			kept = append(kept, d)
		}
	}
	kept = append(kept, Device{
		ID:         uuid.New().String(),
		Name:       "bootstrap",
		TokenHash:  hashToken(bootstrap),
		Scope:      "bootstrap",
		CreatedAt:  now,
		LastSeenAt: now,
	})
	s.devices = kept
	s.byHash = make(map[string]*Device, len(s.devices))
	for i := range s.devices {
		s.byHash[s.devices[i].TokenHash] = &s.devices[i]
	}
	if err := s.save(); err != nil {
		return err
	}
	slog.Info("regenerated bootstrap token",
		"bootstrap_path", filepath.Join(configDir, "bootstrap-token.txt"))
	return nil
}

// load reads devices.json into memory. Missing file is not an error;
// callers are expected to bootstrap when the store is empty.
func (s *DeviceStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var file struct {
		Devices []Device `json:"devices"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices = file.Devices
	s.byHash = make(map[string]*Device, len(s.devices))
	for i := range s.devices {
		s.byHash[s.devices[i].TokenHash] = &s.devices[i]
	}
	return nil
}

// save writes devices.json atomically. Caller must hold s.mu.
func (s *DeviceStore) save() error {
	file := struct {
		Devices []Device `json:"devices"`
	}{Devices: s.devices}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// bootstrapFromLegacyOrFresh handles two first-boot paths:
//
//   - Fresh install: generate a random bootstrap token, write it to
//     bootstrap-token.txt for the user to copy-paste when enrolling.
//   - Legacy install: detect the pre-phase-5 `auth-token.txt`, keep
//     it as a device entry (so existing desktop clients authenticate
//     unchanged) and generate a separate, fresh bootstrap token. The
//     legacy token is NEVER promoted to bootstrap scope — that would
//     let it add new devices, which the user never opted into.
func (s *DeviceStore) bootstrapFromLegacyOrFresh(configDir string) error {
	legacyPath := filepath.Join(configDir, "auth-token.txt")
	legacyData, legacyErr := os.ReadFile(legacyPath)

	// Always generate a fresh bootstrap token — even on legacy
	// migration. Bootstrap and device tokens are logically distinct
	// credentials; conflating them was a phase-5 implementation bug.
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return err
	}
	bootstrap := hex.EncodeToString(buf[:])

	if err := os.WriteFile(
		filepath.Join(configDir, "bootstrap-token.txt"),
		[]byte(bootstrap+"\n"), 0o600,
	); err != nil {
		return err
	}

	now := time.Now().UTC()
	s.mu.Lock()
	s.devices = append(s.devices, Device{
		ID:         uuid.New().String(),
		Name:       "bootstrap",
		TokenHash:  hashToken(bootstrap),
		Scope:      "bootstrap",
		CreatedAt:  now,
		LastSeenAt: now,
	})
	// Legacy path: import the pre-phase-5 shared token as a device so
	// the already-running desktop keeps working without re-enrolment.
	if legacyErr == nil {
		legacy := strings.TrimSpace(string(legacyData))
		if legacy != "" {
			s.devices = append(s.devices, Device{
				ID:         uuid.New().String(),
				Name:       "legacy-desktop",
				TokenHash:  hashToken(legacy),
				Scope:      "device",
				CreatedAt:  now,
				LastSeenAt: now,
			})
		}
	}
	s.byHash = make(map[string]*Device, len(s.devices))
	for i := range s.devices {
		s.byHash[s.devices[i].TokenHash] = &s.devices[i]
	}
	if err := s.save(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	if legacyErr == nil {
		slog.Info("phase-5: migrated legacy auth-token.txt into devices.json",
			"bootstrap_path", filepath.Join(configDir, "bootstrap-token.txt"))
	} else {
		slog.Info("phase-5: generated fresh bootstrap token",
			"bootstrap_path", filepath.Join(configDir, "bootstrap-token.txt"))
	}
	return nil
}

// LookupDevice returns the device matching a plaintext token, or nil
// if none match. Uses constant-time comparison per-entry so an
// attacker can't time the loop.
func (s *DeviceStore) LookupDevice(token string) *Device {
	if token == "" {
		return nil
	}
	h := hashToken(token)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.devices {
		if subtle.ConstantTimeCompare([]byte(d.TokenHash), []byte(h)) == 1 {
			// Return a copy so callers can't mutate the store state.
			dc := d
			return &dc
		}
	}
	return nil
}

// TouchLastSeen records the current time on a device without
// hammering disk on every request — writes are debounced to at most
// once per minute per device.
func (s *DeviceStore) TouchLastSeen(id string) {
	s.touchMu.Lock()
	if last, ok := s.lastWrit[id]; ok && time.Since(last) < time.Minute {
		s.touchMu.Unlock()
		return
	}
	s.lastWrit[id] = time.Now()
	s.touchMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.devices {
		if s.devices[i].ID == id {
			s.devices[i].LastSeenAt = time.Now().UTC()
			break
		}
	}
	_ = s.save()
}

// Enrol issues a new device token under the given name, returning the
// plaintext token exactly once. Fails if the bootstrap token doesn't
// match. Callers are expected to have authenticated separately — this
// method trusts its bootstrap argument.
func (s *DeviceStore) Enrol(bootstrapToken, deviceName string) (string, *Device, error) {
	bootstrap := s.LookupDevice(bootstrapToken)
	if bootstrap == nil || bootstrap.Scope != "bootstrap" {
		return "", nil, fmt.Errorf("invalid bootstrap token")
	}
	if strings.TrimSpace(deviceName) == "" {
		deviceName = "Unnamed device"
	}

	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", nil, err
	}
	plaintext := hex.EncodeToString(buf[:])

	now := time.Now().UTC()
	dev := Device{
		ID:         uuid.New().String(),
		Name:       deviceName,
		TokenHash:  hashToken(plaintext),
		Scope:      "device",
		CreatedAt:  now,
		LastSeenAt: now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices = append(s.devices, dev)
	s.byHash[dev.TokenHash] = &s.devices[len(s.devices)-1]
	if err := s.save(); err != nil {
		return "", nil, err
	}
	dc := dev
	return plaintext, &dc, nil
}

// hashToken returns the hex-encoded SHA-256 of a plaintext token.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
