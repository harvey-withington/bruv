// Truth Social resolver — the ONLY file on the server side that knows
// what Truth Social is. Port of clipper/src/lib/plugins/truthsocial.ts's
// enrich() half: the public Mastodon-compatible statuses endpoint is
// authoritative for everything (the extension's DOM pass was explicitly
// best-effort and existed only to find the status id — which the share
// URL hands us directly).
//
// Truth Social sits behind Cloudflare; a challenged fetch here is the
// expected bot-wall case and lands on the pending ladder by design.

package capture

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type truthsocialResolver struct{}

func (truthsocialResolver) ID() string { return "truthsocial" }

var truthsocialURLRe = regexp.MustCompile(`^https?://([a-z0-9-]+\.)?truthsocial\.com/`)

func (truthsocialResolver) Matches(rawURL string) bool {
	return truthsocialURLRe.MatchString(rawURL)
}

// Truth Social permalinks look like /@<user>/posts/<id> (numeric
// Mastodon-style snowflakes).
var truthStatusIDRe = regexp.MustCompile(`/@[^/]+/posts/(\d+)`)

func truthStatusIDFromURL(rawURL string) string {
	m := truthStatusIDRe.FindStringSubmatch(rawURL)
	if m == nil {
		return ""
	}
	return m[1]
}

// mastodonStatus is the public statuses endpoint's shape, trimmed to the
// fields we use. Undocumented but it's the same contract every
// Mastodon-fork frontend renders from — and isolated here, so a schema
// change is a one-file fix.
type mastodonStatus struct {
	Account *struct {
		DisplayName string `json:"display_name"`
		Username    string `json:"username"`
		Avatar      string `json:"avatar"`
	} `json:"account"`
	Content          string `json:"content"`
	CreatedAt        string `json:"created_at"`
	URL              string `json:"url"`
	MediaAttachments []struct {
		Type       string `json:"type"` // "image" | "video" | "gifv" | ...
		URL        string `json:"url"`
		PreviewURL string `json:"preview_url"`
	} `json:"media_attachments"`
}

func (r truthsocialResolver) Resolve(ctx context.Context, c *Client, rawURL string) (*Clip, error) {
	id := truthStatusIDFromURL(rawURL)
	if id == "" {
		return nil, fmt.Errorf("truthsocial: no status id in %s", rawURL)
	}

	var st mastodonStatus
	endpoint := "https://truthsocial.com/api/v1/statuses/" + url.PathEscape(id)
	if err := c.GetJSON(ctx, endpoint, &st); err != nil {
		return nil, fmt.Errorf("truthsocial statuses: %w", err)
	}

	clip := &Clip{
		Platform:     "truthsocial",
		CanonicalURL: rawURL,
		Text:         mastodonHTMLToText(st.Content),
		PublishedAt:  st.CreatedAt,
		Extras:       map[string]string{"statusId": id},
	}
	if st.URL != "" {
		clip.CanonicalURL = st.URL
	}
	if st.Account != nil {
		clip.Author = st.Account.DisplayName
		if st.Account.Username != "" {
			clip.Handle = "@" + st.Account.Username
		}
		clip.AvatarURL = st.Account.Avatar
		if clip.Author == "" {
			clip.Author = clip.Handle
		}
	}

	for _, m := range st.MediaAttachments {
		if m.URL == "" {
			continue
		}
		switch m.Type {
		case "video", "gifv":
			clip.Media = append(clip.Media, Media{URL: m.URL, Kind: MediaVideo, PosterURL: m.PreviewURL})
		case "image":
			clip.Media = append(clip.Media, Media{URL: m.URL, Kind: MediaImage})
		}
	}

	if clip.Text == "" && len(clip.Media) == 0 {
		return nil, fmt.Errorf("truthsocial statuses: status %s resolved empty", id)
	}
	return clip, nil
}

// mastodonHTMLToText ports htmlContentToText: <br> and </p> become
// newlines (paragraph breaks matter for readability); every other tag is
// stripped; entity decoding covers the handful Mastodon-style servers
// actually emit in status content.
var (
	brTagRe   = regexp.MustCompile(`(?i)<br\s*/?>`)
	pCloseRe  = regexp.MustCompile(`(?i)</p>`)
	anyTagRe  = regexp.MustCompile(`<[^>]+>`)
	decEntRe  = regexp.MustCompile(`&#(\d+);`)
	namedEnts = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'")
)

func mastodonHTMLToText(html string) string {
	s := brTagRe.ReplaceAllString(html, "\n")
	s = pCloseRe.ReplaceAllString(s, "\n\n")
	s = anyTagRe.ReplaceAllString(s, "")
	s = namedEnts.Replace(s)
	s = decEntRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := decEntRe.FindStringSubmatch(m)
		n, err := strconv.Atoi(sub[1])
		if err != nil || n < 0 || n > 0x10FFFF {
			return m
		}
		return string(rune(n))
	})
	return strings.TrimSpace(s)
}
