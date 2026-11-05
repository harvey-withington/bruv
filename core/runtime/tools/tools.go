package tools

// LLM tool implementations and staging.
//
// This is the execution surface for every tool the chat/agent system
// calls. Two dispatchers live here:
//
//   - ExecuteCard: the edit-mode card-chat executor. Mutates the
//     current card directly and returns (result, action, pin suggestion)
//     so the chat loop can log the action and surface suggestions.
//   - ExecuteProject: the project-chat executor. Operates on a
//     ProjectChatScope (see app_chat.go) and refuses to touch cards
//     outside scope to defend against LLM hallucinations.
//
// Plus two staging paths for suggest mode (StageCard,
// StageProject) that convert a tool call into PendingEdits on
// the chat message. Applying those later goes back through the
// executor — see app_pending.go.
//
// Value coercion (coerceBlockValue and friends) also lives here because
// every tool that writes to a block has to funnel its input through
// coercion first. LLMs send numbers as strings, booleans as "yes"/"no",
// checklists as comma-separated strings; the coercion layer normalises
// all of that so the block renderer doesn't have to defend against it.
//
// Extracted from app.go so tool additions, §13 dispatch-table work, and
// prompt tuning don't collide in the same 7k-line file.

import (
	"bruv/internal/llm"
	"bruv/internal/model"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// humanizeBlockKey converts "recording_status" → "Recording Status".
func humanizeBlockKey(key string) string {
	words := strings.Split(key, "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
func coerceBlockValue(blockType string, val any) any {
	switch blockType {
	case model.BlockChecklist:
		return coerceChecklist(val)
	case model.BlockList:
		return coerceList(val)
	case model.BlockCheckbox:
		return coerceCheckbox(val)
	case model.BlockNumber:
		return coerceNumber(val)
	case model.BlockSlideDeck:
		return coerceSlideDeck(val)
	default:
		// text, select, radio, date, url, image, video — all string, pass through
		return val
	}
}

// coerceBlockValueForBlock is the meta-aware variant: given a full block,
// apply the same type coercion AND additional constraints that need the
// block's Meta (select/radio allowed options, rating max). Used when the
// caller has the whole block in hand, such as update_self targeting an
// existing block on the card.
//
// Returns the coerced value and optionally an error describing a
// constraint violation. On a constraint violation we return the coerced
// value anyway (best effort — a bad select value is rendered as plain
// text, not corruption) so callers can still write it if they choose.
func CoerceBlockValueForBlock(b *model.Block, val any) (any, error) {
	coerced := coerceBlockValue(b.Type, val)

	switch b.Type {
	case model.BlockDate:
		// Normalise whatever date/timestamp the LLM sent into a shape
		// the frontend input can render. LLMs produce all sorts of
		// inputs — "2026-04-12", "2026-04-12T01:45:09+07:00", "April 12
		// 2026", "now" — and passing any of those through verbatim to
		// an <input type="date"> leaves the field empty.
		s, ok := coerced.(string)
		if !ok || s == "" {
			return coerced, nil
		}
		format := ""
		if b.Meta != nil {
			format, _ = b.Meta["format"].(string)
		}
		normalised, err := normaliseDateValue(s, format)
		if err != nil {
			return coerced, fmt.Errorf("date block: could not parse %q: %v", s, err)
		}
		return normalised, nil

	case model.BlockSelect, model.BlockRadio:
		// If the block has an options list, the value must be one of
		// them. LLMs frequently invent options that aren't configured.
		s, ok := coerced.(string)
		if !ok {
			return coerced, nil
		}
		opts := extractBlockOptions(b.Meta)
		if len(opts) == 0 {
			return coerced, nil // no constraint configured
		}
		for _, o := range opts {
			if o == s {
				return coerced, nil
			}
		}
		return coerced, fmt.Errorf("value %q is not in the allowed options %v", s, opts)

	case model.BlockRating:
		// Clamp to [0, max] where max defaults to 5. Also coerces
		// string → float64 via the existing number path.
		n, ok := coerced.(float64)
		if !ok {
			// coerceBlockValue only converts BlockNumber; rating goes
			// through the passthrough branch. Try harder here.
			if parsed := coerceNumber(val); parsed != 0 || val == float64(0) || val == "0" {
				n = parsed
				ok = true
			}
		}
		if !ok {
			return coerced, nil
		}
		maxRating := 5.0
		if b.Meta != nil {
			if m, ok := b.Meta["max"].(float64); ok && m > 0 {
				maxRating = m
			} else if m, ok := b.Meta["max"].(int); ok && m > 0 {
				maxRating = float64(m)
			}
		}
		if n < 0 {
			n = 0
		}
		if n > maxRating {
			n = maxRating
		}
		return n, nil

	case model.BlockProgress:
		// Progress is conceptually 0–100. Same clamping treatment.
		n, ok := coerced.(float64)
		if !ok {
			if parsed := coerceNumber(val); parsed != 0 || val == float64(0) || val == "0" {
				n = parsed
				ok = true
			}
		}
		if !ok {
			return coerced, nil
		}
		if n < 0 {
			n = 0
		}
		if n > 100 {
			n = 100
		}
		return n, nil
	}

	return coerced, nil
}

// normaliseDateValue takes an LLM-supplied date/timestamp string and
// returns it in a form the frontend DateBlock can parse:
//
//   - format == "date-time": full ISO 8601 with timezone (RFC 3339)
//   - format == "" or "date": YYYY-MM-DD only
//
// Accepts a wide range of inputs — full RFC 3339, just a date,
// Go's RFC3339Nano, or a Unix-ish "2006-01-02 15:04:05" — and fails
// loudly for anything it can't parse so the caller can surface a
// useful error to the LLM.
func normaliseDateValue(raw, format string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	// Try the common formats in order of specificity. time.Parse returns
	// on the first match.
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
	}
	// A zone-less date-time is a wall-clock time, so it is read in the
	// server's local zone (as agentsvc's timeArg does), never as UTC.
	var parsed time.Time
	var parseErr error
	for _, layout := range layouts {
		parsed, parseErr = time.ParseInLocation(layout, raw, time.Local)
		if parseErr == nil {
			break
		}
	}
	if parseErr != nil {
		return "", parseErr
	}

	if format == "date-time" {
		return parsed.Format(time.RFC3339), nil
	}
	return parsed.Format("2006-01-02"), nil
}

// extractBlockOptions pulls the string options list out of a block's
// Meta map. Options are stored as []any of strings; this flattens that
// into a plain []string for easy comparison.
func extractBlockOptions(meta map[string]any) []string {
	if meta == nil {
		return nil
	}
	raw, ok := meta["options"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, o := range raw {
		if s, ok := o.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// CoerceChecklistItems is coerceChecklist for the board tools'
// add_checklist_items: the items to append, in the stored shape.
func CoerceChecklistItems(val any) []map[string]any { return coerceChecklist(val) }

// coerceChecklist converts []any of strings, []any of {id?, text, done}
// maps, or a single newline-separated string into [{id, text, done}]. A
// supplied id is kept — items are ID-keyed state (a rewrite of the list
// must not orphan whatever refers to an item); only new items get one.
func coerceChecklist(val any) []map[string]any {
	var items []map[string]any
	add := func(id, text string, done bool) {
		if id == "" {
			id = fmt.Sprintf("cli-%s", uuid.New().String()[:8])
		}
		items = append(items, map[string]any{"id": id, "text": text, "done": done})
	}
	switch v := val.(type) {
	case []any:
		for _, item := range v {
			switch it := item.(type) {
			case string:
				if it != "" {
					add("", it, false)
				}
			case map[string]any:
				text, _ := it["text"].(string)
				if text == "" {
					continue
				}
				done, _ := it["done"].(bool)
				add(itemID(it), text, done)
			}
		}
	case string:
		for _, line := range strings.Split(v, "\n") {
			line = stripListPrefix(line)
			if line != "" {
				add("", line, false)
			}
		}
	}
	if items == nil {
		items = []map[string]any{}
	}
	return items
}

// coerceList converts []any of strings, []any of {id?, text} maps, or a
// single newline-separated string into [{id, text}]. This is the fix for
// the bug where agent `update_self` was writing plain strings directly to
// list blocks, which the frontend renderer couldn't parse. A supplied id
// is kept, as for checklists.
func coerceList(val any) []map[string]any {
	var items []map[string]any
	add := func(id, text string) {
		if id == "" {
			id = fmt.Sprintf("li-%s", uuid.New().String()[:8])
		}
		items = append(items, map[string]any{"id": id, "text": text})
	}
	switch v := val.(type) {
	case []any:
		for _, item := range v {
			switch it := item.(type) {
			case string:
				if it != "" {
					add("", it)
				}
			case map[string]any:
				if text, ok := it["text"].(string); ok && text != "" {
					add(itemID(it), text)
				}
			}
		}
	case string:
		// Fallback for LLMs that send a formatted string instead of an
		// array. Newline-split and strip markdown bullets so the result
		// is a clean list.
		for _, line := range strings.Split(v, "\n") {
			line = stripListPrefix(line)
			if line != "" {
				add("", line)
			}
		}
	}
	if items == nil {
		items = []map[string]any{}
	}
	return items
}

// itemID is a list/checklist item's supplied id, or "".
func itemID(item map[string]any) string {
	id, _ := item["id"].(string)
	return strings.TrimSpace(id)
}

// slideContentTypeFields lists the allowed field keys per built-in content
// type, mirroring shared/slideContentTypes.ts, so AI-authored slides keep only
// recognised fields. An unknown content type passes its values through as-is.
var slideContentTypeFields = map[string][]string{
	"title":       {"title", "subtitle"},
	"statement":   {"statement"},
	"quote":       {"quote", "author"},
	"image":       {"image", "caption"},
	"video":       {"video", "caption"},
	"lower_third": {"name", "subtitle"},
	"post":        {"author", "handle", "avatar", "text", "media", "video", "date", "url", "platform"},
}

func sliceContains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// coerceSlideDeck normalises an AI/MCP-authored slide_deck value into the
// {slides:[{id,contentTypeId,values,...}]} shape the frontend expects.
// Accepts the full object, a bare array of slides, or bare strings (each
// becomes a title slide), and stamps a stable id on any slide missing one.
// A supplied currentIndex is dropped: live presentation position is block
// live state (SetBlockLiveState), not persisted card content.
func coerceSlideDeck(val any) map[string]any {
	var rawSlides []any
	var theme any
	switch v := val.(type) {
	case map[string]any:
		rawSlides, _ = v["slides"].([]any)
		theme = v["theme"]
	case []any:
		rawSlides = v
	}
	slides := make([]map[string]any, 0, len(rawSlides))
	for _, item := range rawSlides {
		m, ok := item.(map[string]any)
		if !ok {
			// A bare string becomes a title slide carrying that text.
			s, isStr := item.(string)
			if !isStr || strings.TrimSpace(s) == "" {
				continue
			}
			m = map[string]any{"contentTypeId": "title", "values": map[string]any{"title": s}}
		}
		slides = append(slides, coerceSlide(m))
	}
	out := map[string]any{"slides": slides}
	if t, ok := theme.(map[string]any); ok {
		out["theme"] = t
	}
	return out
}

// coerceSlide normalises one slide map: stable id, a content type (default
// "title"), a string→string values map filtered to the content type's known
// fields, and pass-through of the optional reference/meta fields.
func coerceSlide(m map[string]any) map[string]any {
	id, _ := m["id"].(string)
	if strings.TrimSpace(id) == "" {
		id = fmt.Sprintf("sld-%s", uuid.New().String()[:8])
	}
	contentTypeID, _ := m["contentTypeId"].(string)
	contentTypeID = strings.TrimSpace(contentTypeID)
	if contentTypeID == "" {
		contentTypeID = "title"
	}
	values := map[string]any{}
	if rawVals, ok := m["values"].(map[string]any); ok {
		allowed := slideContentTypeFields[contentTypeID]
		for k, v := range rawVals {
			if allowed != nil && !sliceContains(allowed, k) {
				continue
			}
			if s, ok := v.(string); ok {
				values[k] = s
			}
		}
	}
	slide := map[string]any{
		"id":            id,
		"contentTypeId": contentTypeID,
		"values":        values,
	}
	for _, k := range []string{"templateId", "cardId", "title", "notes", "thumbnail", "overflow"} {
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			slide[k] = s
		}
	}
	if raw, ok := m["bindings"].(map[string]any); ok {
		bindings := map[string]any{}
		for k, v := range raw {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				bindings[k] = s
			}
		}
		if len(bindings) > 0 {
			slide["bindings"] = bindings
		}
	}
	if d := int(coerceNumber(m["durationSec"])); d > 0 {
		slide["durationSec"] = d
	}
	return slide
}

// stripListPrefix normalises a single line by trimming whitespace and
// removing leading markdown list markers like "- ", "* ", "• ", or
// "1. " / "2) ". Shared by coerceList and coerceChecklist.
func stripListPrefix(line string) string {
	line = strings.TrimSpace(line)
	// Numbered prefix: "1. " or "12) "
	if len(line) > 0 && line[0] >= '0' && line[0] <= '9' {
		for i := 0; i < len(line); i++ {
			c := line[i]
			if c >= '0' && c <= '9' {
				continue
			}
			if (c == '.' || c == ')') && i+1 < len(line) && line[i+1] == ' ' {
				line = strings.TrimSpace(line[i+2:])
			}
			break
		}
	}
	line = strings.TrimPrefix(line, "- ")
	line = strings.TrimPrefix(line, "* ")
	line = strings.TrimPrefix(line, "• ")
	return strings.TrimSpace(line)
}

// coerceCheckbox converts string representations ("true", "yes", "1") to bool.
func coerceCheckbox(val any) bool {
	switch v := val.(type) {
	case bool:
		return v
	case string:
		v = strings.ToLower(strings.TrimSpace(v))
		return v == "true" || v == "yes" || v == "1"
	case float64:
		return v != 0
	default:
		return false
	}
}

// coerceNumber converts string representations to float64.
func coerceNumber(val any) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

// cardToolHandler is the signature every entry in cardToolHandlers
// satisfies. Shared inputs cover everything a handler might need
// without callers having to resolve per-tool parameter permutations.
type cardToolHandler func(d *Dispatcher, cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion)

// cardToolHandlers is the dispatch registry for the card-chat
// edit-mode executor. Adding a new card tool means adding an entry
// here plus the implementing func — no switch-case editing required.
//
// Why a var over a method returning the map: this evaluates once at
// startup, not per call. Method values on *Dispatcher are first-class
// functions that accept the receiver as their first argument, so
// the closure cost is zero.
//
// Follow-up candidates: ExecuteProject, StageCard, and
// StageProject all still use switches and should be migrated
// to the same pattern for consistency. Deferred as lower-priority —
// those switches do mostly-trivial PendingEdit wrapping while this
// one ran the real 400-line card-mutation logic the audit called out.
var cardToolHandlers = map[string]cardToolHandler{
	"set_fields":     (*Dispatcher).toolSetFields,
	"update_blocks":  (*Dispatcher).toolSetFields, // alias — same handler
	"add_field":      (*Dispatcher).toolAddField,
	"suggest_pin":    (*Dispatcher).toolSuggestPin,
	"web_fetch":      (*Dispatcher).toolWeb,
	"web_search":     (*Dispatcher).toolWeb,
	"read_card_file": (*Dispatcher).toolReadCardFile,
}

// ExecuteCard runs a single tool and returns (result string, action record, pin suggestion).
func (d *Dispatcher) ExecuteCard(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	if d.isNative(tc.Name) {
		result, action := d.executeNative(d.cardScope(cardID, allCats), tc)
		return result, action, nil
	}
	if handler, ok := cardToolHandlers[tc.Name]; ok {
		return handler(d, cardID, card, tc, allCats)
	}
	return "error: unknown tool " + tc.Name, nil, nil
}

// toolSetFields is card chat's set_fields: an adapter over the native
// set_card_fields (cardtools.ApplyFieldValues) for THIS card. It only adds
// what's chat-specific — the dynamic per-card schema puts field keys at
// the top level, so those are gathered into the fields map first.
func (d *Dispatcher) toolSetFields(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	fields, _ := tc.Arguments["fields"].(map[string]any)
	if len(fields) == 0 {
		fields, _ = tc.Arguments["blocks"].(map[string]any)
	}
	if len(fields) == 0 {
		fields = d.flatFieldArgs(cardID, tc.Arguments)
	}
	if len(fields) == 0 {
		return "error: fields map is empty", nil, nil
	}
	result, action := d.executeNative(d.cardScope(cardID, allCats), llm.ToolCall{
		ID: tc.ID, Name: "set_card_fields", Arguments: map[string]any{"card_id": cardID, "fields": fields},
	})
	action.Tool, action.Input = "set_fields", tc.Arguments
	return result, action, nil
}

// flatFieldArgs picks the top-level arguments that name a field of the
// card — an existing block key or one its type's schema defines.
func (d *Dispatcher) flatFieldArgs(cardID string, args map[string]any) map[string]any {
	c, err := d.deps.Repo().GetCard(cardID)
	if err != nil {
		return nil
	}
	known := map[string]bool{"description": true}
	for _, b := range c.Blocks {
		if b.Key != "" {
			known[b.Key] = true
		}
	}
	if d.deps.Registry() != nil && c.Type != "" {
		for _, b := range d.deps.Registry().SchemaToBlocks(c.Type) {
			known[b.Key] = true
		}
	}
	flat := map[string]any{}
	for k, v := range args {
		if known[k] {
			flat[k] = v
		}
	}
	return flat
}

// toolAddField is card chat's add_field: an adapter over the native
// add_card_blocks for THIS card, limited to the field types a chat request
// can sensibly create (llm.AddFieldTypes).
func (d *Dispatcher) toolAddField(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	key, _ := tc.Arguments["key"].(string)
	label, _ := tc.Arguments["label"].(string)
	fieldType, _ := tc.Arguments["field_type"].(string)
	if key == "" || label == "" || fieldType == "" {
		return "error: key, label, and field_type are required", nil, nil
	}
	if !slices.Contains(llm.AddFieldTypes, fieldType) {
		return "error: invalid field_type " + fieldType + ". Must be one of: " + strings.Join(llm.AddFieldTypes, ", "), nil, nil
	}
	block := map[string]any{"type": fieldType, "label": label, "key": key}
	if v, ok := tc.Arguments["value"]; ok {
		block["value"] = v
	}
	result, action := d.executeNative(d.cardScope(cardID, allCats), llm.ToolCall{
		ID: tc.ID, Name: "add_card_blocks", Arguments: map[string]any{"card_id": cardID, "blocks": []any{block}},
	})
	action.Tool, action.Input = "add_field", tc.Arguments
	if strings.HasPrefix(result, "error") {
		return result, action, nil
	}
	action.Result = fmt.Sprintf("Added field: %s (%s)", label, fieldType)
	return fmt.Sprintf("Added %s field '%s' (key: %s). Use set_fields with key '%s' to update its value.", fieldType, label, key, key), action, nil
}
func (d *Dispatcher) toolSuggestPin(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	catID, _ := tc.Arguments["category_id"].(string)
	reason, _ := tc.Arguments["reason"].(string)
	confidence, _ := tc.Arguments["confidence"].(string)

	// Only Inbox cards get filed by the AI. Checked here, not just when
	// the tool is offered: a Suggest-mode batch can be accepted long after
	// it was staged, and the card may have been filed by hand meanwhile.
	if existing, _ := d.deps.Repo().GetCardPins(cardID); len(existing) > 0 {
		return "error: this card is already filed on a board — the AI never pins a card twice; the user moves cards by hand", nil, nil
	}

	var catName, breadcrumb string

	if catID != "" {
		// Existing category — look it up
		for _, c := range allCats {
			if c.CategoryID == catID {
				catName = c.CategoryName
				breadcrumb = c.Breadcrumb
				break
			}
		}
		if catName == "" {
			return "error: category not found", nil, nil
		}
		// Same refusal the Suggest path gives at staging, so the model is
		// steered the same way (change the type, keep the location) in
		// edit mode. Type read from disk: a set_card_type earlier in this
		// response has already landed there.
		if current, err := d.deps.Repo().GetCard(cardID); err == nil {
			if conflict := PinTypeConflict(allCats, catID, current.Type); conflict != "" {
				return conflict, nil, nil
			}
		}
	} else {
		// Create new hierarchy from brand/stream/project/category names
		brandName, _ := tc.Arguments["brand"].(string)
		streamName, _ := tc.Arguments["stream"].(string)
		projectName, _ := tc.Arguments["project"].(string)
		categoryName, _ := tc.Arguments["category"].(string)
		if brandName == "" || streamName == "" || projectName == "" || categoryName == "" {
			return "error: provide either category_id OR all of brand, stream, project, category names", nil, nil
		}
		resolvedCatID, resolvedBreadcrumb, err := d.resolveOrCreateHierarchy(brandName, streamName, projectName, categoryName)
		if err != nil {
			return "error creating hierarchy: " + err.Error(), nil, nil
		}
		catID = resolvedCatID
		catName = categoryName
		breadcrumb = resolvedBreadcrumb
	}

	// Check if card is already pinned to this category — skip if duplicate
	existingPins, _ := d.deps.Repo().GetCardPins(cardID)
	for _, p := range existingPins {
		if p.CategoryID == catID {
			return "Card is already pinned to " + breadcrumb, nil, nil
		}
	}

	// Edit mode pins directly — consistent with every other card tool
	// (set_title, set_fields, add_tags all mutate without an extra
	// approval step). Suggest mode stages the call via the separate
	// executeToolCallSuggest path, so this branch only runs when the
	// user has opted into direct edits.
	if err := d.deps.Card().Pin(cardID, catID); err != nil {
		return "error pinning card: " + err.Error(), nil, nil
	}
	ps := &model.PinSuggestion{
		CategoryID:   catID,
		CategoryName: catName,
		Breadcrumb:   breadcrumb,
		Reason:       reason,
		Confidence:   confidence,
		Status:       "accepted",
	}
	action := &model.ToolAction{Tool: "suggest_pin", Input: tc.Arguments, Result: "Pinned to " + breadcrumb}
	return "Card pinned to " + breadcrumb, action, ps
}

func (d *Dispatcher) toolWeb(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	result, action, _ := RunWebTool(tc)
	return result, action, nil
}

// ExecuteProject runs a single project-level tool and returns (result, action).
//
// When `scope.CardIDs` is non-nil, any tool call referencing a card_id outside
// the set is rejected with a clear error so the LLM can correct itself. Pass
// a nil cardIDs map to disable scope checking (e.g. for ApplyProjectPendingEdits
// where we recompute scope at apply time).
func (d *Dispatcher) ExecuteProject(tc llm.ToolCall, scope ProjectChatScope) (string, *model.ToolAction) {
	// Validate every card_id mentioned in the call against the project scope.
	// Single id, plural ids, and per-update entries are all checked.
	if scope.CardIDs != nil {
		var bad []string
		check := func(id string) {
			if id != "" && !scope.CardIDs[id] {
				bad = append(bad, id)
			}
		}
		if id, ok := tc.Arguments["card_id"].(string); ok {
			check(id)
		}
		if raw, ok := tc.Arguments["card_ids"].([]any); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					check(s)
				}
			}
		}
		if raw, ok := tc.Arguments["updates"].([]any); ok {
			for _, item := range raw {
				if m, ok := item.(map[string]any); ok {
					if s, ok := m["card_id"].(string); ok {
						check(s)
					}
				}
			}
		}
		if len(bad) > 0 {
			return "error: card(s) not in current project: " + strings.Join(bad, ", "), nil
		}
	}
	if d.isNative(tc.Name) {
		return d.executeNative(&scope, tc)
	}
	if result, action, ok := RunWebTool(tc); ok {
		return result, action
	}
	// Workspace tools are read-only and project-scoped — shared handler.
	if IsWorkspaceTool(tc.Name) {
		return d.execWorkspaceTool(tc, scope)
	}
	switch tc.Name {
	case "add_tags_to_cards":
		return d.addTagsBatch(tc, &scope)

	case "update_cards":
		return d.updateCardsBatch(tc, &scope)

	default:
		return "error: unknown tool " + tc.Name, nil
	}
}

