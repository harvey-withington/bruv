package capture

// Resolver tests run against checked-in fixtures via a URL-routing
// RoundTripper — no live network in CI. The syndication-token vectors are
// ground truth generated from Node (`(...).toString(36)`), covering the
// edge shapes: leading-zero fractions, stripped inner zeros, and the
// near-carry zzzzz tail.

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- syndication token (V8 radix port) ------------------------------------

func TestJSFormatRadix36(t *testing.T) {
	// value | Node's Number.prototype.toString(36)
	cases := []struct {
		in   float64
		want string
	}{
		{0.5, "0.i"},
		{4712.382417, "3mw.drm1pml3"},
		{3.141592653589793, "3.53i5ab8p5f"},
		{100, "2s"},
		{0.037, "0.1by9sifjve3i"},
		{0.001, "0.01anm6c3gez6"},
		{5876.99999999, "4j8.zzzzze8d"},
	}
	for _, c := range cases {
		if got := jsFormatRadix36(c.in); got != c.want {
			t.Errorf("jsFormatRadix36(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSyndicationToken(t *testing.T) {
	// id | token, generated with Node from the exact twitter.ts formula.
	cases := []struct{ id, want string }{
		{"1629307668568633344", "3y6mctgwzxo"},
		{"1791559109055086992", "4ccck7xm3tn"},
		{"20", "6dq1a2xwd93"},
		{"1234567890123456789", "2zqic77uqyk"},
		{"1858392847561234567", "4i6ba24vz47"},
		{"999999999999999999", "2f9lc2ug9mm"},
		{"1500000000000000000", "3mwe49oefy"},
		{"1849999999999999999", "4hfy2jnxph"},
	}
	for _, c := range cases {
		if got := syndicationToken(c.id); got != c.want {
			t.Errorf("syndicationToken(%s) = %q, want %q", c.id, got, c.want)
		}
	}
}

// --- URL matching ----------------------------------------------------------

func TestMatch(t *testing.T) {
	cases := []struct{ url, want string }{
		{"https://x.com/someone/status/1629307668568633344", "twitter"},
		{"https://twitter.com/someone/status/123", "twitter"},
		{"https://mobile.twitter.com/a/status/9", "twitter"},
		{"https://truthsocial.com/@realsomeone/posts/109", "truthsocial"},
		{"https://www.reddit.com/r/golang/comments/abc123/title/", "reddit"},
		{"https://www.reddit.com/r/golang/s/AbCdEfGh", "reddit"},
		{"https://redd.it/abc123", "reddit"},
		{"https://old.reddit.com/r/golang/comments/abc123/", "reddit"},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "youtube"},
		{"https://www.youtube.com/shorts/abc_-123XYZ", "youtube"},
		{"https://youtu.be/dQw4w9WgXcQ", "youtube"},
		{"https://example.com/article", ""},
		{"https://news.ycombinator.com/item?id=1", ""},
		{"not a url", ""},
	}
	for _, c := range cases {
		if got := Match(c.url); got != c.want {
			t.Errorf("Match(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestVideoIDFromURL(t *testing.T) {
	cases := []struct{ url, want string }{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=42s", "dQw4w9WgXcQ"},
		{"https://www.youtube.com/shorts/abc_-123", "abc_-123"},
		{"https://youtu.be/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://www.youtube.com/", ""},
	}
	for _, c := range cases {
		if got := videoIDFromURL(c.url); got != c.want {
			t.Errorf("videoIDFromURL(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

// --- fixture transport ------------------------------------------------------

// routeFn answers one route; the transport matches on host+path.
type routeFn func(req *http.Request) *http.Response

type fixtureTransport struct {
	t      *testing.T
	routes map[string]routeFn // "HOST /path" (query ignored) -> responder
}

func (f *fixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if ua := req.Header.Get("User-Agent"); !strings.HasPrefix(ua, "Mozilla/5.0") {
		f.t.Errorf("request to %s missing browser UA (got %q)", req.URL, ua)
	}
	key := req.URL.Host + " " + req.URL.Path
	fn, ok := f.routes[key]
	if !ok {
		f.t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		return nil, nil
	}
	res := fn(req)
	// The real http.Transport stamps Response.Request; a bare RoundTripper
	// must do it itself or FinalURL's res.Request.URL derefs nil.
	res.Request = req
	return res, nil
}

func respond(status int, body string, header http.Header) routeFn {
	return func(*http.Request) *http.Response {
		h := header
		if h == nil {
			h = http.Header{}
		}
		return &http.Response{
			StatusCode: status,
			Header:     h,
			Body:       io.NopCloser(strings.NewReader(body)),
		}
	}
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return string(data)
}

func testClient(t *testing.T, routes map[string]routeFn) *Client {
	t.Helper()
	return &Client{http: &http.Client{Transport: &fixtureTransport{t: t, routes: routes}}}
}

// --- resolvers --------------------------------------------------------------

func TestTwitterResolve(t *testing.T) {
	c := testClient(t, map[string]routeFn{
		"cdn.syndication.twimg.com /tweet-result": respond(200, fixture(t, "twitter_tweet.json"), nil),
	})
	clip, err := Resolve(context.Background(), c, "https://x.com/NASA/status/1629307668568633344")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if clip.Platform != "twitter" || clip.Author != "NASA" || clip.Handle != "@NASA" {
		t.Errorf("author mapping wrong: %+v", clip)
	}
	if clip.CanonicalURL != "https://x.com/NASA/status/1629307668568633344" {
		t.Errorf("canonical = %s", clip.CanonicalURL)
	}
	if !strings.Contains(clip.AvatarURL, "_400x400.") {
		t.Errorf("avatar not upgraded: %s", clip.AvatarURL)
	}
	if clip.Text != "One year ago, our Artemis I Moon rocket launched." {
		t.Errorf("text = %q", clip.Text)
	}
	// Fixture carries one photo + one video: photo upgraded to name=large,
	// video resolved to the highest-bitrate mp4 with the poster carried.
	if len(clip.Media) != 2 {
		t.Fatalf("media count = %d, want 2", len(clip.Media))
	}
	if clip.Media[0].Kind != MediaImage || !strings.Contains(clip.Media[0].URL, "name=large") {
		t.Errorf("photo not upgraded: %+v", clip.Media[0])
	}
	if clip.Media[1].Kind != MediaVideo || !strings.Contains(clip.Media[1].URL, "1280x720") {
		t.Errorf("video variant selection wrong (want highest bitrate): %+v", clip.Media[1])
	}
	if clip.Media[1].PosterURL == "" {
		t.Errorf("video poster missing")
	}
}

func TestTwitterResolveTombstone(t *testing.T) {
	c := testClient(t, map[string]routeFn{
		"cdn.syndication.twimg.com /tweet-result": respond(200, `{"tombstone":true}`, nil),
	})
	if _, err := Resolve(context.Background(), c, "https://x.com/gone/status/123"); err == nil {
		t.Fatal("expected error for tombstoned tweet")
	}
}

func TestRedditResolveTextPost(t *testing.T) {
	c := testClient(t, map[string]routeFn{
		"www.reddit.com /r/golang/comments/abc123/some_title.json": respond(200, fixture(t, "reddit_post.json"), nil),
	})
	clip, err := Resolve(context.Background(), c, "https://www.reddit.com/r/golang/comments/abc123/some_title/")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if clip.Author != "u/gopher" || clip.Handle != "r/golang" {
		t.Errorf("author mapping wrong: %+v", clip)
	}
	if !strings.HasPrefix(clip.Text, "A post title") || !strings.Contains(clip.Text, "\n\n") {
		t.Errorf("text should be title + selftext preview: %q", clip.Text)
	}
	if clip.PublishedAt != "2026-07-30T12:00:00Z" {
		t.Errorf("publishedAt = %s", clip.PublishedAt)
	}
	if len(clip.Media) != 1 || clip.Media[0].Kind != MediaImage {
		t.Errorf("expected single preview image: %+v", clip.Media)
	}
}

func TestRedditResolveShareLinkAndGallery(t *testing.T) {
	// The Android app's opaque /s/ link 302s to the real permalink; the
	// resolver must follow it, then order the gallery by gallery_data.
	c := testClient(t, map[string]routeFn{
		"www.reddit.com /r/pics/s/AbCd1234": respond(302, "", http.Header{
			"Location": []string{"https://www.reddit.com/r/pics/comments/xyz789/gallery_title/?share_id=q"},
		}),
		"www.reddit.com /r/pics/comments/xyz789/gallery_title/":     respond(200, "", nil),
		"www.reddit.com /r/pics/comments/xyz789/gallery_title.json": respond(200, fixture(t, "reddit_gallery.json"), nil),
	})
	clip, err := Resolve(context.Background(), c, "https://www.reddit.com/r/pics/s/AbCd1234")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(clip.Media) != 3 {
		t.Fatalf("gallery media count = %d, want 3", len(clip.Media))
	}
	// gallery_data order is authoritative: b, a, c.
	for i, want := range []string{"img-b", "img-a", "img-c"} {
		if !strings.Contains(clip.Media[i].URL, want) {
			t.Errorf("gallery[%d] = %s, want containing %s", i, clip.Media[i].URL, want)
		}
	}
}

func TestYouTubeResolve(t *testing.T) {
	c := testClient(t, map[string]routeFn{
		"www.youtube.com /oembed": respond(200, fixture(t, "youtube_oembed.json"), nil),
	})
	clip, err := Resolve(context.Background(), c, "https://youtu.be/dQw4w9WgXcQ")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if clip.Text != "A Video Title" || clip.Author != "Some Channel" || clip.Handle != "@somechannel" {
		t.Errorf("oembed mapping wrong: %+v", clip)
	}
	if clip.EmbedVideo == nil || clip.EmbedVideo.Provider != "youtube" || clip.EmbedVideo.ID != "dQw4w9WgXcQ" {
		t.Errorf("embedVideo wrong: %+v", clip.EmbedVideo)
	}
	// No media by design (ruled 2026-07-31): the embed IS the content —
	// a thumbnail in the media field breaks cross-template value purity,
	// and a failed embed should look like the error it is.
	if len(clip.Media) != 0 {
		t.Errorf("youtube must produce no media, got %+v", clip.Media)
	}
}

func TestTruthSocialResolve(t *testing.T) {
	c := testClient(t, map[string]routeFn{
		"truthsocial.com /api/v1/statuses/112233445566778899": respond(200, fixture(t, "truthsocial_status.json"), nil),
	})
	clip, err := Resolve(context.Background(), c, "https://truthsocial.com/@someone/posts/112233445566778899")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if clip.Author != "Some One" || clip.Handle != "@someone" {
		t.Errorf("account mapping wrong: %+v", clip)
	}
	if clip.Text != "Hello & welcome.\n\nSecond paragraph." {
		t.Errorf("html-to-text wrong: %q", clip.Text)
	}
	if clip.CanonicalURL != "https://truthsocial.com/@someone/posts/112233445566778899" {
		t.Errorf("canonical = %s", clip.CanonicalURL)
	}
	if len(clip.Media) != 2 || clip.Media[0].Kind != MediaImage || clip.Media[1].Kind != MediaVideo {
		t.Errorf("media mapping wrong: %+v", clip.Media)
	}
	if clip.Media[1].PosterURL == "" {
		t.Errorf("video poster missing")
	}
}

func TestResolveUnmatchedURL(t *testing.T) {
	clip, err := Resolve(context.Background(), testClient(t, nil), "https://example.com/article")
	if err != nil || clip != nil {
		t.Fatalf("unmatched URL should be (nil, nil), got (%v, %v)", clip, err)
	}
}

func TestMastodonHTMLToText(t *testing.T) {
	in := `<p>Line one<br/>line two</p><p>Para &amp; &quot;quotes&quot; &#8217;</p>`
	want := "Line one\nline two\n\nPara & \"quotes\" ’"
	if got := mastodonHTMLToText(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
