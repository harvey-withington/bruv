// YouTube resolver — the ONLY file on the server side that knows what
// YouTube is. Port of clipper/src/lib/plugins/youtube.ts: the capture
// unit is a VIDEO, not a "post" — playback is ciphered MSE/DASH with no
// downloadable URL, so this resolver never attempts one. It records an
// embedVideo reference (the deck renders YouTube's own iframe player at
// Present time) and the thumbnail is the only media it ever produces.
//
// oEmbed (youtube.com/oembed) is public — no key, no auth — and returns
// title / channel name / channel URL for any video id. The channel avatar
// is the one field it lacks; server captures go without it rather than
// guessing (matching the extension's thumbnail-capture behaviour).

package capture

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

type youtubeResolver struct{}

func (youtubeResolver) ID() string { return "youtube" }

var youtubeURLRe = regexp.MustCompile(`^https?://([a-z0-9-]+\.)?(youtube\.com|youtu\.be)/`)

func (youtubeResolver) Matches(rawURL string) bool {
	return youtubeURLRe.MatchString(rawURL)
}

var shortsPathRe = regexp.MustCompile(`/shorts/([a-zA-Z0-9_-]+)`)

// videoIDFromURL ports videoIdFromHref for the three URL shapes:
// watch?v=<id>, /shorts/<id>, youtu.be/<id>.
func videoIDFromURL(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	if v := u.Query().Get("v"); v != "" {
		return v
	}
	if m := shortsPathRe.FindStringSubmatch(u.Path); m != nil {
		return m[1]
	}
	if strings.HasSuffix(u.Hostname(), "youtu.be") {
		if id := strings.TrimPrefix(u.Path, "/"); id != "" {
			return id
		}
	}
	return ""
}

var channelHandleRe = regexp.MustCompile(`/(@[A-Za-z0-9_.-]+)`)

func (r youtubeResolver) Resolve(ctx context.Context, c *Client, rawURL string) (*Clip, error) {
	id := videoIDFromURL(rawURL)
	if id == "" {
		return nil, fmt.Errorf("youtube: no video id in %s", rawURL)
	}
	canonical := "https://www.youtube.com/watch?v=" + id

	// oEmbed is authoritative for title/channel — with no DOM fallback on
	// the server, its failure is the resolver's failure (pending ladder).
	var o struct {
		Title      string `json:"title"`
		AuthorName string `json:"author_name"`
		AuthorURL  string `json:"author_url"`
	}
	endpoint := "https://www.youtube.com/oembed?url=" + url.QueryEscape(canonical) + "&format=json"
	if err := c.GetJSON(ctx, endpoint, &o); err != nil {
		return nil, fmt.Errorf("youtube oembed: %w", err)
	}

	handle := ""
	if m := channelHandleRe.FindStringSubmatch(o.AuthorURL); m != nil {
		handle = m[1]
	}

	// Thumbnail: i.ytimg.com URLs are stable and unauthenticated, but
	// maxresdefault 404s for uploads that never got a high-res thumbnail —
	// probe it and downgrade to hqdefault (generated for effectively every
	// upload). Known caveat carried over from the extension: YouTube
	// sometimes serves a tiny grey placeholder with a 200 for missing
	// maxres; the rare slip-through is accepted over dimension checks.
	thumb := fmt.Sprintf("https://i.ytimg.com/vi/%s/maxresdefault.jpg", id)
	if !c.Exists(ctx, thumb) {
		thumb = fmt.Sprintf("https://i.ytimg.com/vi/%s/hqdefault.jpg", id)
	}

	// Field mapping mirrors the extension: the slide schema is a
	// social-post shape, so text = video title, author = channel name.
	return &Clip{
		Platform:     "youtube",
		CanonicalURL: canonical,
		Author:       o.AuthorName,
		Handle:       handle,
		Text:         o.Title,
		Media:        []Media{{URL: thumb, Kind: MediaImage}},
		// No downloadable stream — the deck resolves this to
		// "embed://youtube/<id>" and renders the official iframe player.
		EmbedVideo: &EmbedVideo{Provider: "youtube", ID: id},
		Extras:     map[string]string{"videoId": id},
	}, nil
}
