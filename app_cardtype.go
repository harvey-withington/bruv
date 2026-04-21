package main

// Card type schema, templates, and portable import/export.
//
// This file owns everything a user sees under Settings → Card Types and
// Settings → Templates, plus the machinery that keeps a card's blocks
// aligned with its type when the type changes. Two related concepts
// share this space:
//
//   - A "card type" (builtin or user-defined) is a category a card
//     belongs to — Brainstorm, Task, Reference, Agent, or anything the
//     user invents. Each type can link to a default CardTemplate.
//   - A "card template" is a set of blocks that seeds a new card or
//     gets re-merged onto an existing one via RefreshTypeBlocks.
//
// Merges are NEVER destructive: existing block values are preserved,
// template blocks with keys already on the card are skipped, and only
// missing keys get appended. This invariant is load-bearing — the LLM
// agent relies on RefreshTypeBlocks being safe to call any time.
//
// Extracted from app.go so changes to the type/template system don't
// require swimming through 7k lines of unrelated Wails bindings.

import (
	"bruv/internal/config"
	"bruv/internal/model"
	"bruv/internal/repo"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// --- Schema ---

// CardTypeInfo is the rich card type metadata returned to the frontend.
type CardTypeInfo struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Color       string `json:"color"`
	Icon        string `json:"icon,omitempty"`
	Description string `json:"description"`
	AIHint      string `json:"ai_hint,omitempty"`
	TemplateID  string `json:"template_id,omitempty"`
	Builtin     bool   `json:"builtin"`
}

// builtinTypes defines the built-in card types in display order.
var builtinTypes = []CardTypeInfo{
	{ID: "brainstorm", Label: "Brainstorm", Color: "#84cc16", Builtin: true},  // lime green
	{ID: "task", Label: "Task", Color: "#38bdf8", Builtin: true},              // light blue
	{ID: "reference", Label: "Reference", Color: "#fb923c", Builtin: true},    // orange
	{ID: "agent", Label: "Agent", Color: "#ef4444", Builtin: true},            // red
}

// seedTypes are pre-installed as user types on first run so they can be
// fully edited or deleted.
var seedTypes = []config.UserCardType{
	{ID: "feature", Label: "Feature", Color: "#6366f1"},
	{ID: "episode", Label: "Episode", Color: "#ec4899"},
}

// ensureSeeded adds the pre-installed seed types on first run.
func (a *App) ensureSeeded(store *config.UserTypeStore) bool {
	if store.Seeded {
		return false
	}
	store.Seeded = true
	for _, seed := range seedTypes {
		store.Types = append(store.Types, seed)
	}
	return true
}

// ensureStarterTemplates runs once (guarded by StarterTemplatesSeeded) to create
// starter templates from built-in schemas and link them to their card types.
// Runs for all users including those who were already seeded before this feature.
func (a *App) ensureStarterTemplates(store *config.UserTypeStore) bool {
	if store.StarterTemplatesSeeded || a.registry == nil {
		return false
	}
	store.StarterTemplatesSeeded = true

	if store.BuiltinOverrides == nil {
		store.BuiltinOverrides = make(map[string]config.BuiltinOverride)
	}

	// Create one starter template per schema.
	schemaTemplateIDs := make(map[string]string) // typeID → templateID
	for _, typeName := range a.registry.List() {
		blocks := a.registry.SchemaToBlocks(typeName)
		if len(blocks) == 0 {
			continue
		}
		name := typeName
		if s := a.registry.Get(typeName); s != nil && s.Name != "" {
			name = s.Name
		}
		tmpl := config.CardTemplate{
			ID:     uuid.New().String(),
			Name:   name,
			Blocks: blocks,
		}
		store.Templates = append(store.Templates, tmpl)
		schemaTemplateIDs[typeName] = tmpl.ID
	}

	// Link user types that have no template yet.
	for i, ut := range store.Types {
		if ut.TemplateID == "" {
			if tid, ok := schemaTemplateIDs[ut.ID]; ok {
				store.Types[i].TemplateID = tid
			}
		}
	}

	// Link built-in types that have no override template yet.
	for _, bt := range builtinTypes {
		if tid, ok := schemaTemplateIDs[bt.ID]; ok {
			ov := store.BuiltinOverrides[bt.ID]
			if ov.TemplateID == "" {
				ov.TemplateID = tid
				store.BuiltinOverrides[bt.ID] = ov
			}
		}
	}

	return true
}

