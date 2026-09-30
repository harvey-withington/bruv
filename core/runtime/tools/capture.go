package tools

// Card capture shared by every LLM surface that creates cards: the MCP
// server's create_card and the agent's built-in create_card. One parser
// and one create path keep filing, tags, description, due date and
// blocks behaving identically wherever a card comes from.

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"bruv/core/services/card"
	"bruv/core/services/catalog"
	projectsvc "bruv/core/services/project"
	"bruv/internal/model"

	"github.com/google/uuid"
)

// TypeResolver canonicalises a card type by id or label and refuses an
// unknown one (ruling 2026-09-30: LLM surfaces never create types
// implicitly; create_card_type is the explicit act). Satisfied by
// *catalog.Service.
type TypeResolver interface {
	ResolveType(input string) (id string, err error)
}

// Location names a category by its Brand → Stream → Project → Category
// path. Each level matches by name or slug, case-insensitively.
type Location struct {
	Brand, Stream, Project, Category string
}

// IsEmpty reports whether no level is named.
func (l Location) IsEmpty() bool { return l == Location{} }

func (l Location) complete() bool {
	return l.Brand != "" && l.Stream != "" && l.Project != "" && l.Category != ""
}

// CardSpec is a card to create and populate in one call.
type CardSpec struct {
	Title       string
	Type        string // id or label of an existing type; empty = catalog.DefaultCardType
	Description string
	DueDate     string // YYYY-MM-DD; empty = none
	Tags        []string
	Blocks      []model.Block
	Location    Location // empty = leave in the inbox
}

// CreatedCard is the outcome of CreateCard.
type CreatedCard struct {
	Card     *model.Card
	PinnedTo string // breadcrumb, or "" when left in the inbox
}

// ParseCardSpec reads create_card tool arguments: title, card_type,
// description, due_date, tags, blocks and brand/stream/project/category.
func ParseCardSpec(a map[string]any) (CardSpec, error) {
	str := func(k string) string {
		s, _ := a[k].(string)
		return strings.TrimSpace(s)
	}
	spec := CardSpec{
		Title:       str("title"),
		Type:        str("card_type"),
		Description: str("description"),
		DueDate:     str("due_date"),
		Tags:        stringList(a["tags"]),
		Location:    LocationArgs(a),
	}
	var errs []error
	if spec.Title == "" {
		errs = append(errs, errors.New("title is required"))
	}
	if !spec.Location.IsEmpty() && !spec.Location.complete() {
		errs = append(errs, errors.New("to file the card, provide all of brand, stream, project and category (or none to leave it in the inbox)"))
	}
	if spec.DueDate != "" {
		if _, err := card.ParseDueDate(spec.DueDate); err != nil {
			errs = append(errs, fmt.Errorf("due_date: %w", err))
		}
	}
	blocks, err := ParseBlocks(a["blocks"])
	if err != nil {
		errs = append(errs, err)
	} else if err := CheckNewBlockKeys(&model.Card{}, blocks); err != nil {
		errs = append(errs, err)
	}
	spec.Blocks = blocks
	return spec, errors.Join(errs...)
}

// CreateCard creates a card and applies everything in spec, creating any
// missing level of the filing path. Everything that can be refused — the
// type, the category's accepted types — is checked BEFORE the card
// exists, and a step that still fails afterwards deletes the new card:
// a refused create must never leave an orphan (each model retry used to
// add another).
func CreateCard(cs *card.Service, ps *projectsvc.Service, types TypeResolver, spec CardSpec) (*CreatedCard, error) {
	cardType := catalog.DefaultCardType
	if spec.Type != "" {
		var err error
		if cardType, err = types.ResolveType(spec.Type); err != nil {
			return nil, err
		}
	}
	var catID, breadcrumb string
	if !spec.Location.IsEmpty() {
		var err error
		if catID, breadcrumb, err = ResolveOrCreateCategory(ps, spec.Location); err != nil {
			return nil, err
		}
		if err := cs.CheckCategoryAcceptsType(catID, cardType); err != nil {
			return nil, fmt.Errorf("pin card: %w", err)
		}
	}
	c, err := cs.Create(cardType, spec.Title)
	if err != nil {
		return nil, err
	}
	if c, err = populateCard(cs, c, catID, spec); err != nil {
		if delErr := cs.Delete(c.ID); delErr != nil {
			return nil, fmt.Errorf("%w (and removing the half-made card %s failed: %v)", err, c.ID, delErr)
		}
		return nil, err
	}
	return &CreatedCard{Card: c, PinnedTo: breadcrumb}, nil
}

