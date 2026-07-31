// Package capture resolves a social-post URL into a structured Clip —
// the server-side half of the genericity contract established by the web
// clipper (clipper/src/lib/types.ts). Exactly ONE artifact knows a
// platform: the resolver. Everything downstream (supervisor ingest, card
// mapping, deck append) consumes only Clip and stays platform-blind.
//
// Resolvers are ports of the clipper plugins' enrich() halves — the pure
// URL/ID → JSON-API lookups — widened to read the fields the extension
// got from the DOM (the same API responses carry full text/author/media;
// the extension's enrich() only topped up what its DOM pass lacked).
// The DOM halves (resolveCaptureUnit/extract) remain extension-only.
//
// A resolver error never kills a capture: the supervisor's ingest
// degrades to a pending clip (url-only card, completed later from a real
// browser via the extension). Resolvers therefore return honest errors
// rather than best-effort empties — the ladder is the caller's job.
package capture

import (
	"context"
	"strings"
)

// MediaKind mirrors ClipMediaKind in clipper/src/lib/types.ts.
type MediaKind string

const (
	MediaImage MediaKind = "image"
	MediaVideo MediaKind = "video"
)

// Media is one downloadable media reference on a clip. PosterURL carries a
// video's preview frame — the degrade target when the video itself can't
// be fetched, and the attachment thumbnail source.
type Media struct {
	URL       string    `json:"url"`
	Kind      MediaKind `json:"kind"`
	PosterURL string    `json:"posterUrl,omitempty"`
}

// EmbedVideo references playback via the platform's official player for
// sources whose streams can't be downloaded (YouTube). Becomes the slide's
// video value as "embed://<provider>/<id>"; never downloaded.
type EmbedVideo struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

// Clip is the Go mirror of ClipResult (clipper/src/lib/types.ts) — json
// tags match exactly so a ClipResult posted by the extension (the
// CompleteCapture path) decodes straight into it.
type Clip struct {
	// Resolver id, e.g. "twitter". Becomes the card tag; never branches logic.
	Platform string `json:"platform"`
	// Permalink of the captured unit (a reply's own status URL, not the page's).
	CanonicalURL string      `json:"canonicalUrl"`
	Author       string      `json:"author"`
	Handle       string      `json:"handle,omitempty"`
	AvatarURL    string      `json:"avatarUrl,omitempty"`
	Text         string      `json:"text"`
	Media        []Media     `json:"media"`
	PublishedAt  string      `json:"publishedAt,omitempty"`
	EmbedVideo   *EmbedVideo `json:"embedVideo,omitempty"`
	// Platform-specific extras the generic pipeline stores but never reads.
	Extras map[string]string `json:"extras,omitempty"`
}

// Resolver turns a URL into a Clip using public JSON endpoints only —
// no HTML scraping (JS-shell platforms render nothing without a browser),
// no credentials (auth stays in the user's browser; the completion flow
// covers what anonymous endpoints can't reach).
type Resolver interface {
	// ID is the platform id — the clip tag and the template urlHint key.
	ID() string
	// Matches reports whether rawURL belongs to this platform. Patterns
	// mirror the clipper plugins' matchesUrl.
	Matches(rawURL string) bool
	// Resolve fetches and maps the post. Implementations degrade
	// internally where the clipper's enrich() did (a missing video falls
	// back to its poster) but return an error when the post itself is
	// unreachable — the pending ladder handles that.
	Resolve(ctx context.Context, c *Client, rawURL string) (*Clip, error)
}

// resolvers in clipper registry order (registry.ts PLUGINS).
var resolvers = []Resolver{
	twitterResolver{},
	truthsocialResolver{},
	redditResolver{},
	youtubeResolver{},
}

// ResolverFor returns the resolver claiming rawURL, or nil.
func ResolverFor(rawURL string) Resolver {
	rawURL = strings.TrimSpace(rawURL)
	for _, r := range resolvers {
		if r.Matches(rawURL) {
			return r
		}
	}
	return nil
}

// Match returns the platform id claiming rawURL, or "" — the cheap
// preflight behind the MatchCaptureURL RPC.
func Match(rawURL string) string {
	if r := ResolverFor(rawURL); r != nil {
		return r.ID()
	}
	return ""
}

// Resolve is the package entry point: match + resolve in one call.
// Returns (nil, nil) when no resolver claims the URL — "not capturable"
// is a normal answer, not an error.
func Resolve(ctx context.Context, c *Client, rawURL string) (*Clip, error) {
	r := ResolverFor(rawURL)
	if r == nil {
		return nil, nil
	}
	return r.Resolve(ctx, c, rawURL)
}

// EmbedForURL derives an embed reference from the URL alone, without any
// network fetch — possible only where the platform's video id is part of
// the URL (YouTube). This is what lets a PENDING YouTube clip carry a
// playable embed immediately: bot walls can block resolution, but they
// can't hide the id sitting in the share link.
func EmbedForURL(rawURL string) *EmbedVideo {
	if !(youtubeResolver{}).Matches(rawURL) {
		return nil
	}
	if id := videoIDFromURL(rawURL); id != "" {
		return &EmbedVideo{Provider: "youtube", ID: id}
	}
	return nil
}