// ensureMissingBuiltinTemplates creates templates for any built-in types
// that were added after the initial StarterTemplatesSeeded run.
func (a *App) ensureMissingBuiltinTemplates(store *config.UserTypeStore) bool {
	if a.registry == nil {
		return false
	}
	if store.BuiltinOverrides == nil {
		store.BuiltinOverrides = make(map[string]config.BuiltinOverride)
	}
	changed := false
	for _, bt := range builtinTypes {
		ov := store.BuiltinOverrides[bt.ID]
		if ov.TemplateID != "" {
			continue // already has a template
		}
		blocks := a.registry.SchemaToBlocks(bt.ID)
		if len(blocks) == 0 {
			continue
		}
		name := bt.Label
		if s := a.registry.Get(bt.ID); s != nil && s.Name != "" {
			name = s.Name
		}
		tmpl := config.CardTemplate{
			ID:     uuid.New().String(),
			Name:   name,
			Blocks: blocks,
		}
		store.Templates = append(store.Templates, tmpl)
		ov.TemplateID = tmpl.ID
		store.BuiltinOverrides[bt.ID] = ov
		changed = true
	}
	return changed
}

// ListCardTypes returns all card types (built-in first, then user-defined).
//
// Called early in the frontend boot sequence (from loadCardTypes() in
// App.svelte) — often BEFORE tryReopenLastRepo() has finished, so a.repo
// may be nil. In that case we fall back to returning only the built-ins
// so the UI has something to render; as soon as a repo opens the
// frontend re-fetches and picks up the user-defined types.
func (a *App) ListCardTypes() []CardTypeInfo {
	var store config.UserTypeStore
	if a.repo != nil {
		store, _ = a.repo.LoadUserTypeStore()
		dirty := a.ensureSeeded(&store)
		dirty = a.ensureStarterTemplates(&store) || dirty
		dirty = a.ensureMissingBuiltinTemplates(&store) || dirty
		if dirty {
			_ = a.repo.SaveUserTypeStore(store)
		}
	}

	result := make([]CardTypeInfo, 0, len(builtinTypes)+len(store.Types))
	for _, b := range builtinTypes {
		info := b
		if a.registry != nil {
			if s := a.registry.Get(b.ID); s != nil {
				info.Description = s.Description
			}
		}
		if ov, ok := store.BuiltinOverrides[b.ID]; ok {
			if ov.Color != "" {
				info.Color = ov.Color
			}
			if ov.TemplateID != "" {
				info.TemplateID = ov.TemplateID
			}
		}
		result = append(result, info)
	}
	for _, t := range store.Types {
		result = append(result, CardTypeInfo{
			ID:          t.ID,
			Label:       t.Label,
			Color:       t.Color,
			Icon:        t.Icon,
			Description: t.Description,
			AIHint:      t.AIHint,
			TemplateID:  t.TemplateID,
			Builtin:     false,
		})
	}
	return result
}

func (a *App) ValidateCardFields(cardType string, fields map[string]any) []string {
	if a.registry == nil {
		return []string{"schema registry not loaded"}
	}
	return a.registry.Validate(cardType, fields)
}

// applyTypeBlocks non-destructively merges a type's template blocks into a card.
// Existing blocks are NEVER removed or overwritten. Template blocks are only
// appended when no existing block shares the same key. Legacy field values
// (card.Fields) are carried forward into matching template blocks so content
// is never lost during a type change.
// For built-in types it uses the schema registry; for user types it uses the
// associated CardTemplate.
func (a *App) applyTypeBlocks(cardID, cardType string) {
	templateBlocks := a.resolveTemplateBlocks(cardType)
	if len(templateBlocks) == 0 {
		return
	}
	a.mergeTemplateBlocks(cardID, templateBlocks)
}

