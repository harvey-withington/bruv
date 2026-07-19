package http

// Present surface — the read-only slide-deck output page for OBS Browser
// Sources / fullscreen displays. Two routes, both mounted only when a host
// wires PresentConfig:
//
//	GET /present/<repo>/<cardID>?exp=&sig=[&bg=transparent]
//	    the self-contained output page (static HTML; the page itself reads
//	    its URL and calls the data route below).
//	GET /present-data/<repo>/<cardID>?exp=&sig=
//	    the card JSON, gated by an HMAC-signed URL — OBS can't send an
//	    Authorization header, so the signature IS the auth (same approach as
//	    signed attachment URLs). Long expiry vs. the attachments' ~5 min.
//
// Live control rides the existing data path: the presenter console bumps the
// deck's currentIndex via UpdateCardBlocks; the page picks the change up by
// polling this endpoint. (An SSE upgrade is a tracked follow-up.)

import (
	"crypto/hmac"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	nethttp "net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed present.html
var presentPage []byte

// PresentConfig wires the present routes. Secret is the same per-server HMAC
// key used for attachment URLs. ResolveCardJSON returns the raw card JSON for
// (repoID, cardID), or ok=false when the card is missing. Leave the whole
// config nil to skip mounting the present surface (headless/no-UI builds).
type PresentConfig struct {
	Secret          []byte
	ResolveCardJSON func(repoID, cardID string) (json []byte, ok bool)
}

// SignPresentMAC computes the HMAC-SHA256 tag for a present URL's parameters.
// Exposed so the host (desktop App / presenter console) can mint matching
// signatures when it builds the "Present" URL.
func SignPresentMAC(secret []byte, repoID, cardID string, exp int64) []byte {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "present|%s|%s|%d", repoID, cardID, exp)
	return mac.Sum(nil)
}

// parsePresentPath extracts (repoID, cardID) from a "<repo>/<cardID>" tail.
func parsePresentPath(tail string) (repoID, cardID string, ok bool) {
	segments := strings.Split(tail, "/")
	if len(segments) != 2 || segments[0] == "" || segments[1] == "" {
		return "", "", false
	}
	return segments[0], segments[1], true
}

// verifyPresentSig checks the exp + sig query parameters against the secret.
// Returns an HTTP status + message on failure, or 0 on success.
func verifyPresentSig(cfg *PresentConfig, r *nethttp.Request, repoID, cardID string) (int, string) {
	expStr := r.URL.Query().Get("exp")
	sigHex := r.URL.Query().Get("sig")
	if expStr == "" || sigHex == "" {
		return nethttp.StatusUnauthorized, "missing exp or sig"
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return nethttp.StatusUnauthorized, "bad exp"
	}
	if time.Now().Unix() > exp {
		return nethttp.StatusUnauthorized, "url expired"
	}
	providedSig, err := hex.DecodeString(sigHex)
	if err != nil {
		return nethttp.StatusUnauthorized, "bad sig"
	}
	expectedSig := SignPresentMAC(cfg.Secret, repoID, cardID, exp)
	if !hmac.Equal(expectedSig, providedSig) {
		return nethttp.StatusUnauthorized, "bad sig"
	}
	return 0, ""
}

// presentPageHandler serves the static output page for any /present/<...>
// path. The shell is public (like /m/*); the sig-gated data route is what
// protects the card content.
func presentPageHandler() nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method != nethttp.MethodGet {
			nethttp.Error(w, "method must be GET", nethttp.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(presentPage)
	}
}

// presentDataHandler serves the signed card JSON for /present-data/<repo>/<cardID>.
func presentDataHandler(cfg *PresentConfig) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method != nethttp.MethodGet {
			nethttp.Error(w, "method must be GET", nethttp.StatusMethodNotAllowed)
			return
		}
		tail := strings.TrimPrefix(r.URL.Path, "/present-data/")
		repoID, cardID, ok := parsePresentPath(tail)
		if !ok {
			nethttp.Error(w, "bad present path", nethttp.StatusBadRequest)
			return
		}
		if status, msg := verifyPresentSig(cfg, r, repoID, cardID); status != 0 {
			nethttp.Error(w, msg, status)
			return
		}
		data, found := cfg.ResolveCardJSON(repoID, cardID)
		if !found {
			nethttp.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Short cache — the page polls for currentIndex changes, so let a
		// proxy hold a copy for at most a second.
		w.Header().Set("Cache-Control", "private, max-age=1")
		_, _ = w.Write(data)
	}
}
