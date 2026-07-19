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
	"bruv/internal/agent"
	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/model"
	"fmt"
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
	var parsed time.Time
	var parseErr error
	for _, layout := range layouts {
		parsed, parseErr = time.Parse(layout, raw)
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

// coerceChecklist converts []any of strings, []any of {text, done} maps,
// or a single newline-separated string into [{id, text, done}].
func coerceChecklist(val any) []map[string]any {
	type entry struct {
		text string
		done bool
	}
	var entries []entry
	switch v := val.(type) {
	case []any:
		for _, item := range v {
			switch it := item.(type) {
			case string:
				if it != "" {
					entries = append(entries, entry{text: it})
				}
			case map[string]any:
				text, _ := it["text"].(string)
				if text == "" {
					continue
				}
				done, _ := it["done"].(bool)
				entries = append(entries, entry{text: text, done: done})
			}
		}
	case string:
		for _, line := range strings.Split(v, "\n") {
			line = stripListPrefix(line)
			if line != "" {
				entries = append(entries, entry{text: line})
			}
		}
	}
	items := make([]map[string]any, len(entries))
	for i, e := range entries {
		items[i] = map[string]any{
			"id":   fmt.Sprintf("cli-%s", uuid.New().String()[:8]),
			"text": e.text,
			"done": e.done,
		}
	}
	return items
}

// coerceList converts []any of strings, []any of {id?, text} maps, or a
// single newline-separated string into [{id, text}]. This is the fix for
// the bug where agent `update_self` was writing plain strings directly to
// list blocks, which the frontend renderer couldn't parse.
func coerceList(val any) []map[string]any {
	var texts []string
	switch v := val.(type) {
	case []any:
		for _, item := range v {
			switch it := item.(type) {
			case string:
				if it != "" {
					texts = append(texts, it)
				}
			case map[string]any:
				if text, ok := it["text"].(string); ok && text != "" {
					texts = append(texts, text)
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
				texts = append(texts, line)
			}
		}
	}
	items := make([]map[string]any, len(texts))
	for i, t := range texts {
		items[i] = map[string]any{
			"id":   fmt.Sprintf("li-%s", uuid.New().String()[:8]),
			"text": t,
		}
	}
	return items
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
// {slides:[{id,contentTypeId,values,...}], currentIndex} shape the frontend
// expects. Accepts the full object, a bare array of slides, or bare strings
// (each becomes a title slide), and stamps a stable id on any slide missing one.
func coerceSlideDeck(val any) map[string]any {
	var rawSlides []any
	currentIndex := 0
	var theme any
	switch v := val.(type) {
	case map[string]any:
		rawSlides, _ = v["slides"].([]any)
		currentIndex = int(coerceNumber(v["currentIndex"]))
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
	if currentIndex < 0 || currentIndex >= len(slides) {
		currentIndex = 0
	}
	out := map[string]any{"slides": slides, "currentIndex": currentIndex}
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
	for _, k := range []string{"templateId", "cardId", "notes", "thumbnail"} {
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
	"set_title":       (*Dispatcher).toolSetTitle,
	"set_due_date":    (*Dispatcher).toolSetDueDate,
	"set_card_type":   (*Dispatcher).toolSetCardType,
	"set_fields":      (*Dispatcher).toolSetFields,
	"update_blocks":   (*Dispatcher).toolSetFields, // alias — same handler
	"add_tags":        (*Dispatcher).toolAddTags,
	"add_field":       (*Dispatcher).toolAddField,
	"suggest_pin":     (*Dispatcher).toolSuggestPin,
	"configure_agent": (*Dispatcher).toolConfigureAgent,
	"web_fetch":       (*Dispatcher).toolWebFetch,
	"web_search":      (*Dispatcher).toolWebSearch,
}

// ExecuteCard runs a single tool and returns (result string, action record, pin suggestion).
func (d *Dispatcher) ExecuteCard(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	if handler, ok := cardToolHandlers[tc.Name]; ok {
		return handler(d, cardID, card, tc, allCats)
	}
	return "error: unknown tool " + tc.Name, nil, nil
}

func (d *Dispatcher) toolSetTitle(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	title, _ := tc.Arguments["title"].(string)
	if title == "" {
		return "error: title is required", nil, nil
	}
	if card != nil && card.Title == title {
		return "Title is already " + title + " — no change needed", nil, nil
	}
	_, err := d.deps.Card().UpdateTitle(cardID, title)
	if err != nil {
		return "error: " + err.Error(), nil, nil
	}
	action := &model.ToolAction{Tool: "set_title", Input: tc.Arguments, Result: "Title set to " + title}
	return "Card title set to " + title, action, nil
}

func (d *Dispatcher) toolSetDueDate(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	dueDate, _ := tc.Arguments["due_date"].(string)
	if dueDate != "" && card != nil && card.DueDate != nil && card.DueDate.Format("2006-01-02") == dueDate {
		return "Due date is already " + dueDate + " — no change needed", nil, nil
	}
	_, err := d.deps.Card().UpdateDueDate(cardID, dueDate)
	if err != nil {
		return "error: " + err.Error(), nil, nil
	}
	result := "Due date cleared"
	if dueDate != "" {
		result = "Due date set to " + dueDate
	}
	action := &model.ToolAction{Tool: "set_due_date", Input: tc.Arguments, Result: result}
	return result, action, nil
}

func (d *Dispatcher) toolSetCardType(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	cardType, _ := tc.Arguments["card_type"].(string)
	if cardType == "" {
		return "error: card_type is required", nil, nil
	}
	if card != nil && card.Type == cardType {
		return "Type is already " + cardType + " — no change needed. Use update_blocks to fill in block values.", nil, nil
	}
	_, err := d.deps.Card().UpdateType(cardID, cardType)
	if err != nil {
		return "error: " + err.Error(), nil, nil
	}
	// Block application is handled by UpdateCardType via applyTypeBlocks
	// Build a helpful result listing available block keys
	resultMsg := "Card type set to " + cardType + ". "
	updatedCard, _ := d.deps.Repo().GetCard(cardID)
	if updatedCard != nil && len(updatedCard.Blocks) > 0 {
		var blockKeys []string
		for _, b := range updatedCard.Blocks {
			if b.Key != "" {
				blockKeys = append(blockKeys, b.Key)
			}
		}
		if len(blockKeys) > 0 {
			resultMsg += "NOW call set_fields to fill these field keys: " + strings.Join(blockKeys, ", ")
		}
	}
	action := &model.ToolAction{Tool: "set_card_type", Input: tc.Arguments, Result: "Set type to " + cardType}
	return resultMsg, action, nil
}

func (d *Dispatcher) toolSetFields(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	// Accept nested "fields"/"blocks" key OR flat top-level arguments
	// (dynamic tool schema puts block keys at the top level)
	fieldsMap, _ := tc.Arguments["fields"].(map[string]any)
	if len(fieldsMap) == 0 {
		fieldsMap, _ = tc.Arguments["blocks"].(map[string]any)
	}
	if len(fieldsMap) == 0 {
		// Try flat arguments: the dynamic schema puts block keys directly in tc.Arguments.
		// Match against existing blocks AND schema fields for the card's type so that
		// the LLM can set fields that haven't been created yet.
		currentCard2, err2 := d.deps.Repo().GetCard(cardID)
		if err2 == nil {
			knownKeys := make(map[string]bool)
			for _, b := range currentCard2.Blocks {
				if b.Key != "" {
					knownKeys[b.Key] = true
				}
			}
			if d.deps.Registry() != nil && currentCard2.Type != "" {
				if s := d.deps.Registry().Get(currentCard2.Type); s != nil {
					for k := range s.Properties {
						knownKeys[k] = true
					}
				}
			}
			flat := make(map[string]any)
			for k, v := range tc.Arguments {
				if knownKeys[k] {
					flat[k] = v
				}
			}
			if len(flat) > 0 {
				fieldsMap = flat
			}
		}
	}
	if len(fieldsMap) == 0 {
		return "error: fields map is empty", nil, nil
	}
	currentCard, err := d.deps.Repo().GetCard(cardID)
	if err != nil {
		return "error: " + err.Error(), nil, nil
	}

	// Auto-create blocks for schema fields that don't exist on the card yet
	existingKeys := make(map[string]bool)
	for _, b := range currentCard.Blocks {
		if b.Key != "" {
			existingKeys[b.Key] = true
		}
	}
	if d.deps.Registry() != nil && currentCard.Type != "" {
		schemaBlocks := d.deps.Registry().SchemaToBlocks(currentCard.Type)
		for _, sb := range schemaBlocks {
			if _, wantSet := fieldsMap[sb.Key]; wantSet && !existingKeys[sb.Key] {
				currentCard.Blocks = append(currentCard.Blocks, sb)
				existingKeys[sb.Key] = true
			}
		}
	}

	updated := false
	var updatedKeys []string
	for i, b := range currentCard.Blocks {
		if val, ok := fieldsMap[b.Key]; ok {
			val = coerceBlockValue(b.Type, val)
			currentCard.Blocks[i].Value = val
			updated = true
			updatedKeys = append(updatedKeys, b.Key)
		}
	}
	if !updated {
		var available []string
		for _, b := range currentCard.Blocks {
			if b.Key != "" {
				available = append(available, b.Key)
			}
		}
		return "error: no matching field keys found. Available keys: " + strings.Join(available, ", "), nil, nil
	}
	d.deps.Card().UpdateBlocks(cardID, currentCard.Blocks)
	result := "Updated fields: " + strings.Join(updatedKeys, ", ")
	action := &model.ToolAction{Tool: "set_fields", Input: tc.Arguments, Result: result}
	return result, action, nil
}

func (d *Dispatcher) toolAddTags(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	tagsRaw, _ := tc.Arguments["tags"].([]any)
	if len(tagsRaw) == 0 {
		return "error: tags array is empty", nil, nil
	}
	var newTags []string
	for _, t := range tagsRaw {
		if s, ok := t.(string); ok && s != "" {
			newTags = append(newTags, s)
		}
	}
	currentCard, err := d.deps.Repo().GetCard(cardID)
	if err != nil {
		return "error: " + err.Error(), nil, nil
	}
	existing := make(map[string]bool)
	for _, t := range currentCard.Tags {
		existing[strings.ToLower(t)] = true
	}
	merged := currentCard.Tags
	var added []string
	for _, t := range newTags {
		if !existing[strings.ToLower(t)] {
			merged = append(merged, t)
			existing[strings.ToLower(t)] = true
			added = append(added, t)
		}
	}
	if len(added) > 0 {
		d.deps.Card().UpdateTags(cardID, merged)
	}
	result := "Added tags: " + strings.Join(added, ", ")
	action := &model.ToolAction{Tool: "add_tags", Input: tc.Arguments, Result: result}
	return result, action, nil
}

func (d *Dispatcher) toolAddField(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	key, _ := tc.Arguments["key"].(string)
	label, _ := tc.Arguments["label"].(string)
	fieldType, _ := tc.Arguments["field_type"].(string)
	if key == "" || label == "" || fieldType == "" {
		return "error: key, label, and field_type are required", nil, nil
	}
	// Validate field_type
	validTypes := map[string]bool{"text": true, "checklist": true, "checkbox": true, "number": true, "date": true, "url": true}
	if !validTypes[fieldType] {
		return "error: invalid field_type " + fieldType + ". Must be one of: text, checklist, checkbox, number, date, url", nil, nil
	}
	currentCard, err := d.deps.Repo().GetCard(cardID)
	if err != nil {
		return "error: " + err.Error(), nil, nil
	}
	// Check for duplicate key
	for _, b := range currentCard.Blocks {
		if b.Key == key {
			return "Field with key " + key + " already exists — use set_fields to update it.", nil, nil
		}
	}
	// Build default value for the type
	var defaultVal any
	switch fieldType {
	case "checklist":
		defaultVal = []any{}
	case "checkbox":
		defaultVal = false
	case "number":
		defaultVal = 0.0
	default:
		defaultVal = ""
	}
	// If the LLM provided an initial value, coerce and use it
	if rawVal, hasVal := tc.Arguments["value"]; hasVal && rawVal != nil {
		defaultVal = coerceBlockValue(fieldType, rawVal)
	}
	newBlock := model.Block{
		ID:    fmt.Sprintf("blk-%s", uuid.New().String()[:8]),
		Type:  fieldType,
		Label: label,
		Key:   key,
		Value: defaultVal,
	}
	currentCard.Blocks = append(currentCard.Blocks, newBlock)
	d.deps.Card().UpdateBlocks(cardID, currentCard.Blocks)
	resultMsg := fmt.Sprintf("Added %s field '%s' (key: %s). Use set_fields with key '%s' to update its value.", fieldType, label, key, key)
	action := &model.ToolAction{Tool: "add_field", Input: tc.Arguments, Result: fmt.Sprintf("Added field: %s (%s)", label, fieldType)}
	return resultMsg, action, nil
}

func (d *Dispatcher) toolSuggestPin(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	catID, _ := tc.Arguments["category_id"].(string)
	reason, _ := tc.Arguments["reason"].(string)
	confidence, _ := tc.Arguments["confidence"].(string)

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

func (d *Dispatcher) toolConfigureAgent(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	enabled, _ := tc.Arguments["enabled"].(bool)
	goal, _ := tc.Arguments["goal"].(string)
	schedule, _ := tc.Arguments["schedule"].(string)

	var allowedTools []string
	if tools, ok := tc.Arguments["allowed_tools"].([]any); ok {
		for _, t := range tools {
			if s, ok := t.(string); ok {
				allowedTools = append(allowedTools, s)
			}
		}
	}

	var notifyOn []string
	if triggers, ok := tc.Arguments["notify_on"].([]any); ok {
		for _, t := range triggers {
			if s, ok := t.(string); ok {
				notifyOn = append(notifyOn, s)
			}
		}
	}
	notifyChannel, _ := tc.Arguments["notify_channel"].(string)

	af, err := d.deps.Repo().GetAgentConfig(cardID)
	if err != nil {
		return "error: " + err.Error(), nil, nil
	}

	af.Config.Enabled = enabled
	if goal != "" {
		af.Config.Goal = goal
	}
	if schedule != "" {
		af.Config.Schedule = schedule
	}
	if len(allowedTools) > 0 {
		af.Config.AllowedTools = allowedTools
	}
	if len(notifyOn) > 0 {
		af.Config.NotifyOn = notifyOn
	}
	if notifyChannel != "" {
		af.Config.NotifyChannel = notifyChannel
	}

	// Handle dynamic rescheduling: next_run_at and new_schedule
	if nextRunAtStr, ok := tc.Arguments["next_run_at"].(string); ok && nextRunAtStr != "" {
		if t, err := time.Parse(time.RFC3339, nextRunAtStr); err == nil {
			af.Config.NextRunAt = &t
		}
	}
	if newSchedule, ok := tc.Arguments["new_schedule"].(string); ok && newSchedule != "" {
		af.Config.Schedule = newSchedule
		schedule = newSchedule
	}

	// Set status and calculate next run
	if enabled {
		af.Config.Status = model.AgentStatusIdle
		if af.Config.NextRunAt == nil && af.Config.Schedule != "" {
			now := time.Now()
			opts := agent.ScheduleOpts{
				StartDate:         af.Config.StartDate,
				EndDate:           af.Config.EndDate,
				ActiveWindowStart: af.Config.ActiveWindowStart,
				ActiveWindowEnd:   af.Config.ActiveWindowEnd,
				OneShot:           af.Config.OneShot,
				LastRunAt:         af.Config.LastRunAt,
				Timezone:          af.Config.Timezone,
			}
			if next, err := agent.NextRunTimeWithOpts(af.Config.Schedule, now, opts); err == nil {
				af.Config.NextRunAt = &next
			}
		}
	} else {
		af.Config.Status = model.AgentStatusDisabled
	}

	if err := d.deps.Repo().SaveAgentConfig(cardID, af.Config); err != nil {
		return "error: " + err.Error(), nil, nil
	}

	// Notify any open CardDetail so the Agent tab re-fetches and
	// shows the updated goal / schedule / tools immediately. Without
	// this, the user sees the chat say "I updated the goal" but the
	// Agent tab still shows the old value until they close + reopen
	// the card — which is exactly the bug Harvey reported on
	// 2026-04-12.
	d.emitCardUpdated(cardID)

	summary := fmt.Sprintf("Agent %s — schedule: %s, tools: %s", map[bool]string{true: "enabled", false: "disabled"}[enabled], schedule, strings.Join(allowedTools, ", "))
	action := &model.ToolAction{Tool: "configure_agent", Input: tc.Arguments, Result: summary}
	return summary, action, nil
}

func (d *Dispatcher) toolWebFetch(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	url, _ := tc.Arguments["url"].(string)
	result, err := agent.WebFetch(url)
	if err != nil {
		return "error: " + err.Error(), &model.ToolAction{Tool: "web_fetch", Input: tc.Arguments, Result: "error: " + err.Error()}, nil
	}
	return result, &model.ToolAction{Tool: "web_fetch", Input: tc.Arguments, Result: "fetched " + url}, nil
}

func (d *Dispatcher) toolWebSearch(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	query, _ := tc.Arguments["query"].(string)
	result, err := agent.WebSearch(query)
	if err != nil {
		return "error: " + err.Error(), &model.ToolAction{Tool: "web_search", Input: tc.Arguments, Result: "error: " + err.Error()}, nil
	}
	return result, &model.ToolAction{Tool: "web_search", Input: tc.Arguments, Result: "searched: " + query}, nil
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
	// Workspace tools are read-only and project-scoped — shared handler.
	if IsWorkspaceTool(tc.Name) {
		return d.execWorkspaceTool(tc, scope)
	}
	switch tc.Name {
	case "create_card":
		title, _ := tc.Arguments["title"].(string)
		if title == "" {
			return "error: title is required", nil
		}
		cardType, _ := tc.Arguments["card_type"].(string)
		if cardType == "" {
			cardType = "idea"
		}
		card, err := d.deps.Card().Create(cardType, title)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		cardID := card.ID
		// Pin to category if specified. Accept either category_id or
		// category_name — the latter lets the LLM chain create_card after
		// create_category in the same conversation, since the new category
		// won't have a known ID until apply time.
		categoryID, _ := tc.Arguments["category_id"].(string)
		categoryName, _ := tc.Arguments["category_name"].(string)
		if categoryID != "" || categoryName != "" {
			resolvedID, err := d.resolveCategoryID(scope, categoryID, categoryName)
			if err != nil {
				return "error: " + err.Error(), nil
			}
			_ = d.deps.Card().Pin(cardID, resolvedID)
			categoryID = resolvedID
		}
		// Add tags if specified
		if tagsRaw, ok := tc.Arguments["tags"].([]any); ok && len(tagsRaw) > 0 {
			var tags []string
			for _, t := range tagsRaw {
				if s, ok := t.(string); ok && s != "" {
					tags = append(tags, s)
				}
			}
			if len(tags) > 0 {
				d.deps.Card().UpdateTags(cardID, tags)
			}
		}
		// Set description if specified. Description is an intrinsic card
		// property, not a block — set it directly.
		if desc, ok := tc.Arguments["description"].(string); ok && desc != "" {
			d.deps.Card().UpdateDescription(cardID, desc)
		}
		result := fmt.Sprintf("Created card '%s' (ID: %s)", title, cardID)
		if categoryID != "" {
			result += " and pinned to category"
		}
		action := &model.ToolAction{Tool: "create_card", Input: tc.Arguments, Result: result}
		return result, action

	case "add_tags_to_cards":
		cardIDsRaw, _ := tc.Arguments["card_ids"].([]any)
		tagsRaw, _ := tc.Arguments["tags"].([]any)
		if len(cardIDsRaw) == 0 || len(tagsRaw) == 0 {
			return "error: card_ids and tags are required", nil
		}
		var newTags []string
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok && s != "" {
				newTags = append(newTags, s)
			}
		}
		var updated int
		for _, raw := range cardIDsRaw {
			cid, ok := raw.(string)
			if !ok || cid == "" {
				continue
			}
			c, err := d.deps.Repo().GetCard(cid)
			if err != nil {
				continue
			}
			existing := make(map[string]bool)
			for _, t := range c.Tags {
				existing[strings.ToLower(t)] = true
			}
			merged := c.Tags
			for _, t := range newTags {
				if !existing[strings.ToLower(t)] {
					merged = append(merged, t)
					existing[strings.ToLower(t)] = true
				}
			}
			if len(merged) > len(c.Tags) {
				d.deps.Card().UpdateTags(cid, merged)
				updated++
			}
		}
		result := fmt.Sprintf("Added tags [%s] to %d cards", strings.Join(newTags, ", "), updated)
		action := &model.ToolAction{Tool: "add_tags_to_cards", Input: tc.Arguments, Result: result}
		return result, action

	case "move_card":
		cardID, _ := tc.Arguments["card_id"].(string)
		if cardID == "" {
			return "error: card_id is required", nil
		}
		// Destination: accept ID or name. Name lets the LLM chain after a
		// just-staged create_category — apply order ensures the category
		// exists by the time this resolves.
		toCatID, _ := tc.Arguments["to_category_id"].(string)
		toCatName, _ := tc.Arguments["to_category_name"].(string)
		if toCatID == "" && toCatName == "" {
			return "error: to_category_id or to_category_name is required", nil
		}
		toCat, err := d.resolveCategoryID(scope, toCatID, toCatName)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		// Source: optional. Auto-detect from the card's current pin if missing.
		fromCat, _ := tc.Arguments["from_category_id"].(string)
		if fromCat == "" {
			detected, err := d.findCardCurrentCategory(scope, cardID)
			if err != nil {
				return "error: " + err.Error(), nil
			}
			fromCat = detected
		}
		if fromCat == toCat {
			return "error: source and destination categories are the same", nil
		}
		if err := d.deps.Card().MoveToCategory(cardID, fromCat, toCat, 0); err != nil {
			return "error: " + err.Error(), nil
		}
		result := "Card moved to new category"
		action := &model.ToolAction{Tool: "move_card", Input: tc.Arguments, Result: result}
		return result, action

	case "update_card":
		cardID, _ := tc.Arguments["card_id"].(string)
		if cardID == "" {
			return "error: card_id is required", nil
		}
		changes, err := d.applyCardUpdate(cardID, tc.Arguments)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		if len(changes) == 0 {
			return "No changes applied", nil
		}
		result := "Updated: " + strings.Join(changes, ", ")
		action := &model.ToolAction{Tool: "update_card", Input: tc.Arguments, Result: result}
		return result, action

	case "update_cards":
		updatesRaw, _ := tc.Arguments["updates"].([]any)
		if len(updatesRaw) == 0 {
			return "error: updates array is required", nil
		}
		type cardResult struct {
			cardID  string
			changes []string
			err     error
		}
		var results []cardResult
		var totalChanges int
		var failures int
		for _, raw := range updatesRaw {
			entry, ok := raw.(map[string]any)
			if !ok {
				failures++
				continue
			}
			cardID, _ := entry["card_id"].(string)
			if cardID == "" {
				failures++
				continue
			}
			changes, err := d.applyCardUpdate(cardID, entry)
			results = append(results, cardResult{cardID: cardID, changes: changes, err: err})
			if err != nil {
				failures++
			} else {
				totalChanges += len(changes)
			}
		}
		var summary strings.Builder
		successes := len(results) - failures
		summary.WriteString(fmt.Sprintf("Updated %d cards (%d field changes total)", successes, totalChanges))
		if failures > 0 {
			summary.WriteString(fmt.Sprintf("; %d failed", failures))
		}
		action := &model.ToolAction{Tool: "update_cards", Input: tc.Arguments, Result: summary.String()}
		return summary.String(), action

	case "configure_agent":
		cardID, _ := tc.Arguments["card_id"].(string)
		if cardID == "" {
			return "error: card_id is required", nil
		}
		af, err := d.deps.Repo().GetAgentConfig(cardID)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		var changes []string
		if v, ok := tc.Arguments["enabled"].(bool); ok {
			af.Config.Enabled = v
			changes = append(changes, fmt.Sprintf("enabled=%t", v))
		}
		if v, ok := tc.Arguments["schedule"].(string); ok {
			af.Config.Schedule = v
			if v == "" {
				changes = append(changes, "schedule=cleared")
			} else {
				changes = append(changes, "schedule="+v)
			}
		}
		if v, ok := tc.Arguments["goal"].(string); ok {
			af.Config.Goal = v
			changes = append(changes, "goal")
		}
		if raw, ok := tc.Arguments["allowed_tools"].([]any); ok {
			tools := make([]string, 0, len(raw))
			for _, t := range raw {
				if s, ok := t.(string); ok && s != "" {
					tools = append(tools, s)
				}
			}
			af.Config.AllowedTools = tools
			changes = append(changes, "allowed_tools")
		}
		if len(changes) == 0 {
			return "No changes applied", nil
		}
		if err := d.deps.Repo().SaveAgentConfig(cardID, af.Config); err != nil {
			return "error: " + err.Error(), nil
		}
		d.emitCardUpdated(cardID)
		result := "Configured agent: " + strings.Join(changes, ", ")
		action := &model.ToolAction{Tool: "configure_agent", Input: tc.Arguments, Result: result}
		return result, action

	// --- Project metadata ---
	case "update_project":
		var changes []string
		if name, ok := tc.Arguments["name"].(string); ok && name != "" {
			if _, err := d.deps.Project().RenameProject(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug, name); err != nil {
				return "error: " + err.Error(), nil
			}
			changes = append(changes, "name")
			// Slug may have changed after rename — refresh it for subsequent calls.
			if p, err := d.deps.Repo().GetProject(scope.BrandSlug, scope.StreamSlug, name); err == nil {
				scope.ProjectSlug = p.Slug
			}
		}
		if v, ok := tc.Arguments["description"].(string); ok {
			if _, err := d.deps.Project().UpdateProjectDescription(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug, v); err == nil {
				changes = append(changes, "description")
			}
		}
		if v, ok := tc.Arguments["icon"].(string); ok {
			if _, err := d.deps.Project().UpdateProjectIcon(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug, v); err == nil {
				changes = append(changes, "icon")
			}
		}
		if len(changes) == 0 {
			return "No changes applied", nil
		}
		result := "Updated project: " + strings.Join(changes, ", ")
		action := &model.ToolAction{Tool: "update_project", Input: tc.Arguments, Result: result}
		return result, action

	// --- Project tags ---
	case "create_project_tag":
		name, _ := tc.Arguments["name"].(string)
		if name == "" {
			return "error: name is required", nil
		}
		color, _ := tc.Arguments["color"].(string)
		// Underlying type is model.Label / AddProjectLabel — that's just the
		// historical persistence name. The user-facing concept is "tag".
		labels, err := d.deps.Catalog().AddProjectLabel(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug, name, color)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		// Optional icon — set in a follow-up call once we know the new ID.
		if icon, ok := tc.Arguments["icon"].(string); ok && icon != "" {
			for _, l := range labels {
				if strings.EqualFold(l.Name, name) {
					_, _ = d.deps.Catalog().SetProjectLabelIcon(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug, l.ID, icon)
					break
				}
			}
		}
		result := "Created tag: " + name
		action := &model.ToolAction{Tool: "create_project_tag", Input: tc.Arguments, Result: result}
		return result, action

	case "update_project_tag":
		tagID, _ := tc.Arguments["tag_id"].(string)
		if tagID == "" {
			tagName, _ := tc.Arguments["tag_name"].(string)
			if tagName == "" {
				return "error: tag_id or tag_name is required", nil
			}
			id, err := d.findProjectTagID(scope, tagName)
			if err != nil {
				return "error: " + err.Error(), nil
			}
			tagID = id
		}
		var changes []string
		// Name + color go through UpdateProjectLabel together. Empty strings
		// preserve the existing value (per repo.UpdateProjectLabel semantics).
		newName, hasName := tc.Arguments["name"].(string)
		newColor, hasColor := tc.Arguments["color"].(string)
		if hasName || hasColor {
			passName := ""
			if hasName {
				passName = newName
			}
			passColor := ""
			if hasColor {
				passColor = newColor
			}
			if _, err := d.deps.Catalog().UpdateProjectLabel(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug, tagID, passName, passColor); err != nil {
				return "error: " + err.Error(), nil
			}
			if hasName {
				changes = append(changes, "name")
			}
			if hasColor {
				changes = append(changes, "color")
			}
		}
		if v, ok := tc.Arguments["icon"].(string); ok {
			if _, err := d.deps.Catalog().SetProjectLabelIcon(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug, tagID, v); err == nil {
				changes = append(changes, "icon")
			}
		}
		if len(changes) == 0 {
			return "No changes applied", nil
		}
		result := "Updated tag: " + strings.Join(changes, ", ")
		action := &model.ToolAction{Tool: "update_project_tag", Input: tc.Arguments, Result: result}
		return result, action

	case "delete_project_tag":
		tagID, _ := tc.Arguments["tag_id"].(string)
		if tagID == "" {
			tagName, _ := tc.Arguments["tag_name"].(string)
			if tagName == "" {
				return "error: tag_id or tag_name is required", nil
			}
			id, err := d.findProjectTagID(scope, tagName)
			if err != nil {
				return "error: " + err.Error(), nil
			}
			tagID = id
		}
		if _, err := d.deps.Catalog().RemoveProjectLabel(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug, tagID); err != nil {
			return "error: " + err.Error(), nil
		}
		result := "Deleted tag"
		action := &model.ToolAction{Tool: "delete_project_tag", Input: tc.Arguments, Result: result}
		return result, action

	// --- Categories ---
	case "create_category":
		name, _ := tc.Arguments["name"].(string)
		if name == "" {
			return "error: name is required", nil
		}
		position := 0
		if v, ok := tc.Arguments["position"].(float64); ok {
			position = int(v)
		}
		cat, err := d.deps.Project().CreateCategory(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug, name, position)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		result := fmt.Sprintf("Created category '%s' (id: %s)", name, cat.ID)
		action := &model.ToolAction{Tool: "create_category", Input: tc.Arguments, Result: result}
		return result, action

	case "update_category":
		catID, _ := tc.Arguments["category_id"].(string)
		catName, _ := tc.Arguments["category_name"].(string)
		resolvedID, err := d.resolveCategoryID(scope, catID, catName)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		catID = resolvedID
		// Find the category's slug — the existing app methods all key by slug.
		catSlug, err := d.findCategorySlug(scope, catID)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		var changes []string
		if name, ok := tc.Arguments["name"].(string); ok && name != "" {
			if _, err := d.deps.Project().RenameCategory(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug, catSlug, name); err != nil {
				return "error: " + err.Error(), nil
			}
			changes = append(changes, "name")
			// Slug may have changed after rename.
			if newSlug, err := d.findCategorySlug(scope, catID); err == nil {
				catSlug = newSlug
			}
		}
		if v, ok := tc.Arguments["description"].(string); ok {
			if _, err := d.deps.Project().UpdateCategoryDescription(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug, catSlug, v); err == nil {
				changes = append(changes, "description")
			}
		}
		if v, ok := tc.Arguments["icon"].(string); ok {
			if _, err := d.deps.Project().UpdateCategoryIcon(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug, catSlug, v); err == nil {
				changes = append(changes, "icon")
			}
		}
		if raw, ok := tc.Arguments["accepted_types"].([]any); ok {
			types := make([]string, 0, len(raw))
			for _, t := range raw {
				if s, ok := t.(string); ok {
					types = append(types, s)
				}
			}
			if _, err := d.deps.Project().UpdateCategoryAcceptedTypes(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug, catSlug, types); err == nil {
				changes = append(changes, "accepted_types")
			}
		}
		if len(changes) == 0 {
			return "No changes applied", nil
		}
		result := "Updated category: " + strings.Join(changes, ", ")
		action := &model.ToolAction{Tool: "update_category", Input: tc.Arguments, Result: result}
		return result, action

	case "delete_category":
		catID, _ := tc.Arguments["category_id"].(string)
		catName, _ := tc.Arguments["category_name"].(string)
		resolvedID, err := d.resolveCategoryID(scope, catID, catName)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		catID = resolvedID
		catSlug, err := d.findCategorySlug(scope, catID)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		if err := d.deps.Project().DeleteCategory(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug, catSlug); err != nil {
			return "error: " + err.Error(), nil
		}
		result := "Deleted category"
		action := &model.ToolAction{Tool: "delete_category", Input: tc.Arguments, Result: result}
		return result, action

	case "web_fetch":
		url, _ := tc.Arguments["url"].(string)
		result, err := agent.WebFetch(url)
		if err != nil {
			return "error: " + err.Error(), &model.ToolAction{Tool: "web_fetch", Input: tc.Arguments, Result: "error: " + err.Error()}
		}
		return result, &model.ToolAction{Tool: "web_fetch", Input: tc.Arguments, Result: "fetched " + url}

	case "web_search":
		query, _ := tc.Arguments["query"].(string)
		result, err := agent.WebSearch(query)
		if err != nil {
			return "error: " + err.Error(), &model.ToolAction{Tool: "web_search", Input: tc.Arguments, Result: "error: " + err.Error()}
		}
		return result, &model.ToolAction{Tool: "web_search", Input: tc.Arguments, Result: "searched: " + query}

	default:
		return "error: unknown tool " + tc.Name, nil
	}
}

// findProjectTagID looks up a tag by name (case-insensitive) within the
// current project and returns its ID. Used by the tag tools when they accept
// `tag_name` as a fallback to `tag_id`.
//
// (Underlying repo type is `model.Label` for historical persistence reasons,
// but the user-facing concept is "tag" — see also feedback_tags_not_labels.)
func (d *Dispatcher) findProjectTagID(scope ProjectChatScope, name string) (string, error) {
	labels, err := d.deps.Catalog().GetProjectLabels(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug)
	if err != nil {
		return "", err
	}
	for _, l := range labels {
		if strings.EqualFold(l.Name, name) {
			return l.ID, nil
		}
	}
	return "", fmt.Errorf("no tag named %q in this project", name)
}

// findCategorySlug looks up a category's slug by its ID within the current
// project. The existing repo methods key categories by slug rather than ID,
// so this bridges the gap when tools accept category_id.
func (d *Dispatcher) findCategorySlug(scope ProjectChatScope, catID string) (string, error) {
	cats, err := d.deps.Repo().ListCategories(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug)
	if err != nil {
		return "", err
	}
	for _, c := range cats {
		if c.ID == catID {
			return c.Slug, nil
		}
	}
	return "", fmt.Errorf("category %s not found in current project", catID)
}

// resolveCategoryID resolves either an ID or a name (case-insensitive) into a
// canonical category ID for the current project. Used by tools that accept
// `category_id` and `category_name` as alternatives — this lets the LLM refer
// to a category it just created in the same conversation by name, since the ID
// won't be known until apply time.
//
// If both `id` and `name` are supplied, ID takes precedence. Returns an error
// if neither resolves to a category in this project.
func (d *Dispatcher) resolveCategoryID(scope ProjectChatScope, id, name string) (string, error) {
	cats, err := d.deps.Repo().ListCategories(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug)
	if err != nil {
		return "", err
	}
	if id != "" {
		for _, c := range cats {
			if c.ID == id {
				return c.ID, nil
			}
		}
		// ID supplied but not in this project — fall through to try name lookup
		// in case the LLM mixed them up.
	}
	if name != "" {
		for _, c := range cats {
			if strings.EqualFold(c.Name, name) {
				return c.ID, nil
			}
		}
	}
	if id == "" && name == "" {
		return "", fmt.Errorf("category_id or category_name is required")
	}
	if id != "" && name == "" {
		return "", fmt.Errorf("category %s not found in current project", id)
	}
	return "", fmt.Errorf("no category named %q in current project", name)
}

// findCardCurrentCategory returns the category ID a card is currently pinned
// to within the given project. Used by `move_card` to auto-detect the source
// category when the LLM doesn't supply `from_category_id`.
//
// Walks the project's categories looking for a pin matching the card. If the
// card is pinned to multiple categories in this project (rare), returns the
// first one found. Returns an error if the card isn't pinned anywhere here.
func (d *Dispatcher) findCardCurrentCategory(scope ProjectChatScope, cardID string) (string, error) {
	cats, err := d.deps.Repo().ListCategories(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug)
	if err != nil {
		return "", err
	}
	for _, cat := range cats {
		pins, _ := d.deps.Repo().ListCardsInCategory(cat.ID)
		for _, p := range pins {
			if p.CardID == cardID {
				return cat.ID, nil
			}
		}
	}
	return "", fmt.Errorf("card %s is not pinned to any category in this project", cardID)
}

// applyCardUpdate applies a partial update to a single card. Used by both
// update_card (single) and update_cards (plural). Returns the list of fields
// that actually changed (used to build the tool-action result string).
//
// Supported keys in args:
//   - title (string)
//   - card_type (string)
//   - tags ([]string)              — REPLACE the card's tags
//   - tags_to_add ([]string)       — APPEND to existing tags (deduped)
//   - due_date (string)            — ISO 8601 date or datetime; "" clears
//   - description (string)         — sets the card's intrinsic description
//   - blocks ([]map)               — REPLACE the card's blocks entirely
//
// Unknown keys are ignored.
func (d *Dispatcher) applyCardUpdate(cardID string, args map[string]any) ([]string, error) {
	var changes []string

	if title, ok := args["title"].(string); ok && title != "" {
		if _, err := d.deps.Card().UpdateTitle(cardID, title); err == nil {
			changes = append(changes, "title")
		}
	}
	if cardType, ok := args["card_type"].(string); ok && cardType != "" {
		if _, err := d.deps.Card().UpdateType(cardID, cardType); err == nil {
			changes = append(changes, "type")
		}
	}

	// Tag handling: `tags` REPLACES, `tags_to_add` APPENDS. Both can be present.
	if tagsRaw, ok := args["tags"].([]any); ok {
		var newTags []string
		for _, raw := range tagsRaw {
			if s, ok := raw.(string); ok && s != "" {
				newTags = append(newTags, s)
			}
		}
		if _, err := d.deps.Card().UpdateTags(cardID, newTags); err == nil {
			changes = append(changes, "tags")
		}
	}
	if tagsRaw, ok := args["tags_to_add"].([]any); ok && len(tagsRaw) > 0 {
		c, err := d.deps.Repo().GetCard(cardID)
		if err == nil {
			existing := make(map[string]bool)
			for _, t := range c.Tags {
				existing[strings.ToLower(t)] = true
			}
			merged := c.Tags
			added := false
			for _, raw := range tagsRaw {
				if s, ok := raw.(string); ok && s != "" && !existing[strings.ToLower(s)] {
					merged = append(merged, s)
					existing[strings.ToLower(s)] = true
					added = true
				}
			}
			if added {
				d.deps.Card().UpdateTags(cardID, merged)
				if !contains(changes, "tags") {
					changes = append(changes, "tags")
				}
			}
		}
	}
	if tagsRaw, ok := args["tags_to_remove"].([]any); ok && len(tagsRaw) > 0 {
		c, err := d.deps.Repo().GetCard(cardID)
		if err == nil {
			remove := make(map[string]bool, len(tagsRaw))
			for _, raw := range tagsRaw {
				if s, ok := raw.(string); ok && s != "" {
					remove[strings.ToLower(s)] = true
				}
			}
			filtered := make([]string, 0, len(c.Tags))
			removed := false
			for _, t := range c.Tags {
				if remove[strings.ToLower(t)] {
					removed = true
					continue
				}
				filtered = append(filtered, t)
			}
			if removed {
				d.deps.Card().UpdateTags(cardID, filtered)
				if !contains(changes, "tags") {
					changes = append(changes, "tags")
				}
			}
		}
	}

	// Due date: empty string clears, non-empty parses as ISO date or datetime.
	if v, ok := args["due_date"].(string); ok {
		if v == "" {
			if _, err := d.deps.Card().UpdateDueDate(cardID, ""); err == nil {
				changes = append(changes, "due_date")
			}
		} else {
			if _, err := d.deps.Card().UpdateDueDate(cardID, v); err == nil {
				changes = append(changes, "due_date")
			}
		}
	}

	// Description is an intrinsic card property — set it directly rather
	// than smuggling it into a text block.
	if desc, ok := args["description"].(string); ok {
		if _, err := d.deps.Card().UpdateDescription(cardID, desc); err == nil {
			changes = append(changes, "description")
		}
	}

	// Block-level replacement (full restructure).
	if raw, ok := args["blocks"].([]any); ok {
		blocks := make([]model.Block, 0, len(raw))
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			b := model.Block{
				Type:  asString(m["type"]),
				Label: asString(m["label"]),
				Key:   asString(m["key"]),
				Value: m["value"],
			}
			if id := asString(m["id"]); id != "" {
				b.ID = id
			} else {
				b.ID = fmt.Sprintf("blk-%s", uuid.New().String()[:8])
			}
			blocks = append(blocks, b)
		}
		if _, err := d.deps.Card().UpdateBlocks(cardID, blocks); err == nil {
			changes = append(changes, "blocks")
		}
	}

	return changes, nil
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
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
func (d *Dispatcher) StageCard(tc llm.ToolCall, allCats []CategoryPath) (string, []model.PendingEdit) {
	one := func(tool string, input map[string]any, label, detail string) []model.PendingEdit {
		return []model.PendingEdit{{
			ID: uuid.New().String(), Tool: tool, Input: input,
			Label: label, Detail: detail, Status: "pending",
		}}
	}

	switch tc.Name {
	case "set_title":
		title, _ := tc.Arguments["title"].(string)
		return "Title will be set to " + title, one(tc.Name, tc.Arguments, "Set title", `"`+title+`"`)

	case "set_due_date":
		dueDate, _ := tc.Arguments["due_date"].(string)
		label, detail := "Set due date", dueDate
		if dueDate == "" {
			label, detail = "Clear due date", "Remove existing due date"
		}
		return "Due date staged", one(tc.Name, tc.Arguments, label, detail)

	case "set_card_type":
		cardType, _ := tc.Arguments["card_type"].(string)
		fakeResult := "Card type will be set to " + cardType + "."
		var previewBlocks []model.Block
		var store config.UserTypeStore
		if d.deps.Repo() != nil {
			store, _ = d.deps.Repo().LoadUserTypeStore()
		}
		for _, ut := range store.Types {
			if ut.ID == cardType && ut.TemplateID != "" {
				for _, tmpl := range store.Templates {
					if tmpl.ID == ut.TemplateID {
						previewBlocks = tmpl.Blocks
						break
					}
				}
				break
			}
		}
		if len(previewBlocks) == 0 {
			if ov, ok := store.BuiltinOverrides[cardType]; ok && ov.TemplateID != "" {
				for _, tmpl := range store.Templates {
					if tmpl.ID == ov.TemplateID {
						previewBlocks = tmpl.Blocks
						break
					}
				}
			}
		}
		if len(previewBlocks) == 0 && d.deps.Registry() != nil {
			previewBlocks = d.deps.Registry().SchemaToBlocks(cardType)
		}
		if len(previewBlocks) > 0 {
			var keys []string
			for _, b := range previewBlocks {
				if b.Key != "" {
					keys = append(keys, b.Key)
				}
			}
			if len(keys) > 0 {
				fakeResult += " NOW call set_fields to fill these field keys: " + strings.Join(keys, ", ")
			}
		}
		return fakeResult, one(tc.Name, tc.Arguments, "Set type", cardType)

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

	case "add_tags":
		tagsRaw, _ := tc.Arguments["tags"].([]any)
		var tags []string
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok {
				tags = append(tags, "+"+s)
			}
		}
		return "Tags staged", one(tc.Name, tc.Arguments, "Add tags", strings.Join(tags, ", "))

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

	case "configure_agent":
		enabled, _ := tc.Arguments["enabled"].(bool)
		goal, _ := tc.Arguments["goal"].(string)
		schedule, _ := tc.Arguments["schedule"].(string)
		label := "Configure agent"
		detail := fmt.Sprintf("Enabled: %v, Schedule: %s\nGoal: %s", enabled, schedule, goal)
		return "Agent configuration staged", one(tc.Name, tc.Arguments, label, detail)

	// Read-only tools bypass suggest-mode staging: there's nothing to
	// preview or approve — fetching a page or searching the web can't
	// mutate the card. Execute directly and feed the real content back
	// to the model so it can reason over it on the next iteration.
	case "web_fetch":
		url, _ := tc.Arguments["url"].(string)
		result, err := agent.WebFetch(url)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		return result, nil

	case "web_search":
		query, _ := tc.Arguments["query"].(string)
		result, err := agent.WebSearch(query)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		return result, nil

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
	case "create_card":
		title, _ := tc.Arguments["title"].(string)
		cardType, _ := tc.Arguments["card_type"].(string)
		if cardType == "" {
			cardType = "idea"
		}
		label := "Create card: " + title
		detail := fmt.Sprintf("Type: %s", cardType)
		// Pin destination — prefer name (more meaningful in the row), fall
		// back to resolving the ID to a name, finally raw ID.
		catName, _ := tc.Arguments["category_name"].(string)
		catID, _ := tc.Arguments["category_id"].(string)
		pinDisplay := catName
		if pinDisplay == "" && catID != "" {
			pinDisplay = d.categoryDisplayName(scope, catID)
		}
		if pinDisplay != "" {
			detail += "\nPin to category: " + pinDisplay
		}
		if desc, _ := tc.Arguments["description"].(string); desc != "" {
			detail += "\n\n" + desc
		}
		return "Card creation staged", []model.PendingEdit{{
			ID: uuid.New().String(), Tool: tc.Name, Input: tc.Arguments,
			Label: label, Detail: detail, Status: "pending",
		}}

	case "update_card":
		cardID, _ := tc.Arguments["card_id"].(string)
		if !inScope(cardID) {
			return "error: card " + cardID + " is not in the current project. Use only the card IDs listed in the system prompt.", nil
		}
		return formatStageResult(tc.Name), d.stageProjectCardUpdates(cardID, tc.Arguments)

	case "update_cards":
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
			// Stage as singular update_card edits regardless of which tool the
			// LLM called. The plural-vs-singular distinction only matters at
			// LLM-call time; on apply, each pending edit is one card / one
			// field, so the singular executor branch is the right path.
			edits := d.stageProjectCardUpdates(cardID, entry)
			allEdits = append(allEdits, edits...)
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
		cardIDsRaw, _ := tc.Arguments["card_ids"].([]any)
		tagsRaw, _ := tc.Arguments["tags"].([]any)
		var tags []string
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok && s != "" {
				tags = append(tags, s)
			}
		}
		var edits []model.PendingEdit
		var rejected []string
		for _, raw := range cardIDsRaw {
			cid, ok := raw.(string)
			if !ok || cid == "" {
				continue
			}
			if !inScope(cid) {
				rejected = append(rejected, cid)
				continue
			}
			cardLabel := d.cardDisplayLabel(cid)
			// Stage one edit per card so each can be approved individually,
			// but keep the `card_ids` (plural) shape so the executor matches
			// the original tool contract — it loops the array even when len=1.
			edits = append(edits, model.PendingEdit{
				ID:     uuid.New().String(),
				Tool:   tc.Name,
				Input:  map[string]any{"card_ids": []any{cid}, "tags": tagsRaw},
				Label:  cardLabel + " — add tags",
				Detail: "+" + strings.Join(tags, ", +"),
				Status: "pending",
			})
		}
		if len(rejected) > 0 && len(edits) == 0 {
			return "error: none of the supplied card_ids belong to the current project: " + strings.Join(rejected, ", "), nil
		}
		summary := fmt.Sprintf("Tag additions staged for %d cards", len(edits))
		if len(rejected) > 0 {
			summary += fmt.Sprintf(" (skipped %d out-of-project: %s)", len(rejected), strings.Join(rejected, ", "))
		}
		return summary, edits

	case "move_card":
		cardID, _ := tc.Arguments["card_id"].(string)
		if !inScope(cardID) {
			return "error: card " + cardID + " is not in the current project.", nil
		}
		// Display the destination by name when one is provided. The actual
		// resolution (id-or-name → id) happens at apply time, by which point
		// any just-staged create_category will have been applied first.
		toCatName, _ := tc.Arguments["to_category_name"].(string)
		toCatID, _ := tc.Arguments["to_category_id"].(string)
		toDisplay := toCatName
		if toDisplay == "" && toCatID != "" {
			toDisplay = d.categoryDisplayName(scope, toCatID)
		}
		if toDisplay == "" {
			toDisplay = "(unspecified)"
		}
		return "Move staged", []model.PendingEdit{{
			ID: uuid.New().String(), Tool: tc.Name, Input: tc.Arguments,
			Label:  d.cardDisplayLabel(cardID) + " — move",
			Detail: "To category: " + toDisplay,
			Status: "pending",
		}}

	case "configure_agent":
		cardID, _ := tc.Arguments["card_id"].(string)
		if !inScope(cardID) {
			return "error: card " + cardID + " is not in the current project.", nil
		}
		cardLabel := d.cardDisplayLabel(cardID)
		var edits []model.PendingEdit
		if v, ok := tc.Arguments["enabled"].(bool); ok {
			edits = append(edits, model.PendingEdit{
				ID: uuid.New().String(), Tool: tc.Name,
				Input:  map[string]any{"card_id": cardID, "enabled": v},
				Label:  cardLabel + " — agent enabled",
				Detail: fmt.Sprintf("Set agent enabled to %t", v),
				Status: "pending",
			})
		}
		if v, ok := tc.Arguments["schedule"].(string); ok {
			detail := "Schedule: " + v
			if v == "" {
				detail = "Clear schedule"
			}
			edits = append(edits, model.PendingEdit{
				ID: uuid.New().String(), Tool: tc.Name,
				Input:  map[string]any{"card_id": cardID, "schedule": v},
				Label:  cardLabel + " — agent schedule",
				Detail: detail,
				Status: "pending",
			})
		}
		if v, ok := tc.Arguments["goal"].(string); ok {
			edits = append(edits, model.PendingEdit{
				ID: uuid.New().String(), Tool: tc.Name,
				Input:  map[string]any{"card_id": cardID, "goal": v},
				Label:  cardLabel + " — agent goal",
				Detail: v,
				Status: "pending",
			})
		}
		if raw, ok := tc.Arguments["allowed_tools"].([]any); ok {
			var tools []string
			for _, t := range raw {
				if s, ok := t.(string); ok {
					tools = append(tools, s)
				}
			}
			edits = append(edits, model.PendingEdit{
				ID: uuid.New().String(), Tool: tc.Name,
				Input:  map[string]any{"card_id": cardID, "allowed_tools": raw},
				Label:  cardLabel + " — agent tools",
				Detail: strings.Join(tools, ", "),
				Status: "pending",
			})
		}
		return "Agent configuration staged", edits

	// --- Project metadata ---
	case "update_project":
		var edits []model.PendingEdit
		mk := func(field string, fieldArg any, detail string) {
			edits = append(edits, model.PendingEdit{
				ID:     uuid.New().String(),
				Tool:   "update_project",
				Input:  map[string]any{field: fieldArg},
				Label:  "Project — " + field,
				Detail: detail,
				Status: "pending",
			})
		}
		if v, ok := tc.Arguments["name"].(string); ok && v != "" {
			mk("name", v, v)
		}
		if v, ok := tc.Arguments["description"].(string); ok {
			detail := v
			if v == "" {
				detail = "Clear description"
			}
			mk("description", v, detail)
		}
		if v, ok := tc.Arguments["icon"].(string); ok {
			detail := v
			if v == "" {
				detail = "Clear icon"
			}
			mk("icon", v, detail)
		}
		return "Project update staged", edits

	// --- Project tags ---
	case "create_project_tag":
		name, _ := tc.Arguments["name"].(string)
		var detailParts []string
		if c, _ := tc.Arguments["color"].(string); c != "" {
			detailParts = append(detailParts, "color "+c)
		}
		if i, _ := tc.Arguments["icon"].(string); i != "" {
			detailParts = append(detailParts, "icon "+i)
		}
		detail := name
		if len(detailParts) > 0 {
			detail += " (" + strings.Join(detailParts, ", ") + ")"
		}
		return "Tag creation staged", []model.PendingEdit{{
			ID: uuid.New().String(), Tool: "create_project_tag", Input: tc.Arguments,
			Label: "Create tag — " + name, Detail: detail, Status: "pending",
		}}

	case "update_project_tag":
		// Resolve tag name for the row label so the user knows what's changing.
		tagLabel := "tag"
		if id, _ := tc.Arguments["tag_id"].(string); id != "" {
			tagLabel = d.tagDisplayName(scope, id, "")
		} else if name, _ := tc.Arguments["tag_name"].(string); name != "" {
			tagLabel = name
		}
		var edits []model.PendingEdit
		mk := func(field string, detail string) {
			edits = append(edits, model.PendingEdit{
				ID:     uuid.New().String(),
				Tool:   "update_project_tag",
				Input:  shallowCopyArgs(tc.Arguments, []string{"tag_id", "tag_name"}, field),
				Label:  tagLabel + " — " + field,
				Detail: detail,
				Status: "pending",
			})
		}
		if v, ok := tc.Arguments["name"].(string); ok {
			mk("name", v)
		}
		if v, ok := tc.Arguments["color"].(string); ok {
			mk("color", v)
		}
		if v, ok := tc.Arguments["icon"].(string); ok {
			detail := v
			if v == "" {
				detail = "Clear icon"
			}
			mk("icon", detail)
		}
		return "Tag update staged", edits

	case "delete_project_tag":
		tagLabel := "tag"
		if id, _ := tc.Arguments["tag_id"].(string); id != "" {
			tagLabel = d.tagDisplayName(scope, id, "")
		} else if name, _ := tc.Arguments["tag_name"].(string); name != "" {
			tagLabel = name
		}
		return "Tag deletion staged", []model.PendingEdit{{
			ID: uuid.New().String(), Tool: "delete_project_tag", Input: tc.Arguments,
			Label: "Delete tag — " + tagLabel, Detail: "Delete from project", Status: "pending",
		}}

	// --- Categories ---
	case "create_category":
		name, _ := tc.Arguments["name"].(string)
		return "Category creation staged", []model.PendingEdit{{
			ID: uuid.New().String(), Tool: "create_category", Input: tc.Arguments,
			Label: "Create category — " + name, Detail: name, Status: "pending",
		}}

	case "update_category":
		catID, _ := tc.Arguments["category_id"].(string)
		catName, _ := tc.Arguments["category_name"].(string)
		// Use the supplied name if present so the row label is meaningful even
		// when the LLM only provided category_name (a category that doesn't
		// exist yet at staging time). Apply will resolve the actual ID.
		catLabel := catName
		if catLabel == "" && catID != "" {
			catLabel = d.categoryDisplayName(scope, catID)
		}
		if catLabel == "" {
			catLabel = "category"
		}
		// Lookup keys preserved on each per-field PendingEdit so apply can
		// resolve the right category whichever form the LLM used.
		lookup := map[string]any{}
		if catID != "" {
			lookup["category_id"] = catID
		}
		if catName != "" {
			lookup["category_name"] = catName
		}
		var edits []model.PendingEdit
		mk := func(field string, fieldArg any, detail string) {
			input := map[string]any{}
			for k, v := range lookup {
				input[k] = v
			}
			input[field] = fieldArg
			edits = append(edits, model.PendingEdit{
				ID:     uuid.New().String(),
				Tool:   "update_category",
				Input:  input,
				Label:  catLabel + " — " + field,
				Detail: detail,
				Status: "pending",
			})
		}
		if v, ok := tc.Arguments["name"].(string); ok && v != "" {
			mk("name", v, v)
		}
		if v, ok := tc.Arguments["description"].(string); ok {
			detail := v
			if v == "" {
				detail = "Clear description"
			}
			mk("description", v, detail)
		}
		if v, ok := tc.Arguments["icon"].(string); ok {
			detail := v
			if v == "" {
				detail = "Clear icon"
			}
			mk("icon", v, detail)
		}
		if raw, ok := tc.Arguments["accepted_types"].([]any); ok {
			var types []string
			for _, t := range raw {
				if s, ok := t.(string); ok {
					types = append(types, s)
				}
			}
			detail := "Accept: " + strings.Join(types, ", ")
			if len(types) == 0 {
				detail = "Accept all card types"
			}
			mk("accepted_types", raw, detail)
		}
		return "Category update staged", edits

	case "delete_category":
		catID, _ := tc.Arguments["category_id"].(string)
		catName, _ := tc.Arguments["category_name"].(string)
		catLabel := catName
		if catLabel == "" && catID != "" {
			catLabel = d.categoryDisplayName(scope, catID)
		}
		if catLabel == "" {
			catLabel = "category"
		}
		return "Category deletion staged", []model.PendingEdit{{
			ID: uuid.New().String(), Tool: "delete_category", Input: tc.Arguments,
			Label: "Delete category — " + catLabel, Detail: "Cards will be unpinned to inbox", Status: "pending",
		}}

	// Read-only tools execute even in suggest mode — nothing to stage.
	case "web_fetch":
		url, _ := tc.Arguments["url"].(string)
		result, err := agent.WebFetch(url)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		return result, nil

	case "web_search":
		query, _ := tc.Arguments["query"].(string)
		result, err := agent.WebSearch(query)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		return result, nil

	default:
		return "Staged unknown tool " + tc.Name, nil
	}
}

// tagDisplayName returns a human-friendly name for a project tag, given
// either an ID or a name. Used in pending-edit row labels for the tag tools.
// Falls back to whichever value was provided if lookup fails.
func (d *Dispatcher) tagDisplayName(scope ProjectChatScope, tagID, tagName string) string {
	if tagName != "" {
		return tagName
	}
	if tagID == "" {
		return "tag"
	}
	labels, err := d.deps.Catalog().GetProjectLabels(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug)
	if err != nil {
		return tagID
	}
	for _, l := range labels {
		if l.ID == tagID {
			return l.Name
		}
	}
	return tagID
}

// categoryDisplayName returns the human name of a category by ID, falling back
// to the ID itself if the lookup fails.
func (d *Dispatcher) categoryDisplayName(scope ProjectChatScope, catID string) string {
	if catID == "" {
		return "category"
	}
	cats, err := d.deps.Repo().ListCategories(scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug)
	if err != nil {
		return catID
	}
	for _, c := range cats {
		if c.ID == catID {
			return c.Name
		}
	}
	return catID
}

// shallowCopyArgs builds a new map containing the listed lookup keys from src
// (e.g. tag_id, tag_name for the tag tools) plus a single named field. Used
// when staging multi-field updates so each PendingEdit's Input contains only
// the lookup info plus the one field that edit applies.
func shallowCopyArgs(src map[string]any, lookupKeys []string, field string) map[string]any {
	out := make(map[string]any, len(lookupKeys)+1)
	for _, k := range lookupKeys {
		if v, ok := src[k]; ok {
			out[k] = v
		}
	}
	if v, ok := src[field]; ok {
		out[field] = v
	}
	return out
}

// stageProjectCardUpdates expands a single-card update into one PendingEdit
// per field. Each edit is staged with `Tool: "update_card"` (the singular
// executor) regardless of whether the original LLM call was update_card or
// update_cards — see the call site comment for why.
func (d *Dispatcher) stageProjectCardUpdates(cardID string, args map[string]any) []model.PendingEdit {
	cardLabel := d.cardDisplayLabel(cardID)
	var edits []model.PendingEdit
	mkEdit := func(field string, fieldArg any, detail string) {
		edits = append(edits, model.PendingEdit{
			ID:     uuid.New().String(),
			Tool:   "update_card",
			Input:  map[string]any{"card_id": cardID, field: fieldArg},
			Label:  cardLabel + " — " + field,
			Detail: detail,
			Status: "pending",
		})
	}
	if v, ok := args["title"].(string); ok && v != "" {
		mkEdit("title", v, v)
	}
	if v, ok := args["card_type"].(string); ok && v != "" {
		mkEdit("card_type", v, v)
	}
	if raw, ok := args["tags"].([]any); ok {
		var tags []string
		for _, t := range raw {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
		detail := "Replace with: " + strings.Join(tags, ", ")
		if len(tags) == 0 {
			detail = "Remove all tags"
		}
		mkEdit("tags", raw, detail)
	}
	if raw, ok := args["tags_to_add"].([]any); ok {
		var tags []string
		for _, t := range raw {
			if s, ok := t.(string); ok {
				tags = append(tags, "+"+s)
			}
		}
		mkEdit("tags_to_add", raw, strings.Join(tags, ", "))
	}
	if raw, ok := args["tags_to_remove"].([]any); ok {
		var tags []string
		for _, t := range raw {
			if s, ok := t.(string); ok {
				tags = append(tags, "−"+s)
			}
		}
		mkEdit("tags_to_remove", raw, strings.Join(tags, ", "))
	}
	if v, ok := args["due_date"].(string); ok {
		detail := v
		if v == "" {
			detail = "Clear due date"
		}
		mkEdit("due_date", v, detail)
	}
	if v, ok := args["description"].(string); ok {
		mkEdit("description", v, v)
	}
	if raw, ok := args["blocks"].([]any); ok {
		mkEdit("blocks", raw, fmt.Sprintf("Replace with %d blocks", len(raw)))
	}
	return edits
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

// formatStageResult builds a human-readable acknowledgement string for the LLM.
func formatStageResult(toolName string) string {
	return "Edits staged for " + toolName + " — awaiting user approval"
}