// resolveTemplateBlocks returns the template/schema blocks for a card type.
// Safe to call with a.repo nil — falls through to the built-in schema
// registry, which is always available.
func (a *App) resolveTemplateBlocks(cardType string) []model.Block {
	var store config.UserTypeStore
	if a.repo != nil {
		store, _ = a.repo.LoadUserTypeStore()
	}

	// Priority 1: user-defined type template
	for _, ut := range store.Types {
		if ut.ID == cardType && ut.TemplateID != "" {
			for _, tmpl := range store.Templates {
				if tmpl.ID == ut.TemplateID {
					return cloneBlocksWithFreshIDs(tmpl.Blocks)
				}
			}
			break
		}
	}

	// Priority 2: builtin override template
	if ov, ok := store.BuiltinOverrides[cardType]; ok && ov.TemplateID != "" {
		for _, tmpl := range store.Templates {
			if tmpl.ID == ov.TemplateID {
				return cloneBlocksWithFreshIDs(tmpl.Blocks)
			}
		}
	}

	// Priority 3: built-in schema
	if a.registry != nil {
		blocks := a.registry.SchemaToBlocks(cardType)
		if len(blocks) > 0 {
			return blocks
		}
	}

	return nil
}

// mergeTemplateBlocks merges template blocks into an existing card non-destructively.
// - Existing blocks are never removed or reordered.
// - If a template block's key matches an existing block, the existing value is kept.
// - Template blocks whose key matches an intrinsic field are skipped entirely.
// - Template blocks with keys not present in the card are appended.
func (a *App) mergeTemplateBlocks(cardID string, templateBlocks []model.Block) {
	existingCard, _ := a.repo.GetCard(cardID)
	if existingCard == nil {
		return
	}

	// Intrinsic fields are managed outside the block system (description, due_date,
	// labels, etc.). Template blocks with these keys must never be added as blocks.
	intrinsicKeys := map[string]bool{
		"description": true,
	}

	// Index existing blocks by key for lookup
	existingByKey := make(map[string]int) // key → index in Blocks slice
	for i, b := range existingCard.Blocks {
		if b.Key != "" {
			existingByKey[b.Key] = i
		}
	}

	// Start from the card's current blocks (preserving everything)
	merged := make([]model.Block, len(existingCard.Blocks))
	copy(merged, existingCard.Blocks)

	for _, tb := range templateBlocks {
		if tb.Key != "" && intrinsicKeys[tb.Key] {
			continue
		}
		if idx, exists := existingByKey[tb.Key]; exists {
			// Block already present — only fill in the value if the user hasn't set one
			if isBlockValueEmpty(merged[idx].Value) && !isBlockValueEmpty(tb.Value) {
				merged[idx].Value = tb.Value
			}
			continue
		}
		merged = append(merged, tb)
	}

	a.UpdateCardBlocks(cardID, merged)
}

// isBlockValueEmpty returns true if a block value is nil, empty string, empty slice, or zero.
func isBlockValueEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return val == ""
	case []any:
		return len(val) == 0
	case []map[string]any:
		return len(val) == 0
	case float64:
		return val == 0
	case bool:
		return false // false is a valid user-set value
	}
	return false
}

// RefreshTypeBlocks re-merges the current card type's template blocks into the card.
// Missing template blocks are added (with empty values), existing blocks are untouched.
// This is the "refresh" action — safe to call any time.
func (a *App) RefreshTypeBlocks(cardID string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.GetCard(cardID)
	if err != nil {
		return nil, err
	}
	if card.Type == "" {
		return card, nil
	}
	templateBlocks := a.resolveTemplateBlocks(card.Type)
	if len(templateBlocks) == 0 {
		return card, nil
	}
	a.mergeTemplateBlocks(cardID, templateBlocks)
	return a.repo.GetCard(cardID)
}

// cloneBlocksWithFreshIDs returns a copy of blocks with new UUIDs to avoid
// ID collisions when a template is applied to multiple cards.
func cloneBlocksWithFreshIDs(blocks []model.Block) []model.Block {
	cloned := make([]model.Block, len(blocks))
	for i, b := range blocks {
		cloned[i] = b
		cloned[i].ID = uuid.New().String()
	}
	return cloned
}

// --- User Card Types & Templates ---