// resolveOrCreateHierarchy finds or creates brand/stream/project/category by name.
// Returns (categoryID, breadcrumb, error).
func (d *Dispatcher) resolveOrCreateHierarchy(brandName, streamName, projectName, categoryName string) (string, string, error) {
	// Check if provided names match an existing category path.
	// The LLM sometimes scrambles the hierarchy order (e.g. puts card title as brand),
	// so we check if any existing path contains all provided names regardless of position.
	allCats, _ := d.deps.Card().ListAllCategories()
	inputNames := []string{brandName, streamName, projectName, categoryName}

	// Exact positional match first (brand=brand, stream=stream, etc.)
	for _, c := range allCats {
		if strings.EqualFold(c.BrandName, brandName) &&
			strings.EqualFold(c.StreamName, streamName) &&
			strings.EqualFold(c.ProjectName, projectName) &&
			strings.EqualFold(c.CategoryName, categoryName) {
			return c.CategoryID, c.Breadcrumb, nil
		}
	}

	// Fuzzy match: if >=3 of the 4 provided names appear somewhere in an existing path
	// (regardless of position), use that path instead of creating new hierarchy
	for _, c := range allCats {
		pathNames := []string{c.BrandName, c.StreamName, c.ProjectName, c.CategoryName}
		matches := 0
		for _, input := range inputNames {
			for _, pn := range pathNames {
				if strings.EqualFold(input, pn) {
					matches++
					break
				}
			}
		}
		if matches >= 3 {
			return c.CategoryID, c.Breadcrumb, nil
		}
	}

	// 1. Find or create brand
	brandSlug := ""
	brands, _ := d.deps.Project().ListBrands()
	for _, b := range brands {
		if strings.EqualFold(b.Name, brandName) {
			brandSlug = b.Slug
			brandName = b.Name // use canonical name
			break
		}
	}
	if brandSlug == "" {
		b, err := d.deps.Project().CreateBrand(brandName)
		if err != nil {
			return "", "", fmt.Errorf("creating brand %q: %w", brandName, err)
		}
		brandSlug = b.Slug
	}

	// 2. Find or create stream
	streamSlug := ""
	streams, _ := d.deps.Project().ListStreams(brandSlug)
	for _, s := range streams {
		if strings.EqualFold(s.Name, streamName) {
			streamSlug = s.Slug
			streamName = s.Name
			break
		}
	}
	if streamSlug == "" {
		s, err := d.deps.Project().CreateStream(brandSlug, streamName)
		if err != nil {
			return "", "", fmt.Errorf("creating stream %q: %w", streamName, err)
		}
		streamSlug = s.Slug
	}

	// 3. Find or create project
	projectSlug := ""
	projects, _ := d.deps.Project().ListProjects(brandSlug, streamSlug)
	for _, p := range projects {
		if strings.EqualFold(p.Name, projectName) {
			projectSlug = p.Slug
			projectName = p.Name
			break
		}
	}
	if projectSlug == "" {
		p, err := d.deps.Project().CreateProject(brandSlug, streamSlug, projectName)
		if err != nil {
			return "", "", fmt.Errorf("creating project %q: %w", projectName, err)
		}
		projectSlug = p.Slug
	}

	// 4. Find or create category
	var catID string
	cats, _ := d.deps.Project().ListCategories(brandSlug, streamSlug, projectSlug)
	for _, c := range cats {
		if strings.EqualFold(c.Name, categoryName) {
			catID = c.ID
			categoryName = c.Name
			break
		}
	}
	if catID == "" {
		c, err := d.deps.Project().CreateCategory(brandSlug, streamSlug, projectSlug, categoryName, len(cats))
		if err != nil {
			return "", "", fmt.Errorf("creating category %q: %w", categoryName, err)
		}
		catID = c.ID
	}

	breadcrumb := brandName + " / " + streamName + " / " + projectName + " / " + categoryName
	return catID, breadcrumb, nil
}

