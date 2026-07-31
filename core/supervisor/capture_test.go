package supervisor

// Ingest tests exercise the capture ladder end-to-end against a temp
// repo (newTestRuntime) with outbound HTTP routed at fixtures. The
// block-ID-stability test is the load-bearing one: live slide bindings
// point at the pending card's pre-created blocks, so completion filling
// them IN PLACE (never recreating) is the invariant the whole
// pending-clip design rests on.

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"bruv/core/capture"
	"bruv/internal/config"
	"bruv/internal/model"
)

// --- fixture transport (host+path routing, else 404) -----------------------

type captureRoutes map[string]struct {
	status int
	body   string
}

type captureTransport struct{ routes captureRoutes }

func (ct *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	key := req.URL.Host + " " + req.URL.Path
	r, ok := ct.routes[key]
	if !ok {
		r.status, r.body = 404, "not found"
	}
	res := &http.Response{
		StatusCode: r.status,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(r.body)),
		Request:    req,
	}
	return res, nil
}

// swapCaptureHTTP routes the package's outbound client at fixtures for
// one test, restoring the real client afterwards.
func swapCaptureHTTP(t *testing.T, routes captureRoutes) {
	t.Helper()
	captureHTTP = capture.NewClientWithHTTP(&http.Client{Transport: &captureTransport{routes: routes}})
	t.Cleanup(func() { captureHTTP = capture.NewClient() })
}

const tweetURL = "https://x.com/someone/status/1629307668568633344"

// tweetJSON is a minimal syndication payload: text + one photo.
const tweetJSON = `{
  "user": {"name": "Some One", "screen_name": "someone", "profile_image_url_https": "https://pbs.twimg.com/profile_images/1/a_normal.jpg"},
  "text": "Hello from the fixture tweet.",
  "created_at": "2026-07-30T12:00:00.000Z",
  "mediaDetails": [{"type": "photo", "media_url_https": "https://pbs.twimg.com/media/pic.jpg"}]
}`

func successRoutes() captureRoutes {
	return captureRoutes{
		"cdn.syndication.twimg.com /tweet-result": {200, tweetJSON},
		"pbs.twimg.com /media/pic.jpg":            {200, "fake-image-bytes"},
		"pbs.twimg.com /profile_images/1/a_400x400.jpg": {200, "fake-avatar-bytes"},
	}
}

func failRoutes() captureRoutes {
	return captureRoutes{"cdn.syndication.twimg.com /tweet-result": {403, "blocked"}}
}

// newDeckCard creates a card holding an empty slide_deck block "d1".
func newDeckCard(t *testing.T, rt *Runtime) *model.Card {
	t.Helper()
	deck, err := rt.CreateCard("", "Stream Deck")
	if err != nil {
		t.Fatalf("create deck card: %v", err)
	}
	deck, err = rt.UpdateCardBlocks(deck.ID, []model.Block{
		{ID: "d1", Type: model.BlockSlideDeck, Label: "Deck", Value: map[string]any{"slides": []any{}}},
	})
	if err != nil {
		t.Fatalf("deck block: %v", err)
	}
	return deck
}

func deckSlides(t *testing.T, rt *Runtime, deckID string) []any {
	t.Helper()
	deck, err := rt.GetCard(deckID)
	if err != nil {
		t.Fatalf("get deck: %v", err)
	}
	for _, b := range deck.Blocks {
		if b.ID == "d1" {
			v, _ := b.Value.(map[string]any)
			slides, _ := v["slides"].([]any)
			return slides
		}
	}
	t.Fatal("deck block missing")
	return nil
}

func blocksByKey(card *model.Card) map[string]model.Block {
	out := map[string]model.Block{}
	for _, b := range card.Blocks {
		if b.Key != "" {
			out[b.Key] = b
		}
	}
	return out
}

// --- tests ------------------------------------------------------------------