// CreateUserCardType creates a new user-defined card type.
func (a *App) CreateUserCardType(label, color, description, aiHint, templateID string) (config.UserCardType, error) {
	if label == "" {
		return config.UserCardType{}, fmt.Errorf("label is required")
	}
	if a.repo == nil {
		return config.UserCardType{}, fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return config.UserCardType{}, err
	}
	id := repo.Slugify(label)
	if id == "" {
		id = uuid.New().String()
	}
	// Ensure ID is unique; append suffix if needed
	base := id
	for i := 2; isTypeIDTaken(store, id); i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	t := config.UserCardType{
		ID: id, Label: label, Color: color,
		Description: description, AIHint: aiHint, TemplateID: templateID,
	}
	store.Types = append(store.Types, t)
	return t, a.repo.SaveUserTypeStore(store)
}

// UpdateUserCardType updates an existing user-defined card type by ID.
func (a *App) UpdateUserCardType(id, label, color, description, aiHint, templateID string) (config.UserCardType, error) {
	if a.repo == nil {
		return config.UserCardType{}, fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return config.UserCardType{}, err
	}
	for i, t := range store.Types {
		if t.ID == id {
			store.Types[i].Label = label
			store.Types[i].Color = color
			store.Types[i].Description = description
			store.Types[i].AIHint = aiHint
			store.Types[i].TemplateID = templateID
			return store.Types[i], a.repo.SaveUserTypeStore(store)
		}
	}
	return config.UserCardType{}, fmt.Errorf("card type %q not found", id)
}

// UpdateUserCardTypeIcon sets or clears the icon on a user-defined card type.
func (a *App) UpdateUserCardTypeIcon(id, icon string) (config.UserCardType, error) {
	if a.repo == nil {
		return config.UserCardType{}, fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return config.UserCardType{}, err
	}
	for i, t := range store.Types {
		if t.ID == id {
			store.Types[i].Icon = icon
			return store.Types[i], a.repo.SaveUserTypeStore(store)
		}
	}
	return config.UserCardType{}, fmt.Errorf("card type %q not found", id)
}

// DeleteUserCardType removes a user-defined card type by ID.
func (a *App) DeleteUserCardType(id string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return err
	}
	for i, t := range store.Types {
		if t.ID == id {
			store.Types = append(store.Types[:i], store.Types[i+1:]...)
			return a.repo.SaveUserTypeStore(store)
		}
	}
	return fmt.Errorf("card type %q not found", id)
}

// UpdateBuiltinCardType updates the color and/or template of a built-in card type.
func (a *App) UpdateBuiltinCardType(id, color, templateID string) error {
	isBuiltin := false
	for _, b := range builtinTypes {
		if b.ID == id {
			isBuiltin = true
			break
		}
	}
	if !isBuiltin {
		return fmt.Errorf("card type %q is not a built-in type", id)
	}
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return err
	}
	if store.BuiltinOverrides == nil {
		store.BuiltinOverrides = make(map[string]config.BuiltinOverride)
	}
	store.BuiltinOverrides[id] = config.BuiltinOverride{
		Color:      color,
		TemplateID: templateID,
	}
	return a.repo.SaveUserTypeStore(store)
}

// ListCardTemplates returns all user-defined card templates.
// Safe to call before a repo is open — returns an empty list so the
// frontend's early-boot loadCardTypes() call doesn't nil-panic.
func (a *App) ListCardTemplates() ([]config.CardTemplate, error) {
	if a.repo == nil {
		return []config.CardTemplate{}, nil
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return nil, err
	}
	if store.Templates == nil {
		return []config.CardTemplate{}, nil
	}
	return store.Templates, nil
}

// CreateCardTemplate creates a new card template.
func (a *App) CreateCardTemplate(name string, blocks []model.Block) (config.CardTemplate, error) {
	if name == "" {
		return config.CardTemplate{}, fmt.Errorf("name is required")
	}
	if a.repo == nil {
		return config.CardTemplate{}, fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return config.CardTemplate{}, err
	}
	tmpl := config.CardTemplate{
		ID:     uuid.New().String(),
		Name:   name,
		Blocks: blocks,
	}
	store.Templates = append(store.Templates, tmpl)
	return tmpl, a.repo.SaveUserTypeStore(store)
}

