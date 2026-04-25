package tools

import (
	"bruv/internal/model"
	"testing"
)

// ---------------------------------------------------------------------------
// coerceBlockValue — type dispatch
// ---------------------------------------------------------------------------

func TestCoerceBlockValue_Text(t *testing.T) {
	got := coerceBlockValue(model.BlockText, "hello")
	if got != "hello" {
		t.Fatalf("expected 'hello', got %v", got)
	}
}

// ---------------------------------------------------------------------------
// coerceNumber
// ---------------------------------------------------------------------------

func TestCoerceNumber(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want float64
	}{
		{"float64", float64(42), 42},
		{"int", int(7), 7},
		{"string", "3.14", 3.14},
		{"string with spaces", " 99 ", 99},
		{"unparseable", "abc", 0},
		{"nil", nil, 0},
		{"bool", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coerceNumber(tt.in)
			if got != tt.want {
				t.Errorf("coerceNumber(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// coerceCheckbox
// ---------------------------------------------------------------------------

func TestCoerceCheckbox(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want bool
	}{
		{"bool true", true, true},
		{"bool false", false, false},
		{"string true", "true", true},
		{"string True", "True", true},
		{"string yes", "yes", true},
		{"string 1", "1", true},
		{"string false", "false", false},
		{"string no", "no", false},
		{"float nonzero", float64(1), true},
		{"float zero", float64(0), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coerceCheckbox(tt.in)
			if got != tt.want {
				t.Errorf("coerceCheckbox(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// coerceList
// ---------------------------------------------------------------------------

func TestCoerceList_Array(t *testing.T) {
	input := []any{"alpha", "bravo", "charlie"}
	got := coerceList(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got))
	}
	for i, want := range []string{"alpha", "bravo", "charlie"} {
		text, _ := got[i]["text"].(string)
		if text != want {
			t.Errorf("item %d text = %q, want %q", i, text, want)
		}
		if _, ok := got[i]["id"].(string); !ok {
			t.Errorf("item %d missing id", i)
		}
	}
}

func TestCoerceList_MapItems(t *testing.T) {
	input := []any{
		map[string]any{"text": "item 1"},
		map[string]any{"text": "item 2"},
	}
	got := coerceList(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	if got[0]["text"] != "item 1" {
		t.Errorf("item 0 text = %v, want 'item 1'", got[0]["text"])
	}
}

func TestCoerceList_NewlineSplit(t *testing.T) {
	got := coerceList("- alpha\n- bravo\n- charlie")
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got))
	}
	if got[0]["text"] != "alpha" {
		t.Errorf("item 0 = %q, want 'alpha' (prefix should be stripped)", got[0]["text"])
	}
}

func TestCoerceList_EmptyItems(t *testing.T) {
	got := coerceList([]any{"", "only"})
	if len(got) != 1 {
		t.Fatalf("expected 1 item (empty filtered), got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// coerceChecklist
// ---------------------------------------------------------------------------

func TestCoerceChecklist_StringArray(t *testing.T) {
	input := []any{"task one", "task two"}
	got := coerceChecklist(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	if got[0]["text"] != "task one" {
		t.Errorf("item 0 text = %v", got[0]["text"])
	}
	if got[0]["done"] != false {
		t.Errorf("item 0 done should default to false")
	}
}

func TestCoerceChecklist_MapArray(t *testing.T) {
	input := []any{
		map[string]any{"text": "done task", "done": true},
		map[string]any{"text": "pending task", "done": false},
	}
	got := coerceChecklist(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	if got[0]["done"] != true {
		t.Errorf("item 0 should be done")
	}
	if got[1]["done"] != false {
		t.Errorf("item 1 should not be done")
	}
}

func TestCoerceChecklist_NewlineSplit(t *testing.T) {
	got := coerceChecklist("* buy milk\n* walk dog")
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	if got[0]["text"] != "buy milk" {
		t.Errorf("item 0 = %q, want 'buy milk'", got[0]["text"])
	}
}

func TestCoerceChecklist_EmptyFiltered(t *testing.T) {
	got := coerceChecklist([]any{"", "keep"})
	if len(got) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// stripListPrefix
// ---------------------------------------------------------------------------

func TestStripListPrefix(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"- item", "item"},
		{"* item", "item"},
		{"• item", "item"},
		{"1. item", "item"},
		{"12) item", "item"},
		{"  - indented", "indented"},
		{"no prefix", "no prefix"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := stripListPrefix(tt.in)
			if got != tt.want {
				t.Errorf("stripListPrefix(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// normaliseDateValue
// ---------------------------------------------------------------------------

func TestNormaliseDateValue(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		format string
		want   string
		err    bool
	}{
		{"date only", "2026-04-12", "", "2026-04-12", false},
		{"RFC3339", "2026-04-12T10:30:00Z", "", "2026-04-12", false},
		{"RFC3339 datetime format", "2026-04-12T10:30:00Z", "date-time", "2026-04-12T10:30:00Z", false},
		{"datetime without tz", "2026-04-12T10:30:00", "", "2026-04-12", false},
		{"space separator", "2026-04-12 10:30:00", "", "2026-04-12", false},
		{"short datetime", "2026-04-12T10:30", "", "2026-04-12", false},
		{"empty", "", "", "", false},
		{"unparseable", "not a date", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normaliseDateValue(tt.raw, tt.format)
			if (err != nil) != tt.err {
				t.Fatalf("normaliseDateValue(%q, %q) error = %v, wantErr %v", tt.raw, tt.format, err, tt.err)
			}
			if got != tt.want {
				t.Errorf("normaliseDateValue(%q, %q) = %q, want %q", tt.raw, tt.format, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CoerceBlockValueForBlock — meta-aware constraints
// ---------------------------------------------------------------------------

func TestCoerceBlockValueForBlock_SelectValid(t *testing.T) {
	b := &model.Block{Type: model.BlockSelect, Meta: map[string]any{"options": []any{"red", "green", "blue"}}}
	got, err := CoerceBlockValueForBlock(b, "green")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "green" {
		t.Errorf("got %v, want 'green'", got)
	}
}

func TestCoerceBlockValueForBlock_SelectInvalid(t *testing.T) {
	b := &model.Block{Type: model.BlockSelect, Meta: map[string]any{"options": []any{"red", "green", "blue"}}}
	_, err := CoerceBlockValueForBlock(b, "purple")
	if err == nil {
		t.Fatal("expected error for invalid select option")
	}
}

func TestCoerceBlockValueForBlock_RadioValid(t *testing.T) {
	b := &model.Block{Type: model.BlockRadio, Meta: map[string]any{"options": []any{"yes", "no"}}}
	got, err := CoerceBlockValueForBlock(b, "yes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "yes" {
		t.Errorf("got %v, want 'yes'", got)
	}
}

func TestCoerceBlockValueForBlock_RadioInvalid(t *testing.T) {
	b := &model.Block{Type: model.BlockRadio, Meta: map[string]any{"options": []any{"yes", "no"}}}
	_, err := CoerceBlockValueForBlock(b, "maybe")
	if err == nil {
		t.Fatal("expected error for invalid radio option")
	}
}

func TestCoerceBlockValueForBlock_RatingClamp(t *testing.T) {
	b := &model.Block{Type: model.BlockRating, Meta: map[string]any{"max": float64(5)}}
	tests := []struct {
		name string
		in   any
		want float64
	}{
		{"in range", float64(3), 3},
		{"at max", float64(5), 5},
		{"over max", float64(10), 5},
		{"negative", float64(-1), 0},
		{"string", "4", 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := CoerceBlockValueForBlock(b, tt.in)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCoerceBlockValueForBlock_RatingCustomMax(t *testing.T) {
	b := &model.Block{Type: model.BlockRating, Meta: map[string]any{"max": float64(10)}}
	got, _ := CoerceBlockValueForBlock(b, float64(8))
	if got != float64(8) {
		t.Errorf("got %v, want 8", got)
	}
	got, _ = CoerceBlockValueForBlock(b, float64(15))
	if got != float64(10) {
		t.Errorf("got %v, want 10 (clamped)", got)
	}
}

func TestCoerceBlockValueForBlock_ProgressClamp(t *testing.T) {
	b := &model.Block{Type: model.BlockProgress}
	tests := []struct {
		name string
		in   any
		want float64
	}{
		{"in range", float64(50), 50},
		{"at 100", float64(100), 100},
		{"over 100", float64(150), 100},
		{"negative", float64(-10), 0},
		{"string", "75", 75},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := CoerceBlockValueForBlock(b, tt.in)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCoerceBlockValueForBlock_DateNormalisation(t *testing.T) {
	b := &model.Block{Type: model.BlockDate}
	got, err := CoerceBlockValueForBlock(b, "2026-04-12T10:30:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2026-04-12" {
		t.Errorf("got %v, want '2026-04-12'", got)
	}
}

func TestCoerceBlockValueForBlock_DateTimeFormat(t *testing.T) {
	b := &model.Block{Type: model.BlockDate, Meta: map[string]any{"format": "date-time"}}
	got, err := CoerceBlockValueForBlock(b, "2026-04-12T10:30:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2026-04-12T10:30:00Z" {
		t.Errorf("got %v, want '2026-04-12T10:30:00Z'", got)
	}
}

func TestCoerceBlockValueForBlock_DateInvalid(t *testing.T) {
	b := &model.Block{Type: model.BlockDate}
	_, err := CoerceBlockValueForBlock(b, "not a date")
	if err == nil {
		t.Fatal("expected error for invalid date")
	}
}

func TestCoerceBlockValueForBlock_SelectNoOptions(t *testing.T) {
	// No meta options = no constraint, anything passes
	b := &model.Block{Type: model.BlockSelect}
	got, err := CoerceBlockValueForBlock(b, "anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "anything" {
		t.Errorf("got %v, want 'anything'", got)
	}
}

func TestCoerceBlockValueForBlock_CheckboxGroup(t *testing.T) {
	// checkbox_group passes through (not specially handled)
	b := &model.Block{Type: model.BlockCheckboxGroup}
	got, err := CoerceBlockValueForBlock(b, []any{"opt1", "opt2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	arr, ok := got.([]any)
	if !ok || len(arr) != 2 {
		t.Errorf("expected passthrough of array, got %v", got)
	}
}