func TestCaptureFromURLPendingLadder(t *testing.T) {
	config.SetConfigDir(t.TempDir())
	t.Cleanup(func() { config.SetConfigDir("") })
	rt := newTestRuntime(t)
	swapCaptureHTTP(t, failRoutes())
	deck := newDeckCard(t, rt)

	res, err := rt.CaptureFromURL(tweetURL, CaptureOpts{
		IncludeInDeck: true, DeckCardID: deck.ID, DeckBlockID: "d1",
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if !res.Pending || res.Platform != "twitter" || !res.SlideAppended {
		t.Fatalf("result = %+v, want pending twitter with slide", res)
	}

	card, err := rt.GetCard(res.CardID)
	if err != nil {
		t.Fatalf("get card: %v", err)
	}
	byKey := blocksByKey(card)
	for _, key := range []string{"author", "handle", "avatar", "text", "media", "video", "date", "url"} {
		if _, ok := byKey[key]; !ok {
			t.Errorf("pending card missing %q block (full empty set required for stable bindings)", key)
		}
	}
	if v, _ := byKey["url"].Value.(map[string]any); v["url"] != tweetURL {
		t.Errorf("url block = %v", byKey["url"].Value)
	}
	wantTags := map[string]bool{"twitter": true, ClipPendingTag: true}
	for _, tag := range card.Tags {
		delete(wantTags, tag)
	}
	if len(wantTags) != 0 {
		t.Errorf("tags = %v, missing %v", card.Tags, wantTags)
	}
	if !strings.HasPrefix(card.Title, "twitter: http") {
		t.Errorf("pending title = %q", card.Title)
	}

	slides := deckSlides(t, rt, deck.ID)
	if len(slides) != 1 {
		t.Fatalf("deck slides = %d, want 1", len(slides))
	}
	slide, _ := slides[0].(map[string]any)
	if slide["templateId"] != "auto" || slide["contentTypeId"] != "post" || slide["cardId"] != card.ID {
		t.Errorf("slide envelope wrong: %v", slide)
	}
	bindings, _ := slide["bindings"].(map[string]any)
	for key, b := range byKey {
		if bindings[key] != b.ID {
			t.Errorf("binding[%s] = %v, want block ID %s", key, bindings[key], b.ID)
		}
	}

	// Dispatcher channels are fire-and-forget goroutines — poll briefly.
	deadline := time.Now().Add(2 * time.Second)
	for {
		notifs, err := config.LoadNotifications()
		if err == nil && len(notifs) == 1 && notifs[0].CardID == card.ID {
			break
		}
		if time.Now().After(deadline) {
			t.Errorf("expected one pending-clip notification for card, got %v (err %v)", notifs, err)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCaptureFromURLFullClip(t *testing.T) {
	rt := newTestRuntime(t)
	swapCaptureHTTP(t, successRoutes())
	deck := newDeckCard(t, rt)

	res, err := rt.CaptureFromURL(tweetURL, CaptureOpts{
		IncludeInDeck: true, DeckCardID: deck.ID, DeckBlockID: "d1", CategoryID: "",
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if res.Pending || !res.SlideAppended {
		t.Fatalf("result = %+v, want non-pending with slide", res)
	}

	card, err := rt.GetCard(res.CardID)
	if err != nil {
		t.Fatalf("get card: %v", err)
	}
	byKey := blocksByKey(card)
	if byKey["text"].Value != "Hello from the fixture tweet." {
		t.Errorf("text block = %v", byKey["text"].Value)
	}
	if byKey["author"].Value != "Some One" || byKey["handle"].Value != "@someone" {
		t.Errorf("author/handle = %v / %v", byKey["author"].Value, byKey["handle"].Value)
	}
	if byKey["date"].Value != "Jul 30, 2026" {
		t.Errorf("date block = %v", byKey["date"].Value)
	}
	// Photo + avatar landed as attachments; the media/avatar blocks
	// reference them by attachment: refs, never remote URLs.
	if len(card.FileAttachments) != 2 {
		t.Fatalf("attachments = %d, want 2 (photo + avatar)", len(card.FileAttachments))
	}
	if v, _ := byKey["media"].Value.(map[string]any); v == nil || !strings.HasPrefix(v["url"].(string), "attachment:"+card.ID+"/") {
		t.Errorf("media block should reference an attachment: %v", byKey["media"].Value)
	}
	if v, _ := byKey["avatar"].Value.(map[string]any); v == nil || !strings.HasPrefix(v["url"].(string), "attachment:") {
		t.Errorf("avatar block should reference an attachment: %v", byKey["avatar"].Value)
	}
	if !strings.HasPrefix(card.Title, "@someone: Hello") {
		t.Errorf("title = %q", card.Title)
	}
	for _, tag := range card.Tags {
		if tag == ClipPendingTag {
			t.Errorf("full clip must not carry %s", ClipPendingTag)
		}
	}
	// The card got the Social Post type provisioned on first use.
	typed := false
	for _, ct := range rt.ListCardTypes() {
		if ct.Label == socialPostTypeLabel && ct.ID == card.Type {
			typed = true
		}
	}
	if !typed {
		t.Errorf("card type = %q, want the provisioned Social Post type", card.Type)
	}
	if len(deckSlides(t, rt, deck.ID)) != 1 {
		t.Error("expected one appended slide")
	}
}

func TestCompleteCaptureBlockIDStability(t *testing.T) {
	rt := newTestRuntime(t)
	swapCaptureHTTP(t, failRoutes())
	deck := newDeckCard(t, rt)

	res, err := rt.CaptureFromURL(tweetURL, CaptureOpts{IncludeInDeck: true, DeckCardID: deck.ID, DeckBlockID: "d1"})
	if err != nil || !res.Pending {
		t.Fatalf("pending capture failed: %v %+v", err, res)
	}
	before, _ := rt.GetCard(res.CardID)
	beforeIDs := map[string]string{}
	for key, b := range blocksByKey(before) {
		beforeIDs[key] = b.ID
	}

	done, err := rt.CompleteCapture(res.CardID, capture.Clip{
		Platform:     "twitter",
		CanonicalURL: tweetURL,
		Author:       "Some One",
		Handle:       "@someone",
		Text:         "Hello from the completed tweet.",
		PublishedAt:  "2026-07-30T12:00:00.000Z",
	}, []CompleteMedia{{Name: "twitter-1.jpg", Base64: "aGVsbG8=", Kind: "image"}})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if done.Pending || done.SlideAppended {
		t.Fatalf("completion result = %+v (must not re-append the slide)", done)
	}

	after, _ := rt.GetCard(res.CardID)
	afterByKey := blocksByKey(after)
	// THE invariant: completion fills blocks in place — IDs survive, so
	// the pre-bound slide's bindings still resolve.
	for key, id := range beforeIDs {
		if afterByKey[key].ID != id {
			t.Errorf("block %q ID changed %s → %s (breaks live slide bindings)", key, id, afterByKey[key].ID)
		}
	}
	if afterByKey["text"].Value != "Hello from the completed tweet." {
		t.Errorf("text not filled: %v", afterByKey["text"].Value)
	}
	if v, _ := afterByKey["media"].Value.(map[string]any); v == nil || !strings.HasPrefix(v["url"].(string), "attachment:") {
		t.Errorf("media not filled from posted attachment: %v", afterByKey["media"].Value)
	}
	for _, tag := range after.Tags {
		if tag == ClipPendingTag {
			t.Error("clip-pending tag must clear on completion")
		}
	}
	if !strings.HasPrefix(after.Title, "@someone: Hello") {
		t.Errorf("machine title should un-freeze on completion, got %q", after.Title)
	}
	if len(deckSlides(t, rt, deck.ID)) != 1 {
		t.Error("completion must not append a second slide")
	}
}

func TestCompleteCaptureKeepsHumanTitle(t *testing.T) {
	rt := newTestRuntime(t)
	swapCaptureHTTP(t, failRoutes())

	res, err := rt.CaptureFromURL(tweetURL, CaptureOpts{})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if _, err := rt.UpdateCardTitle(res.CardID, "My renamed clip"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := rt.CompleteCapture(res.CardID, capture.Clip{
		Platform: "twitter", CanonicalURL: tweetURL, Handle: "@someone", Text: "content",
	}, nil); err != nil {
		t.Fatalf("complete: %v", err)
	}
	card, _ := rt.GetCard(res.CardID)
	if card.Title != "My renamed clip" {
		t.Errorf("completion overwrote a human rename: %q", card.Title)
	}
}

func TestRetryCapture(t *testing.T) {
	rt := newTestRuntime(t)
	swapCaptureHTTP(t, failRoutes())
	res, err := rt.CaptureFromURL(tweetURL, CaptureOpts{})
	if err != nil || !res.Pending {
		t.Fatalf("pending capture failed: %v %+v", err, res)
	}

	// The transient flake clears — retry resolves server-side.
	swapCaptureHTTP(t, successRoutes())
	retried, err := rt.RetryCapture(res.CardID)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if retried.Pending || retried.CardID != res.CardID {
		t.Fatalf("retry result = %+v", retried)
	}
	card, _ := rt.GetCard(res.CardID)
	if blocksByKey(card)["text"].Value != "Hello from the fixture tweet." {
		t.Errorf("retry did not fill blocks: %v", blocksByKey(card)["text"].Value)
	}
	for _, tag := range card.Tags {
		if tag == ClipPendingTag {
			t.Error("clip-pending tag must clear on retry success")
		}
	}
}

func TestRetryCaptureStaysPendingOnFailure(t *testing.T) {
	rt := newTestRuntime(t)
	swapCaptureHTTP(t, failRoutes())
	res, err := rt.CaptureFromURL(tweetURL, CaptureOpts{})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if _, err := rt.RetryCapture(res.CardID); err == nil {
		t.Fatal("retry against a still-failing resolver must error")
	}
	card, _ := rt.GetCard(res.CardID)
	if !slicesContains(card.Tags, ClipPendingTag) {
		t.Error("failed retry must leave the pending state untouched")
	}
}

func slicesContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
