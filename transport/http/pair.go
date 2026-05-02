package http

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"html/template"
	"net"
	nethttp "net/http"
	"os"
	"path/filepath"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// pairHandler serves a self-contained HTML page that renders a QR
// encoding the mobile EnrolPage URL with the bootstrap token baked in.
//
// Auth: query-string ?token=<bootstrap>, validated against the on-disk
// bootstrap token via constant-time compare. The bootstrap token is
// already the auth root for /auth/enrol, so reusing it here doesn't
// expand the trust surface — anyone who can read it from disk can do
// either operation.
//
// UX flow:
//
//   1. Operator boots bruv-server, sees a "Pair a phone" line with a
//      pre-filled URL in the startup logs.
//   2. Clicks the link in their terminal — desktop browser opens to
//      this handler, page renders the QR plus a copyable link.
//   3. Phone scans the QR (via camera/Lens), browser opens the mobile
//      EnrolPage with ?token= already populated, user taps Pair, done.
//
// The QR encodes <scheme>://<host>/m/enrol?token=<bootstrap>, where
// scheme + host are derived from the *incoming request*. Tailscale
// serve forwards the original Host + X-Forwarded-Proto, so the QR
// auto-targets whichever URL the operator used to reach this page —
// no --public-url flag, no per-deploy config.
func pairHandler(configDir string) nethttp.HandlerFunc {
	tmpl := template.Must(template.New("pair").Parse(pairPageTemplate))

	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method != nethttp.MethodGet {
			nethttp.Error(w, "method must be GET", nethttp.StatusMethodNotAllowed)
			return
		}

		provided := r.URL.Query().Get("token")
		if provided == "" {
			renderPairError(w, nethttp.StatusUnauthorized, "Missing ?token= query parameter. Append your bootstrap token from "+filepath.Join(configDir, "bootstrap-token.txt")+".")
			return
		}

		expected, err := readBootstrapToken(configDir)
		if err != nil {
			// Don't leak the raw filesystem error to the page — log it,
			// give the user a generic message.
			nethttp.Error(w, "pairing unavailable: server has no bootstrap token configured", nethttp.StatusServiceUnavailable)
			return
		}

		if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			renderPairError(w, nethttp.StatusUnauthorized, "The token in ?token= does not match this server's bootstrap token.")
			return
		}

		// Build the URL the QR will encode. Precedence:
		//
		//   1. Explicit ?host= and ?scheme= query params. Useful when
		//      the operator opens /pair via http://127.0.0.1:9870 (the
		//      only path that works from the same machine that hosts
		//      tailscale serve, since hairpin to your own tailnet IP
		//      doesn't work) but needs the QR to encode the public
		//      tailnet URL the phone will actually hit.
		//   2. X-Forwarded-Host / X-Forwarded-Proto, set by reverse
		//      proxies (tailscale serve sets these on the upstream).
		//   3. The request's own Host header / TLS state.
		//
		// In the common case — operator on phone or other tailnet
		// device hits the tailnet URL directly — (3) gives the right
		// answer without any params.
		host := firstNonEmpty(
			r.URL.Query().Get("host"),
			r.Header.Get("X-Forwarded-Host"),
			r.Host,
		)
		scheme := r.URL.Query().Get("scheme")
		if scheme == "" {
			scheme = r.Header.Get("X-Forwarded-Proto")
		}
		if scheme == "" {
			if r.TLS != nil {
				scheme = "https"
			} else {
				scheme = "http"
			}
		}
		enrolURL := fmt.Sprintf("%s://%s/m/enrol?token=%s", scheme, host, provided)

		png, err := qrcode.Encode(enrolURL, qrcode.Medium, 320)
		if err != nil {
			nethttp.Error(w, "QR generation failed: "+err.Error(), nethttp.StatusInternalServerError)
			return
		}
		qrDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// No cache — the bootstrap token can rotate; we don't want a
		// stale page lingering with an expired QR.
		w.Header().Set("Cache-Control", "no-store")
		// Strict referrer policy: the page URL contains the bootstrap
		// token. Don't leak it via Referer when the operator clicks
		// any link from this page (there are none today, but future-
		// proofing is cheap).
		w.Header().Set("Referrer-Policy", "no-referrer")

		_ = tmpl.Execute(w, pairPageData{
			EnrolURL: enrolURL,
			// template.URL signals to html/template that this is a
			// trusted URL and should bypass the default URL-context
			// filter (which would otherwise rewrite data: URLs to
			// "#ZgotmplZ" out of caution against javascript: payloads).
			QRDataURL: template.URL(qrDataURL),
			// IsLoopback drives the form's "set a phone-reachable
			// hostname" affordance. When the page is reached via
			// 127.0.0.1 (the only path that works from the same
			// machine that hosts tailscale serve), the QR encodes a
			// loopback URL that's useless to a phone — so the form is
			// shown prominently and the QR area carries a warning.
			IsLoopback:    isLoopbackHost(host),
			CurrentHost:   host,
			CurrentToken:  provided,
			CurrentScheme: scheme,
		})
	}
}

