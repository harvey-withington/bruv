package http

import (
	"encoding/hex"
	"fmt"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Attachments are served from the origin the mobile PWA keeps its device
// token on. A captured/imported/AI-added HTML or SVG file opened directly
// must not run script with that origin's access, and nothing may be
// sniffed up to HTML.
func TestAttachmentSecurityHeaders(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"page.html":  "text/html",
		"logo.svg":   "image/svg+xml",
		"photo.jpg":  "image/jpeg",
		"report.pdf": "application/pdf",
	}
	for name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	secret := []byte("test-secret")
	h := attachmentHandler(&AttachmentConfig{
		Secret: secret,
		Resolve: func(_, attID string) (string, string, string, bool) {
			mime, ok := files[attID]
			return filepath.Join(dir, attID), mime, attID, ok
		},
	})

	get := func(att string) *httptest.ResponseRecorder {
		exp := time.Now().Add(time.Minute).Unix()
		sig := hex.EncodeToString(SignAttachmentMAC(secret, "card", att, exp))
		req := httptest.NewRequest(nethttp.MethodGet, fmt.Sprintf("/attachments/card/%s?exp=%d&sig=%s", att, exp, sig), nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != nethttp.StatusOK {
			t.Fatalf("%s: status %d", att, rec.Code)
		}
		return rec
	}

	for att := range files {
		if got := get(att).Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options = %q, want nosniff", att, got)
		}
	}
	for _, att := range []string{"page.html", "logo.svg"} {
		if got := get(att).Header().Get("Content-Security-Policy"); got != "sandbox allow-scripts" {
			t.Errorf("%s: CSP = %q, want an opaque-origin sandbox", att, got)
		}
	}
	// Images and PDFs render normally — a sandbox would break Chrome's
	// PDF viewer and gains nothing for passive types.
	for _, att := range []string{"photo.jpg", "report.pdf"} {
		if got := get(att).Header().Get("Content-Security-Policy"); got != "" {
			t.Errorf("%s: unexpected CSP %q", att, got)
		}
	}
}
