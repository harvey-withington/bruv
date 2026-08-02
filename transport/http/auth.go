package http

import (
	nethttp "net/http"
	"strings"
)

// requireAuth wraps a handler, rejecting requests without a valid
// Authorization: Bearer header. Validates against the device store;
// bootstrap-scoped tokens are rejected for regular traffic — they're
// only useful against POST /auth/enrol (see requireBootstrap).
func requireAuth(store *DeviceStore, next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		// Bypass bearer token check for signed attachment URLs. These are verified
		// using HMAC-SHA256 signature/expiry check inside attachmentHandler.
		if strings.Contains(r.URL.Path, "/attachments/") {
			next.ServeHTTP(w, r)
			return
		}

		provided := extractBearer(r)
		if provided == "" {
			challengeIfGit(w, r)
			nethttp.Error(w, `{"error":"missing bearer token"}`, nethttp.StatusUnauthorized)
			return
		}
		dev := store.LookupDevice(provided)
		if dev == nil {
			challengeIfGit(w, r)
			nethttp.Error(w, `{"error":"invalid bearer token"}`, nethttp.StatusUnauthorized)
			return
		}
		if dev.Scope == "bootstrap" {
			// Bootstrap tokens can only enrol — not touch data.
			nethttp.Error(w, `{"error":"bootstrap token cannot access this surface"}`, nethttp.StatusUnauthorized)
			return
		}
		store.TouchLastSeen(dev.ID)
		next.ServeHTTP(w, r)
	})
}

// requireBootstrap gates the enrolment endpoint: only valid bootstrap
// tokens may proceed. Keeps the public attack surface small — an
// attacker who steals a device token can't use it to add more devices.
func requireBootstrap(store *DeviceStore, next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		provided := extractBearer(r)
		if provided == "" {
			nethttp.Error(w, `{"error":"missing bearer token"}`, nethttp.StatusUnauthorized)
			return
		}
		dev := store.LookupDevice(provided)
		if dev == nil || dev.Scope != "bootstrap" {
			nethttp.Error(w, `{"error":"bootstrap token required"}`, nethttp.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extractBearer reads the token from the Authorization header OR ?token=
// query param (EventSource can't set headers — query fallback is a
// pragmatic compromise; locked-down transport means loopback-only
// traffic for phase 3).
//
// HTTP Basic is accepted as a third form, carrying the same device token
// as the password. That exists for git: the smart-HTTP workspace transport
// (git.go) is spoken by the system git binary, which has no way to send a
// bearer token but full support for Basic — so `git clone` against a
// published workspace authenticates with the device token like everything
// else, and a user can clone one by hand with the same credential.
func extractBearer(r *nethttp.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		const prefix = "Bearer "
		if strings.HasPrefix(h, prefix) {
			return strings.TrimSpace(h[len(prefix):])
		}
		if _, pass, ok := r.BasicAuth(); ok && pass != "" {
			return pass
		}
	}
	return r.URL.Query().Get("token")
}

// challengeIfGit adds the Basic challenge to a 401 on the git transport.
// Git sends its first request unauthenticated and only supplies stored
// credentials after being challenged, so without this header a clone fails
// on the first 401 instead of retrying with the device token. Restricted
// to git paths so browser clients never get a native password prompt.
func challengeIfGit(w nethttp.ResponseWriter, r *nethttp.Request) {
	if strings.Contains(r.URL.Path, "/git/") {
		w.Header().Set("WWW-Authenticate", `Basic realm="BRUV workspace"`)
	}
}
