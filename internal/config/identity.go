package config

// Per-device identity.
//
// LoadDeviceID returns a stable UUID identifying this machine. Used
// as the actor key when sharding the activity log by writer (see
// internal/repo/activity.go) so two devices syncing the same repo
// never append to the same shard file.
//
// The identity is intentionally per-DEVICE rather than per-user:
// the same human across two devices ends up with two device IDs.
// That matches the activity log's "who at this terminal did this"
// posture, and keeps clientdata genuinely client-owned. A future
// per-user identity (cloud account etc.) would live alongside, not
// replace, the device ID.
//
// Storage: <clientdata>/device-id.txt — single line, the UUID.

import (
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

const deviceIDFileName = "device-id.txt"

func deviceIDPath() (string, error) {
	dir, err := ClientDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, deviceIDFileName), nil
}

// LoadDeviceID returns this machine's stable identifier, generating
// and persisting one on first call. Subsequent calls return the same
// value for the lifetime of the clientdata directory.
//
// On error (can't resolve clientdata, can't write the file), returns
// a fresh UUID for this process so callers don't have to handle the
// nil case — but won't be stable across restarts in that scenario.
// Errors are intentionally swallowed because identity allocation must
// never block a real user action.
func LoadDeviceID() string {
	path, err := deviceIDPath()
	if err != nil {
		return uuid.NewString()
	}
	if data, err := os.ReadFile(path); err == nil {
		id := trimSpace(string(data))
		if id != "" {
			return id
		}
	}
	id := uuid.NewString()
	_ = os.WriteFile(path, []byte(id+"\n"), 0o644)
	return id
}
