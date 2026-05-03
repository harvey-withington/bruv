package push

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// registryFile is the on-disk filename inside configDir.
const registryFile = "push-subscriptions.json"

// Subscription mirrors the subset of the W3C Push API's
// PushSubscription.toJSON() shape that we need to forward to the
// push service. The keys carry the client's public key + auth secret;
// without them the encryption pipeline can't deliver a payload.
type Subscription struct {
	DeviceID     string    `json:"device_id"`
	Endpoint     string    `json:"endpoint"`
	P256dh       string    `json:"p256dh"`
	Auth         string    `json:"auth"`
	RegisteredAt time.Time `json:"registered_at"`
}

// Registry is a process-wide, file-backed map from device ID →
// Subscription. One subscription per device — a re-registration
// replaces the prior one. Operations write through to disk.
type Registry struct {
	mu      sync.Mutex
	path    string
	entries map[string]Subscription
}

// LoadRegistry reads <configDir>/push-subscriptions.json from disk.
// Missing file = empty registry. Malformed file is an error — the
// operator can delete it manually if it really has gone bad.
func LoadRegistry(configDir string) (*Registry, error) {
	if configDir == "" {
		return nil, fmt.Errorf("push: configDir is required")
	}
	path := filepath.Join(configDir, registryFile)
	r := &Registry{path: path, entries: make(map[string]Subscription)}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, fmt.Errorf("push: read %s: %w", path, err)
	}
	if len(data) == 0 {
		return r, nil
	}

	var list []Subscription
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("push: parse %s: %w", path, err)
	}
	for _, s := range list {
		if s.DeviceID == "" || s.Endpoint == "" {
			continue // skip malformed
		}
		r.entries[s.DeviceID] = s
	}
	return r, nil
}

// Upsert installs (or replaces) the subscription for the given device.
// RegisteredAt is set to the current time on every upsert so we can
// reason about staleness later if needed.
func (r *Registry) Upsert(s Subscription) error {
	if s.DeviceID == "" {
		return fmt.Errorf("push: subscription missing device_id")
	}
	if s.Endpoint == "" {
		return fmt.Errorf("push: subscription missing endpoint")
	}
	if s.P256dh == "" || s.Auth == "" {
		return fmt.Errorf("push: subscription missing keys")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s.RegisteredAt = time.Now().UTC()
	r.entries[s.DeviceID] = s
	return r.persistLocked()
}

// Remove deletes the subscription for a device. No-op if the device
// has no subscription. Used both by the explicit Unregister RPC and
// by the device-store hook (when a device is revoked, its push
// subscription is dropped too).
func (r *Registry) Remove(deviceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[deviceID]; !ok {
		return nil
	}
	delete(r.entries, deviceID)
	return r.persistLocked()
}

// Get returns the subscription for a device, or false if none.
func (r *Registry) Get(deviceID string) (Subscription, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.entries[deviceID]
	return s, ok
}

// All returns a snapshot of every registered subscription. Order is
// not guaranteed.
func (r *Registry) All() []Subscription {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Subscription, 0, len(r.entries))
	for _, s := range r.entries {
		out = append(out, s)
	}
	return out
}

// persistLocked writes the current entries to disk. Caller holds r.mu.
// Atomic via writeJSONAtomically (tmp+rename).
func (r *Registry) persistLocked() error {
	list := make([]Subscription, 0, len(r.entries))
	for _, s := range r.entries {
		list = append(list, s)
	}
	return writeJSONAtomically(r.path, list)
}
