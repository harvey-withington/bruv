package config

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"

	"github.com/google/uuid"
)

// UserProfile holds the user's editable identity, visible to collaborators and LLMs.
//
// UserID is a machine-local stable identifier — a UUID generated on
// first profile load and persisted forever after. It's the key used
// by the activity log to shard each user's writes into their own
// file (avoids merge conflicts when a repo is shared) and is never
// shown to the user. The DisplayName is the human-friendly name and
// is mutable; UserID stays stable across renames.
type UserProfile struct {
	UserID      string   `json:"user_id"`
	DisplayName string   `json:"display_name"`
	Role        string   `json:"role"`         // e.g. "Software Engineer", "Content Creator"
	Bio         string   `json:"bio"`          // Short personal description
	Expertise   []string `json:"expertise"`    // Areas of knowledge / skill
	AvatarURL   string   `json:"avatar_url"`   // Profile picture URL or path
}

func profilePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profile.json"), nil
}

// LoadProfile reads the user profile from disk.
// On first load (no file), auto-populates DisplayName from the OS account.
// UserID is auto-generated on first load OR backfilled on existing
// profiles that pre-date the field, then persisted immediately so
// subsequent loads return a stable value.
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
			p.UserID = uuid.NewString()
			_ = SaveProfile(p) // best effort: a failed write here just means the next load regenerates
			return p, nil
		}
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return UserProfile{}, err
	}
	// UserID is a required field; if it's missing on an existing
	// profile, generate one and persist immediately so the activity
	// log gets a stable shard key from the very next write.
	if p.UserID == "" {
		p.UserID = uuid.NewString()
		_ = SaveProfile(p)
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
