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
	transporthttp "bruv/transport/http"
)

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
}

// PresentCardJSON returns the card as JSON with all slide-deck bindings and
// attachment references resolved for the present page. ok=false when the
// card doesn't exist. Plugged into transport/http.PresentConfig by the hosts.
func (r *Runtime) PresentCardJSON(cardID string) ([]byte, bool) {
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

	// 2. Attachment refs on media fields → signed URLs.
	for fieldKey, raw := range values {
		s, ok := raw.(string)
		if !ok || !strings.HasPrefix(s, "attachment:") {
			continue
		}
		if ft := fieldTypes[fieldKey]; ft != "image" && ft != "video" {
			continue
		}
		if signed, ok := r.signAttachmentRef(s); ok {
			values[fieldKey] = signed
		}
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
// a URL for media fields, readable text otherwise.
func blockValueForField(b *model.Block, fieldType string) string {
	v := b.Value
	if fieldType == "image" || fieldType == "video" {
		return urlFromBlockValue(v)
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

func urlFromBlockValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		if u, ok := t["url"].(string); ok {
			return u
		}
	case []any:
		if len(t) > 0 {
			if im, ok := t[0].(map[string]any); ok {
				if u, ok := im["url"].(string); ok {
					return u
				}
			}
		}
	}
	return ""
}