// UpdateCardTemplate updates an existing card template by ID.
func (a *App) UpdateCardTemplate(id, name string, blocks []model.Block) (config.CardTemplate, error) {
	if a.repo == nil {
		return config.CardTemplate{}, fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return config.CardTemplate{}, err
	}
	for i, tmpl := range store.Templates {
		if tmpl.ID == id {
			store.Templates[i].Name = name
			store.Templates[i].Blocks = blocks
			return store.Templates[i], a.repo.SaveUserTypeStore(store)
		}
	}
	return config.CardTemplate{}, fmt.Errorf("template %q not found", id)
}

// DeleteCardTemplate removes a card template by ID.
func (a *App) DeleteCardTemplate(id string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return err
	}
	for i, tmpl := range store.Templates {
		if tmpl.ID == id {
			store.Templates = append(store.Templates[:i], store.Templates[i+1:]...)
			return a.repo.SaveUserTypeStore(store)
		}
	}
	return fmt.Errorf("template %q not found", id)
}

// --- Card type export / import ---
//
// The portable export format is a JSON file that mirrors UserTypeStore but
// also carries a format marker so we can evolve the schema safely. Import
// supports three merge modes so users can choose between a full replace
// (dangerous, confirm in UI), merging only non-colliding types, or merging
// and overwriting on ID collisions.

// CardTypesExport is the on-wire shape for exported card types. It's the
// same data as UserTypeStore minus the per-repo seeding flags, plus a
// format version for forward compatibility.
type CardTypesExport struct {
	Format           string                            `json:"format"`
	Version          int                               `json:"version"`
	Types            []config.UserCardType             `json:"types"`
	Templates        []config.CardTemplate             `json:"templates"`
	BuiltinOverrides map[string]config.BuiltinOverride `json:"builtin_overrides,omitempty"`
}

// CardTypesImportResult reports what an import actually did so the UI can
// show an accurate summary toast.
type CardTypesImportResult struct {
	TypesAdded         int `json:"types_added"`
	TypesOverwritten   int `json:"types_overwritten"`
	TypesSkipped       int `json:"types_skipped"`
	TemplatesAdded     int `json:"templates_added"`
	TemplatesOverwritten int `json:"templates_overwritten"`
	TemplatesSkipped   int `json:"templates_skipped"`
}

// ExportCardTypesToFile writes the current repo's card types + templates +
// built-in overrides to a portable JSON file the user can share.
func (a *App) ExportCardTypesToFile(filePath string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return fmt.Errorf("load card types: %w", err)
	}
	exp := CardTypesExport{
		Format:           "bruv-card-types",
		Version:          1,
		Types:            store.Types,
		Templates:        store.Templates,
		BuiltinOverrides: store.BuiltinOverrides,
	}
	data, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal export: %w", err)
	}
	return os.WriteFile(filePath, data, 0o644)
}

// ImportCardTypesFromFile reads a portable export JSON and merges it into
// the current repo's card types store. mode is one of:
//   - "replace"         — overwrite the current store entirely
//   - "merge"           — add non-colliding entries, skip collisions
//   - "merge_overwrite" — add everything, overwrite on ID collisions
func (a *App) ImportCardTypesFromFile(filePath, mode string) (CardTypesImportResult, error) {
	var result CardTypesImportResult
	if a.repo == nil {
		return result, fmt.Errorf("no repository open")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return result, fmt.Errorf("read import file: %w", err)
	}
	var exp CardTypesExport
	if err := json.Unmarshal(data, &exp); err != nil {
		return result, fmt.Errorf("parse import file: %w", err)
	}
	if exp.Format != "bruv-card-types" {
		return result, fmt.Errorf("not a BRUV card types export (format=%q)", exp.Format)
	}
	return a.applyCardTypesImport(exp, mode)
}

