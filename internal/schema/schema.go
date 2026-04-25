package schema

import (
	"bruv/internal/model"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
)

//go:embed types/*.schema.json
var embeddedTypes embed.FS

// CardTypeSchema represents a loaded card type schema.
type CardTypeSchema struct {
	Name        string                 `json:"title"`
	Description string                 `json:"description"`
	Properties  map[string]FieldSchema `json:"properties"`
	Required    []string               `json:"required"`
}

// FieldSchema describes a single field within a card type.
type FieldSchema struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Format      string   `json:"format,omitempty"`
}

// Registry holds all loaded card type schemas, keyed by type name.
type Registry struct {
	types map[string]*CardTypeSchema
}

// NewRegistry creates a Registry pre-loaded with the built-in card types.
func NewRegistry() (*Registry, error) {
	reg := &Registry{types: make(map[string]*CardTypeSchema)}

	entries, err := embeddedTypes.ReadDir("types")
	if err != nil {
		return nil, fmt.Errorf("read embedded types: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".schema.json") {
			continue
		}

		data, err := embeddedTypes.ReadFile(path.Join("types", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read schema %s: %w", entry.Name(), err)
		}

		var schema CardTypeSchema
		if err := json.Unmarshal(data, &schema); err != nil {
			return nil, fmt.Errorf("parse schema %s: %w", entry.Name(), err)
		}

		// Derive type key from filename: "feature.schema.json" → "feature"
		typeName := strings.TrimSuffix(entry.Name(), ".schema.json")
		reg.types[typeName] = &schema
	}

	return reg, nil
}

// LoadExternalTypes loads additional card type schemas from a directory on disk.
// This is how community types get registered — drop a .schema.json into the types/ dir.
func (reg *Registry) LoadExternalTypes(dir string) error {
	entries, err := filepath.Glob(filepath.Join(dir, "*.schema.json"))
	if err != nil {
		return fmt.Errorf("glob external types: %w", err)
	}

	for _, path := range entries {
		data, err := readFileBytes(path)
		if err != nil {
			return fmt.Errorf("read external schema %s: %w", path, err)
		}

		var schema CardTypeSchema
		if err := json.Unmarshal(data, &schema); err != nil {
			return fmt.Errorf("parse external schema %s: %w", path, err)
		}

		base := filepath.Base(path)
		typeName := strings.TrimSuffix(base, ".schema.json")
		reg.types[typeName] = &schema
	}

	return nil
}

// Get returns the schema for a given card type, or nil if not found.
func (reg *Registry) Get(typeName string) *CardTypeSchema {
	return reg.types[typeName]
}

// List returns the names of all registered card types.
func (reg *Registry) List() []string {
	names := make([]string, 0, len(reg.types))
	for name := range reg.types {
		names = append(names, name)
	}
	return names
}

// Validate checks that a card's fields conform to its type schema.
// Returns a list of validation errors (empty if valid).
func (reg *Registry) Validate(typeName string, fields map[string]any) []string {
	schema := reg.Get(typeName)
	if schema == nil {
		return []string{fmt.Sprintf("unknown card type: %q", typeName)}
	}

	var errs []string

	// Check required fields
	for _, req := range schema.Required {
		if _, ok := fields[req]; !ok {
			errs = append(errs, fmt.Sprintf("missing required field: %q", req))
		}
	}

	// Check field types and enum constraints
	for fieldName, value := range fields {
		propSchema, ok := schema.Properties[fieldName]
		if !ok {
			// Unknown fields are allowed (extensible)
			continue
		}

		if len(propSchema.Enum) > 0 {
			strVal, ok := value.(string)
			if !ok {
				errs = append(errs, fmt.Sprintf("field %q must be a string", fieldName))
				continue
			}
			found := false
			for _, allowed := range propSchema.Enum {
				if strVal == allowed {
					found = true
					break
				}
			}
			if !found {
				errs = append(errs, fmt.Sprintf("field %q value %q not in allowed values %v", fieldName, strVal, propSchema.Enum))
			}
		}
	}

	return errs
}

// readFileBytes reads a file from disk (not embedded).
func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// SchemaToBlocks converts a card type schema into an ordered slice of empty Blocks,
// ready to be used as the initial content of a new card of that type.
// Properties are sorted alphabetically, with required fields first.
func (reg *Registry) SchemaToBlocks(typeName string) []model.Block {
	schema := reg.Get(typeName)
	if schema == nil {
		return nil
	}

	requiredSet := make(map[string]bool, len(schema.Required))
	for _, r := range schema.Required {
		requiredSet[r] = true
	}

	// Collect property keys sorted: required first, then alphabetical
	keys := make([]string, 0, len(schema.Properties))
	for k := range schema.Properties {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ri, rj := requiredSet[keys[i]], requiredSet[keys[j]]
		if ri != rj {
			return ri
		}
		return keys[i] < keys[j]
	})

	blocks := make([]model.Block, 0, len(keys))
	for _, key := range keys {
		prop := schema.Properties[key]
		blocks = append(blocks, model.Block{
			ID:       fmt.Sprintf("blk-%s", uuid.New().String()[:8]),
			Type:     fieldSchemaToBlockType(prop),
			Label:    humanizeKey(key),
			Key:      key,
			Value:    fieldSchemaDefaultValue(prop),
			Required: requiredSet[key],
			Meta:     fieldSchemaToMeta(prop),
		})
	}
	return blocks
}

// fieldSchemaToBlockType maps a JSON schema field type to a Block type.
func fieldSchemaToBlockType(f FieldSchema) string {
	if len(f.Enum) > 0 {
		return model.BlockSelect
	}
	switch f.Type {
	case "string":
		if f.Format == "date" || f.Format == "date-time" {
			return model.BlockDate
		}
		return model.BlockText
	case "integer", "number":
		return model.BlockNumber
	case "boolean":
		return model.BlockCheckbox
	case "array":
		return model.BlockChecklist
	default:
		return model.BlockText
	}
}

// fieldSchemaDefaultValue returns the zero value for a block based on schema type.
func fieldSchemaDefaultValue(f FieldSchema) any {
	if len(f.Enum) > 0 {
		return ""
	}
	switch f.Type {
	case "string":
		return ""
	case "integer":
		return 0
	case "number":
		return 0.0
	case "boolean":
		return false
	case "array":
		return []any{}
	default:
		return ""
	}
}

// fieldSchemaToMeta builds the Meta map for a Block from a FieldSchema.
func fieldSchemaToMeta(f FieldSchema) map[string]any {
	meta := make(map[string]any)
	if len(f.Enum) > 0 {
		meta["options"] = f.Enum
	}
	if f.Format != "" {
		meta["format"] = f.Format
	}
	if f.Description != "" {
		meta["description"] = f.Description
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

// humanizeKey converts "recording_status" → "Recording Status".
func humanizeKey(key string) string {
	parts := strings.Split(key, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
