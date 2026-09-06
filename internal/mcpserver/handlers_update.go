package mcpserver

// Tools that operate on an existing card beyond its blocks: intrinsic
// properties (title, description, type, due date), attachments,
// comments, filing (pin/unpin) and browsing (list_cards, recent_cards).
// Added 2026-09-06 after the first real MCP session had to fall back to
// raw RPC for a description and an attachment.

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"bruv/core/supervisor"
	"bruv/internal/mcp"
	"bruv/internal/model"
)

// maxAttachmentBytes caps a decoded MCP attachment. The transport already
// limits a POST body to maxBody (5 MiB); base64 inflates by a third, so
// anything larger than this cannot arrive in one message anyway.
const maxAttachmentBytes = 3 * 1024 * 1024

// defaultCommentAuthor is used when the client doesn't name one. The
// MCP handshake is stateless per request, so clientInfo isn't available
// here; callers that care pass `author` explicitly.
const defaultCommentAuthor = "MCP"

// --- Intrinsic card properties ---

func hSetCardTitle(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	cardID, title := argStr(a, "card_id"), strings.TrimSpace(argStr(a, "title"))
	if cardID == "" || title == "" {
		return errResult("card_id and title are required")
	}
	card, err := rt.UpdateCardTitle(cardID, title)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(map[string]any{"card_id": card.ID, "title": card.Title})
}

func hSetCardDescription(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	if cardID == "" {
		return errResult("card_id is required")
	}
	desc, ok := a["description"].(string)
	if !ok {
		return errResult("description is required (pass an empty string to clear it)")
	}
	card, err := rt.UpdateCardDescription(cardID, desc)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(map[string]any{"card_id": card.ID, "description_length": len(card.Description)})
}

func hSetCardType(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	cardID, cardType := argStr(a, "card_id"), strings.TrimSpace(argStr(a, "card_type"))
	if cardID == "" || cardType == "" {
		return errResult("card_id and card_type are required")
	}
	card, err := rt.UpdateCardType(cardID, cardType)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(map[string]any{"card_id": card.ID, "type": card.Type})
}

func hSetCardDueDate(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	if cardID == "" {
		return errResult("card_id is required")
	}
	dueDate, ok := a["due_date"].(string)
	if !ok {
		return errResult("due_date is required: YYYY-MM-DD, or an empty string to clear")
	}
	dueDate = strings.TrimSpace(dueDate)
	if dueDate != "" {
		if _, err := time.Parse("2006-01-02", dueDate); err != nil {
			return errResult("due_date %q is not YYYY-MM-DD", dueDate)
		}
	}
	card, err := rt.UpdateCardDueDate(cardID, dueDate)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(map[string]any{"card_id": card.ID, "due_date": card.DueDate})
}

// --- Attachments ---

