package supervisor

// Present support: minting signed /present URLs and resolving a card's
// slide-deck content for the read-only output page.
//
// The output page (OBS Browser Source) is unauthenticated by design — the
// HMAC-signed URL is the auth — and it can't resolve field→block bindings
// or attachment references itself. So resolution happens HERE, server-side:
// PresentCardJSON returns the card with every slide's bound fields flattened
// into literal values and attachment references replaced by signed URLs. The
// page stays dumb; the secret stays server-side; access stays scoped to the
// one signed card.

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"bruv/internal/model"
	"bruv/internal/repo"
	transporthttp "bruv/transport/http"
)

// TemplatePrefs re-exports the repo type for the RPC surface (stable TS
// binding name, same pattern as CardTypeInfo).
type TemplatePrefs = repo.TemplatePrefs

// GetTemplatePrefs returns the vault's slide-template preferences (Auto
// priority order + per-template urlHint overrides).
func (r *Runtime) GetTemplatePrefs() (TemplatePrefs, error) {
	if r.repo == nil {
		return TemplatePrefs{}, fmt.Errorf("repo not loaded")
	}
	return r.repo.LoadTemplatePrefs()
}

// SetTemplatePrefs replaces the vault's slide-template preferences. The
// server stores them blindly — regex validity is a renderer concern (an
// uncompilable override falls back to the built-in hint at render time).
func (r *Runtime) SetTemplatePrefs(p TemplatePrefs) error {
	if r.repo == nil {
		return fmt.Errorf("repo not loaded")
	}
	return r.repo.SaveTemplatePrefs(p)
}

// presentTTL is deliberately long (vs the attachments' 5 min): a present URL
// lives in an OBS scene config across a whole stream session. Attachment URLs
// minted inside PresentCardJSON get the same window so media doesn't die
// mid-show; the page re-polls, so each poll re-mints fresh ones anyway.
const presentTTL = 12 * time.Hour

// SignPresentURL mints a signed, server-relative URL for this repo's present
// page for one card. The client prepends its scheme://host (same contract as
// SignAttachmentURL).
func (r *Runtime) SignPresentURL(cardID string) (string, error) {
	if cardID == "" {
		return "", fmt.Errorf("cardID is required")
	}
	if r.repo == nil || r.repo.Manifest == nil {
		return "", fmt.Errorf("repo not loaded")
	}
	repoID := r.repo.Manifest.ID
	exp := time.Now().Add(presentTTL).Unix()
	sig := transporthttp.SignPresentMAC(r.secret, repoID, cardID, exp)
	return fmt.Sprintf("/present/%s/%s?exp=%d&sig=%s",
		repoID, cardID, exp, hex.EncodeToString(sig)), nil
}

// slideFieldTypes mirrors shared/slideContentTypes.ts: content type → field
// key → field type. Used to pick the right extraction (URL vs text) when
// resolving a bound block, and to spot media fields for attachment signing.
var slideFieldTypes = map[string]map[string]string{
	"title":       {"title": "text", "subtitle": "text"},
	"statement":   {"statement": "longtext"},
	"quote":       {"quote": "longtext", "author": "text"},
	"image":       {"image": "image", "caption": "text"},
	"video":       {"video": "video", "caption": "text"},
	"lower_third": {"name": "text", "subtitle": "text"},
	"post":        {"author": "text", "handle": "text", "avatar": "image", "text": "longtext", "media": "image", "video": "video", "date": "text", "url": "text", "platform": "text"},
}

// PresentCardJSON returns the card as JSON with all slide-deck bindings and
// attachment references resolved for the present page. ok=false when the
// card doesn't exist. Plugged into transport/http.PresentConfig by the hosts.
func (r *Runtime) PresentCardJSON(cardID string) ([]byte, bool) {
	// Gate first, and before the card lookup: a closed gate serves the
	// same "not presenting" payload whether or not the card exists, so a
	// leaked-but-gated URL can't even probe card existence. The output
	// page renders a waiting state and resumes when the gate reopens.
	if !r.isPresenting(cardID) {
		return []byte(`{"presenting":false}`), true
	}
	card, err := r.Card.Get(cardID)
	if err != nil || card == nil {
		return nil, false
	}
	// JSON round-trip = deep copy. The card may be shared/cached state; the
	// resolver must never mutate the live model.
	raw, err := json.Marshal(card)
	if err != nil {
		return nil, false
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, false
	}
	blocks, _ := m["blocks"].([]any)
	for _, b := range blocks {
		bm, ok := b.(map[string]any)
		if !ok || bm["type"] != model.BlockSlideDeck {
			continue
		}
		val, ok := bm["value"].(map[string]any)
		if !ok {
			continue
		}
		slides, _ := val["slides"].([]any)
		for _, s := range slides {
			if sm, ok := s.(map[string]any); ok {
				r.resolvePresentSlide(sm)
			}
		}
		// Live presentation state (in-memory block state, set by the
		// deck's console via SetBlockLiveState) overlays the stored value:
		// currentIndex (position), videoSeq/videoAction (play-pause
		// commands), and any future console controls flow through here —
		// this is the only place the output page learns presenter intent.
		// slides/theme are the persisted deck's own keys; live state must
		// never shadow them.
		if blockID, _ := bm["id"].(string); blockID != "" {
			if ls := r.blockLiveState(cardID, blockID); ls != nil {
				for k, v := range ls {
					if k == "slides" || k == "theme" {
						continue
					}
					val[k] = v
				}
			}
		}
	}
	// Template prefs ride the payload so the output page resolves Auto
	// templates with the same inputs as the app (it can't call authed RPCs).
	// Additive top-level key beside the card fields; absence = defaults.
	if prefs, err := r.repo.LoadTemplatePrefs(); err == nil {
		m["templatePrefs"] = prefs
	}
	out, err := json.Marshal(m)
	if err != nil {
		return nil, false
	}
	return out, true
}

