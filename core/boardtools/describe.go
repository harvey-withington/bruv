package boardtools

// DescribeCall words a board-tool call for the user to decide on before
// it runs: a change the UNCLI chat's Suggest mode staged, on its review
// card (supervisor.StageUncliEdit). Title is what it does and to which
// card ("Update Preparation to Leave on “Trip Planning Melbourne”");
// Detail is what changes, a line each, plain text.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// promptVerbs turns Summary's past tense into a prompt's imperative.
var promptVerbs = map[string]string{
	"Updated": "Update", "Added": "Add", "Created": "Create", "Removed": "Remove",
	"Cleared": "Clear", "Configured": "Configure", "Set": "Set", "Moved": "Move", "Deleted": "Delete",
}

// projectTools are the tools whose brand/stream/project only say which
// project they change (a project chat fills its own in), not a change.
var projectTools = func() map[string]bool {
	m := map[string]bool{}
	for _, t := range projectDefs("") {
		m[t.Name] = true
	}
	return m
}()

// DescribeCall needs b only to name the card; a nil b leaves it out.
func DescribeCall(b Board, name string, args map[string]any) (title, detail string) {
	title = Summary(name, args, "")
	if verb, rest, ok := strings.Cut(title, " "); ok && promptVerbs[verb] != "" {
		title = promptVerbs[verb] + " " + rest
	} else if r := []rune(title); len(r) > 0 {
		title = "Set " + strings.ToLower(string(r[:1])) + string(r[1:]) // "Title → …" → "Set title → …"
	}
	if id := argStr(args, "card_id"); id != "" && b != nil {
		if card, err := b.GetCard(id); err == nil && card.Title != "" {
			title += " on “" + card.Title + "”"
		}
	}
	return title, strings.Join(changeLines(name, args), "\n")
}

// changeLines lists what a call would change, one line per field.
func changeLines(name string, args map[string]any) []string {
	var lines []string
	if name == "update_card" {
		for _, k := range []string{"title", "due_date", "tags"} {
			if v, ok := args[k]; ok {
				lines = append(lines, fmt.Sprintf("%s: %s", k, preview(v)))
			}
		}
		updates, _ := args["updates"].([]any)
		for _, u := range updates {
			if m, ok := u.(map[string]any); ok {
				lines = append(lines, fmt.Sprintf("%s: %s", argStr(m, "key"), preview(m["value"])))
			}
		}
		return lines
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		if k == "card_id" || (projectTools[name] && (k == "brand" || k == "stream" || k == "project")) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s: %s", k, preview(args[k])))
	}
	return lines
}

const previewMax = 140

// preview is a short, one-line rendering of an argument's value: a list's
// items by their text, an object as compact JSON.
func preview(v any) string {
	var s string
	switch x := v.(type) {
	case nil:
		s = "(empty)"
	case string:
		s = x
		if s == "" {
			s = "(empty)"
		}
	case []any:
		items := make([]string, 0, len(x))
		for _, it := range x {
			switch e := it.(type) {
			case string:
				items = append(items, e)
			case map[string]any:
				items = append(items, firstNonEmpty(argStr(e, "text"), firstNonEmpty(argStr(e, "label"), argStr(e, "key"))))
			default:
				items = append(items, fmt.Sprint(e))
			}
		}
		s = fmt.Sprintf("%d item(s): %s", len(x), strings.Join(items, ", "))
	case map[string]any:
		b, _ := json.Marshal(x)
		s = string(b)
	default:
		s = fmt.Sprint(x)
	}
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > previewMax {
		s = string(r[:previewMax-1]) + "…"
	}
	return s
}