func hAddCardAttachment(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	cardID, name := argStr(a, "card_id"), strings.TrimSpace(argStr(a, "name"))
	if cardID == "" || name == "" {
		return errResult("card_id and name are required")
	}
	if strings.ContainsAny(name, `/\`) {
		return errResult("name must be a bare file name, not a path")
	}
	text, hasText := a["text"].(string)
	b64, hasB64 := a["content_base64"].(string)
	switch {
	case hasText == hasB64:
		return errResult("pass exactly one of text (UTF-8 file content) or content_base64")
	case hasText:
		if len(text) > maxAttachmentBytes {
			return errResult("attachment exceeds %d bytes", maxAttachmentBytes)
		}
		b64 = base64.StdEncoding.EncodeToString([]byte(text))
	default:
		if base64.StdEncoding.DecodedLen(len(b64)) > maxAttachmentBytes {
			return errResult("attachment exceeds %d bytes", maxAttachmentBytes)
		}
	}
	card, err := rt.AddCardAttachment(cardID, name, b64)
	if err != nil {
		return errResult("%v", err)
	}
	if len(card.FileAttachments) == 0 {
		return errResult("attachment was not recorded on the card")
	}
	att := card.FileAttachments[len(card.FileAttachments)-1]
	return jsonResult(map[string]any{
		"card_id": card.ID, "attachment_id": att.ID, "name": att.Name, "mime": att.Mime, "size": att.Size,
	})
}

// --- Comments ---

func hAddCardComment(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	cardID, text := argStr(a, "card_id"), strings.TrimSpace(argStr(a, "text"))
	if cardID == "" || text == "" {
		return errResult("card_id and text are required")
	}
	author := strings.TrimSpace(argStr(a, "author"))
	if author == "" {
		author = defaultCommentAuthor
	}
	comment, err := rt.AddCardComment(cardID, author, text)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(comment)
}

func hListCardComments(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	if cardID == "" {
		return errResult("card_id is required")
	}
	comments, err := rt.ListCardComments(cardID)
	if err != nil {
		return errResult("%v", err)
	}
	if comments == nil {
		comments = []model.Comment{}
	}
	return jsonResult(comments)
}

// --- Filing ---

func hPinCard(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	brand, stream := argStr(a, "brand"), argStr(a, "stream")
	project, category := argStr(a, "project"), argStr(a, "category")
	if cardID == "" || brand == "" || stream == "" || project == "" || category == "" {
		return errResult("card_id, brand, stream, project and category are required")
	}
	if _, err := rt.GetCard(cardID); err != nil {
		return errResult("%v", err)
	}
	catID, breadcrumb, err := resolveOrCreateHierarchy(rt, brand, stream, project, category)
	if err != nil {
		return errResult("%v", err)
	}
	if err := rt.PinCard(cardID, catID); err != nil {
		return errResult("pin card: %v", err)
	}
	return jsonResult(map[string]any{"card_id": cardID, "pinned_to": breadcrumb, "category_id": catID})
}

func hUnpinCard(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	brand, stream := argStr(a, "brand"), argStr(a, "stream")
	project, category := argStr(a, "project"), argStr(a, "category")
	if cardID == "" || brand == "" || stream == "" || project == "" || category == "" {
		return errResult("card_id, brand, stream, project and category are required")
	}
	cat, breadcrumb, err := resolveExistingCategory(rt, brand, stream, project, category)
	if err != nil {
		return errResult("%v", err)
	}
	if err := rt.UnpinCard(cardID, cat.ID); err != nil {
		return errResult("unpin card: %v", err)
	}
	return jsonResult(map[string]any{"card_id": cardID, "unpinned_from": breadcrumb})
}

// --- Browsing ---

// cardSummary is the compact per-card shape list_cards returns: enough to
// pick a card without paying for its blocks. Fetch details with get_card.
type cardSummary struct {
	CardID   string     `json:"card_id"`
	Title    string     `json:"title"`
	Type     string     `json:"type"`
	Position int        `json:"position"`
	DueDate  *time.Time `json:"due_date,omitempty"`
	Tags     []string   `json:"tags,omitempty"`
}

type categoryCards struct {
	Category   string        `json:"category"`
	CategoryID string        `json:"category_id"`
	Cards      []cardSummary `json:"cards"`
}

func hListCards(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	brand, stream, project := argStr(a, "brand"), argStr(a, "stream"), argStr(a, "project")
	if brand == "" || stream == "" || project == "" {
		return errResult("brand, stream and project are required (category is optional)")
	}
	brandSlug, _, ok := resolveBrandSlug(rt, brand)
	if !ok {
		return errResult("brand %q not found", brand)
	}
	streamSlug, _, ok := resolveStreamSlug(rt, brandSlug, stream)
	if !ok {
		return errResult("stream %q not found", stream)
	}
	projectSlug, _, ok := resolveProjectSlug(rt, brandSlug, streamSlug, project)
	if !ok {
		return errResult("project %q not found", project)
	}
	cats, err := rt.ListCategories(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return errResult("%v", err)
	}
	onlyCategory := argStr(a, "category")
	out := []categoryCards{}
	for _, cat := range cats {
		if onlyCategory != "" && !strings.EqualFold(cat.Name, onlyCategory) && !strings.EqualFold(cat.Slug, onlyCategory) {
			continue
		}
		pins, err := rt.ListCategoryCards(cat.ID)
		if err != nil {
			return errResult("list category %q: %v", cat.Name, err)
		}
		// The store orders by Position only, and Position is per-card (the
		// index within that card's own pin file), so cards pinned fresh
		// into the same category all tie at 0 and fall back to directory
		// order. Break ties by pin time, then id, so repeated calls agree.
		sort.SliceStable(pins, func(i, j int) bool {
			if pins[i].Position != pins[j].Position {
				return pins[i].Position < pins[j].Position
			}
			if !pins[i].PinnedAt.Equal(pins[j].PinnedAt) {
				return pins[i].PinnedAt.Before(pins[j].PinnedAt)
			}
			return pins[i].CardID < pins[j].CardID
		})
		entry := categoryCards{Category: cat.Name, CategoryID: cat.ID, Cards: []cardSummary{}}
		for _, p := range pins {
			card, err := rt.GetCard(p.CardID)
			if err != nil {
				continue // a dangling pin shouldn't sink the whole listing
			}
			entry.Cards = append(entry.Cards, cardSummary{
				CardID: card.ID, Title: card.Title, Type: card.Type,
				Position: p.Position, DueDate: card.DueDate, Tags: card.Tags,
			})
		}
		out = append(out, entry)
	}
	if onlyCategory != "" && len(out) == 0 {
		return errResult("category %q not found in that project", onlyCategory)
	}
	return jsonResult(out)
}

func hRecentCards(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	limit := argInt(a, "limit", 20)
	if limit <= 0 {
		limit = 20
	}
	results, err := rt.RecentCards(limit)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(results)
}

// resolveExistingCategory walks Brand → Stream → Project → Category
// WITHOUT creating anything — the counterpart to resolveOrCreateHierarchy
// for tools where a typo must fail rather than spawn a new column.
func resolveExistingCategory(rt *supervisor.Runtime, brand, stream, project, category string) (*model.Category, string, error) {
	brandSlug, brandName, ok := resolveBrandSlug(rt, brand)
	if !ok {
		return nil, "", fmt.Errorf("brand %q not found", brand)
	}
	streamSlug, streamName, ok := resolveStreamSlug(rt, brandSlug, stream)
	if !ok {
		return nil, "", fmt.Errorf("stream %q not found", stream)
	}
	projectSlug, projectName, ok := resolveProjectSlug(rt, brandSlug, streamSlug, project)
	if !ok {
		return nil, "", fmt.Errorf("project %q not found", project)
	}
	cats, _ := rt.ListCategories(brandSlug, streamSlug, projectSlug)
	for i := range cats {
		if strings.EqualFold(cats[i].Name, category) || strings.EqualFold(cats[i].Slug, category) {
			breadcrumb := strings.Join([]string{brandName, streamName, projectName, cats[i].Name}, " / ")
			return &cats[i], breadcrumb, nil
		}
	}
	return nil, "", fmt.Errorf("category %q not found in %s / %s / %s", category, brandName, streamName, projectName)
}

// --- Attachment download ---

// maxInlineDownloadBytes is the largest attachment returned inline. Above
// it the tool hands back metadata plus a signed URL so a client with the
// server's base address can fetch it over plain HTTP instead of pushing
// megabytes of base64 through the model's context.
const maxInlineDownloadBytes = 4 * 1024 * 1024

func hGetCardAttachment(rt *supervisor.Runtime, a map[string]any) mcp.CallToolResult {
	cardID := argStr(a, "card_id")
	attID, name := argStr(a, "attachment_id"), strings.TrimSpace(argStr(a, "name"))
	if cardID == "" || (attID == "" && name == "") {
		return textResult("error: card_id plus attachment_id or name are required", true)
	}
	if attID == "" {
		card, err := rt.GetCard(cardID)
		if err != nil {
			return textResult("error: "+err.Error(), true)
		}
		// Newest match wins when the same name was attached more than once.
		for i := len(card.FileAttachments) - 1; i >= 0; i-- {
			if strings.EqualFold(card.FileAttachments[i].Name, name) {
				attID = card.FileAttachments[i].ID
				break
			}
		}
		if attID == "" {
			return textResult(fmt.Sprintf("error: no attachment named %q on card %s", name, cardID), true)
		}
	}
	data, att, err := rt.ReadCardAttachment(cardID, attID)
	if err != nil {
		return textResult("error: "+err.Error(), true)
	}
	meta := map[string]any{
		"card_id": cardID, "attachment_id": att.ID, "name": att.Name,
		"mime": att.Mime, "size": att.Size, "added_at": att.AddedAt,
	}
	if len(data) > maxInlineDownloadBytes {
		if url, err := rt.SignAttachmentURL(cardID, att.ID); err == nil {
			meta["download_url"] = url
			meta["download_url_note"] = "server-relative path, valid for 5 minutes; prepend the BRUV server's scheme://host"
		}
		meta["inline"] = false
		text, _ := jsonResult(meta)
		return textResult(text, false)
	}
	meta["inline"] = true
	metaText, _ := jsonResult(meta)
	uri := fmt.Sprintf("bruv://cards/%s/attachments/%s", cardID, att.ID)

	var body mcp.Content
	if isTextAttachment(att.Mime, data) {
		// Plain text content is the most widely rendered block type, so a
		// spec or a Markdown doc lands straight in the model's context.
		body = mcp.Content{Type: "text", Text: string(data)}
	} else {
		body = mcp.Content{Type: "resource", Resource: &mcp.Resource{
			URI: uri, MimeType: att.Mime, Blob: base64.StdEncoding.EncodeToString(data),
		}}
	}
	return mcp.CallToolResult{Content: []mcp.Content{body, {Type: "text", Text: metaText}}}
}

// isTextAttachment decides whether to return an attachment as text.
// Stored MIME types come from the file extension and default to
// octet-stream for anything unlisted (Markdown included), so the bytes
// get the final say: valid UTF-8 that isn't a known binary type is text.
func isTextAttachment(mime string, data []byte) bool {
	switch {
	case strings.HasPrefix(mime, "image/"), strings.HasPrefix(mime, "video/"),
		strings.HasPrefix(mime, "audio/"), mime == "application/pdf", mime == "application/zip":
		return false
	}
	return utf8.Valid(data)
}
