// Package catalog is the CatalogService — card types, templates, tags,
// and labels. Everything a user sees under Settings → Card Types and
// Settings → Templates, plus the per-project label CRUD and repo-wide
// tag colours.
//
// Template merges are non-destructive by design: existing block values
// are preserved, template blocks with keys already on the card are
// skipped, only missing keys are appended. RefreshTypeBlocks relies on
// this invariant being safe to call any time.
package catalog

import (
	"bruv/internal/config"
	"bruv/internal/index"
	"bruv/internal/model"
	"bruv/internal/repo"
	"bruv/internal/schema"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Deps is the narrow host contract for CatalogService.
type Deps interface {
	Repo() *repo.Repository
	Registry() *schema.Registry
	Index() *index.Index
	// UpdateCardBlocks is consulted by mergeTemplateBlocks because card
	// mutation + indexing lives on App (until the card service is
	// extracted). Once CardService lands this becomes an internal call.
	UpdateCardBlocks(id string, blocks []model.Block) (*model.Card, error)
	// Publish announces a domain event. Emitted from label CRUD and
	// card-type mutations so other devices see catalog changes live.
	Publish(topic string, payload any)
}

// Service exposes card-type, template, tag, and label operations.
type Service struct{ deps Deps }

// New constructs a CatalogService.
func New(deps Deps) *Service { return &Service{deps: deps} }

// --- Types (Wails-exposed response shapes; aliased in main) ---

// CardTypeInfo is the rich card-type metadata returned to the UI.
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

// CardTypesExport is the portable on-wire shape for exported types.
type CardTypesExport struct {
	Format           string                            `json:"format"`
	Version          int                               `json:"version"`
	Types            []config.UserCardType             `json:"types"`
	Templates        []config.CardTemplate             `json:"templates"`
	BuiltinOverrides map[string]config.BuiltinOverride `json:"builtin_overrides,omitempty"`
}

// CardTypesImportResult reports what an import actually did.
type CardTypesImportResult struct {
	TypesAdded           int `json:"types_added"`
	TypesOverwritten     int `json:"types_overwritten"`
	TypesSkipped         int `json:"types_skipped"`
	TemplatesAdded       int `json:"templates_added"`
	TemplatesOverwritten int `json:"templates_overwritten"`
	TemplatesSkipped     int `json:"templates_skipped"`
}

// BuiltinTypes defines the built-in card types in display order.
var BuiltinTypes = []CardTypeInfo{
	{ID: "brainstorm", Label: "Brainstorm", Color: "#84cc16", Builtin: true},
	{ID: "task", Label: "Task", Color: "#38bdf8", Builtin: true},
	{ID: "reference", Label: "Reference", Color: "#fb923c", Builtin: true},
	{ID: "agent", Label: "Agent", Color: "#ef4444", Builtin: true},
}

// seedTypes are pre-installed as user types on first run.
var seedTypes = []config.UserCardType{
	{ID: "feature", Label: "Feature", Color: "#6366f1"},
	{ID: "episode", Label: "Episode", Color: "#ec4899"},
}

// --- Card type listing + schema ---

// ListCardTypes returns all card types (built-in first, then user).
// Safe to call before a repo is open — returns only built-ins in that
// case so the UI has something to render during early boot.
func (s *Service) ListCardTypes() []CardTypeInfo {
	var store config.UserTypeStore
	r := s.deps.Repo()
	if r != nil {
		store, _ = r.LoadUserTypeStore()
		dirty := s.ensureSeeded(&store)
		dirty = s.ensureStarterTemplates(&store) || dirty
		dirty = s.ensureMissingBuiltinTemplates(&store) || dirty
		if dirty {
			_ = r.SaveUserTypeStore(store)
		}
	}

	result := make([]CardTypeInfo, 0, len(BuiltinTypes)+len(store.Types))
	reg := s.deps.Registry()
	for _, b := range BuiltinTypes {
		info := b
		if reg != nil {
			if sch := reg.Get(b.ID); sch != nil {
				info.Description = sch.Description
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
			ID: t.ID, Label: t.Label, Color: t.Color, Icon: t.Icon,
			Description: t.Description, AIHint: t.AIHint,
			TemplateID: t.TemplateID, Builtin: false,
		})
	}
	return result
}

// ValidateCardFields delegates to the schema registry.
func (s *Service) ValidateCardFields(cardType string, fields map[string]any) []string {
	reg := s.deps.Registry()
	if reg == nil {
		return []string{"schema registry not loaded"}
	}
	return reg.Validate(cardType, fields)
}

// --- Card type mutations ---

func (s *Service) CreateUserCardType(label, color, description, aiHint, templateID string) (config.UserCardType, error) {
	if label == "" {
		return config.UserCardType{}, fmt.Errorf("label is required")
	}
	r := s.deps.Repo()
	if r == nil {
		return config.UserCardType{}, fmt.Errorf("no repository open")
	}
	store, err := r.LoadUserTypeStore()
	if err != nil {
		return config.UserCardType{}, err
	}
	id := repo.Slugify(label)
	if id == "" {
		id = uuid.New().String()
	}
	base := id
	for i := 2; isTypeIDTaken(store, id); i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	t := config.UserCardType{
		ID: id, Label: label, Color: color,
		Description: description, AIHint: aiHint, TemplateID: templateID,
	}
	store.Types = append(store.Types, t)
	if err := r.SaveUserTypeStore(store); err != nil {
		return t, err
	}
	s.deps.Publish("cardtype:updated", t)
	return t, nil
}

func (s *Service) UpdateUserCardType(id, label, color, description, aiHint, templateID string) (config.UserCardType, error) {
	r := s.deps.Repo()
	if r == nil {
		return config.UserCardType{}, fmt.Errorf("no repository open")
	}
	store, err := r.LoadUserTypeStore()
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
			if err := r.SaveUserTypeStore(store); err != nil {
				return store.Types[i], err
			}
			s.deps.Publish("cardtype:updated", store.Types[i])
			return store.Types[i], nil
		}
	}
	return config.UserCardType{}, fmt.Errorf("card type %q not found", id)
}

