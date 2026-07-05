package schema

import (
	"bruv/internal/model"
	"testing"
)

func TestNewRegistryLoadsBuiltinTypes(t *testing.T) {
	reg, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	expected := []string{"agent", "brainstorm", "episode", "feature", "reference", "task"}
	types := reg.List()

	if len(types) != len(expected) {
		t.Fatalf("type count = %d, want %d", len(types), len(expected))
	}

	for _, name := range expected {
		schema := reg.Get(name)
		if schema == nil {
			t.Errorf("missing type: %q", name)
			continue
		}
		if schema.Name == "" {
			t.Errorf("type %q has empty title", name)
		}
		if schema.Description == "" {
			t.Errorf("type %q has empty description", name)
		}
	}
}

func TestGetUnknownTypeReturnsNil(t *testing.T) {
	reg, _ := NewRegistry()
	if got := reg.Get("nonexistent"); got != nil {
		t.Errorf("expected nil for unknown type, got %+v", got)
	}
}

func TestValidateRequiredFields(t *testing.T) {
	reg, _ := NewRegistry()

	// Feature requires "description"
	errs := reg.Validate("feature", map[string]any{})
	if len(errs) == 0 {
		t.Error("expected validation error for missing required field")
	}

	// Provide the required field
	errs = reg.Validate("feature", map[string]any{
		"description": "A valid feature",
	})
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestValidateEnumConstraints(t *testing.T) {
	reg, _ := NewRegistry()

	// Valid enum value
	errs := reg.Validate("feature", map[string]any{
		"description": "Test",
		"complexity":  "medium",
	})
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	// Invalid enum value
	errs = reg.Validate("feature", map[string]any{
		"description": "Test",
		"complexity":  "enormous",
	})
	if len(errs) == 0 {
		t.Error("expected validation error for invalid enum value")
	}
}

func TestValidateUnknownType(t *testing.T) {
	reg, _ := NewRegistry()
	errs := reg.Validate("spaceship", map[string]any{})
	if len(errs) == 0 {
		t.Error("expected error for unknown type")
	}
}

func TestValidateUnknownFieldsAllowed(t *testing.T) {
	reg, _ := NewRegistry()

	// Brainstorm has no required fields — extra fields should be fine
	errs := reg.Validate("brainstorm", map[string]any{
		"custom_field": "whatever",
		"notes":        "some ideas",
	})
	if len(errs) != 0 {
		t.Errorf("unexpected errors for extensible fields: %v", errs)
	}
}

func TestSchemaToBlocks_Episode(t *testing.T) {
	reg, _ := NewRegistry()
	blocks := reg.SchemaToBlocks("episode")
	if blocks == nil {
		t.Fatal("expected blocks for episode schema")
	}

	// Episode has 5 properties: description (required), recording_status, publish_date, show_notes, duration_minutes
	if len(blocks) != 5 {
		t.Fatalf("expected 5 blocks, got %d", len(blocks))
	}

	// First block should be the required field (description)
	if blocks[0].Key != "description" {
		t.Errorf("expected first block key = 'description' (required), got %q", blocks[0].Key)
	}
	if !blocks[0].Required {
		t.Error("description block should be required")
	}
	if blocks[0].Type != model.BlockText {
		t.Errorf("description should be text, got %s", blocks[0].Type)
	}

	// Check that enum fields become select blocks
	blocksByKey := make(map[string]model.Block)
	for _, b := range blocks {
		blocksByKey[b.Key] = b
	}

	rsBlock, ok := blocksByKey["recording_status"]
	if !ok {
		t.Fatal("missing recording_status block")
	}
	if rsBlock.Type != model.BlockSelect {
		t.Errorf("recording_status should be select, got %s", rsBlock.Type)
	}
	if rsBlock.Meta == nil {
		t.Fatal("recording_status should have meta with options")
	}

	// Check date field
	pdBlock, ok := blocksByKey["publish_date"]
	if !ok {
		t.Fatal("missing publish_date block")
	}
	if pdBlock.Type != model.BlockDate {
		t.Errorf("publish_date should be date, got %s", pdBlock.Type)
	}

	// Check integer field
	dmBlock, ok := blocksByKey["duration_minutes"]
	if !ok {
		t.Fatal("missing duration_minutes block")
	}
	if dmBlock.Type != model.BlockNumber {
		t.Errorf("duration_minutes should be number, got %s", dmBlock.Type)
	}
}

func TestSchemaToBlocks_UnknownType(t *testing.T) {
	reg, _ := NewRegistry()
	blocks := reg.SchemaToBlocks("nonexistent")
	if blocks != nil {
		t.Errorf("expected nil for unknown type, got %v", blocks)
	}
}

func TestSchemaToBlocks_AllBlocksHaveIDs(t *testing.T) {
	reg, _ := NewRegistry()
	for _, typeName := range reg.List() {
		blocks := reg.SchemaToBlocks(typeName)
		for i, b := range blocks {
			if b.ID == "" {
				t.Errorf("type %q block %d has empty ID", typeName, i)
			}
			if b.Key == "" {
				t.Errorf("type %q block %d has empty Key", typeName, i)
			}
			if b.Label == "" {
				t.Errorf("type %q block %d has empty Label", typeName, i)
			}
		}
	}
}