// populateCard applies the rest of spec to a just-created card. It
// always returns a card (the latest good state) so the caller can roll
// back by id.
func populateCard(cs *card.Service, c *model.Card, catID string, spec CardSpec) (*model.Card, error) {
	step := func(name string, fn func() (*model.Card, error)) error {
		updated, err := fn()
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		c = updated
		return nil
	}
	if catID != "" {
		if err := cs.Pin(c.ID, catID); err != nil {
			return c, fmt.Errorf("pin card: %w", err)
		}
	}
	if len(spec.Tags) > 0 {
		if err := step("set tags", func() (*model.Card, error) { return cs.UpdateTags(c.ID, spec.Tags) }); err != nil {
			return c, err
		}
	}
	if spec.Description != "" {
		if err := step("set description", func() (*model.Card, error) { return cs.UpdateDescription(c.ID, spec.Description) }); err != nil {
			return c, err
		}
	}
	if spec.DueDate != "" {
		if err := step("set due date", func() (*model.Card, error) { return cs.UpdateDueDate(c.ID, spec.DueDate) }); err != nil {
			return c, err
		}
	}
	if len(spec.Blocks) > 0 {
		blocks, err := mergeIntoSeeded(c.Blocks, spec.Blocks)
		if err != nil {
			return c, err
		}
		if err := step("add blocks", func() (*model.Card, error) { return cs.UpdateBlocks(c.ID, blocks) }); err != nil {
			return c, err
		}
	}
	return c, nil
}

// mergeIntoSeeded adds a new card's blocks after the ones its type's
// template seeded. A block whose key the template already gave the card
// fills that field (shaped by the field's own settings) instead of
// becoming a second block with the same key — the model can't know the
// template's keys in advance.
func mergeIntoSeeded(seeded, blocks []model.Block) ([]model.Block, error) {
	out := slices.Clone(seeded)
	byKey := make(map[string]int, len(out))
	for i, b := range out {
		if b.Key != "" {
			byKey[b.Key] = i
		}
	}
	for _, b := range blocks {
		i, ok := byKey[b.Key]
		if b.Key == "" || !ok {
			out = append(out, b)
			continue
		}
		coerced, err := CoerceBlockValueForBlock(&out[i], b.Value)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", b.Key, err)
		}
		out[i].Value = coerced
	}
	return out, nil
}

// knownBlockTypes are the block types a tool may create — the frontend's
// BLOCK_TYPES (shared/types.ts); legacy "video" is read-only.
var knownBlockTypes = []string{
	model.BlockText, model.BlockChecklist, model.BlockList, model.BlockMedia, model.BlockURL,
	model.BlockDivider, model.BlockSelect, model.BlockNumber, model.BlockDate, model.BlockRating,
	model.BlockCheckbox, model.BlockRadio, model.BlockCheckboxGroup, model.BlockImage,
	model.BlockProgress, model.BlockAlarm, model.BlockSurvey, model.BlockSlideDeck, model.BlockWorkspaceFiles,
}

// ParseBlocks converts the tool block shape ({type,label,value,key?})
// into model.Block values with fresh ids, coercing each value the same
// way chat edits are coerced (checklists become arrays, dates normalise).
// An unknown block type is an error listing the valid ones — it would
// otherwise render as nothing.
func ParseBlocks(raw any) ([]model.Block, error) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, nil
	}
	out := make([]model.Block, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := m["type"].(string)
		blockType = strings.TrimSpace(blockType)
		if blockType == "" {
			blockType = model.BlockText
		}
		if !slices.Contains(knownBlockTypes, blockType) {
			return nil, fmt.Errorf("unknown block type %q; use one of: %s", blockType, strings.Join(knownBlockTypes, ", "))
		}
		label, _ := m["label"].(string)
		key, _ := m["key"].(string)
		value, hasValue := m["value"]
		if !hasValue || value == nil {
			value = DefaultBlockValue(blockType)
		}
		b := model.Block{
			ID:    "blk-" + uuid.New().String()[:8],
			Type:  blockType,
			Label: strings.TrimSpace(label),
			Key:   strings.TrimSpace(key),
			Value: value,
		}
		// A date block is date-only unless asked otherwise; "date-time"
		// keeps the time and UTC offset (e.g. a race's jump time).
		if format, _ := m["format"].(string); blockType == model.BlockDate && format == "date-time" {
			b.Meta = map[string]any{"format": format}
		}
		// The coerced value is best-effort even when a constraint is
		// violated, so it is always taken and the advisory error ignored.
		if coerced, _ := CoerceBlockValueForBlock(&b, b.Value); coerced != nil {
			b.Value = coerced
		}
		out = append(out, b)
	}
	return out, nil
}