// StageCard builds a PendingEdit record for Suggest mode without applying any changes.
// It returns a fake result string (fed back to the LLM so the conversation continues naturally)
// and the PendingEdit to be stored on the message.
func (d *Dispatcher) StageCard(cardID string, tc llm.ToolCall, allCats []CategoryPath) (string, []model.PendingEdit) {
	if d.isNative(tc.Name) {
		return d.stageNative(d.cardScope(cardID, allCats), tc)
	}
	if result, _, ok := RunWebTool(tc); ok {
		return result, nil // read-only: runs even in Suggest mode
	}
	one := func(tool string, input map[string]any, label, detail string) []model.PendingEdit {
		return []model.PendingEdit{{
			ID: uuid.New().String(), Tool: tool, Input: input,
			Label: label, Detail: detail, Status: "pending",
		}}
	}

	switch tc.Name {
	case "set_fields", "update_blocks":
		fieldsMap, _ := tc.Arguments["fields"].(map[string]any)
		if len(fieldsMap) == 0 {
			fieldsMap, _ = tc.Arguments["blocks"].(map[string]any)
		}
		if len(fieldsMap) == 0 {
			fieldsMap = make(map[string]any)
			for k, v := range tc.Arguments {
				fieldsMap[k] = v
			}
		}
		// One PendingEdit per field so the user can review each individually
		var edits []model.PendingEdit
		var keys []string
		for k, v := range fieldsMap {
			keys = append(keys, k)
			detail := fmt.Sprintf("%v", v)
			if s, ok := v.(string); ok && len(s) > 120 {
				detail = s[:120] + "…"
			}
			edits = append(edits, model.PendingEdit{
				ID:     uuid.New().String(),
				Tool:   tc.Name,
				Input:  map[string]any{k: v},
				Label:  humanizeBlockKey(k),
				Detail: detail,
				Status: "pending",
			})
		}
		return "Fields staged: " + strings.Join(keys, ", "), edits

	case "add_field":
		label, _ := tc.Arguments["label"].(string)
		fieldType, _ := tc.Arguments["field_type"].(string)
		detail := "Type: " + fieldType
		// If the LLM supplied an inline value, include a preview so the
		// user can see what the new field will contain before approving.
		// Long text values are truncated in the preview — the full value
		// still flows through tc.Arguments and is applied on accept.
		if rawVal, ok := tc.Arguments["value"]; ok && rawVal != nil {
			preview := fmt.Sprintf("%v", rawVal)
			const maxPreview = 200
			if len(preview) > maxPreview {
				preview = preview[:maxPreview] + "…"
			}
			if preview != "" {
				detail += "\nValue: " + preview
			}
		}
		return "Field staged: " + label, one(tc.Name, tc.Arguments, "Add field: "+label, detail)

	case "suggest_pin":
		reason, _ := tc.Arguments["reason"].(string)
		catID, _ := tc.Arguments["category_id"].(string)
		var breadcrumb string
		if catID != "" {
			for _, c := range allCats {
				if c.CategoryID == catID {
					breadcrumb = c.Breadcrumb
					break
				}
			}
		} else {
			brand, _ := tc.Arguments["brand"].(string)
			stream, _ := tc.Arguments["stream"].(string)
			project, _ := tc.Arguments["project"].(string)
			category, _ := tc.Arguments["category"].(string)
			var parts []string
			for _, p := range []string{brand, stream, project, category} {
				if p != "" {
					parts = append(parts, p)
				}
			}
			breadcrumb = strings.Join(parts, " / ")
		}
		detail := breadcrumb
		if reason != "" {
			detail += "\n" + reason
		}
		return "Pin suggestion staged for " + breadcrumb, one(tc.Name, tc.Arguments, "Pin to "+breadcrumb, detail)

	default:
		return "Staged unknown tool " + tc.Name, nil
	}
}