func (s *Service) UpdateUserCardTypeIcon(id, icon string) (config.UserCardType, error) {
	r := s.deps.Repo()
	if r == nil {
		return config.UserCardType{}, fmt.Errorf("no repository open")
	}
	store, err := r.LoadUserTypeStore()
	if err != nil {
		return config.UserCardType{}, err
	}
	for i, t := range store.Types {
		if t.ID == id {
			store.Types[i].Icon = icon
			if err := r.SaveUserTypeStore(store); err != nil {
				return store.Types[i], err
			}
			s.deps.Publish("cardtype:updated", store.Types[i])
			return store.Types[i], nil
		}
	}
	return config.UserCardType{}, fmt.Errorf("card type %q not found", id)
}

func (s *Service) DeleteUserCardType(id string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	store, err := r.LoadUserTypeStore()
	if err != nil {
		return err
	}
	for i, t := range store.Types {
		if t.ID == id {
			store.Types = append(store.Types[:i], store.Types[i+1:]...)
			if err := r.SaveUserTypeStore(store); err != nil {
				return err
			}
			s.deps.Publish("cardtype:deleted", map[string]any{"id": id})
			return nil
		}
	}
	return fmt.Errorf("card type %q not found", id)
}

func (s *Service) UpdateBuiltinCardType(id, color, templateID string) error {
	isBuiltin := false
	for _, b := range BuiltinTypes {
		if b.ID == id {
			isBuiltin = true
			break
		}
	}
	if !isBuiltin {
		return fmt.Errorf("card type %q is not a built-in type", id)
	}
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	store, err := r.LoadUserTypeStore()
	if err != nil {
		return err
	}
	if store.BuiltinOverrides == nil {
		store.BuiltinOverrides = make(map[string]config.BuiltinOverride)
	}
	store.BuiltinOverrides[id] = config.BuiltinOverride{Color: color, TemplateID: templateID}
	return r.SaveUserTypeStore(store)
}

