package http

import (
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUncliGuard: who reaches the server's UNCLI, which can run tools on
// the machine. Only a device token in the cookie or a bearer header gets
// through (never ?token=), from this server's own origin; UNCLI's signed
// links pass without one; BRUV's cookie and the Origin aren't forwarded.
func TestUncliGuard(t *testing.T) {
	dir := t.TempDir()
	store, token := enrolHelper(t, dir)
	bootstrap, err := os.ReadFile(filepath.Join(dir, "bootstrap-token.txt"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		path   string
		cookie string
		bearer string
		origin string
		remote string
		want   int
	}{
		{name: "no credentials", path: "/uncli/rpc", want: 401},
		{name: "token in the query", path: "/uncli/rpc?token=" + token, want: 401},
		{name: "cookie", path: "/uncli/rpc", cookie: token, want: 200},
		{name: "bearer", path: "/uncli/rpc", bearer: token, want: 200},
		{name: "bootstrap token", path: "/uncli/rpc", cookie: strings.TrimSpace(string(bootstrap)), want: 401},
		{name: "unknown token", path: "/uncli/rpc", cookie: "nope", want: 401},
		{name: "signed link", path: "/uncli/l/artifact?sig=x", want: 200},
		{name: "same origin", path: "/uncli/rpc", cookie: token, origin: "https://bruv.example", want: 200},
		{name: "other origin", path: "/uncli/rpc", cookie: token, origin: "https://evil.example", want: 403},
		{name: "other origin, signed link", path: "/uncli/l/artifact", origin: "https://evil.example", want: 403},
		{name: "dev server via loopback", path: "/uncli/rpc", cookie: token, origin: "http://localhost:5174", remote: "127.0.0.1:50000", want: 200},
		{name: "loopback origin from afar", path: "/uncli/rpc", cookie: token, origin: "http://localhost:5174", remote: "100.64.0.9:50000", want: 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var forwarded *nethttp.Request
			h := uncliGuard(store, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
				forwarded = r
			}))
			req := httptest.NewRequest(nethttp.MethodPost, "https://bruv.example"+tc.path, nil)
			if tc.cookie != "" {
				req.AddCookie(&nethttp.Cookie{Name: uncliCookie, Value: tc.cookie})
			}
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.remote != "" {
				req.RemoteAddr = tc.remote
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
			if tc.want != 200 {
				return
			}
			if forwarded.Header.Get("Origin") != "" || forwarded.Header.Get("Cookie") != "" {
				t.Errorf("forwarded Origin %q / Cookie %q to the core", forwarded.Header.Get("Origin"), forwarded.Header.Get("Cookie"))
			}
		})
	}
}

// The cookie the page swaps its token for: the device token, HttpOnly,
// SameSite=Strict, and only sent to /uncli.
func TestUncliCookieHandler(t *testing.T) {
	req := httptest.NewRequest(nethttp.MethodPost, "/auth/uncli-cookie", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	uncliCookieHandler().ServeHTTP(rec, req)
	if rec.Code != nethttp.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %v", cookies)
	}
	c := cookies[0]
	if c.Name != uncliCookie || c.Value != "tok" || c.Path != "/uncli" || !c.HttpOnly || c.SameSite != nethttp.SameSiteStrictMode {
		t.Errorf("cookie = %+v", c)
	}

	get := httptest.NewRecorder()
	uncliCookieHandler().ServeHTTP(get, httptest.NewRequest(nethttp.MethodGet, "/auth/uncli-cookie", nil))
	if get.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want 405", get.Code)
	}
}
