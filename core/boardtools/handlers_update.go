package boardtools

// Tools that operate on an existing card beyond its blocks: intrinsic
// properties (title, description, type, due date), attachments,
// comments, filing (pin/unpin) and browsing (list_cards, recent_cards).
// Added 2026-09-06 after the first real MCP session had to fall back to
// raw RPC for a description and an attachment.

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	cardtools "bruv/core/runtime/tools"
	"bruv/internal/mcp"
	"bruv/internal/model"
)

// maxAttachmentBytes caps a decoded MCP attachment. The transport already
// limits a POST body to maxBody (5 MiB); base64 inflates by a third, so
// anything larger than this cannot arrive in one message anyway.
const maxAttachmentBytes = 3 * 1024 * 1024

// DefaultCommentAuthor is used when the client doesn't name one. The
// MCP handshake is stateless per request, so clientInfo isn't available
// here; callers that care pass `author` explicitly.
const DefaultCommentAuthor = "MCP"

// --- Intrinsic card properties ---

func hSetCardTitle(rt Board, a map[string]any) (string, bool) {
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

func hSetCardDescription(rt Board, a map[string]any) (string, bool) {
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

func hSetCardType(rt Board, a map[string]any) (string, bool) {
	cardID, input := argStr(a, "card_id"), argStr(a, "card_type")
	if cardID == "" || input == "" {
		return errResult("card_id and card_type are required")
	}
	// Check the card first so a typo'd id can't leave a stray new type.
	if _, err := rt.GetCard(cardID); err != nil {
		return errResult("%v", err)
	}
	cardType, typeCreated, err := rt.ResolveOrCreateCardType(input)
	if err != nil {
		return errResult("%v", err)
	}
	card, err := rt.UpdateCardType(cardID, cardType)
	if err != nil {
		return errResult("%v", err)
	}
	out := map[string]any{"card_id": card.ID, "type": card.Type}
	if typeCreated {
		out["type_created"] = true
	}
	return jsonResult(out)
}

// hUpdateCard is the general "update a card" tool: intrinsic fields plus
// block values by key or label (a new key adds a text block). The update
// rules are tools.ApplyCardUpdates — the agent's update_self uses the same
// ones; each changed part is saved through the card service so events and
// the activity log see every edit.
func hUpdateCard(rt Board, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	if cardID == "" {
		return errResult("card_id is required")
	}
	before, err := rt.GetCard(cardID)
	if err != nil {
		return errResult("%v", err)
	}
	after, err := rt.GetCard(cardID) // an independent copy to mutate
	if err != nil {
		return errResult("%v", err)
	}
	if err := cardtools.ApplyCardUpdates(after, a); err != nil {
		return errResult("%v", err)
	}
	changed := []string{}
	// Each part is saved in turn; on a failure the parts already listed
	// in `changed` have been saved and the rest haven't.
	fail := func(field string, err error) (string, bool) {
		return errResult("set %s: %v (already saved: %v)", field, err, changed)
	}
	if after.Title != before.Title {
		if _, err := rt.UpdateCardTitle(cardID, after.Title); err != nil {
			return fail("title", err)
		}
		changed = append(changed, "title")
	}
	if after.Description != before.Description {
		if _, err := rt.UpdateCardDescription(cardID, after.Description); err != nil {
			return fail("description", err)
		}
		changed = append(changed, "description")
	}
	if !sameDate(before.DueDate, after.DueDate) {
		due := ""
		if after.DueDate != nil {
			due = after.DueDate.Format("2006-01-02")
		}
		if _, err := rt.UpdateCardDueDate(cardID, due); err != nil {
			return fail("due_date", err)
		}
		changed = append(changed, "due_date")
	}
	if !slices.Equal(before.Tags, after.Tags) {
		if _, err := rt.UpdateCardTags(cardID, after.Tags); err != nil {
			return fail("tags", err)
		}
		changed = append(changed, "tags")
	}
	if !reflect.DeepEqual(before.Blocks, after.Blocks) {
		if _, err := rt.UpdateCardBlocks(cardID, after.Blocks); err != nil {
			return fail("blocks", err)
		}
		changed = append(changed, "blocks")
	}
	return jsonResult(map[string]any{"card_id": cardID, "updated": changed})
}

func sameDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func hSetCardDueDate(rt Board, a map[string]any) (string, bool) {
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

func hAddCardAttachment(rt Board, a map[string]any) (string, bool) {
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

func hAddCardComment(rt Board, a map[string]any) (string, bool) {
	cardID, text := argStr(a, "card_id"), strings.TrimSpace(argStr(a, "text"))
	if cardID == "" || text == "" {
		return errResult("card_id and text are required")
	}
	author := strings.TrimSpace(argStr(a, "author"))
	if author == "" {
		author = DefaultCommentAuthor
	}
	comment, err := rt.AddCardComment(cardID, author, text)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(comment)
}

func hListCardComments(rt Board, a map[string]any) (string, bool) {
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

func hPinCard(rt Board, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	brand, stream := argStr(a, "brand"), argStr(a, "stream")
	project, category := argStr(a, "project"), argStr(a, "category")
	if cardID == "" || brand == "" || stream == "" || project == "" || category == "" {
		return errResult("card_id, brand, stream, project and category are required")
	}
	if _, err := rt.GetCard(cardID); err != nil {
		return errResult("%v", err)
	}
	catID, breadcrumb, err := cardtools.ResolveOrCreateCategory(rt.ProjectService(), cardtools.Location{Brand: brand, Stream: stream, Project: project, Category: category})
	if err != nil {
		return errResult("%v", err)
	}
	if err := rt.PinCard(cardID, catID); err != nil {
		return errResult("pin card: %v", err)
	}
	return jsonResult(map[string]any{"card_id": cardID, "pinned_to": breadcrumb, "category_id": catID})
}

func hUnpinCard(rt Board, a map[string]any) (string, bool) {
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

func hListCards(rt Board, a map[string]any) (string, bool) {
	board, err := cardtools.ListBoard(rt.Repo(), rt.ProjectService(), cardtools.LocationArgs(a))
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(board)
}

func hRecentCards(rt Board, a map[string]any) (string, bool) {
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
// WITHOUT creating anything — the counterpart to ResolveOrCreateCategory
// for tools where a typo must fail rather than spawn a new column.
func resolveExistingCategory(rt Board, brand, stream, project, category string) (*model.Category, string, error) {
	brandSlug, brandName, ok := cardtools.FindBrand(rt.ProjectService(), brand)
	if !ok {
		return nil, "", fmt.Errorf("brand %q not found", brand)
	}
	streamSlug, streamName, ok := cardtools.FindStream(rt.ProjectService(), brandSlug, stream)
	if !ok {
		return nil, "", fmt.Errorf("stream %q not found", stream)
	}
	projectSlug, projectName, ok := cardtools.FindProject(rt.ProjectService(), brandSlug, streamSlug, project)
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

func hGetCardAttachment(rt Board, a map[string]any) mcp.CallToolResult {
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