// --- Templates ---

func (s *Service) ListCardTemplates() ([]config.CardTemplate, error) {
	r := s.deps.Repo()
	if r == nil {
		return []config.CardTemplate{}, nil
	}
	store, err := r.LoadUserTypeStore()
	if err != nil {
		return nil, err
	}
	if store.Templates == nil {
		return []config.CardTemplate{}, nil
	}
	return store.Templates, nil
}

func (s *Service) CreateCardTemplate(name string, blocks []model.Block) (config.CardTemplate, error) {
	if name == "" {
		return config.CardTemplate{}, fmt.Errorf("name is required")
	}
	r := s.deps.Repo()
	if r == nil {
		return config.CardTemplate{}, fmt.Errorf("no repository open")
	}
	store, err := r.LoadUserTypeStore()
	if err != nil {
		return config.CardTemplate{}, err
	}
	tmpl := config.CardTemplate{ID: uuid.New().String(), Name: name, Blocks: blocks}
	store.Templates = append(store.Templates, tmpl)
	return tmpl, r.SaveUserTypeStore(store)
}

func (s *Service) UpdateCardTemplate(id, name string, blocks []model.Block) (config.CardTemplate, error) {
	r := s.deps.Repo()
	if r == nil {
		return config.CardTemplate{}, fmt.Errorf("no repository open")
	}
	store, err := r.LoadUserTypeStore()
	if err != nil {
		return config.CardTemplate{}, err
	}
	for i, tmpl := range store.Templates {
		if tmpl.ID == id {
			store.Templates[i].Name = name
			store.Templates[i].Blocks = blocks
			return store.Templates[i], r.SaveUserTypeStore(store)
		}
	}
	return config.CardTemplate{}, fmt.Errorf("template %q not found", id)
}

func (s *Service) DeleteCardTemplate(id string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	store, err := r.LoadUserTypeStore()
	if err != nil {
		return err
	}
	for i, tmpl := range store.Templates {
		if tmpl.ID == id {
			store.Templates = append(store.Templates[:i], store.Templates[i+1:]...)
			return r.SaveUserTypeStore(store)
		}
	}
	return fmt.Errorf("template %q not found", id)
}

// --- Type block merging ---

// ApplyTypeBlocks non-destructively merges a type's template blocks
// into a card. Called by the card creation flow when a type is set.
func (s *Service) ApplyTypeBlocks(cardID, cardType string) {
	templateBlocks := s.ResolveTemplateBlocks(cardType)
	if len(templateBlocks) == 0 {
		return
	}
	s.mergeTemplateBlocks(cardID, templateBlocks)
}

// ResolveTemplateBlocks returns the template/schema blocks for a card
// type. Priority: user template > builtin-override template > schema.
func (s *Service) ResolveTemplateBlocks(cardType string) []model.Block {
	var store config.UserTypeStore
	r := s.deps.Repo()
	if r != nil {
		store, _ = r.LoadUserTypeStore()
	}

	// user-defined type template
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

	// builtin override template
	if ov, ok := store.BuiltinOverrides[cardType]; ok && ov.TemplateID != "" {
		for _, tmpl := range store.Templates {
			if tmpl.ID == ov.TemplateID {
				return cloneBlocksWithFreshIDs(tmpl.Blocks)
			}
		}
	}

	// built-in schema
	if reg := s.deps.Registry(); reg != nil {
		blocks := reg.SchemaToBlocks(cardType)
		if len(blocks) > 0 {
			return blocks
		}
	}

	return nil
}

