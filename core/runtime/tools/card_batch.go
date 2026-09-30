package tools

// Project chat's batch card edits (update_cards, add_tags_to_cards) as
// loops over the native registry (core/boardtools): each per-card field
// becomes the native tool that really performs it — set_card_title,
// set_card_type, add_card_tags, remove_card_tags, set_card_due_date,
// set_card_description, update_card (block values), add_card_blocks.
// Edit mode runs those calls; Suggest mode stages each one as its own
// pending edit, so accepting a row replays exactly the call that makes
// the change (a staged edit the native tools can't perform used to be
// ticked green while the card stayed the same).

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"bruv/internal/llm"
	"bruv/internal/model"
)

// nativeCall is one native registry call a batch edit expands into.
type nativeCall struct {
	Tool   string
	Args   map[string]any
	Field  string // what the call changes, for a pending-edit row label
	Detail string
}

// cardUpdateCalls expands one update_cards entry into native calls, in a
// stable field order.
func (d *Dispatcher) cardUpdateCalls(cardID string, entry map[string]any) []nativeCall {
	var calls []nativeCall
	add := func(tool, field, detail string, args map[string]any) {
		args["card_id"] = cardID
		calls = append(calls, nativeCall{Tool: tool, Args: args, Field: field, Detail: detail})
	}
	if v, ok := entry["title"].(string); ok && v != "" {
		add("set_card_title", "title", v, map[string]any{"title": v})
	}
	if v, ok := entry["card_type"].(string); ok && v != "" {
		add("set_card_type", "card_type", v, map[string]any{"card_type": v})
	}
	if raw, ok := entry["tags"].([]any); ok {
		if tags := stringList(raw); len(tags) > 0 {
			add("update_card", "tags", "Replace with: "+strings.Join(tags, ", "), map[string]any{"tags": toAnySlice(tags)})
		} else {
			add("remove_card_tags", "tags", "Remove all tags", map[string]any{"all": true})
		}
	}
	if tags := stringList(entry["tags_to_add"]); len(tags) > 0 {
		add("add_card_tags", "tags_to_add", "+"+strings.Join(tags, ", +"), map[string]any{"tags": toAnySlice(tags)})
	}
	if tags := stringList(entry["tags_to_remove"]); len(tags) > 0 {
		add("remove_card_tags", "tags_to_remove", "−"+strings.Join(tags, ", −"), map[string]any{"tags": toAnySlice(tags)})
	}
	if v, ok := entry["due_date"].(string); ok {
		detail := v
		if v == "" {
			detail = "Clear due date"
		}
		add("set_card_due_date", "due_date", detail, map[string]any{"due_date": v})
	}
	if v, ok := entry["description"].(string); ok {
		detail := v
		if v == "" {
			detail = "Clear description"
		}
		add("set_card_description", "description", detail, map[string]any{"description": v})
	}
	if raw, ok := entry["blocks"].([]any); ok && len(raw) > 0 {
		updates, fresh := d.splitBlockEdits(cardID, raw)
		if len(updates) > 0 {
			add("update_card", "blocks", fmt.Sprintf("Set %d field value(s)", len(updates)), map[string]any{"updates": updates})
		}
		if len(fresh) > 0 {
			add("add_card_blocks", "blocks", fmt.Sprintf("Add %d field(s)", len(fresh)), map[string]any{"blocks": fresh})
		}
	}
	return calls
}

// splitBlockEdits sorts update_cards blocks into values for fields the
// card already has (matched by key, then label — set through update_card,
// which shapes the value to the field and keeps its settings) and new
// fields (added through add_card_blocks). Nothing is ever deleted.
func (d *Dispatcher) splitBlockEdits(cardID string, raw []any) (updates, fresh []any) {
	var existing []model.Block
	if c, err := d.deps.Repo().GetCard(cardID); err == nil {
		existing = c.Blocks
	}
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key, _ := m["key"].(string)
		label, _ := m["label"].(string)
		if target := matchBlock(existing, strings.TrimSpace(key), strings.TrimSpace(label)); target != "" {
			updates = append(updates, map[string]any{"key": target, "value": m["value"]})
			continue
		}
		fresh = append(fresh, m)
	}
	return updates, fresh
}

// matchBlock returns the handle update_card resolves an existing block by
// (its key, else its label), or "" when no block matches key or label.
func matchBlock(blocks []model.Block, key, label string) string {
	for _, b := range blocks {
		if key != "" && b.Key == key {
			return b.Key
		}
	}
	for _, b := range blocks {
		if label != "" && strings.EqualFold(b.Label, label) {
			if b.Key != "" {
				return b.Key
			}
			return b.Label
		}
	}
	return ""
}

func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// cardOutcome is what a batch did to one card.
type cardOutcome struct {
	cardID string
	done   []string
	errs   []string
}

