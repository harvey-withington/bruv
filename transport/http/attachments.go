package http

// Signed-URL attachment download handler.
//
// Path shape: /attachments/<cardID>/<attachmentID>?exp=<unix>&sig=<hex>
//
// sig = HMAC-SHA256(secret, "<cardID>|<attachmentID>|<exp>")
//
// Reasons for HMAC instead of a bearer header:
//
//   - <img src="..."> doesn't attach Authorization headers, so
//     embedding attachments anywhere in the UI requires a URL the
//     browser will fetch without ceremony.
//   - The bearer token is a long-lived device credential; baking it
//     into URLs (which get logged, screenshotted, copy/pasted) is
//     a much worse leak than a 5-minute-window HMAC.
//
// Expiry is verified server-side; the server's wall clock is the
// authority, the URL just states what the client thinks the bound is.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	nethttp "net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

func attachmentHandler(cfg *AttachmentConfig) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method != nethttp.MethodGet && r.Method != nethttp.MethodHead {
			nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
			return
		}

		// /attachments/<cardID>/<attachmentID>
		trimmed := strings.TrimPrefix(r.URL.Path, "/attachments/")
		segments := strings.Split(trimmed, "/")
		if len(segments) != 2 || segments[0] == "" || segments[1] == "" {
			nethttp.Error(w, "bad attachment path", nethttp.StatusBadRequest)
			return
		}
		cardID := segments[0]
		attachmentID := segments[1]

		expStr := r.URL.Query().Get("exp")
		sigHex := r.URL.Query().Get("sig")
		if expStr == "" || sigHex == "" {
			nethttp.Error(w, "missing exp or sig", nethttp.StatusUnauthorized)
			return
		}
		exp, err := strconv.ParseInt(expStr, 10, 64)
		if err != nil {
			nethttp.Error(w, "bad exp", nethttp.StatusUnauthorized)
			return
		}
		if time.Now().Unix() > exp {
			nethttp.Error(w, "url expired", nethttp.StatusUnauthorized)
			return
		}
		expectedSig := SignAttachmentMAC(cfg.Secret, cardID, attachmentID, exp)
		providedSig, err := hex.DecodeString(sigHex)
		if err != nil || !hmac.Equal(expectedSig, providedSig) {
			nethttp.Error(w, "bad sig", nethttp.StatusUnauthorized)
			return
		}

		filePath, mime, name, ok := cfg.Resolve(cardID, attachmentID)
		if !ok {
			nethttp.NotFound(w, r)
			return
		}
		f, err := os.Open(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				nethttp.NotFound(w, r)
				return
			}
			nethttp.Error(w, "open failed", nethttp.StatusInternalServerError)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			nethttp.Error(w, "stat failed", nethttp.StatusInternalServerError)
			return
		}
		if mime == "" {
			mime = "application/octet-stream"
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
		// Inline by default — most attachments are images the UI wants
		// to render in place. Browsers ignore the filename for inline
		// disposition; downloads still get the right name via <a download>.
		if name != "" {
			w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", path.Base(name)))
		}
		// 5-min URL TTL also bounds cache lifetime — once the URL
		// expires it can't be re-fetched anyway, so cache it for the
		// short window remaining.
		ttl := exp - time.Now().Unix()
		if ttl > 0 {
			w.Header().Set("Cache-Control", fmt.Sprintf("private, max-age=%d", ttl))
		}
		if r.Method == nethttp.MethodHead {
			return
		}
		_, _ = io.Copy(w, f)
	})
}

// SignAttachmentMAC computes the HMAC-SHA256 tag for the given
// attachment URL parameters. Exposed so the desktop App can
// generate matching signatures via app.SignAttachmentURL.
func SignAttachmentMAC(secret []byte, cardID, attachmentID string, exp int64) []byte {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s|%s|%d", cardID, attachmentID, exp)
	return mac.Sum(nil)
}
