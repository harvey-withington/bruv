package http

import (
	nethttp "net/http"
	"net/url"
	"strings"
)

// uncliGuard is BRUV's sign-in in front of its UNCLI (UNCLI decision
// 0027): anyone it lets through can make the CLIs run tools on this
// machine as the server's account.
//
//   - A device token in an Authorization header, which <uncli-chat> sends
//     through its host.headers hook. Never ?token=: these are tool-running
//     URLs, and nothing here needs one.
//   - UNCLI's signed links (/uncli/l/…) pass without one, like BRUV's own
//     signed attachments: they open in a new tab, and the core checks
//     their signature.
//
// Origins are the core's to check (uncliengine passes UncliOriginAllowed
// as its AllowOrigin), and preflights never get here: withCORS answers
// OPTIONS for the whole server. The core's Handler replaces the
// Authorization header with its own key, so BRUV's token isn't forwarded.
func uncliGuard(store *DeviceStore, next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if !strings.HasPrefix(r.URL.Path, "/uncli/l/") {
			dev := store.LookupDevice(bearerHeader(r))
			if dev == nil || dev.Scope == "bootstrap" {
				nethttp.Error(w, `{"error":"sign in to BRUV first"}`, nethttp.StatusUnauthorized)
				return
			}
			store.TouchLastSeen(dev.ID)
		}
		next.ServeHTTP(w, r)
	})
}

// bearerHeader is the token in an Authorization: Bearer header, and only
// there.
func bearerHeader(r *nethttp.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return ""
}

// UncliOriginAllowed says which pages may call the server's UNCLI: those
// served by this server under the name the request used (it's reached
// under several: LAN, tailnet IP, *.ts.net), the desktop app's webview
// (wails.localhost), and a loopback page arriving over a loopback
// connection (the vite dev servers, whose proxies rewrite Host). A
// loopback reverse proxy (tailscale serve) may also rewrite Host, so its
// X-Forwarded-Host counts there. Anything else is refused.
func UncliOriginAllowed(origin string, r *nethttp.Request) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) || strings.EqualFold(u.Hostname(), "wails.localhost") {
		return true
	}
	if !isLoopbackAddr(r.RemoteAddr) {
		return false
	}
	return isLoopbackHost(u.Host) || strings.EqualFold(u.Host, r.Header.Get("X-Forwarded-Host"))
}
