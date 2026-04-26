package http

import nethttp "net/http"

// withCORS wraps a handler so browsers can talk to us from a
// different origin. Needed because the Wails webview serves itself
// from `wails.localhost:<port>` while this HTTP server binds to
// `127.0.0.1:<port>` — the browser treats them as distinct origins
// and preflights every fetch that carries custom headers (like the
// Authorization bearer we send on every /rpc call).
//
// Origin policy: we echo back whatever Origin the browser sent,
// which is effectively "allow any origin". Safe because:
//
//   - Auth is bearer-token, not cookie-based, so Access-Control-
//     Allow-Credentials is explicitly off and the classic
//     cookie-CSRF attack surface doesn't apply.
//   - Tokens are carried in the Authorization header (for /rpc) or
//     the ?token= query param (for /events via EventSource). Either
//     way, an attacker page in another origin still needs the
//     token to reach the data.
//   - Tailnet / loopback binding means reachability is already
//     gated at the network layer for remote deployments.
//
// When Mode B tightens up in a future pass (origin allowlist in
// config, per-device CORS policies, etc.) this middleware is the
// single point to change.
func withCORS(next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			// Echo the origin back rather than returning `*`. Browsers
			// enforce that `*` can't coexist with credential-bearing
			// headers; echoing keeps the door open if we ever need
			// cookies, and it makes the server's intent explicit in
			// every response.
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		// Browsers cache a successful preflight for this many seconds
		// so we're not preflighting every single RPC call.
		w.Header().Set("Access-Control-Max-Age", "600")

		if r.Method == nethttp.MethodOptions {
			w.WriteHeader(nethttp.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