// ImportCardTypesFromRepo reads another BRUV repo's .bruv/card_types.json
// directly — no portable export file needed. The caller passes the path
// to the other repo's root folder. This is the "steal types from my
// other repo" workflow: no export step, no intermediate file.
func (a *App) ImportCardTypesFromRepo(otherRepoPath, mode string) (CardTypesImportResult, error) {
	var result CardTypesImportResult
	if a.repo == nil {
		return result, fmt.Errorf("no repository open")
	}
	src := filepath.Join(otherRepoPath, ".bruv", "card_types.json")
	data, err := os.ReadFile(src)
	if err != nil {
		// Not a valid repo, or a legacy repo that predates the move —
		// surface a clear error so the UI can say so.
		if os.IsNotExist(err) {
			return result, fmt.Errorf("no card types found in %q (not a BRUV repo, or a legacy repo without repo-scoped types)", otherRepoPath)
		}
		return result, fmt.Errorf("read source repo types: %w", err)
	}
	var store config.UserTypeStore
	if err := json.Unmarshal(data, &store); err != nil {
		return result, fmt.Errorf("parse source repo types: %w", err)
	}
	exp := CardTypesExport{
		Format:           "bruv-card-types",
		Version:          1,
		Types:            store.Types,
		Templates:        store.Templates,
		BuiltinOverrides: store.BuiltinOverrides,
	}
	return a.applyCardTypesImport(exp, mode)
}

// applyCardTypesImport is shared between the file and repo import paths.
// It loads the current store, merges per the requested mode, saves, and
// returns a count of what happened.
func (a *App) applyCardTypesImport(exp CardTypesExport, mode string) (CardTypesImportResult, error) {
	var result CardTypesImportResult

	current, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return result, fmt.Errorf("load current card types: %w", err)
	}

	switch mode {
	case "replace":
		// Count everything being added as new, since the previous state
		// is being discarded entirely.
		result.TypesAdded = len(exp.Types)
		result.TemplatesAdded = len(exp.Templates)
		current.Types = append([]config.UserCardType(nil), exp.Types...)
		current.Templates = append([]config.CardTemplate(nil), exp.Templates...)
		if exp.BuiltinOverrides != nil {
			copied := make(map[string]config.BuiltinOverride, len(exp.BuiltinOverrides))
			for k, v := range exp.BuiltinOverrides {
				copied[k] = v
			}
			current.BuiltinOverrides = copied
		}

	case "merge", "merge_overwrite":
		overwrite := mode == "merge_overwrite"

		typeIdx := make(map[string]int, len(current.Types))
		for i, t := range current.Types {
			typeIdx[t.ID] = i
		}
		for _, t := range exp.Types {
			if existing, ok := typeIdx[t.ID]; ok {
				if overwrite {
					current.Types[existing] = t
					result.TypesOverwritten++
				} else {
					result.TypesSkipped++
				}
				continue
			}
			current.Types = append(current.Types, t)
			result.TypesAdded++
		}

		tmplIdx := make(map[string]int, len(current.Templates))
		for i, tmpl := range current.Templates {
			tmplIdx[tmpl.ID] = i
		}
		for _, tmpl := range exp.Templates {
			if existing, ok := tmplIdx[tmpl.ID]; ok {
				if overwrite {
					current.Templates[existing] = tmpl
					result.TemplatesOverwritten++
				} else {
					result.TemplatesSkipped++
				}
				continue
			}
			current.Templates = append(current.Templates, tmpl)
			result.TemplatesAdded++
		}

		// Built-in overrides merge additively — overwrite-mode replaces
		// existing per-ID overrides, merge-mode leaves them alone.
		if exp.BuiltinOverrides != nil {
			if current.BuiltinOverrides == nil {
				current.BuiltinOverrides = make(map[string]config.BuiltinOverride)
			}
			for k, v := range exp.BuiltinOverrides {
				if _, exists := current.BuiltinOverrides[k]; exists && !overwrite {
					continue
				}
				current.BuiltinOverrides[k] = v
			}
		}

	default:
		return result, fmt.Errorf("unknown import mode %q (expected replace, merge, or merge_overwrite)", mode)
	}

	if err := a.repo.SaveUserTypeStore(current); err != nil {
		return result, fmt.Errorf("save merged card types: %w", err)
	}
	return result, nil
}

func isTypeIDTaken(store config.UserTypeStore, id string) bool {
	for _, t := range store.Types {
		if t.ID == id {
			return true
		}
	}
	// Also check against built-in IDs
	for _, b := range builtinTypes {
		if b.ID == id {
			return true
		}
	}
	return false
}