// firstNonEmpty returns the first argument that isn't the empty string,
// or "" if they all are. Used to pick a Host value from a precedence
// list of query param > forwarded header > request host.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// readBootstrapToken returns the trimmed contents of bootstrap-token.txt
// inside configDir. Missing file or empty contents both produce an
// error — the caller should treat these as "no bootstrap configured."
func readBootstrapToken(configDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(configDir, "bootstrap-token.txt"))
	if err != nil {
		return "", err
	}
	tok := strings.TrimSpace(string(data))
	if tok == "" {
		return "", os.ErrNotExist
	}
	return tok, nil
}

// renderPairError writes a tiny HTML response for failure cases. We
// don't reuse the main template — these are dead-end pages with no
// QR to render.
func renderPairError(w nethttp.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, errorPageTemplate, template.HTMLEscapeString(message))
}

type pairPageData struct {
	EnrolURL      string
	QRDataURL     template.URL
	IsLoopback    bool
	CurrentHost   string
	CurrentToken  string
	CurrentScheme string
}

// isLoopbackHost returns true when the given host (with optional port)
// is a loopback address — 127.0.0.1, localhost, or ::1. Handles bare
// hosts, host:port, and bracketed IPv6 [::1]:port.
func isLoopbackHost(host string) bool {
	// Try SplitHostPort first (handles 127.0.0.1:9870, [::1]:9870,
	// localhost:8080). Errors for the no-port forms (127.0.0.1, ::1,
	// localhost), in which case the original input is the host.
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

const pairPageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Pair a phone — BRUV</title>
<style>
  :root {
    color-scheme: dark;
    --bg: #18181b;
    --bg-elev: #27272a;
    --text: #fafafa;
    --text-muted: #a1a1aa;
    --text-faint: #71717a;
    --accent: #f59e0b;
    --border: #3f3f46;
    --warn-bg: rgba(245, 158, 11, 0.12);
    --warn-border: rgba(245, 158, 11, 0.5);
  }
  * { box-sizing: border-box; }
  html, body {
    margin: 0; padding: 0; min-height: 100vh;
    background: var(--bg); color: var(--text);
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  }
  main {
    max-width: 480px; margin: 0 auto; padding: 2.5rem 1.5rem;
  }
  h1 { margin: 0 0 0.5rem; font-size: 1.5rem; }
  .subtitle { margin: 0 0 1.75rem; color: var(--text-muted); font-size: 0.9rem; line-height: 1.5; }
  .host-form {
    background: var(--bg-elev); border: 1px solid var(--border);
    border-radius: 8px; padding: 0.85rem 1rem; margin-bottom: 1.25rem;
  }
  .host-form.warn { border-color: var(--warn-border); background: var(--warn-bg); }
  .host-form label {
    display: block; font-size: 0.78rem; font-weight: 600;
    color: var(--text); margin-bottom: 0.4rem;
  }
  .host-form .row {
    display: flex; gap: 0.5rem;
  }
  .host-form input {
    flex: 1; min-width: 0; padding: 0.5rem 0.65rem;
    background: var(--bg); border: 1px solid var(--border); border-radius: 6px;
    color: var(--text); font: inherit; font-size: 0.85rem; outline: none;
  }
  .host-form input:focus { border-color: var(--accent); }
  .host-form button {
    background: var(--accent); border: none; color: #18181b;
    padding: 0.5rem 0.85rem; border-radius: 6px; font-weight: 600;
    font-size: 0.8rem; cursor: pointer; flex-shrink: 0;
  }
  .host-form button:hover { filter: brightness(1.1); }
  .host-form .hint {
    margin: 0.5rem 0 0; color: var(--text-muted); font-size: 0.75rem; line-height: 1.4;
  }
  .qr-frame {
    background: #fff; padding: 1rem; border-radius: 12px; width: 100%;
    aspect-ratio: 1 / 1; display: flex; align-items: center; justify-content: center;
    margin-bottom: 1.25rem;
  }
  .qr-frame img { width: 100%; height: 100%; image-rendering: pixelated; }
  .qr-frame.warn { outline: 3px solid var(--warn-border); outline-offset: -3px; }
  .url-row {
    display: flex; align-items: center; gap: 0.5rem;
    background: var(--bg-elev); border: 1px solid var(--border);
    border-radius: 8px; padding: 0.6rem 0.75rem; margin-bottom: 1rem;
  }
  .url-label {
    font-size: 0.7rem; font-weight: 600; color: var(--text-muted);
    text-transform: uppercase; letter-spacing: 0.04em; flex-shrink: 0;
  }
  .url-value {
    flex: 1; min-width: 0; font-size: 0.8rem; color: var(--text);
    overflow-x: auto; white-space: nowrap; font-family: ui-monospace, monospace;
  }
  .copy-btn {
    background: var(--accent); border: none; color: #18181b;
    padding: 0.4rem 0.75rem; border-radius: 6px; font-weight: 600;
    font-size: 0.8rem; cursor: pointer; flex-shrink: 0;
  }
  .copy-btn:hover { filter: brightness(1.1); }
  .copy-btn.copied { background: #22c55e; }
  .footnote {
    margin-top: 2rem; color: var(--text-faint); font-size: 0.8rem;
    line-height: 1.5;
  }
  .footnote ol { padding-left: 1.25rem; margin: 0.5rem 0 0; }
  .footnote li { margin-bottom: 0.25rem; }
</style>
</head>
<body>
<main>
  <h1>Pair a phone with BRUV</h1>
  <p class="subtitle">Scan this QR with your phone's camera. The link opens BRUV mobile with the bootstrap token already filled in — just tap Pair.</p>

  <form class="host-form{{if .IsLoopback}} warn{{end}}" id="host-form">
    <label for="host-input">Phone-reachable hostname</label>
    <div class="row">
      <input
        id="host-input"
        type="text"
        value="{{.CurrentHost}}"
        placeholder="your-machine.tail-XXXX.ts.net"
        autocomplete="off"
        spellcheck="false"
      />
      <button type="submit">Update QR</button>
    </div>
    <p class="hint">
      {{if .IsLoopback -}}
        The QR currently encodes <strong>{{.CurrentHost}}</strong>, which your phone can't reach. Enter your Tailscale hostname (just the host — no <code>:port</code> or <code>https://</code>) and click Update QR. Saved for next time.
      {{- else -}}
        Set this if the QR's hostname doesn't match how your phone reaches the server. Saved for next time.
      {{- end}}
    </p>
  </form>

  <div class="qr-frame{{if .IsLoopback}} warn{{end}}">
    <img src="{{.QRDataURL}}" alt="Pairing QR code" />
  </div>

  <div class="url-row">
    <span class="url-label">Link</span>
    <span class="url-value" id="enrol-url">{{.EnrolURL}}</span>
    <button class="copy-btn" id="copy-btn" type="button">Copy</button>
  </div>

  <div class="footnote">
    <strong>Notes</strong>
    <ol>
      <li>Your phone must be signed in to the same Tailscale network.</li>
      <li>The bootstrap token rotates when the server is reset — re-open the pairing link from the server logs after that.</li>
      <li>Already paired? Re-pairing creates a new device entry rather than replacing the old one.</li>
    </ol>
  </div>
</main>

<script>
  const enrolURL = {{.EnrolURL}};
  const token = {{.CurrentToken}};
  const HOST_KEY = 'bruv:pair_host';
  const SCHEME_KEY = 'bruv:pair_scheme';

  // Saved-host redirect: if the URL has no ?host= and we saved one
  // previously, replace the URL so the QR uses the saved host without
  // the operator having to re-type. Skipped when the URL explicitly
  // sets host (operator's intent wins) or when the saved host matches
  // what we'd already render with.
  (function applySavedHost() {
    const urlParams = new URLSearchParams(window.location.search);
    if (urlParams.get('host')) return; // explicit override in URL
    const savedHost = localStorage.getItem(HOST_KEY);
    if (!savedHost) return;
    const savedScheme = localStorage.getItem(SCHEME_KEY) || 'https';
    urlParams.set('host', savedHost);
    urlParams.set('scheme', savedScheme);
    window.location.replace(window.location.pathname + '?' + urlParams.toString());
  })();

  // Normalise whatever the operator types into a bare hostname:
  //   - "deviant.ts.net"             → "deviant.ts.net"
  //   - "deviant.ts.net:9870"        → "deviant.ts.net"  (port stripped)
  //   - "https://deviant.ts.net/"    → "deviant.ts.net"  (scheme + path stripped)
  //   - "deviant.ts.net:443/m/enrol" → "deviant.ts.net"
  // The phone-reachable URL is on tailscale serve's port (443 by
  // default) — never on whatever localhost port BRUV bound. Stripping
  // ports avoids the obvious paste-the-whole-URL mistake.
  function normaliseHost(raw) {
    const v = raw.trim();
    if (!v) return '';
    try {
      const u = new URL(v.includes('://') ? v : 'https://' + v);
      return u.hostname;
    } catch {
      return v.replace(/:\d+.*$/, '');
    }
  }

  // Hostname form: persist + reload with new params.
  document.getElementById('host-form').addEventListener('submit', (e) => {
    e.preventDefault();
    const input = document.getElementById('host-input');
    const host = normaliseHost(input.value);
    if (!host) return;
    // Reflect the normalised value so the operator sees what was saved.
    input.value = host;
    // Default to https — the only scheme that makes sense for a
    // phone-reachable URL (PWA install + service workers require it).
    const scheme = 'https';
    localStorage.setItem(HOST_KEY, host);
    localStorage.setItem(SCHEME_KEY, scheme);
    const params = new URLSearchParams();
    params.set('token', token);
    params.set('host', host);
    params.set('scheme', scheme);
    window.location.href = window.location.pathname + '?' + params.toString();
  });

  // Copy button.
  const btn = document.getElementById('copy-btn');
  btn.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(enrolURL);
      btn.textContent = 'Copied';
      btn.classList.add('copied');
      setTimeout(() => { btn.textContent = 'Copy'; btn.classList.remove('copied'); }, 1500);
    } catch (e) {
      btn.textContent = 'Copy failed';
    }
  });
</script>
</body>
</html>
`

const errorPageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Pair a phone — BRUV</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; background: #18181b; color: #fafafa; padding: 2rem; max-width: 480px; margin: 0 auto; }
  h1 { font-size: 1.25rem; }
  .err { padding: 1rem; background: rgba(239, 68, 68, 0.15); color: #fca5a5; border: 1px solid rgba(239, 68, 68, 0.4); border-radius: 8px; }
</style>
</head>
<body>
<h1>Can't show the pairing QR</h1>
<p class="err">%s</p>
</body>
</html>
`
