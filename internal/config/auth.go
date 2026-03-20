package config

import "os/user"

// AuthInfo holds authentication state (not user-editable).
type AuthInfo struct {
	ID            string `json:"id"`
	Provider      string `json:"provider"`
	Email         string `json:"email"`
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username"`
}

// GetLocalAuthInfo returns auth info for the local (Wails) mode,
// using the current OS user identity.
func GetLocalAuthInfo() AuthInfo {
	u, err := user.Current()
	name := "local"
	if err == nil {
		name = u.Username
	}
	return AuthInfo{
		ID:            name,
		Provider:      "local",
		Email:         "",
		Authenticated: true,
		Username:      name,
	}
}
