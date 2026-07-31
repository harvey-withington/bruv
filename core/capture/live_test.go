//go:build live

package capture

// Manual live-network smoke tests — excluded from CI by the `live` build
// tag. Run locally when a resolver is suspected of drifting:
//
//	go test ./core/capture/ -tags live -run Live -v
//
// URLs are stable, well-known public posts; a failure here usually means
// the platform endpoint changed (or walled this network), not a code bug.

import (
	"context"
	"testing"
	"time"
)

func liveResolve(t *testing.T, url string) *Clip {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	clip, err := Resolve(ctx, NewClient(), url)
	if err != nil {
		t.Fatalf("resolve %s: %v", url, err)
	}
	if clip == nil {
		t.Fatalf("no resolver claimed %s", url)
	}
	if clip.Text == "" && len(clip.Media) == 0 {
		t.Fatalf("empty clip from %s: %+v", url, clip)
	}
	t.Logf("%s → author=%q handle=%q text=%.60q media=%d", clip.Platform, clip.Author, clip.Handle, clip.Text, len(clip.Media))
	return clip
}

func TestLiveTwitter(t *testing.T) {
	// @jack's first tweet — as permanent as tweets get.
	liveResolve(t, "https://x.com/jack/status/20")
}

func TestLiveReddit(t *testing.T) {
	liveResolve(t, "https://www.reddit.com/r/announcements/comments/pbmy5y/")
}

func TestLiveYouTube(t *testing.T) {
	clip := liveResolve(t, "https://youtu.be/dQw4w9WgXcQ")
	if clip.EmbedVideo == nil || clip.EmbedVideo.ID != "dQw4w9WgXcQ" {
		t.Fatalf("embed missing: %+v", clip.EmbedVideo)
	}
}

func TestLiveTruthSocial(t *testing.T) {
	// The platform's own announcement account; expected to be bot-walled
	// from datacenter IPs — from RIPPED it should pass.
	liveResolve(t, "https://truthsocial.com/@TruthSocial/posts/109519086195988207")
}
