// Reddit resolver — the ONLY file on the server side that knows what
// Reddit is. Port of clipper/src/lib/plugins/reddit.ts's enrich() half
// plus the URL work the extension never needed: the Reddit mobile app
// shares opaque short links (reddit.com/r/<sub>/s/<token>, redd.it/<id>)
// that only a redirect follow can turn into a real permalink — and that
// is the COMMON mobile case, not an edge case.
//
// The public JSON API (<permalink>.json?raw_json=1) is authoritative and
// far simpler than the DOM; raw_json=1 stops Reddit HTML-entity-escaping
// URLs so &amp; never shows up in media links.

package capture

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

type redditResolver struct{}

func (redditResolver) ID() string { return "reddit" }

var redditURLRe = regexp.MustCompile(`^https?://([a-z0-9-]+\.)?(reddit\.com|redd\.it)/`)

func (redditResolver) Matches(rawURL string) bool {
	return redditURLRe.MatchString(rawURL)
}

// opaque share-link shapes that need a redirect follow before the JSON
// API can be used.
var redditShareRe = regexp.MustCompile(`^https?://([a-z0-9-]+\.)?reddit\.com/r/[^/]+/s/`)

func redditIsOpaque(rawURL string) bool {
	return strings.Contains(rawURL, "redd.it/") || redditShareRe.MatchString(rawURL)
}

// --- Reddit's public JSON API shape (only the fields we read) -------------

type redditPost struct {
	Title         string `json:"title"`
	Selftext      string `json:"selftext"`
	Author        string `json:"author"`
	SubredditName string `json:"subreddit_name_prefixed"`
	CreatedUTC    float64 `json:"created_utc"`
	Permalink     string `json:"permalink"`
	IsGallery     bool   `json:"is_gallery"`
	Preview       *struct {
		Images []struct {
			Source *struct {
				URL string `json:"url"`
			} `json:"source"`
		} `json:"images"`
	} `json:"preview"`
	// Gallery images keyed by media ID; each entry's s.u is the full-size
	// image URL once raw_json=1 has de-escaped it. gallery_data carries the
	// author's ordering — something the extension (iterating JS object
	// keys) only got by accident of insertion order.
	MediaMetadata map[string]struct {
		S *struct {
			U string `json:"u"`
		} `json:"s"`
	} `json:"media_metadata"`
	GalleryData *struct {
		Items []struct {
			MediaID string `json:"media_id"`
		} `json:"items"`
	} `json:"gallery_data"`
	SecureMedia *struct {
		RedditVideo *struct {
			FallbackURL string `json:"fallback_url"`
		} `json:"reddit_video"`
	} `json:"secure_media"`
}

type redditListing struct {
	Data *struct {
		Children []struct {
			Data *redditPost `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

// Whole-gallery cap, mirroring reddit.ts — bounds job size well above
// typical posts. Slides render multi-image media as a carousel.
const redditGalleryCap = 12

func (r redditResolver) Resolve(ctx context.Context, c *Client, rawURL string) (*Clip, error) {
	// Opaque share links resolve to their real permalink first; the UA
	// rides the redirect hop too (the hop itself can be bot-walled).
	resolved := rawURL
	if redditIsOpaque(rawURL) {
		final, err := c.FinalURL(ctx, rawURL)
		if err != nil {
			return nil, fmt.Errorf("reddit share link: %w", err)
		}
		resolved = final
	}

	u, err := url.Parse(resolved)
	if err != nil {
		return nil, fmt.Errorf("reddit: %w", err)
	}
	permalink := strings.TrimSuffix(u.Path, "/")
	if permalink == "" {
		return nil, fmt.Errorf("reddit: no permalink in %s", resolved)
	}

	var listings []redditListing
	endpoint := fmt.Sprintf("https://www.reddit.com%s.json?raw_json=1", permalink)
	if err := c.GetJSON(ctx, endpoint, &listings); err != nil {
		return nil, fmt.Errorf("reddit api: %w", err)
	}
	if len(listings) == 0 || listings[0].Data == nil || len(listings[0].Data.Children) == 0 ||
		listings[0].Data.Children[0].Data == nil {
		return nil, fmt.Errorf("reddit api: no post data at %s", permalink)
	}
	post := listings[0].Data.Children[0].Data

	// Slides are for the headline; the card keeps the link to the full
	// post, so a long selftext only needs a preview, not the whole thing.
	text := post.Title
	if post.Selftext != "" {
		preview := post.Selftext
		if len(preview) > 500 {
			preview = truncateUTF8(preview, 500) + "…"
		}
		text = post.Title + "\n\n" + preview
	}

	clip := &Clip{
		Platform:     "reddit",
		CanonicalURL: "https://www.reddit.com" + post.Permalink,
		Author:       "u/" + post.Author,
		Handle:       post.SubredditName,
		// Reddit doesn't surface a reliable author avatar alongside the
		// post (separate hover-card fetch) — leave unset rather than
		// guess, mirroring the extension.
		Text:   text,
		Extras: map[string]string{"permalink": post.Permalink},
	}
	if post.CreatedUTC > 0 {
		clip.PublishedAt = time.Unix(int64(post.CreatedUTC), 0).UTC().Format(time.RFC3339)
	}

	switch {
	case post.SecureMedia != nil && post.SecureMedia.RedditVideo != nil && post.SecureMedia.RedditVideo.FallbackURL != "":
		// fallback_url is the video-only DASH track (no sound). Accepted
		// trade-off — muted video beats no video — with the poster carried
		// for players/thumbnails that can't play it.
		m := Media{URL: post.SecureMedia.RedditVideo.FallbackURL, Kind: MediaVideo}
		if p := post.previewImage(); p != "" {
			m.PosterURL = p
		}
		clip.Media = append(clip.Media, m)
	case post.IsGallery && len(post.MediaMetadata) > 0:
		for _, id := range post.galleryOrder() {
			item := post.MediaMetadata[id]
			if item.S == nil || item.S.U == "" {
				continue
			}
			clip.Media = append(clip.Media, Media{URL: item.S.U, Kind: MediaImage})
			if len(clip.Media) >= redditGalleryCap {
				break
			}
		}
	default:
		if p := post.previewImage(); p != "" {
			clip.Media = append(clip.Media, Media{URL: p, Kind: MediaImage})
		}
	}

	return clip, nil
}

func (p *redditPost) previewImage() string {
	if p.Preview == nil || len(p.Preview.Images) == 0 || p.Preview.Images[0].Source == nil {
		return ""
	}
	return p.Preview.Images[0].Source.URL
}

// galleryOrder returns media IDs in the author's order when gallery_data
// carries it, else sorted keys for determinism.
func (p *redditPost) galleryOrder() []string {
	if p.GalleryData != nil && len(p.GalleryData.Items) > 0 {
		ids := make([]string, 0, len(p.GalleryData.Items))
		for _, it := range p.GalleryData.Items {
			ids = append(ids, it.MediaID)
		}
		return ids
	}
	ids := make([]string, 0, len(p.MediaMetadata))
	for id := range p.MediaMetadata {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// truncateUTF8 cuts s to at most n bytes without splitting a rune.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}