// mergeTemplateBlocks preserves existing block values; appends only
// missing keys. Intrinsic fields (description) are skipped.
func (s *Service) mergeTemplateBlocks(cardID string, templateBlocks []model.Block) {
	r := s.deps.Repo()
	if r == nil {
		return
	}
	existingCard, _ := r.GetCard(cardID)
	if existingCard == nil {
		return
	}

	intrinsicKeys := map[string]bool{"description": true}

	existingByKey := make(map[string]int)
	for i, b := range existingCard.Blocks {
		if b.Key != "" {
			existingByKey[b.Key] = i
		}
	}

	merged := make([]model.Block, len(existingCard.Blocks))
	copy(merged, existingCard.Blocks)

	for _, tb := range templateBlocks {
		if tb.Key != "" && intrinsicKeys[tb.Key] {
			continue
		}
		if idx, exists := existingByKey[tb.Key]; exists {
			if isBlockValueEmpty(merged[idx].Value) && !isBlockValueEmpty(tb.Value) {
				merged[idx].Value = tb.Value
			}
			continue
		}
		merged = append(merged, tb)
	}

	_, _ = s.deps.UpdateCardBlocks(cardID, merged)
}

// RefreshTypeBlocks re-merges the current card type's template.
func (s *Service) RefreshTypeBlocks(cardID string) (*model.Card, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := r.GetCard(cardID)
	if err != nil {
		return nil, err
	}
	if card.Type == "" {
		return card, nil
	}
	templateBlocks := s.ResolveTemplateBlocks(card.Type)
	if len(templateBlocks) == 0 {
		return card, nil
	}
	s.mergeTemplateBlocks(cardID, templateBlocks)
	return r.GetCard(cardID)
}

// --- Import / Export ---

func (s *Service) ExportCardTypesToFile(filePath string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	store, err := r.LoadUserTypeStore()
	if err != nil {
		return fmt.Errorf("load card types: %w", err)
	}
	exp := CardTypesExport{
		Format: "bruv-card-types", Version: 1,
		Types: store.Types, Templates: store.Templates,
		BuiltinOverrides: store.BuiltinOverrides,
	}
	data, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal export: %w", err)
	}
	return os.WriteFile(filePath, data, 0o644)
}

