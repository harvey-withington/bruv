package config

// Keychain-backed storage for LLM provider API keys.
//
// Background: prior to this module, API keys were written in plaintext to
// llm_accounts.json inside the user's config directory. That's a known
// limitation flagged in PRIVACY.md — fine for a local-first app on a
// single-user machine, but upgraded to OS keychain storage as part of
// Sprint B so the plaintext footprint goes away.
//
// How it works:
//
//   - On save, SaveLLMAccounts tries to store each account's API key in
//     the OS keychain (Windows Credential Manager, macOS Keychain, or
//     libsecret on Linux) under a stable key derived from the account ID.
//     If the keychain store succeeds, the APIKey field is blanked out
//     before the JSON is written.
//
//   - On load, LoadLLMAccounts checks the keychain for each account whose
//     APIKey field is empty and hydrates it from the keychain if present.
//
//   - On delete, any account whose ID disappears between the previous
//     save and the current save has its keychain entry purged.
//
//   - If the OS keychain is unavailable (broken libsecret, locked down
//     corporate machine, CI environment without a keyring daemon), the
//     code falls back gracefully: keys are written to the JSON in
//     plaintext exactly as before. The goal is to upgrade security when
//     possible, never to lock users out of their own data.
//
//   - MigrateLLMKeysToKeychain is called once on startup to move any
//     existing plaintext keys out of the JSON file. It's idempotent: if
//     the keychain is unavailable it's a no-op; if some keys have already
//     been migrated it only touches the ones that haven't.

import (
	"errors"
	"log"

	"github.com/zalando/go-keyring"
)

// keychainService is the service name under which BRUV registers entries
// in the OS keychain. Users can see these in Credential Manager (Windows)
// or Keychain Access (macOS) so the name is intentionally human-readable.
const keychainService = "BRUV"

// keychainAccountPrefix is prepended to each LLMAccount.ID when building
// the keychain entry name. Namespaces the entries so future secret types
// (e.g. OAuth refresh tokens) can share the same service without colliding.
const keychainAccountPrefix = "llm-account-"

// keychainAvailable caches the availability check so we don't repeatedly
// round-trip a broken keyring. Set on first access; reset never — if the
// keyring comes online mid-session we'll pick it up on the next restart.
var (
	keychainChecked   bool
	keychainWorks     bool
	keychainCheckDone = make(chan struct{})
)

// probeKeychain does a write/read/delete round trip with a harmless value
// to determine whether the OS keychain is actually usable. This avoids
// false negatives from systems where go-keyring is compiled in but the
// underlying daemon isn't running.
func probeKeychain() {
	const probeKey = "__probe__"
	if err := keyring.Set(keychainService, probeKey, "probe"); err != nil {
		log.Printf("keychain: probe write failed, falling back to plaintext: %v", err)
		keychainWorks = false
		return
	}
	if _, err := keyring.Get(keychainService, probeKey); err != nil {
		log.Printf("keychain: probe read failed, falling back to plaintext: %v", err)
		keychainWorks = false
		return
	}
	_ = keyring.Delete(keychainService, probeKey)
	keychainWorks = true
}

// KeychainAvailable reports whether the OS keychain is usable for
// storing BRUV secrets. Exported for tests and diagnostics.
func KeychainAvailable() bool {
	if !keychainChecked {
		probeKeychain()
		keychainChecked = true
		close(keychainCheckDone)
	}
	return keychainWorks
}

// keychainKey returns the entry name used for a given account ID.
func keychainKey(accountID string) string {
	return keychainAccountPrefix + accountID
}

// storeKeychainSecret saves an API key in the OS keychain. Returns an
// error if the keychain is unavailable or the store failed — callers are
// expected to fall back to plaintext JSON in that case.
func storeKeychainSecret(accountID, secret string) error {
	if !KeychainAvailable() {
		return errors.New("keychain unavailable")
	}
	return keyring.Set(keychainService, keychainKey(accountID), secret)
}

// getKeychainSecret fetches a previously-stored API key. Returns an empty
// string and no error if the entry doesn't exist — a missing entry is a
// normal state for accounts that were never migrated, not a failure.
func getKeychainSecret(accountID string) (string, error) {
	if !KeychainAvailable() {
		return "", nil
	}
	secret, err := keyring.Get(keychainService, keychainKey(accountID))
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return secret, nil
}

// deleteKeychainSecret purges an entry. Missing entries are not an error
// — the operation is idempotent so it's safe to call unconditionally on
// account removal.
func deleteKeychainSecret(accountID string) {
	if !KeychainAvailable() {
		return
	}
	err := keyring.Delete(keychainService, keychainKey(accountID))
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		log.Printf("keychain: delete %q failed: %v", accountID, err)
	}
}