// StageProject builds PendingEdit records for project chat in suggest mode.
//
// Strategy: each "logical edit" gets its own PendingEdit so the user can
// approve or reject them individually. For tools that touch multiple cards or
// fields, we expand them into one edit per (card, field) pair. This is what
// gives the review UI a flat list of "this card, this field, this preview"
// rows that the user can mouse-hover for full detail.
//
// `scope.CardIDs` is the set of valid card IDs for the current project.
// Any card_id outside the set is dropped from staging and reported back to
// the LLM via the result string so it can correct itself on the next turn.
// Pass a nil cardIDs map to disable scope checking.
//
// The result string fed back to the LLM acknowledges the staging (and lists
// any rejected IDs) so the conversation continues naturally without the LLM
// thinking the call silently failed.
func (d *Dispatcher) StageProject(tc llm.ToolCall, scope ProjectChatScope) (string, []model.PendingEdit) {
	if d.isNative(tc.Name) {
		return d.stageNative(&scope, tc)
	}
	if result, _, ok := RunWebTool(tc); ok {
		return result, nil // read-only: runs even in Suggest mode
	}
	// Read-only workspace tools execute directly even in suggest mode —
	// same treatment as web_fetch/web_search below: nothing to stage.
	if IsWorkspaceTool(tc.Name) {
		result, _ := d.execWorkspaceTool(tc, scope)
		return result, nil
	}
	inScope := func(id string) bool {
		if scope.CardIDs == nil {
			return true
		}
		return scope.CardIDs[id]
	}
	switch tc.Name {
	case "update_cards":
		// Each field is staged as the native tool that performs it, so
		// accepting a row really applies it (see card_batch.go).
		updatesRaw, _ := tc.Arguments["updates"].([]any)
		var allEdits []model.PendingEdit
		var rejected []string
		for _, raw := range updatesRaw {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			cardID, _ := entry["card_id"].(string)
			if cardID == "" {
				continue
			}
			if !inScope(cardID) {
				rejected = append(rejected, cardID)
				continue
			}
			allEdits = append(allEdits, d.stageCardCalls(cardID, d.cardUpdateCalls(cardID, entry))...)
		}
		if len(rejected) > 0 && len(allEdits) == 0 {
			return "error: none of the supplied card_ids belong to the current project: " + strings.Join(rejected, ", ") + ". Use only the card IDs listed in the system prompt.", nil
		}
		summary := fmt.Sprintf("Staged %d edits across %d cards", len(allEdits), len(updatesRaw)-len(rejected))
		if len(rejected) > 0 {
			summary += fmt.Sprintf(" (skipped %d out-of-project: %s)", len(rejected), strings.Join(rejected, ", "))
		}
		return summary, allEdits

	case "add_tags_to_cards":
		tags := stringList(tc.Arguments["tags"])
		if len(tags) == 0 {
			return "error: tags are required", nil
		}
		var edits []model.PendingEdit
		var rejected []string
		for _, cid := range stringList(tc.Arguments["card_ids"]) {
			if !inScope(cid) {
				rejected = append(rejected, cid)
				continue
			}
			// One native add_card_tags row per card, approved individually.
			edits = append(edits, d.stageCardCalls(cid, []nativeCall{{
				Tool: "add_card_tags", Field: "add tags", Detail: "+" + strings.Join(tags, ", +"),
				Args: map[string]any{"card_id": cid, "tags": toAnySlice(tags)},
			}})...)
		}
		if len(rejected) > 0 && len(edits) == 0 {
			return "error: none of the supplied card_ids belong to the current project: " + strings.Join(rejected, ", "), nil
		}
		summary := fmt.Sprintf("Tag additions staged for %d cards", len(edits))
		if len(rejected) > 0 {
			summary += fmt.Sprintf(" (skipped %d out-of-project: %s)", len(rejected), strings.Join(rejected, ", "))
		}
		return summary, edits

	// Read-only tools execute even in suggest mode — nothing to stage.
	default:
		return "Staged unknown tool " + tc.Name, nil
	}
}

// cardDisplayLabel returns a short label for a card (used in pending edit
// labels). Falls back to the card ID if the title can't be loaded.
func (d *Dispatcher) cardDisplayLabel(cardID string) string {
	if d.deps.Repo() == nil {
		return cardID
	}
	c, err := d.deps.Repo().GetCard(cardID)
	if err != nil || c == nil || c.Title == "" {
		return cardID
	}
	return c.Title
}