func (s *Service) ImportCardTypesFromFile(filePath, mode string) (CardTypesImportResult, error) {
	var result CardTypesImportResult
	if s.deps.Repo() == nil {
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
	return s.applyCardTypesImport(exp, mode)
}

func (s *Service) ImportCardTypesFromRepo(otherRepoPath, mode string) (CardTypesImportResult, error) {
	var result CardTypesImportResult
	if s.deps.Repo() == nil {
		return result, fmt.Errorf("no repository open")
	}
	// Check the current location first; fall back to the legacy
	// .bruv/ location for repos that have not yet been opened (and
	// thereby migrated) by this build.
	src := filepath.Join(otherRepoPath, "card_types.json")
	data, err := os.ReadFile(src)
	if err != nil && os.IsNotExist(err) {
		src = filepath.Join(otherRepoPath, ".bruv", "card_types.json")
		data, err = os.ReadFile(src)
	}
	if err != nil {
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
		Format: "bruv-card-types", Version: 1,
		Types: store.Types, Templates: store.Templates,
		BuiltinOverrides: store.BuiltinOverrides,
	}
	return s.applyCardTypesImport(exp, mode)
}

func (s *Service) applyCardTypesImport(exp CardTypesExport, mode string) (CardTypesImportResult, error) {
	var result CardTypesImportResult
	r := s.deps.Repo()

	current, err := r.LoadUserTypeStore()
	if err != nil {
		return result, fmt.Errorf("load current card types: %w", err)
	}

	switch mode {
	case "replace":
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

	if err := r.SaveUserTypeStore(current); err != nil {
		return result, fmt.Errorf("save merged card types: %w", err)
	}
	return result, nil
}

// --- Seeding (called by ListCardTypes) ---

func (s *Service) ensureSeeded(store *config.UserTypeStore) bool {
	if store.Seeded {
		return false
	}
	store.Seeded = true
	for _, seed := range seedTypes {
		store.Types = append(store.Types, seed)
	}
	return true
}

func (s *Service) ensureStarterTemplates(store *config.UserTypeStore) bool {
	reg := s.deps.Registry()
	if store.StarterTemplatesSeeded || reg == nil {
		return false
	}
	store.StarterTemplatesSeeded = true

	if store.BuiltinOverrides == nil {
		store.BuiltinOverrides = make(map[string]config.BuiltinOverride)
	}

	schemaTemplateIDs := make(map[string]string)
	for _, typeName := range reg.List() {
		blocks := reg.SchemaToBlocks(typeName)
		if len(blocks) == 0 {
			continue
		}
		name := typeName
		if sc := reg.Get(typeName); sc != nil && sc.Name != "" {
			name = sc.Name
		}
		tmpl := config.CardTemplate{ID: uuid.New().String(), Name: name, Blocks: blocks}
		store.Templates = append(store.Templates, tmpl)
		schemaTemplateIDs[typeName] = tmpl.ID
	}

	for i, ut := range store.Types {
		if ut.TemplateID == "" {
			if tid, ok := schemaTemplateIDs[ut.ID]; ok {
				store.Types[i].TemplateID = tid
			}
		}
	}

	for _, bt := range BuiltinTypes {
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

func (s *Service) ensureMissingBuiltinTemplates(store *config.UserTypeStore) bool {
	reg := s.deps.Registry()
	if reg == nil {
		return false
	}
	if store.BuiltinOverrides == nil {
		store.BuiltinOverrides = make(map[string]config.BuiltinOverride)
	}
	changed := false
	for _, bt := range BuiltinTypes {
		ov := store.BuiltinOverrides[bt.ID]
		if ov.TemplateID != "" {
			continue
		}
		blocks := reg.SchemaToBlocks(bt.ID)
		if len(blocks) == 0 {
			continue
		}
		name := bt.Label
		if sc := reg.Get(bt.ID); sc != nil && sc.Name != "" {
			name = sc.Name
		}
		tmpl := config.CardTemplate{ID: uuid.New().String(), Name: name, Blocks: blocks}
		store.Templates = append(store.Templates, tmpl)
		ov.TemplateID = tmpl.ID
		store.BuiltinOverrides[bt.ID] = ov
		changed = true
	}
	return changed
}

// --- Tags (repo-wide colour map) ---

func (s *Service) GetTagColors() (map[string]string, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.GetTagColors()
}

func (s *Service) SetTagColor(tag, color string) (map[string]string, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.SetTagColor(tag, color)
}

func (s *Service) AssignTagColor(tag string) (map[string]string, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.AssignTagColor(tag)
}

// --- Labels (per-project) ---

func (s *Service) GetProjectLabels(brandSlug, streamSlug, projectSlug string) ([]model.Label, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.GetProjectLabels(brandSlug, streamSlug, projectSlug)
}

func (s *Service) AddProjectLabel(brandSlug, streamSlug, projectSlug, name, color string) ([]model.Label, error) {
	name = repo.SanitizeText(name)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	labels, err := r.AddProjectLabel(brandSlug, streamSlug, projectSlug, name, color)
	if err == nil {
		s.emitLabelsUpdated(brandSlug, streamSlug, projectSlug, labels)
	}
	return labels, err
}

func (s *Service) RemoveProjectLabel(brandSlug, streamSlug, projectSlug, labelID string) ([]model.Label, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	labels, err := r.RemoveProjectLabel(brandSlug, streamSlug, projectSlug, labelID)
	if err == nil {
		s.emitLabelsUpdated(brandSlug, streamSlug, projectSlug, labels)
	}
	return labels, err
}

func (s *Service) UpdateProjectLabel(brandSlug, streamSlug, projectSlug, labelID, name, color string) ([]model.Label, error) {
	if name != "" {
		name = repo.SanitizeText(name)
	}
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	labels, err := r.UpdateProjectLabel(brandSlug, streamSlug, projectSlug, labelID, name, color)
	if err == nil {
		s.emitLabelsUpdated(brandSlug, streamSlug, projectSlug, labels)
	}
	return labels, err
}

func (s *Service) SetProjectLabelIcon(brandSlug, streamSlug, projectSlug, labelID, icon string) ([]model.Label, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	labels, err := r.SetProjectLabelIcon(brandSlug, streamSlug, projectSlug, labelID, icon)
	if err == nil {
		s.emitLabelsUpdated(brandSlug, streamSlug, projectSlug, labels)
	}
	return labels, err
}

// emitLabelsUpdated publishes a labels:updated event with the full
// post-mutation label list so subscribers can replace state directly.
func (s *Service) emitLabelsUpdated(brandSlug, streamSlug, projectSlug string, labels []model.Label) {
	s.deps.Publish("labels:updated", map[string]any{
		"brandSlug":   brandSlug,
		"streamSlug":  streamSlug,
		"projectSlug": projectSlug,
		"labels":      labels,
	})
}

// UpdateCardLabels replaces a card's label IDs and re-indexes the card.
func (s *Service) UpdateCardLabels(id string, labelIDs []string) (*model.Card, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := r.UpdateCard(id, func(c *model.Card) {
		c.Labels = labelIDs
	})
	if err == nil {
		if idx := s.deps.Index(); idx != nil {
			if ierr := idx.IndexCard(card, time.Now(), idx.GetCardProjectContext(card.ID)); ierr != nil {
				slog.Warn("index update failed", "op", "IndexCard", "err", ierr)
			}
		}
	}
	return card, err
}

// HealTagColors is a best-effort background repair run on repo open.
// Walks every card and makes sure each tag appears (with its colour)
// as a label in every project that card is pinned to.
func (s *Service) HealTagColors() {
	r := s.deps.Repo()
	if r == nil {
		return
	}
	cards, err := r.ListCards()
	if err != nil || len(cards) == 0 {
		return
	}

	type hierKey struct{ brand, stream, project string }
	catToHier := make(map[string]hierKey)
	flat, _ := r.ListAllCategoriesFlat()
	for _, f := range flat {
		catToHier[f.Category.ID] = hierKey{f.Brand.Slug, f.Stream.Slug, f.Project.Slug}
	}

	for _, card := range cards {
		if len(card.Tags) == 0 {
			continue
		}
		pins, err := r.GetCardPins(card.ID)
		if err != nil {
			continue
		}
		seen := make(map[string]bool)
		for _, pin := range pins {
			h, ok := catToHier[pin.CategoryID]
			if !ok {
				continue
			}
			key := h.brand + "/" + h.stream + "/" + h.project
			if seen[key] {
				continue
			}
			seen[key] = true

			labels, _ := r.GetProjectLabels(h.brand, h.stream, h.project)
			existing := make(map[string]bool, len(labels))
			for _, l := range labels {
				existing[strings.ToLower(l.Name)] = true
			}
			for _, tag := range card.Tags {
				if !existing[strings.ToLower(tag)] {
					r.AddProjectLabel(h.brand, h.stream, h.project, tag, "")
				}
			}
		}
	}
}

// --- Package-level helpers ---

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
		return false
	}
	return false
}

func cloneBlocksWithFreshIDs(blocks []model.Block) []model.Block {
	cloned := make([]model.Block, len(blocks))
	for i, b := range blocks {
		cloned[i] = b
		cloned[i].ID = uuid.New().String()
	}
	return cloned
}

func isTypeIDTaken(store config.UserTypeStore, id string) bool {
	for _, t := range store.Types {
		if t.ID == id {
			return true
		}
	}
	for _, b := range BuiltinTypes {
		if b.ID == id {
			return true
		}
	}
	return false
}
