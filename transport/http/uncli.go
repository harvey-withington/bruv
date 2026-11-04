package http

import (
	nethttp "net/http"
	"net/url"
	"strings"
)

// uncliCookie carries the device token to /uncli/: <uncli-chat> makes its
// own requests (fetch and its event stream) and has no way to add
// BRUV's Authorization header, so the page swaps its token for this
// cookie first (POST /auth/uncli-cookie). HttpOnly and SameSite=Strict,
// and scoped to /uncli so no other route accepts it.
const uncliCookie = "bruv_uncli"

// uncliGuard is BRUV's sign-in in front of its UNCLI (UNCLI decision
// 0027): anyone it lets through can make the CLIs run tools on this
// machine as the server's account.
//
//   - Pages may only call it from this server's own origin. Origins are
//     checked here rather than by the core's fixed list, since the server
//     is reached under several names (LAN, tailnet IP, *.ts.net); the
//     Origin is then dropped so the core's own check passes.
//   - A device token, from the cookie or an Authorization header. Never
//     ?token=: these are tool-running URLs.
//   - UNCLI's signed links (/uncli/l/…) pass without one, like BRUV's own
//     signed attachments: they open in a new tab, and the core checks
//     their signature.
//
// BRUV's credentials aren't forwarded: the core's Handler replaces the
// Authorization header with its own key, and the cookie is removed.
func uncliGuard(store *DeviceStore, next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if !uncliOriginAllowed(r) {
			nethttp.Error(w, `{"error":"cross-origin requests aren't allowed"}`, nethttp.StatusForbidden)
			return
		}
		r = r.Clone(r.Context())
		r.Header.Del("Origin")
		if !strings.HasPrefix(r.URL.Path, "/uncli/l/") {
			dev := store.LookupDevice(uncliToken(r))
			if dev == nil || dev.Scope == "bootstrap" {
				nethttp.Error(w, `{"error":"sign in to BRUV first"}`, nethttp.StatusUnauthorized)
				return
			}
			store.TouchLastSeen(dev.ID)
		}
		r.Header.Del("Cookie")
		next.ServeHTTP(w, r)
	})
}

// uncliToken is the request's device token: the cookie, else a bearer
// header (BRUV's own server-side calls).
func uncliToken(r *nethttp.Request) string {
	if c, err := r.Cookie(uncliCookie); err == nil && c.Value != "" {
		return c.Value
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return ""
}

// uncliOriginAllowed accepts requests without an Origin, and pages served
// by this server under the name the request used. A loopback page reached
// through a loopback connection (the vite dev servers' proxies, which
// rewrite Host) passes too.
func uncliOriginAllowed(r *nethttp.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	return isLoopbackHost(u.Host) && isLoopbackAddr(r.RemoteAddr)
}

// uncliCookieHandler swaps the caller's device token (already checked by
// requireAuth) for the /uncli cookie. Session-only: the page asks again
// each time it opens a chat.
func uncliCookieHandler() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method != nethttp.MethodPost {
			nethttp.Error(w, `{"error":"POST only"}`, nethttp.StatusMethodNotAllowed)
			return
		}
		nethttp.SetCookie(w, &nethttp.Cookie{
			Name:     uncliCookie,
			Value:    extractBearer(r),
			Path:     "/uncli",
			HttpOnly: true,
			SameSite: nethttp.SameSiteStrictMode,
			Secure:   r.TLS != nil,
		})
		w.WriteHeader(nethttp.StatusNoContent)
	})
}
