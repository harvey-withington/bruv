package config

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
)

// UserProfile holds the user's editable identity, visible to collaborators and LLMs.
//
// Server-side storage: a profile is associated with a server, not a
// device. Two devices sharing one BRUV server see the same profile.
// (Per-device identity for activity-log sharding lives separately —
// see identity.go.)
type UserProfile struct {
	DisplayName string   `json:"display_name"`
	Role        string   `json:"role"`       // e.g. "Software Engineer", "Content Creator"
	Bio         string   `json:"bio"`        // Short personal description
	Expertise   []string `json:"expertise"`  // Areas of knowledge / skill
	AvatarURL   string   `json:"avatar_url"` // Profile picture URL or path
}

func profilePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profile.json"), nil
}

// LoadProfile reads the user profile from disk. On first load (no
// file), auto-populates DisplayName from the OS account. Per-device
// identity (UserID) lives separately in clientdata — see identity.go.
func LoadProfile() (UserProfile, error) {
	var p UserProfile
	path, err := profilePath()
	if err != nil {
		return p, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			p.DisplayName = osDisplayName()
			_ = SaveProfile(p) // best effort
			return p, nil
		}
		return p, err
	}
	// Decode into a raw map so we can detect + hoist a stale `user_id`
	// field from older builds that kept the device identity here. The
	// hoisted value seeds <clientdata>/device-id.txt so the activity
	// log's shard key stays stable across the move.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err == nil {
		if uidRaw, ok := raw["user_id"]; ok {
			var uid string
			if json.Unmarshal(uidRaw, &uid) == nil && uid != "" {
				if dpath, derr := deviceIDPath(); derr == nil {
					if _, statErr := os.Stat(dpath); os.IsNotExist(statErr) {
						_ = os.WriteFile(dpath, []byte(uid+"\n"), 0o644)
					}
				}
			}
			delete(raw, "user_id")
			if cleaned, mErr := json.MarshalIndent(raw, "", "  "); mErr == nil {
				_ = os.WriteFile(path, cleaned, 0o644)
			}
		}
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return UserProfile{}, err
	}
	return p, nil
}

// osDisplayName returns the current OS user's display name, or empty string on failure.
func osDisplayName() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	if u.Name != "" {
		return u.Name
	}
	return u.Username
}

// SaveProfile writes the user profile to disk.
func SaveProfile(p UserProfile) error {
	path, err := profilePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
