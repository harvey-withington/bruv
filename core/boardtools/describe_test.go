package boardtools_test

import (
	"strings"
	"testing"

	"bruv/core/boardtools"
)

// The UNCLI chat's Suggest-mode approval card: what a call does, to which
// card, and what changes (dogfooding, 2026-11-05).
func TestDescribeCall(t *testing.T) {
	rt := newBoard(t)
	created, _, _ := call(t, rt, nil, "create_card", map[string]any{"title": "Trip Planning Melbourne"})
	id := created["card_id"].(string)

	title, detail := boardtools.DescribeCall(rt, "update_card", map[string]any{
		"card_id": id,
		"updates": []any{map[string]any{"key": "Preparation to Leave", "value": []any{
			map[string]any{"text": "Flight", "done": true}, "Book airport transport",
		}}},
	})
	if title != "Update Preparation to Leave on “Trip Planning Melbourne”" {
		t.Errorf("title = %q", title)
	}
	if detail != "Preparation to Leave: 2 item(s): Flight, Book airport transport" {
		t.Errorf("detail = %q", detail)
	}

	title, detail = boardtools.DescribeCall(rt, "set_card_title", map[string]any{"card_id": id, "title": "Melbourne, December"})
	if title != "Set title → Melbourne, December on “Trip Planning Melbourne”" || detail != "title: Melbourne, December" {
		t.Errorf("set_card_title = %q / %q", title, detail)
	}

	// A project tool: its project isn't a change.
	title, detail = boardtools.DescribeCall(rt, "move_card", map[string]any{
		"card_id": id, "to_category_name": "Booked", "brand": "home", "stream": "trips", "project": "winter",
	})
	if title != "Move to Booked on “Trip Planning Melbourne”" || detail != "to_category_name: Booked" {
		t.Errorf("move_card = %q / %q", title, detail)
	}
	if title, _ = boardtools.DescribeCall(nil, "delete_category", map[string]any{"category_name": "Old"}); title != "Delete category Old" {
		t.Errorf("delete_category = %q", title)
	}

	// Without a board, and with a long value: no card name, a short line.
	title, detail = boardtools.DescribeCall(nil, "set_card_description", map[string]any{"card_id": id, "description": strings.Repeat("word ", 100)})
	if title != "Set the description" || len([]rune(detail)) > 160 || !strings.HasSuffix(detail, "…") {
		t.Errorf("long description = %q / %q", title, detail)
	}
}
