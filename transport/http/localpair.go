package http

// Automatic same-machine pairing — POST /auth/local-pair.
//
// The desktop app already self-enrols by reading bootstrap-token.txt off
// disk: on a single-user machine, same-user file access IS the
// authentication. Browser-sandboxed clients (the clipper extension, the
// mobile PWA / its Vite dev server) can't read files, so they were stuck
// pasting the token by hand for a server running two centimetres away.
// This endpoint extends the same trust judgement to them: a request that
// provably originates from a local browser-privileged origin gets a
// device token with no bootstrap paste.
//
// "Provably local" is a defense stack — each layer kills a specific
// spoof:
//
//  1. RemoteAddr must be loopback — non-local machines never pass.
//  2. NO forwarding headers (X-Forwarded-*, Forwarded) — reverse proxies
//     on this machine (tailscale serve foremost) make REMOTE requests
//     arrive loopback-sourced; they also stamp forwarding headers, which
//     unmasks them. A proxy that strips these headers is a local process
//     acting maliciously — which could read bootstrap-token.txt anyway,
//     so no privilege is gained.
//  3. Host header must itself be loopback — a DNS-rebinding page uses an
//     attacker hostname resolving to 127.0.0.1; the browser still sends
//     that hostname as Host.
//  4. Origin must be a browser-extension origin (chrome-extension:// /
//     moz-extension://) or a localhost dev origin (the Vite mobile dev
//     server) — web pages can never fake these schemes, and rebinding
//     pages carry their own https?://attacker origin. An ABSENT Origin is
//     rejected too: non-browser local callers should read the token file
//     like the desktop app does.
//
// Deliberately NOT mounted behind requireBootstrap — this endpoint's
// whole point is replacing the paste with the stack above.

import (
	"encoding/json"
	"net"
	nethttp "net/http"
	"os"
	"path/filepath"
	"strings"
)

type localPairRequest struct {
	DeviceName string `json:"device_name"`
}

func localPairHandler(configDir string, store *DeviceStore) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method != nethttp.MethodPost {
			nethttp.Error(w, `{"error":"method must be POST"}`, nethttp.StatusMethodNotAllowed)
			return
		}
		if !isLoopbackAddr(r.RemoteAddr) {
			nethttp.Error(w, `{"error":"local pairing is same-machine only"}`, nethttp.StatusForbidden)
			return
		}
		if hasForwardingHeaders(r) {
			nethttp.Error(w, `{"error":"local pairing refuses proxied requests"}`, nethttp.StatusForbidden)
			return
		}
		if !isLoopbackHost(r.Host) {
			nethttp.Error(w, `{"error":"local pairing requires a loopback host"}`, nethttp.StatusForbidden)
			return
		}
		if !isTrustedLocalOrigin(r.Header.Get("Origin")) {
			nethttp.Error(w, `{"error":"local pairing requires a local browser origin"}`, nethttp.StatusForbidden)
			return
		}

		var req localPairRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			nethttp.Error(w, `{"error":"invalid JSON"}`, nethttp.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(req.DeviceName)
		if name == "" {
			name = "Local device"
		}

		// The gate above establishes the same trust file access proves —
		// so enrol exactly as if the caller had pasted the token.
		bootstrap, err := os.ReadFile(filepath.Join(configDir, "bootstrap-token.txt"))
		if err != nil {
			nethttp.Error(w, `{"error":"bootstrap token unavailable"}`, nethttp.StatusInternalServerError)
			return
		}
		token, dev, err := store.Enrol(strings.TrimSpace(string(bootstrap)), name)
		if err != nil {
			nethttp.Error(w, `{"error":"`+err.Error()+`"}`, nethttp.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(enrolResponse{
			DeviceToken: token,
			DeviceID:    dev.ID,
			DeviceName:  dev.Name,
		})
	}
}

func isLoopbackAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// (Host check reuses pair.go's isLoopbackHost.)

func hasForwardingHeaders(r *nethttp.Request) bool {
	for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-Ip", "Forwarded"} {
		if r.Header.Get(h) != "" {
			return true
		}
	}
	return false
}

// isTrustedLocalOrigin allows browser-extension origins (unforgeable by
// web content) and loopback dev-server origins (the mobile Vite dev
// server; unforgeable by rebinding, which keeps the attacker's own
// hostname in Origin).
func isTrustedLocalOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	if strings.HasPrefix(origin, "chrome-extension://") || strings.HasPrefix(origin, "moz-extension://") {
		return true
	}
	for _, prefix := range []string{"http://localhost:", "http://127.0.0.1:"} {
		if strings.HasPrefix(origin, prefix) {
			return true
		}
	}
	return origin == "http://localhost" || origin == "http://127.0.0.1"
}
