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
		provided := extractBearer(r)
		if provided == "" {
			nethttp.Error(w, `{"error":"missing bearer token"}`, nethttp.StatusUnauthorized)
			return
		}
		dev := store.LookupDevice(provided)
		if dev == nil {
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

// extractBearer reads the token from Authorization header OR ?token=
// query param (EventSource can't set headers — query fallback is a
// pragmatic compromise; locked-down transport means loopback-only
// traffic for phase 3).
func extractBearer(r *nethttp.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		const prefix = "Bearer "
		if strings.HasPrefix(h, prefix) {
			return strings.TrimSpace(h[len(prefix):])
		}
	}
	return r.URL.Query().Get("token")
}
