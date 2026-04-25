package config

// Server-side HMAC secret.
//
// The attachments handler signs download URLs with HMAC-SHA256 so
// that the URL can be embedded in <img src="..."> without exposing
// the bearer token (browsers don't attach Authorization headers to
// img/audio/video). The secret lives in the server's config dir,
// generated on first read, never rotated automatically.
//
// Storage: <configDir>/secret.key — 32 raw bytes, hex-encoded for
// inspectability. This is server-owned: the same secret signs every
// URL handed out by this server, and rotating it invalidates every
// in-flight download URL (acceptable because the URLs are 5-min-lived).

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
)

const serverSecretFileName = "secret.key"

// LoadServerSecret returns the server's 32-byte HMAC secret,
// generating + persisting one on first call. Errors are swallowed
// and a process-local random secret is used as a fallback so the
// server still starts (URLs signed in this session won't survive a
// restart, which is acceptable failure for a transient problem).
func LoadServerSecret() []byte {
	dir, err := configDir()
	if err != nil {
		return randomSecret()
	}
	path := filepath.Join(dir, serverSecretFileName)
	if data, err := os.ReadFile(path); err == nil {
		// Hex-decode; if the file is malformed (e.g. user edited it),
		// fall through to regenerate.
		if decoded, err := hex.DecodeString(string(data)); err == nil && len(decoded) >= 32 {
			return decoded
		}
	}
	secret := randomSecret()
	_ = os.WriteFile(path, []byte(hex.EncodeToString(secret)), 0o600)
	return secret
}

func randomSecret() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Cryptographic RNG failure on a healthy OS is essentially
		// impossible. Panic so the server doesn't quietly run with
		// a predictable secret.
		panic("config: rand.Read failed: " + err.Error())
	}
	return b
}