// resolvePresentSlide flattens one slide in place (on the deep copy):
//  1. bound fields → the linked card's block values, extracted per field type
//  2. attachment references ("attachment:<cardID>/<attID>") → signed URLs
func (r *Runtime) resolvePresentSlide(sm map[string]any) {
	contentTypeID, _ := sm["contentTypeId"].(string)
	fieldTypes := slideFieldTypes[contentTypeID]

	values, _ := sm["values"].(map[string]any)
	if values == nil {
		values = map[string]any{}
		sm["values"] = values
	}

	// 1. Bindings — resolve against the linked card, bound value wins.
	linkedID, _ := sm["cardId"].(string)
	if bindings, ok := sm["bindings"].(map[string]any); ok && linkedID != "" {
		if linked, err := r.Card.Get(linkedID); err == nil && linked != nil {
			for fieldKey, rawBlockID := range bindings {
				blockID, _ := rawBlockID.(string)
				block := findBlock(linked.Blocks, blockID)
				if block == nil {
					continue
				}
				ft := "text"
				if fieldTypes != nil && fieldTypes[fieldKey] != "" {
					ft = fieldTypes[fieldKey]
				}
				if v := blockValueForField(block, ft); v != "" {
					values[fieldKey] = v
				}
			}
		}
	}

	// 2. Attachment refs on media fields → signed URLs. Image values may be
	// multi-URL (newline-joined — a gallery carousel); sign line by line.
	for fieldKey, raw := range values {
		s, ok := raw.(string)
		if !ok || !strings.Contains(s, "attachment:") {
			continue
		}
		if ft := fieldTypes[fieldKey]; ft != "image" && ft != "video" {
			continue
		}
		lines := strings.Split(s, "\n")
		for i, line := range lines {
			if !strings.HasPrefix(line, "attachment:") {
				continue
			}
			if signed, ok := r.signAttachmentRef(line); ok {
				lines[i] = signed
			}
		}
		values[fieldKey] = strings.Join(lines, "\n")
	}
}

// signAttachmentRef converts "attachment:<cardID>/<attachmentID>" into a
// signed, server-relative attachment URL with the present-length TTL.
func (r *Runtime) signAttachmentRef(ref string) (string, bool) {
	rest := strings.TrimPrefix(ref, "attachment:")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	ownerCardID, attID := parts[0], parts[1]
	if r.repo == nil || r.repo.Manifest == nil {
		return "", false
	}
	exp := time.Now().Add(presentTTL).Unix()
	sig := transporthttp.SignAttachmentMAC(r.secret, ownerCardID, attID, exp)
	return fmt.Sprintf("/repos/%s/attachments/%s/%s?exp=%d&sig=%s",
		r.repo.Manifest.ID, ownerCardID, attID, exp, hex.EncodeToString(sig)), true
}

func findBlock(blocks []model.Block, id string) *model.Block {
	for i := range blocks {
		if blocks[i].ID == id {
			return &blocks[i]
		}
	}
	return nil
}

// blockValueForField mirrors shared/slideBindings.ts resolveBlockValueForField:
// URL(s) for media fields — image fields join every item URL with '\n' (a
// multi-image block renders as a carousel), video stays first-URL — and
// readable text otherwise.
func blockValueForField(b *model.Block, fieldType string) string {
	v := b.Value
	if fieldType == "image" || fieldType == "video" {
		return urlFromBlockValue(v, fieldType == "image")
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "Yes"
		}
		return "No"
	case []any:
		var texts []string
		for _, it := range t {
			if im, ok := it.(map[string]any); ok {
				if txt, ok := im["text"].(string); ok && txt != "" {
					texts = append(texts, txt)
				}
			}
		}
		return strings.Join(texts, "\n")
	case map[string]any:
		if u, ok := t["url"].(string); ok {
			return u
		}
	}
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func urlFromBlockValue(v any, multi bool) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		if u, ok := t["url"].(string); ok {
			return u
		}
	case []any:
		var urls []string
		for _, it := range t {
			if im, ok := it.(map[string]any); ok {
				if u, ok := im["url"].(string); ok && u != "" {
					urls = append(urls, u)
				}
			}
		}
		if len(urls) == 0 {
			return ""
		}
		if multi {
			return strings.Join(urls, "\n")
		}
		return urls[0]
	}
	return ""
}
