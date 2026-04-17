package llm

import (
	"testing"
)

func TestCardToolsBaseCount(t *testing.T) {
	tools := CardTools(nil, nil, nil)
	// Base tools: set_title, set_due_date, set_card_type, set_fields,
	// add_tags, add_field, suggest_pin, configure_agent, web_fetch,
	// web_search.
	if len(tools) != 10 {
		t.Errorf("expected 10 base tools, got %d", len(tools))
	}
}

func TestCardToolsAllHaveNameAndDescription(t *testing.T) {
	tools := CardTools([]string{"feature", "task"}, []map[string]string{
		{"id": "cat-1", "name": "Backlog"},
	}, nil)
	for _, tool := range tools {
		if tool.Name == "" {
			t.Error("tool has empty Name")
		}
		if tool.Description == "" {
			t.Errorf("tool %q has empty Description", tool.Name)
		}
		if tool.Parameters == nil {
			t.Errorf("tool %q has nil Parameters", tool.Name)
		}
	}
}

func TestCardToolsSetCardTypeEnum(t *testing.T) {
	types := []string{"feature", "task", "brainstorm"}
	tools := CardTools(types, nil, nil)

	var setCardType *ToolDef
	for i := range tools {
		if tools[i].Name == "set_card_type" {
			setCardType = &tools[i]
			break
		}
	}
	if setCardType == nil {
		t.Fatal("set_card_type tool not found")
	}

	props := setCardType.Parameters["properties"].(map[string]any)
	cardTypeProp := props["card_type"].(map[string]any)
	enum := cardTypeProp["enum"].([]any)

	if len(enum) != len(types) {
		t.Fatalf("expected %d enum values, got %d", len(types), len(enum))
	}
	for i, want := range types {
		if enum[i] != want {
			t.Errorf("enum[%d] = %q, want %q", i, enum[i], want)
		}
	}
}

func TestCardToolsEmptyCardTypes(t *testing.T) {
	tools := CardTools([]string{}, nil, nil)

	var setCardType *ToolDef
	for i := range tools {
		if tools[i].Name == "set_card_type" {
			setCardType = &tools[i]
			break
		}
	}
	if setCardType == nil {
		t.Fatal("set_card_type tool not found")
	}

	props := setCardType.Parameters["properties"].(map[string]any)
	cardTypeProp := props["card_type"].(map[string]any)
	enum := cardTypeProp["enum"].([]any)
	if len(enum) != 0 {
		t.Errorf("expected empty enum, got %d values", len(enum))
	}
}

func TestCardToolsSuggestPinWithCategories(t *testing.T) {
	categories := []map[string]string{
		{"id": "cat-1", "name": "Backlog"},
		{"id": "cat-2", "name": "Done"},
	}
	tools := CardTools(nil, categories, nil)

	var suggestPin *ToolDef
	for i := range tools {
		if tools[i].Name == "suggest_pin" {
			suggestPin = &tools[i]
			break
		}
	}
	if suggestPin == nil {
		t.Fatal("suggest_pin tool not found")
	}

	props := suggestPin.Parameters["properties"].(map[string]any)
	catProp, ok := props["category_id"].(map[string]any)
	if !ok {
		t.Fatal("suggest_pin should have category_id when categories are provided")
	}

	enum := catProp["enum"].([]any)
	if len(enum) != 2 {
		t.Fatalf("expected 2 category enum values, got %d", len(enum))
	}
	if enum[0] != "cat-1" || enum[1] != "cat-2" {
		t.Errorf("unexpected category enum: %v", enum)
	}
}

func TestCardToolsSuggestPinWithoutCategories(t *testing.T) {
	tools := CardTools(nil, nil, nil)

	var suggestPin *ToolDef
	for i := range tools {
		if tools[i].Name == "suggest_pin" {
			suggestPin = &tools[i]
			break
		}
	}
	if suggestPin == nil {
		t.Fatal("suggest_pin tool not found")
	}

	props := suggestPin.Parameters["properties"].(map[string]any)
	if _, ok := props["category_id"]; ok {
		t.Error("suggest_pin should NOT have category_id when no categories provided")
	}
}

func TestCardToolsSuggestPinAlwaysHasHierarchyFields(t *testing.T) {
	tools := CardTools(nil, nil, nil)

	var suggestPin *ToolDef
	for i := range tools {
		if tools[i].Name == "suggest_pin" {
			suggestPin = &tools[i]
			break
		}
	}
	if suggestPin == nil {
		t.Fatal("suggest_pin tool not found")
	}

	props := suggestPin.Parameters["properties"].(map[string]any)
	for _, field := range []string{"brand", "stream", "project", "category", "reason"} {
		if _, ok := props[field]; !ok {
			t.Errorf("suggest_pin missing expected field %q", field)
		}
	}
}

func TestCardToolsExpectedNames(t *testing.T) {
	tools := CardTools([]string{"feature"}, []map[string]string{{"id": "c1", "name": "Cat"}}, nil)
	expected := map[string]bool{
		"set_title":       true,
		"set_due_date":    true,
		"set_card_type":   true,
		"set_fields":      true,
		"add_tags":        true,
		"add_field":       true,
		"suggest_pin":     true,
		"configure_agent": true,
		"web_fetch":       true,
		"web_search":      true,
	}
	for _, tool := range tools {
		if !expected[tool.Name] {
			t.Errorf("unexpected tool name: %q", tool.Name)
		}
		delete(expected, tool.Name)
	}
	for name := range expected {
		t.Errorf("missing expected tool: %q", name)
	}
}

func TestWebToolsExposedToChat(t *testing.T) {
	// Contract test — card and project chat must expose web_fetch and
	// web_search, but never http_request (that's agent-only by design).
	cards := CardTools(nil, nil, nil)
	projects := ProjectTools(nil, nil)
	for _, set := range []struct {
		name  string
		tools []ToolDef
	}{{"CardTools", cards}, {"ProjectTools", projects}} {
		hasFetch, hasSearch, hasHTTP := false, false, false
		for _, tool := range set.tools {
			switch tool.Name {
			case "web_fetch":
				hasFetch = true
			case "web_search":
				hasSearch = true
			case "http_request":
				hasHTTP = true
			}
		}
		if !hasFetch {
			t.Errorf("%s: missing web_fetch", set.name)
		}
		if !hasSearch {
			t.Errorf("%s: missing web_search", set.name)
		}
		if hasHTTP {
			t.Errorf("%s: must not expose http_request to chat (agent-only)", set.name)
		}
	}
}
