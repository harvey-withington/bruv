package http

import (
	"encoding/json"
	"net"
	nethttp "net/http"
	"strings"
)

// mobileManifestHandler serves the mobile PWA's Web App Manifest with
// `name` / `short_name` templated from the request's Host header. This
// is what makes "Add to Home Screen" install distinct icons when the
// user pairs with multiple BRUV servers — without it, every install
// shows up as a generic "BRUV" tile and the user can't tell their
// home server's icon from their work server's.
//
// Mounted at /m/manifest.webmanifest, which by Go's longest-pattern-
// wins routing takes precedence over the /m/ static handler — so the
// static fallback in mobile/public/manifest.webmanifest only ships in
// dev mode (`npm run dev`), where the production server isn't in the
// path.
//
// The disambiguation label is the first DNS segment of the host (e.g.
// "deviant" from "deviant.tail2ebd58.ts.net"). For IPs / localhost /
// other hostnames where a label wouldn't be meaningful we fall back
// to the bare "BRUV" so we don't end up with "BRUV 127" tiles.
func mobileManifestHandler() nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		label := manifestLabelFor(hostFromRequest(r))

		name := "BRUV"
		shortName := "BRUV"
		if label != "" {
			name = "BRUV — " + label
			shortName = "BRUV " + label
		}

		manifest := map[string]any{
			"name":             name,
			"short_name":       shortName,
			"description":      "Your most organised best bud — capture, triage, and review your vault on the go.",
			"id":               "/m/",
			"start_url":        "/m/",
			"scope":            "/m/",
			"display":          "standalone",
			"orientation":      "portrait",
			"theme_color":      "#18181b",
			"background_color": "#18181b",
			"lang":             "en",
			"dir":              "ltr",
			"icons": []map[string]any{
				{"src": "/m/bruv-icon.svg", "sizes": "any", "type": "image/svg+xml"},
				{"src": "/m/icon-192.png", "sizes": "192x192", "type": "image/png"},
				{"src": "/m/icon-512.png", "sizes": "512x512", "type": "image/png"},
			},
			// Android share_target. GET form (the Level-1 spec): the
			// browser navigates to /m/share?title=…&text=…&url=… and
			// our SPA fallback serves the shell, which the SPA's
			// router maps to SharePage.
			//
			// We deliberately don't declare POST + multipart here:
			// that form is for accepting files, and Chrome filters the
			// PWA out of the share sheet when POST is declared without
			// a `files` entry. v1 only handles text/URL/title; if/when
			// we accept image shares, the manifest grows a `files`
			// entry and the handler returns.
			//
			// iOS Safari ignores share_target entirely (deliberate
			// platform restriction) — iOS users get the clipboard
			// import path instead.
			"share_target": map[string]any{
				"action": "/m/share",
				"method": "GET",
				"params": map[string]any{
					"title": "title",
					"text":  "text",
					"url":   "url",
				},
			},
		}

		w.Header().Set("Content-Type", "application/manifest+json")
		// Per-host content; intermediaries shouldn't cache one host's
		// manifest and serve it to another. Browsers may cache after
		// install — that's fine, manifests rarely change.
		w.Header().Set("Cache-Control", "no-cache")
		_ = json.NewEncoder(w).Encode(manifest)
	}
}

// hostFromRequest returns the host the client used to reach the
// server, honouring X-Forwarded-Host when set (e.g. by tailscale serve).
// Strips the port — manifest labels don't care about it.
func hostFromRequest(r *nethttp.Request) string {
	host := r.Host
	if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
		host = forwarded
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return host
}

// manifestLabelFor extracts a short disambiguation label from a host:
//
//   - "deviant.tail2ebd58.ts.net"  → "deviant"
//   - "ripped.tail2ebd58.ts.net"   → "ripped"
//   - "127.0.0.1"                   → ""  (no useful label)
//   - "100.66.105.59"               → ""
//   - "localhost"                   → ""
//   - "[::1]"                       → ""
//   - "single-segment-host"         → "single-segment-host"
//
// Returns the empty string when no meaningful label can be derived,
// in which case the caller falls back to the bare "BRUV" name.
func manifestLabelFor(host string) string {
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if host == "" || host == "localhost" {
		return ""
	}
	if net.ParseIP(host) != nil {
		return ""
	}
	if i := strings.Index(host, "."); i > 0 {
		return host[:i]
	}
	return host
}
