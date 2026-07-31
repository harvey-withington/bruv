// Shared HTTP client for resolvers and media download. One place owns the
// browser-plausible User-Agent, timeouts, and size caps so no resolver
// hand-rolls a fetch. The UA matters: several of these endpoints (Reddit
// most explicitly) throttle or block default library agents while serving
// browser strings happily — and RIPPED's residential IP does the rest.

package capture

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Browser-plausible UA. A stable, honest-looking desktop Chrome string —
// not rotated, not randomized: this is a personal capture tool, not a
// crawler, and consistency is friendlier to rate limiters than churn.
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

const (
	requestTimeout = 20 * time.Second
	// maxJSONBody bounds API responses — the largest legitimate payload
	// (a long Reddit comment listing) is well under 8 MB.
	maxJSONBody = 8 << 20
	// MaxMediaBytes bounds a single media download. Mirrors the clipper's
	// pragmatics: post images are a few MB, mp4s occasionally tens — 64 MB
	// keeps real posts while stopping a share from ballooning the vault.
	MaxMediaBytes = 64 << 20
)

// Client wraps http.Client with the capture defaults. The zero value is
// not usable — construct with NewClient.
type Client struct {
	http *http.Client
}

func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: requestTimeout}}
}

// NewClientWithHTTP builds a Client on a caller-supplied http.Client —
// the seam tests use to route fetches at fixtures instead of the network.
func NewClientWithHTTP(h *http.Client) *Client {
	return &Client{http: h}
}

func (c *Client) do(ctx context.Context, method, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	return c.http.Do(req)
}

// GetJSON fetches url and decodes the response body into v.
func (c *Client) GetJSON(ctx context.Context, url string, v any) error {
	res, err := c.do(ctx, http.MethodGet, url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", url, res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxJSONBody))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("GET %s: decode: %w", url, err)
	}
	return nil
}

// Exists reports whether url answers a HEAD with 200 — the YouTube
// maxresdefault existence probe. Errors count as "no": the caller's
// fallback URL is the safe choice either way.
func (c *Client) Exists(ctx context.Context, url string) bool {
	res, err := c.do(ctx, http.MethodHead, url)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode == http.StatusOK
}

// FinalURL follows redirects and returns where url actually lands —
// how opaque share links (redd.it, reddit.com/r/…/s/…) become real
// permalinks. The body is discarded; the UA rides every hop (the redirect
// hop itself can be bot-walled).
func (c *Client) FinalURL(ctx context.Context, url string) (string, error) {
	res, err := c.do(ctx, http.MethodGet, url)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	// Drain a bounded amount so the connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("GET %s: HTTP %d", url, res.StatusCode)
	}
	return res.Request.URL.String(), nil
}

// Download fetches a media URL into memory, returning the bytes and the
// response Content-Type. Enforces MaxMediaBytes — an oversized file is an
// error, and per the clipper's rule an individual media failure drops the
// item, never the clip.
func (c *Client) Download(ctx context.Context, url string) ([]byte, string, error) {
	res, err := c.do(ctx, http.MethodGet, url)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("GET %s: HTTP %d", url, res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, MaxMediaBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > MaxMediaBytes {
		return nil, "", fmt.Errorf("GET %s: media exceeds %d byte cap", url, int64(MaxMediaBytes))
	}
	return data, res.Header.Get("Content-Type"), nil
}