// runCardBatch runs each card's native calls inside scope and reports
// per card what changed and what failed — every service error reaches
// the model instead of a blanket "Updated N cards".
func (d *Dispatcher) runCardBatch(scope *ProjectChatScope, tool string, args map[string]any, perCard []cardCalls) (string, *model.ToolAction) {
	n := d.deps.Native()
	if n == nil {
		return "error: board tools are unavailable", nil
	}
	outcomes := make([]cardOutcome, 0, len(perCard))
	for _, pc := range perCard {
		o := cardOutcome{cardID: pc.cardID}
		if len(pc.calls) == 0 {
			o.errs = append(o.errs, "nothing to change")
		}
		for _, c := range pc.calls {
			result, isErr := n.Call(scope, c.Tool, c.Args)
			if isErr {
				o.errs = append(o.errs, c.Field+": "+strings.TrimPrefix(result, "error: "))
				continue
			}
			o.done = append(o.done, n.Summary(c.Tool, c.Args, result))
		}
		outcomes = append(outcomes, o)
	}
	result := formatBatch(outcomes)
	headline, _, _ := strings.Cut(result, "\n")
	return result, &model.ToolAction{Tool: tool, Input: args, Result: headline}
}

// cardCalls pairs a card with the native calls for it.
type cardCalls struct {
	cardID string
	calls  []nativeCall
}

// formatBatch renders per-card outcomes for the model. It leads with
// "error:" only when nothing at all was changed.
func formatBatch(outcomes []cardOutcome) string {
	clean, changed := 0, 0
	var lines []string
	for _, o := range outcomes {
		if len(o.errs) == 0 {
			clean++
		}
		if len(o.done) > 0 {
			changed++
			lines = append(lines, fmt.Sprintf("- %s: %s", o.cardID, strings.Join(o.done, "; ")))
		}
		if len(o.errs) > 0 {
			lines = append(lines, fmt.Sprintf("- %s FAILED: %s", o.cardID, strings.Join(o.errs, "; ")))
		}
	}
	var head string
	switch {
	case clean == len(outcomes):
		head = fmt.Sprintf("Updated %d card(s)", len(outcomes))
	case changed == 0:
		head = fmt.Sprintf("error: no card was changed (%d failed)", len(outcomes)-clean)
	default:
		head = fmt.Sprintf("Updated %d of %d card(s); %d had errors — fix and retry only the failed parts", clean, len(outcomes), len(outcomes)-clean)
	}
	return strings.Join(append([]string{head}, lines...), "\n")
}

// updateCardsBatch is edit-mode update_cards.
func (d *Dispatcher) updateCardsBatch(tc llm.ToolCall, scope *ProjectChatScope) (string, *model.ToolAction) {
	updates, _ := tc.Arguments["updates"].([]any)
	if len(updates) == 0 {
		return "error: updates array is required", nil
	}
	var perCard []cardCalls
	for _, raw := range updates {
		entry, _ := raw.(map[string]any)
		cardID, _ := entry["card_id"].(string)
		if cardID == "" {
			perCard = append(perCard, cardCalls{cardID: "(missing card_id)"})
			continue
		}
		perCard = append(perCard, cardCalls{cardID: cardID, calls: d.cardUpdateCalls(cardID, entry)})
	}
	return d.runCardBatch(scope, tc.Name, tc.Arguments, perCard)
}

// addTagsBatch is edit-mode add_tags_to_cards: add_card_tags per card.
func (d *Dispatcher) addTagsBatch(tc llm.ToolCall, scope *ProjectChatScope) (string, *model.ToolAction) {
	cardIDs := stringList(tc.Arguments["card_ids"])
	tags := stringList(tc.Arguments["tags"])
	if len(cardIDs) == 0 || len(tags) == 0 {
		return "error: card_ids and tags are required", nil
	}
	perCard := make([]cardCalls, 0, len(cardIDs))
	for _, id := range cardIDs {
		perCard = append(perCard, cardCalls{cardID: id, calls: []nativeCall{{
			Tool: "add_card_tags", Field: "tags", Args: map[string]any{"card_id": id, "tags": toAnySlice(tags)},
		}}})
	}
	return d.runCardBatch(scope, tc.Name, tc.Arguments, perCard)
}

// stageCardCalls turns one card's native calls into pending edits, one
// row per call, labelled with the card's title.
func (d *Dispatcher) stageCardCalls(cardID string, calls []nativeCall) []model.PendingEdit {
	cardLabel := d.cardDisplayLabel(cardID)
	edits := make([]model.PendingEdit, 0, len(calls))
	for _, c := range calls {
		edits = append(edits, model.PendingEdit{
			ID:     uuid.New().String(),
			Tool:   c.Tool,
			Input:  c.Args,
			Label:  cardLabel + " — " + c.Field,
			Detail: c.Detail,
			Status: "pending",
		})
	}
	return edits
}
