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
	"hash/fnv"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Deps is the narrow host contract for CatalogService.
type Deps interface {
	Repo() *repo.Repository
	Registry() *schema.Registry
	Index() *index.Index
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

// DefaultCardType is the type given to a card created by an LLM surface
// (MCP, chat, agents) that names none. Always a built-in, so it exists
// in every repo and can never mint a phantom type.
const DefaultCardType = "brainstorm"

// CardTypeExists reports whether id names an existing card type exactly.
// It fails closed: when the user type store can't be read only the
// built-ins exist, so a user type is refused rather than guessed at.
func (s *Service) CardTypeExists(id string) bool {
	types, _ := s.LoadCardTypes() // built-ins survive a load error
	for _, t := range types {
		if t.ID == id {
			return true
		}
	}
	return false
}

// seedTypes are pre-installed as user types on first run.
var seedTypes = []config.UserCardType{
	{ID: "feature", Label: "Feature", Color: "#6366f1"},
	{ID: "episode", Label: "Episode", Color: "#ec4899"},
}

// --- Card type listing + schema ---

// ListCardTypes returns all card types (built-in first, then user).
// Safe to call before a repo is open — returns only built-ins in that
// case so the UI has something to render during early boot. A store
// that can't be read is logged and yields the built-ins only; callers
// that can report the failure use LoadCardTypes.
func (s *Service) ListCardTypes() []CardTypeInfo {
	types, err := s.LoadCardTypes()
	if err != nil {
		slog.Error("card types: user type store unreadable; listing built-ins only", "err", err)
	}
	return types
}

// LoadCardTypes is ListCardTypes with the store's load error. On a load
// error it returns the built-ins alone and writes NOTHING: seeding after
// a failed read would overwrite card_types.json with the seed types and
// wipe every user type and template.
func (s *Service) LoadCardTypes() ([]CardTypeInfo, error) {
	var store config.UserTypeStore
	var loadErr error
	if r := s.deps.Repo(); r != nil {
		store, loadErr = s.loadSeededStore(r)
	}
	return s.typeInfos(store), loadErr
}

// loadSeededStore reads the card types store, seeding the defaults it
// lacks under the store's lock. A failed seed write still returns the
// store as read; a failed read returns an empty store and the error.
func (s *Service) loadSeededStore(r *repo.Repository) (config.UserTypeStore, error) {
	store, err := r.UpdateUserTypeStore(func(store *config.UserTypeStore) error {
		dirty := s.ensureSeeded(store)
		dirty = s.ensureStarterTemplates(store) || dirty
		dirty = s.ensureMissingBuiltinTemplates(store) || dirty
		if !dirty {
			return repo.ErrNoChange
		}
		return nil
	})
	if err == nil {
		return store, nil
	}
	// The load failed (nothing was written), or the seed write did.
	if store, loadErr := r.LoadUserTypeStore(); loadErr == nil {
		slog.Warn("card types: saving seeded defaults failed", "err", err)
		return store, nil
	}
	return config.UserTypeStore{}, fmt.Errorf("load card types: %w", err)
}

// typeInfos lists the built-in types (with the store's overrides) then
// the store's user types.
func (s *Service) typeInfos(store config.UserTypeStore) []CardTypeInfo {
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
			if ov.Icon != "" {
				info.Icon = ov.Icon
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

// updateStore is the locked read-modify-write of the card types store
// (repo.UpdateUserTypeStore): an unreadable store is never overwritten.
func (s *Service) updateStore(fn func(store *config.UserTypeStore) error) (config.UserTypeStore, error) {
	r := s.deps.Repo()
	if r == nil {
		return config.UserTypeStore{}, fmt.Errorf("no repository open")
	}
	return r.UpdateUserTypeStore(fn)
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
	return s.createUserCardType(config.UserCardType{
		Label: label, Color: color, Description: description, AIHint: aiHint, TemplateID: templateID,
	}, false)
}

// createUserCardType adds t to the store under an id slugged from its
// label (suffixed while taken). With refuseExisting, a label that already
// names a type — checked under the store's lock — is an error instead.
func (s *Service) createUserCardType(t config.UserCardType, refuseExisting bool) (config.UserCardType, error) {
	_, err := s.updateStore(func(store *config.UserTypeStore) error {
		if refuseExisting {
			if id, ok := matchType(s.typeInfos(*store), t.Label); ok {
				return fmt.Errorf("card type %q already exists (id %s); use it instead of creating another", t.Label, id)
			}
		}
		t.ID = freeTypeID(*store, t.Label)
		store.Types = append(store.Types, t)
		return nil
	})
	if err != nil {
		return config.UserCardType{}, err
	}
	s.deps.Publish("cardtype:updated", t)
	return t, nil
}

// freeTypeID slugs label into a type id not yet taken in store.
func freeTypeID(store config.UserTypeStore, label string) string {
	id := repo.Slugify(label)
	if id == "" {
		id = uuid.New().String()
	}
	base := id
	for i := 2; isTypeIDTaken(store, id); i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	return id
}

// aiTypePalette colours AI-created card types deterministically — a type
// the model just created must never render as the grey unknown-type
// fallback. Hues match the builtin/seed families.
var aiTypePalette = []string{
	"#6366f1", "#ec4899", "#38bdf8", "#fb923c",
	"#22c55e", "#eab308", "#a855f7", "#14b8a6",
}

// hexColor is the colour shape card types store (#rgb or #rrggbb).
var hexColor = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// UnknownTypeError is ResolveType's refusal of a type name that matches
// nothing. Its message lists the available types so a model can retry
// with a real one.
type UnknownTypeError struct {
	Name      string
	Available []CardTypeInfo
}

func (e *UnknownTypeError) Error() string {
	names := make([]string, 0, len(e.Available))
	for _, t := range e.Available {
		if t.Label != "" && !strings.EqualFold(t.Label, t.ID) {
			names = append(names, fmt.Sprintf("%s (%s)", t.ID, t.Label))
		} else {
			names = append(names, t.ID)
		}
	}
	return fmt.Sprintf("unknown card type %q; available types: %s", e.Name, strings.Join(names, ", "))
}

// ResolveType canonicalises a card type named by an LLM surface (chat,
// agents, MCP) WITHOUT creating anything (ruling 2026-09-30: every LLM
// surface refuses an unknown type; creating one is a deliberate act, done
// with the explicit create_card_type tool). A case-insensitive match on
// the id or label of any existing type returns its canonical id; anything
// else is an *UnknownTypeError. Empty input resolves to the empty id (an
// untyped card).
func (s *Service) ResolveType(input string) (string, error) {
	name := strings.TrimSpace(input)
	if name == "" {
		return "", nil
	}
	types, loadErr := s.LoadCardTypes()
	if id, ok := matchType(types, name); ok {
		return id, nil
	}
	if loadErr != nil {
		// A user type may exist in the store we couldn't read.
		return "", fmt.Errorf("card type %q: %w", name, loadErr)
	}
	return "", &UnknownTypeError{Name: name, Available: types}
}

// CreateNamedType is the explicit "create a card type" act behind the
// create_card_type tool. Unlike CreateUserCardType (the settings UI,
// which suffixes a taken id) it refuses a label that already names a
// type — by id or label, case-insensitively — so a model can't mint
// near-duplicates. A blank colour gets a palette colour picked by name.
func (s *Service) CreateNamedType(label, color, description, aiHint string) (config.UserCardType, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return config.UserCardType{}, fmt.Errorf("label is required")
	}
	color = strings.TrimSpace(color)
	if color != "" && !hexColor.MatchString(color) {
		return config.UserCardType{}, fmt.Errorf("color %q is not a hex colour like #6366f1", color)
	}
	if color == "" {
		h := fnv.New32a()
		h.Write([]byte(strings.ToLower(label)))
		color = aiTypePalette[int(h.Sum32())%len(aiTypePalette)]
	}
	return s.createUserCardType(config.UserCardType{
		Label: label, Color: color, Description: strings.TrimSpace(description), AIHint: strings.TrimSpace(aiHint),
	}, true)
}

// FindOrCreateType returns the id of the type t.Label names (by id or
// label, case-insensitively, trimmed). When none does, t is created — with
// a template holding templateBlocks, when given — in the same locked store
// write as the lookup, so concurrent callers never mint duplicates.
func (s *Service) FindOrCreateType(t config.UserCardType, templateBlocks []model.Block) (string, error) {
	t.Label = strings.TrimSpace(t.Label)
	if t.Label == "" {
		return "", fmt.Errorf("label is required")
	}
	created := false
	_, err := s.updateStore(func(store *config.UserTypeStore) error {
		if id, ok := matchType(s.typeInfos(*store), t.Label); ok {
			t.ID = id
			return repo.ErrNoChange
		}
		if len(templateBlocks) > 0 {
			tmpl := config.CardTemplate{ID: uuid.New().String(), Name: t.Label, Blocks: templateBlocks}
			store.Templates = append(store.Templates, tmpl)
			t.TemplateID = tmpl.ID
		}
		t.ID = freeTypeID(*store, t.Label)
		store.Types = append(store.Types, t)
		created = true
		return nil
	})
	if err != nil {
		return "", err
	}
	if created {
		s.deps.Publish("cardtype:updated", t)
	}
	return t.ID, nil
}

// LookupTypeID matches input against the catalog by id or label, case
// insensitively, without creating anything. ok is false for an unknown
// type — the name the model gave is then the only handle there is.
func (s *Service) LookupTypeID(input string) (id string, ok bool) {
	return matchType(s.ListCardTypes(), input)
}

// matchType finds input among types by id or label, case-insensitively.
func matchType(types []CardTypeInfo, input string) (string, bool) {
	name := strings.TrimSpace(input)
	if name == "" {
		return "", false
	}
	for _, t := range types {
		if strings.EqualFold(t.ID, name) || strings.EqualFold(t.Label, name) {
			return t.ID, true
		}
	}
	return "", false
}

func (s *Service) UpdateUserCardType(id, label, color, description, aiHint, templateID string) (config.UserCardType, error) {
	return s.updateUserCardType(id, func(t *config.UserCardType) {
		t.Label = label
		t.Color = color
		t.Description = description
		t.AIHint = aiHint
		t.TemplateID = templateID
	})
}

func (s *Service) UpdateUserCardTypeIcon(id, icon string) (config.UserCardType, error) {
	return s.updateUserCardType(id, func(t *config.UserCardType) { t.Icon = icon })
}

// updateUserCardType edits one user type in place and publishes it.
func (s *Service) updateUserCardType(id string, edit func(t *config.UserCardType)) (config.UserCardType, error) {
	var updated config.UserCardType
	_, err := s.updateStore(func(store *config.UserTypeStore) error {
		for i := range store.Types {
			if store.Types[i].ID == id {
				edit(&store.Types[i])
				updated = store.Types[i]
				return nil
			}
		}
		return fmt.Errorf("card type %q not found", id)
	})
	if err != nil {
		return config.UserCardType{}, err
	}
	s.deps.Publish("cardtype:updated", updated)
	return updated, nil
}

func (s *Service) DeleteUserCardType(id string) error {
	_, err := s.updateStore(func(store *config.UserTypeStore) error {
		for i, t := range store.Types {
			if t.ID == id {
				store.Types = append(store.Types[:i], store.Types[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("card type %q not found", id)
	})
	if err != nil {
		return err
	}
	s.deps.Publish("cardtype:deleted", map[string]any{"id": id})
	return nil
}

// UpdateBuiltinCardType replaces the user's override for a built-in type.
// Every overridable field is passed on each save, so an omitted one is
// cleared rather than silently kept from a stale override.
func (s *Service) UpdateBuiltinCardType(id, color, icon, templateID string) error {
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
	_, err := s.updateStore(func(store *config.UserTypeStore) error {
		if store.BuiltinOverrides == nil {
			store.BuiltinOverrides = make(map[string]config.BuiltinOverride)
		}
		store.BuiltinOverrides[id] = config.BuiltinOverride{Color: color, Icon: icon, TemplateID: templateID}
		return nil
	})
	if err != nil {
		return err
	}
	s.deps.Publish("cardtype:updated", map[string]any{"id": id})
	return nil
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
	tmpl := config.CardTemplate{ID: uuid.New().String(), Name: name, Blocks: blocks}
	_, err := s.updateStore(func(store *config.UserTypeStore) error {
		store.Templates = append(store.Templates, tmpl)
		return nil
	})
	return tmpl, err
}

func (s *Service) UpdateCardTemplate(id, name string, blocks []model.Block) (config.CardTemplate, error) {
	var updated config.CardTemplate
	_, err := s.updateStore(func(store *config.UserTypeStore) error {
		for i := range store.Templates {
			if store.Templates[i].ID == id {
				store.Templates[i].Name = name
				store.Templates[i].Blocks = blocks
				updated = store.Templates[i]
				return nil
			}
		}
		return fmt.Errorf("template %q not found", id)
	})
	return updated, err
}

func (s *Service) DeleteCardTemplate(id string) error {
	_, err := s.updateStore(func(store *config.UserTypeStore) error {
		for i, tmpl := range store.Templates {
			if tmpl.ID == id {
				store.Templates = append(store.Templates[:i], store.Templates[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("template %q not found", id)
	})
	return err
}

// --- Type block merging ---

// ApplyTypeBlocks non-destructively merges a type's template blocks
// into a card. Called by the card service when a type is set on create
// or type change.
func (s *Service) ApplyTypeBlocks(cardID, cardType string) error {
	templateBlocks := s.ResolveTemplateBlocks(cardType)
	if len(templateBlocks) == 0 {
		return nil
	}
	return s.mergeTemplateBlocks(cardID, templateBlocks)
}

// ResolveTemplateBlocks returns the template/schema blocks for a card
// type. Priority: user template > builtin-override template > schema.
func (s *Service) ResolveTemplateBlocks(cardType string) []model.Block {
	var store config.UserTypeStore
	if r := s.deps.Repo(); r != nil {
		var err error
		if store, err = r.LoadUserTypeStore(); err != nil {
			// Read-only here: fall back to the schema, never write.
			slog.Warn("card types: store unreadable; using schema blocks only", "type", cardType, "err", err)
			store = config.UserTypeStore{}
		}
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
//
// The merge runs on a fresh read under the card's file lock, so an edit
// saved meanwhile is merged into rather than overwritten.
func (s *Service) mergeTemplateBlocks(cardID string, templateBlocks []model.Block) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	changed := false
	card, err := r.MutateCard(cardID, func(card *model.Card) error {
		merged := mergeBlocks(card.Blocks, templateBlocks)
		if reflect.DeepEqual(merged, card.Blocks) {
			return repo.ErrNoChange
		}
		card.Blocks = merged
		changed = true
		return nil
	})
	if err != nil {
		return fmt.Errorf("save template blocks: %w", err)
	}
	if changed {
		if idx := s.deps.Index(); idx != nil {
			if err := idx.IndexCard(card, time.Now(), idx.GetCardProjectContext(card.ID)); err != nil {
				slog.Warn("index update failed", "op", "IndexCard", "err", err)
			}
		}
		s.deps.Publish("card:updated", map[string]any{"cardID": card.ID, "card": card})
	}
	return nil
}

// mergeBlocks is the template merge itself: existing with the template's
// missing fields appended, as a new slice (existing is left untouched).
func mergeBlocks(existing, templateBlocks []model.Block) []model.Block {
	intrinsicKeys := map[string]bool{"description": true}

	// A block's key is only meaningful RELATIVE to the template being
	// applied. Blocks whose key appears in the template are claimed by
	// their own field (exact key match, never stolen by a label
	// coincidence). Everything else — empty keys (hand-added blocks) and
	// orphan keys (residue of another type's schema, or of a relabelled
	// field like the "content" block renamed 'Related links' that
	// surfaced this) — is claimable by a template field with the same
	// label (case-insensitive) and same block type: from the user's view
	// a same-named block IS the field, and duplicating it is the bug.
	// Claimed blocks keep their value and adopt the template's key so
	// future refreshes reconcile by key.
	templateKeys := make(map[string]bool, len(templateBlocks))
	for _, tb := range templateBlocks {
		if tb.Key != "" {
			templateKeys[tb.Key] = true
		}
	}

	existingByKey := make(map[string]int)
	claimableByLabel := make(map[string]int)
	for i, b := range existing {
		if b.Key != "" && templateKeys[b.Key] {
			existingByKey[b.Key] = i
			continue
		}
		if lk := freeformLabelKey(b.Label, b.Type); lk != "" {
			if _, dup := claimableByLabel[lk]; !dup { // first occurrence wins
				claimableByLabel[lk] = i
			}
		}
	}

	merged := make([]model.Block, len(existing))
	copy(merged, existing)

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
		if lk := freeformLabelKey(tb.Label, tb.Type); lk != "" {
			if idx, ok := claimableByLabel[lk]; ok {
				delete(claimableByLabel, lk) // each block is claimable once
				merged[idx].Key = tb.Key     // adopt the schema identity
				if isBlockValueEmpty(merged[idx].Value) && !isBlockValueEmpty(tb.Value) {
					merged[idx].Value = tb.Value
				}
				continue
			}
		}
		merged = append(merged, tb)
	}
	return merged
}

// freeformLabelKey builds the case-insensitive (label, type) match key
// used to reconcile keyless hand-added blocks against template fields.
// Empty labels never match — an unlabelled block is not identifiable.
func freeformLabelKey(label, blockType string) string {
	l := strings.ToLower(strings.TrimSpace(label))
	if l == "" {
		return ""
	}
	return l + "\x00" + blockType
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
	if err := s.mergeTemplateBlocks(cardID, templateBlocks); err != nil {
		return nil, err
	}
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
	_, err := s.updateStore(func(current *config.UserTypeStore) error {
		result = CardTypesImportResult{}
		return mergeImport(current, exp, mode, &result)
	})
	if err != nil {
		return CardTypesImportResult{}, fmt.Errorf("import card types: %w", err)
	}
	return result, nil
}

// mergeImport applies an export to the current store in the given mode,
// counting what it did in result.
func mergeImport(current *config.UserTypeStore, exp CardTypesExport, mode string, result *CardTypesImportResult) error {
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
		return fmt.Errorf("unknown import mode %q (expected replace, merge, or merge_overwrite)", mode)
	}
	return nil
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