func stringList(raw any) []string {
	arr, _ := raw.([]any)
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// --- Hierarchy lookup ---

// FindBrand resolves a brand by name or slug without creating it.
func FindBrand(ps *projectsvc.Service, nameOrSlug string) (slug, name string, ok bool) {
	brands, _ := ps.ListBrands()
	for _, b := range brands {
		if matches(b.Name, b.Slug, nameOrSlug) {
			return b.Slug, b.Name, true
		}
	}
	return "", "", false
}

// FindStream resolves a stream within a brand without creating it.
func FindStream(ps *projectsvc.Service, brandSlug, nameOrSlug string) (slug, name string, ok bool) {
	streams, _ := ps.ListStreams(brandSlug)
	for _, s := range streams {
		if matches(s.Name, s.Slug, nameOrSlug) {
			return s.Slug, s.Name, true
		}
	}
	return "", "", false
}

// FindProject resolves a project within a stream without creating it.
func FindProject(ps *projectsvc.Service, brandSlug, streamSlug, nameOrSlug string) (slug, name string, ok bool) {
	projects, _ := ps.ListProjects(brandSlug, streamSlug)
	for _, p := range projects {
		if matches(p.Name, p.Slug, nameOrSlug) {
			return p.Slug, p.Name, true
		}
	}
	return "", "", false
}

// EnsureBrand finds a brand or creates it.
func EnsureBrand(ps *projectsvc.Service, nameOrSlug string) (slug, name string, err error) {
	if s, n, ok := FindBrand(ps, nameOrSlug); ok {
		return s, n, nil
	}
	b, err := ps.CreateBrand(nameOrSlug)
	if err != nil {
		return "", "", fmt.Errorf("create brand %q: %w", nameOrSlug, err)
	}
	return b.Slug, b.Name, nil
}

// EnsureStream finds a stream or creates it.
func EnsureStream(ps *projectsvc.Service, brandSlug, nameOrSlug string) (slug, name string, err error) {
	if s, n, ok := FindStream(ps, brandSlug, nameOrSlug); ok {
		return s, n, nil
	}
	s, err := ps.CreateStream(brandSlug, nameOrSlug)
	if err != nil {
		return "", "", fmt.Errorf("create stream %q: %w", nameOrSlug, err)
	}
	return s.Slug, s.Name, nil
}

// EnsureProject finds a project or creates it.
func EnsureProject(ps *projectsvc.Service, brandSlug, streamSlug, nameOrSlug string) (slug, name string, err error) {
	if s, n, ok := FindProject(ps, brandSlug, streamSlug, nameOrSlug); ok {
		return s, n, nil
	}
	p, err := ps.CreateProject(brandSlug, streamSlug, nameOrSlug)
	if err != nil {
		return "", "", fmt.Errorf("create project %q: %w", nameOrSlug, err)
	}
	return p.Slug, p.Name, nil
}

// ResolveOrCreateCategory walks the location, creating any level that
// doesn't exist, and returns the category id plus a breadcrumb.
func ResolveOrCreateCategory(ps *projectsvc.Service, loc Location) (catID, breadcrumb string, err error) {
	brandSlug, brandName, err := EnsureBrand(ps, loc.Brand)
	if err != nil {
		return "", "", err
	}
	streamSlug, streamName, err := EnsureStream(ps, brandSlug, loc.Stream)
	if err != nil {
		return "", "", err
	}
	projectSlug, projectName, err := EnsureProject(ps, brandSlug, streamSlug, loc.Project)
	if err != nil {
		return "", "", err
	}
	cats, _ := ps.ListCategories(brandSlug, streamSlug, projectSlug)
	categoryName := loc.Category
	for _, c := range cats {
		if matches(c.Name, c.Slug, loc.Category) {
			catID, categoryName = c.ID, c.Name
			break
		}
	}
	if catID == "" {
		c, err := ps.CreateCategory(brandSlug, streamSlug, projectSlug, loc.Category, len(cats))
		if err != nil {
			return "", "", fmt.Errorf("create category %q: %w", loc.Category, err)
		}
		catID, categoryName = c.ID, c.Name
	}
	return catID, strings.Join([]string{brandName, streamName, projectName, categoryName}, " / "), nil
}

func matches(name, slug, query string) bool {
	return strings.EqualFold(name, query) || strings.EqualFold(slug, query)
}
