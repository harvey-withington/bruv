// Twitter/X resolver — the ONLY file on the server side that knows what
// Twitter is. Port of clipper/src/lib/plugins/twitter.ts's enrich() half,
// widened: the syndication API response carries the whole tweet (user,
// text, photos, video variants), so this resolver reads the fields the
// extension's DOM pass provided, not just the video top-up.
//
// The syndication endpoint (cdn.syndication.twimg.com — the react-tweet
// approach) is unofficial and the flakiest of the four platforms. A
// failure here is an honest error: the supervisor's pending ladder turns
// it into a link-only clip completed later from a real browser.

package capture

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type twitterResolver struct{}

func (twitterResolver) ID() string { return "twitter" }

var twitterURLRe = regexp.MustCompile(`^https?://(mobile\.)?(twitter|x)\.com/`)

func (twitterResolver) Matches(rawURL string) bool {
	return twitterURLRe.MatchString(rawURL)
}

var statusIDRe = regexp.MustCompile(`/status(?:es)?/(\d+)`)

func tweetIDFromURL(rawURL string) string {
	m := statusIDRe.FindStringSubmatch(rawURL)
	if m == nil {
		return ""
	}
	return m[1]
}

// --- syndication response (only the fields we read) ----------------------

type syndicationUser struct {
	Name       string `json:"name"`
	ScreenName string `json:"screen_name"`
	AvatarURL  string `json:"profile_image_url_https"`
}

type syndicationVariant struct {
	ContentType string `json:"content_type"`
	Bitrate     int64  `json:"bitrate"`
	URL         string `json:"url"`
}

type syndicationMedia struct {
	Type     string `json:"type"` // "photo" | "video" | "animated_gif"
	MediaURL string `json:"media_url_https"`
	Video    *struct {
		Variants []syndicationVariant `json:"variants"`
	} `json:"video_info"`
}

type syndicationTweet struct {
	User      *syndicationUser   `json:"user"`
	Text      string             `json:"text"`
	CreatedAt string             `json:"created_at"`
	Media     []syndicationMedia `json:"mediaDetails"`
	Photos    []struct {
		URL string `json:"url"`
	} `json:"photos"`
}

func (r twitterResolver) Resolve(ctx context.Context, c *Client, rawURL string) (*Clip, error) {
	id := tweetIDFromURL(rawURL)
	if id == "" {
		return nil, fmt.Errorf("twitter: no status id in %s", rawURL)
	}

	var tw syndicationTweet
	endpoint := fmt.Sprintf(
		"https://cdn.syndication.twimg.com/tweet-result?id=%s&token=%s",
		url.QueryEscape(id), url.QueryEscape(syndicationToken(id)),
	)
	if err := c.GetJSON(ctx, endpoint, &tw); err != nil {
		return nil, fmt.Errorf("twitter syndication: %w", err)
	}
	// Deleted/protected tweets answer 200 with a tombstone body that has no
	// user — treat as unresolvable rather than emitting an empty clip.
	if tw.User == nil {
		return nil, fmt.Errorf("twitter syndication: tweet %s unavailable", id)
	}

	clip := &Clip{
		Platform:     "twitter",
		CanonicalURL: fmt.Sprintf("https://x.com/%s/status/%s", tw.User.ScreenName, id),
		Author:       tw.User.Name,
		Handle:       "@" + tw.User.ScreenName,
		// The API hands back the 48px "_normal" avatar variant; the 400px
		// one lives at the same path. Plain replace — absent suffix means
		// the URL passes through unchanged.
		AvatarURL:   strings.Replace(tw.User.AvatarURL, "_normal.", "_400x400.", 1),
		Text:        tw.Text,
		PublishedAt: tw.CreatedAt,
		Extras:      map[string]string{"statusId": id},
	}

	for _, m := range tw.Media {
		switch m.Type {
		case "photo":
			clip.Media = append(clip.Media, Media{URL: largePhotoURL(m.MediaURL), Kind: MediaImage})
		case "video", "animated_gif":
			// Highest-bitrate mp4 wins (mirrors twitter.ts). No mp4 →
			// degrade the entry to its poster, same as the extension.
			var best syndicationVariant
			if m.Video != nil {
				for _, v := range m.Video.Variants {
					if v.ContentType == "video/mp4" && v.URL != "" && v.Bitrate >= best.Bitrate {
						best = v
					}
				}
			}
			if best.URL != "" {
				clip.Media = append(clip.Media, Media{URL: best.URL, Kind: MediaVideo, PosterURL: m.MediaURL})
			} else if m.MediaURL != "" {
				clip.Media = append(clip.Media, Media{URL: m.MediaURL, Kind: MediaImage})
			}
		}
	}
	// Older payload shapes carry photos only in the `photos` array.
	if len(clip.Media) == 0 {
		for _, p := range tw.Photos {
			if p.URL != "" {
				clip.Media = append(clip.Media, Media{URL: largePhotoURL(p.URL), Kind: MediaImage})
			}
		}
	}

	if clip.Text == "" && len(clip.Media) == 0 {
		return nil, fmt.Errorf("twitter syndication: tweet %s resolved empty", id)
	}
	return clip, nil
}

