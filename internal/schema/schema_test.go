package schema

import (
	"testing"
)

func TestNewRegistryLoadsBuiltinTypes(t *testing.T) {
	reg, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	expected := []string{"brainstorm", "episode", "feature", "reference", "task"}
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