// largePhotoURL upgrades a pbs.twimg.com media URL to its large variant,
// mirroring the extension's ?name=large upgrade.
func largePhotoURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || !strings.Contains(u.Host, "twimg.com") {
		return raw
	}
	q := u.Query()
	q.Set("name", "large")
	u.RawQuery = q.Encode()
	return u.String()
}

// --- syndication token ----------------------------------------------------

// syndicationToken ports twitter.ts's token derivation exactly:
//
//	((Number(id) / 1e15) * Math.PI).toString(36).replace(/(0+|\.)/g, '')
//
// The hard part is Number.prototype.toString(36): JS emits the SHORTEST
// base-36 string that round-trips the double. jsFormatRadix36 below is a
// port of V8's DoubleToRadixCString digit loop (delta-tracked emission
// with final-digit rounding), so tokens match the browser byte-for-byte.
// Unofficial; isolated here so a breakage is a one-file fix.
func syndicationToken(id string) string {
	n, err := strconv.ParseFloat(id, 64)
	if err != nil {
		return ""
	}
	return tokenStripRe.ReplaceAllString(jsFormatRadix36(n/1e15*math.Pi), "")
}

var tokenStripRe = regexp.MustCompile(`(0+|\.)`)

const radix36Digits = "0123456789abcdefghijklmnopqrstuvwxyz"

func jsFormatRadix36(v float64) string {
	integer := math.Floor(v)
	fraction := v - integer

	// delta = half a ULP of the input — the emission loop stops once the
	// remaining fraction can no longer affect which double the string
	// parses back to. This is what makes the output the shortest
	// round-tripping form, matching JS.
	delta := 0.5 * (math.Nextafter(v, math.Inf(1)) - v)
	if min := math.Nextafter(0, 1); delta < min {
		delta = min
	}

	var frac []byte
	if fraction >= delta {
		for {
			fraction *= 36
			delta *= 36
			digit := int(fraction)
			frac = append(frac, radix36Digits[digit])
			fraction -= float64(digit)
			// Round the final digit up when the remainder says so (ties to
			// even, per V8), carrying leftward; a carry off the front bumps
			// the integer part.
			if fraction > 0.5 || (fraction == 0.5 && digit%2 == 1) {
				if fraction+delta > 1 {
					i := len(frac) - 1
					for {
						if i < 0 {
							integer++
							frac = frac[:0]
							break
						}
						d := strings.IndexByte(radix36Digits, frac[i])
						if d+1 < 36 {
							frac[i] = radix36Digits[d+1]
							frac = frac[:i+1]
							break
						}
						i--
					}
					break
				}
			}
			if fraction < delta {
				break
			}
		}
	}

	var intPart []byte
	if integer <= 0 {
		intPart = []byte{'0'}
	} else {
		for integer > 0 {
			rem := math.Mod(integer, 36)
			intPart = append([]byte{radix36Digits[int(rem)]}, intPart...)
			integer = (integer - rem) / 36
		}
	}

	if len(frac) == 0 {
		return string(intPart)
	}
	return string(intPart) + "." + string(frac)
}
