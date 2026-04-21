package main

// Card CRUD, pins, moves, category helpers, comments.
//
// This is the operational core of BRUV: everything the user does to a
// card after it exists. Grouped into three concerns sharing a file:
//
//   - Card lifecycle: create, read, list, duplicate, delete
//   - Card mutations: title, type, fields, blocks, attachments, tags,
//     due date, checklist, labels (labels live in app_tags.go)
//   - Location: pins, category validation, hierarchy lookups, moves
//
// Card comments also live here because a comment is a per-card object
// with identical I/O shape to attachments/checklist, and bringing them
// next to those makes it obvious when to use each.
//
// Index maintenance: every mutation below must call a.logIdxErr on its
// IndexCard/IndexPins/RemoveCard result — never discard the error —
// so a silent index/disk divergence produces a visible breadcrumb the
// user can act on via the Rebuild Search Index button.
//
// Extracted from app.go so changes to the card write path sit next to
// each other instead of being scattered across 700 lines.

import (
	"bruv/internal/config"
	"bruv/internal/model"
	"bruv/internal/repo"
	"fmt"
	"strings"
	"time"
)

// --- Card CRUD ---

func (a *App) CreateCard(cardType, title string) (*model.Card, error) {
	title = repo.SanitizeText(title)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.CreateCard(cardType, title)
	if err != nil {
		return nil, err
	}
	if a.idx != nil {
		a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), ""))
	}
	a.logActivity(card.ID, model.ActivityCreated, "")
	// Apply template blocks if a type was set at creation
	if cardType != "" {
		a.applyTypeBlocks(card.ID, cardType)
		if updated, err := a.repo.GetCard(card.ID); err == nil {
			return updated, nil
		}
	}
	return card, nil
}

func (a *App) GetCard(id string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.GetCard(id)
}

func (a *App) ListCards() ([]model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.ListCards()
}

// DuplicateCard creates a copy of a card with a new ID and pins it to the given category.
func (a *App) DuplicateCard(cardID, categoryID string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	newCard, err := a.repo.DuplicateCard(cardID)
	if err != nil {
		return nil, err
	}
	// Pin with categoryID for both projectID and categoryID (frontend convention)
	if err := a.repo.PinCard(newCard.ID, categoryID, categoryID); err != nil {
		return nil, err
	}
	if a.idx != nil {
		a.idxIncrementalRefresh()
	}
	return newCard, nil
}

// CopyCategory duplicates a category and all its cards within the same project.
func (a *App) CopyCategory(brandSlug, streamSlug, projectSlug, categorySlug string) (*model.Category, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	// Get source category
	srcCats, err := a.repo.ListCategories(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	var srcCat *model.Category
	for _, c := range srcCats {
		if c.Slug == categorySlug {
			cc := c
			srcCat = &cc
			break
		}
	}
	if srcCat == nil {
		return nil, fmt.Errorf("category %q not found", categorySlug)
	}

	// Create new category with " Copy" suffix
	newCat, err := a.repo.CreateCategory(brandSlug, streamSlug, projectSlug, srcCat.Name+" Copy", len(srcCats))
	if err != nil {
		return nil, err
	}

	// Duplicate all cards from source to new category
	if a.idx != nil {
		cardIDs, err := a.idx.ListCardIDsInCategory(srcCat.ID, srcCat.ID)
		if err == nil {
			for i, cardID := range cardIDs {
				newCard, err := a.repo.DuplicateCard(cardID)
				if err != nil {
					continue
				}
				_ = a.repo.PinCard(newCard.ID, newCat.ID, newCat.ID)
				_ = a.repo.MoveCardInCategory(newCard.ID, newCat.ID, newCat.ID, i)
			}
		}
		a.idxIncrementalRefresh()
	}

	return newCat, nil
}

func (a *App) DeleteCard(id string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	// Capture card context before deletion — both will fail once the card is gone.
	cardTitle := ""
	if card, err := a.repo.GetCard(id); err == nil {
		cardTitle = card.Title
	}
	breadcrumbs, _ := a.GetCardPinBreadcrumbs(id)
	if err := a.repo.DeleteCard(id); err != nil {
		return err
	}
	// Chat history lives in the config folder now — the repo layer
	// doesn't know about it, so we clean it up here alongside the card.
	_ = config.DeleteChatFor(a.repo.Manifest.ID, id)
	if a.idx != nil {
		a.logIdxErr("RemoveCard", a.idx.RemoveCard(id))
	}
	a.logActivityWithContext(id, model.ActivityDeleted, "", cardTitle, breadcrumbs)
	return nil
}

// --- Card mutations ---

// UpdateCardTitle updates a card's title.
func (a *App) UpdateCardTitle(id, title string) (*model.Card, error) {
	title = repo.SanitizeText(title)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCard(id, func(c *model.Card) {
		c.Title = title
	})
	if err == nil {
		if a.idx != nil {
			a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
		}
		a.logActivity(id, model.ActivityUpdatedTitle, "title")
	}
	return card, err
}

// UpdateCardType sets the type on a card (e.g. "task", "feature", or "" for none).
// For types that have a schema or template, the corresponding blocks are applied
// to the card (merging existing values by key).
func (a *App) UpdateCardType(id, cardType string) (*model.Card, error) {
	cardType = repo.SanitizeText(cardType)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCard(id, func(c *model.Card) {
		c.Type = cardType
	})
	if err != nil {
		return nil, err
	}
	if a.idx != nil {
		a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
	}
	a.logActivity(id, model.ActivityUpdatedType, cardType)
	if cardType != "" {
		a.applyTypeBlocks(id, cardType)
	}
	// Return the updated card with blocks applied
	updated, readErr := a.repo.GetCard(id)
	if readErr != nil {
		return card, nil
	}
	return updated, nil
}

// UpdateCardFields sets the type-specific fields on a card.
func (a *App) UpdateCardFields(id string, fields map[string]any) (*model.Card, error) {
	for k, v := range fields {
		if s, ok := v.(string); ok {
			fields[k] = repo.SanitizeText(s)
		}
	}
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCard(id, func(c *model.Card) {
		c.Fields = fields
	})
	if err == nil && a.idx != nil {
		a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
	}
	return card, err
}

// UpdateCardBlocks replaces a card's ordered content blocks.
// Also syncs legacy Fields/Checklist for backward compatibility.
func (a *App) UpdateCardBlocks(id string, blocks []model.Block) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCardBlocks(id, blocks)
	if err == nil {
		if a.idx != nil {
			a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
		}
		a.logActivity(id, model.ActivityUpdatedField, "content")
	}
	return card, err
}

// AddCardAttachment adds a file attachment to a card. data is base64-encoded.
func (a *App) AddCardAttachment(cardID, name, data string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.AddCardAttachment(cardID, name, data)
	if err == nil && a.idx != nil {
		a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
	}
	return card, err
}

// RemoveCardAttachment removes a file attachment from a card.
func (a *App) RemoveCardAttachment(cardID, attachmentID string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.RemoveCardAttachment(cardID, attachmentID)
	if err == nil && a.idx != nil {
		a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
	}
	return card, err
}

// UpdateCardTags replaces a card's tags.
func (a *App) UpdateCardTags(id string, tags []string) (*model.Card, error) {
	for i, t := range tags {
		tags[i] = repo.SanitizeText(t)
	}
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCard(id, func(c *model.Card) {
		c.Tags = tags
	})
	if err == nil {
		if a.idx != nil {
			a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
		}
		a.syncTagsToAllPinnedProjects(id)
		a.logActivity(id, model.ActivityUpdatedTags, "tags")
	}
	return card, err
}

// syncTagsToAllPinnedProjects ensures all tags on a card exist in every project it's pinned to.
func (a *App) syncTagsToAllPinnedProjects(cardID string) {
	card, err := a.repo.GetCard(cardID)
	if err != nil || len(card.Tags) == 0 {
		return
	}
	pins, err := a.repo.GetCardPins(cardID)
	if err != nil || len(pins) == 0 {
		return
	}
	// Sync to each unique project the card is pinned to
	seen := make(map[string]bool)
	for _, pin := range pins {
		if seen[pin.CategoryID] {
			continue
		}
		seen[pin.CategoryID] = true
		a.syncCardTagsToProject(cardID, pin.CategoryID)
	}
}

// UpdateCardDueDate sets or clears a card's due date (ISO 8601 string, or empty to clear).
func (a *App) UpdateCardDueDate(id, dueDate string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCard(id, func(c *model.Card) {
		if dueDate == "" {
			c.DueDate = nil
		} else {
			t, err := time.Parse(time.RFC3339, dueDate)
			if err != nil {
				t, _ = time.Parse("2006-01-02", dueDate)
			}
			c.DueDate = &t
		}
	})
	if err == nil {
		if a.idx != nil {
			a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
		}
		a.logActivity(id, model.ActivityUpdatedDate, "due date")
	}
	return card, err
}

// --- Checklist ---

// AddChecklistItem adds a checklist item to a card.
func (a *App) AddChecklistItem(cardID, text string) (*model.Card, error) {
	text = repo.SanitizeText(text)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.AddChecklistItem(cardID, text)
}

// ToggleChecklistItem toggles a checklist item's done state.
func (a *App) ToggleChecklistItem(cardID, itemID string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.ToggleChecklistItem(cardID, itemID)
}

// RemoveChecklistItem removes a checklist item from a card.
func (a *App) RemoveChecklistItem(cardID, itemID string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.RemoveChecklistItem(cardID, itemID)
}

// --- Category helpers ---

// getCategoryByID resolves a category UUID to its model and hierarchy slugs
// by scanning all brands > streams > projects > categories.
func (a *App) getCategoryByID(categoryID string) (*model.Category, string, string, string, error) {
	brands, _ := a.repo.ListBrands()
	for _, b := range brands {
		streams, _ := a.repo.ListStreams(b.Slug)
		for _, s := range streams {
			projects, _ := a.repo.ListProjects(b.Slug, s.Slug)
			for _, p := range projects {
				cats, _ := a.repo.ListCategories(b.Slug, s.Slug, p.Slug)
				for _, c := range cats {
					if c.ID == categoryID {
						return &c, b.Slug, s.Slug, p.Slug, nil
					}
				}
			}
		}
	}
	return nil, "", "", "", fmt.Errorf("category %q not found", categoryID)
}

// GetCategoryAcceptedTypes returns the accepted card types for a category by its ID.
// Returns nil (all types accepted) if the category has no restrictions.
func (a *App) GetCategoryAcceptedTypes(categoryID string) ([]string, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cat, _, _, _, err := a.getCategoryByID(categoryID)
	if err != nil {
		return nil, err
	}
	return cat.AcceptedTypes, nil
}

// validateCardTypeForCategory checks that a card's type is accepted by the target category.
// Returns nil if the type is accepted or the category has no restrictions.
func (a *App) validateCardTypeForCategory(cardID, categoryID string) error {
	card, err := a.repo.GetCard(cardID)
	if err != nil {
		return err
	}
	cat, _, _, _, err := a.getCategoryByID(categoryID)
	if err != nil {
		return err
	}
	if !repo.CategoryAcceptsType(cat, card.Type) {
		return fmt.Errorf("category %q does not accept card type %q", cat.Name, card.Type)
	}
	return nil
}

// --- Pin / unpin ---

func (a *App) PinCard(cardID, projectID, categoryID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	if err := a.validateCardTypeForCategory(cardID, categoryID); err != nil {
		return err
	}
	if err := a.repo.PinCard(cardID, projectID, categoryID); err != nil {
		return err
	}
	if a.idx != nil {
		pins, err := a.repo.GetCardPins(cardID)
		if err == nil {
			a.logIdxErr("IndexPins", a.idx.IndexPins(cardID, pins))
		}
	}
	// Sync card tags to the target project's tag definitions
	a.syncCardTagsToProject(cardID, categoryID)
	a.logActivity(cardID, model.ActivityPinned, "")
	return nil
}

// syncCardTagsToProject ensures all tags on a card exist in the target project's tag definitions.
func (a *App) syncCardTagsToProject(cardID, categoryID string) {
	card, err := a.repo.GetCard(cardID)
	if err != nil || len(card.Tags) == 0 {
		return
	}
	_, brandSlug, streamSlug, projectSlug, err := a.getCategoryByID(categoryID)
	if err != nil {
		return
	}
	labels, _ := a.repo.GetProjectLabels(brandSlug, streamSlug, projectSlug)
	existing := make(map[string]bool, len(labels))
	for _, l := range labels {
		existing[strings.ToLower(l.Name)] = true
	}
	for _, tag := range card.Tags {
		if !existing[strings.ToLower(tag)] {
			// AddProjectLabel now syncs with tags.json automatically.
			a.repo.AddProjectLabel(brandSlug, streamSlug, projectSlug, tag, "")
		}
	}
}

func (a *App) UnpinCard(cardID, projectID, categoryID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	// Log before unpin so the path is still resolvable
	a.logActivity(cardID, model.ActivityUnpinned, "")
	if err := a.repo.UnpinCard(cardID, projectID, categoryID); err != nil {
		return err
	}
	if a.idx != nil {
		pins, err := a.repo.GetCardPins(cardID)
		if err == nil {
			a.logIdxErr("IndexPins", a.idx.IndexPins(cardID, pins))
		}
	}
	return nil
}

func (a *App) GetCardPins(cardID string) ([]model.Pin, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.GetCardPins(cardID)
}

// --- Location lookups ---

// CardLocation describes where a card lives in the brand/stream/project hierarchy.
type CardLocation struct {
	BrandSlug   string `json:"brandSlug"`
	StreamSlug  string `json:"streamSlug"`
	ProjectSlug string `json:"projectSlug"`
}

// GetCardLocation resolves a card's first pin to the brand/stream/project slugs
// so the frontend can navigate to the correct board before opening the card.
func (a *App) GetCardLocation(cardID string) (*CardLocation, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	pins, err := a.repo.GetCardPins(cardID)
	if err != nil || len(pins) == 0 {
		return nil, fmt.Errorf("card %q has no pins", cardID)
	}
	targetCatID := pins[0].CategoryID

	brands, _ := a.repo.ListBrands()
	for _, b := range brands {
		streams, _ := a.repo.ListStreams(b.Slug)
		for _, s := range streams {
			projects, _ := a.repo.ListProjects(b.Slug, s.Slug)
			for _, p := range projects {
				cats, _ := a.repo.ListCategories(b.Slug, s.Slug, p.Slug)
				for _, c := range cats {
					if c.ID == targetCatID {
						return &CardLocation{
							BrandSlug:   b.Slug,
							StreamSlug:  s.Slug,
							ProjectSlug: p.Slug,
						}, nil
					}
				}
			}
		}
	}
	return nil, fmt.Errorf("could not resolve location for card %q", cardID)
}

// GetProjectLocation resolves a project UUID to its brand/stream/project slugs
// so the frontend can navigate to the correct board.
func (a *App) GetProjectLocation(projectID string) (*CardLocation, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	brands, _ := a.repo.ListBrands()
	for _, b := range brands {
		streams, _ := a.repo.ListStreams(b.Slug)
		for _, s := range streams {
			projects, _ := a.repo.ListProjects(b.Slug, s.Slug)
			for _, p := range projects {
				if p.ID == projectID {
					return &CardLocation{
						BrandSlug:   b.Slug,
						StreamSlug:  s.Slug,
						ProjectSlug: p.Slug,
					}, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("could not resolve location for project %q", projectID)
}

// CategoryPath describes a category's full position in the Brand > Stream > Project > Category
// hierarchy. Used by the frontend PinPicker to display breadcrumb results.
type CategoryPath struct {
	BrandSlug           string `json:"brandSlug"`
	StreamSlug          string `json:"streamSlug"`
	ProjectSlug         string `json:"projectSlug"`
	CategorySlug        string `json:"categorySlug"`
	BrandName           string `json:"brandName"`
	StreamName          string `json:"streamName"`
	ProjectName         string `json:"projectName"`
	CategoryName        string `json:"categoryName"`
	BrandDescription    string `json:"brandDescription,omitempty"`
	StreamDescription   string `json:"streamDescription,omitempty"`
	ProjectDescription  string `json:"projectDescription,omitempty"`
	CategoryDescription string `json:"categoryDescription,omitempty"`
	ProjectID           string   `json:"projectId"`
	CategoryID          string   `json:"categoryId"`
	Breadcrumb          string   `json:"breadcrumb"`                 // e.g. "Mandela Daze / YouTube / Narratively Speaking / Episodes"
	AcceptedTypes       []string `json:"acceptedTypes,omitempty"`    // which card types this category accepts; nil/empty = all
	PinnedProjectID     string   `json:"pinnedProjectId,omitempty"` // actual stored pin.ProjectID — set only by GetCardPinBreadcrumbs, used for UnpinCard
}

// ListAllCategories returns every category across the entire hierarchy with full breadcrumb info.
// Used by PinPicker to populate the flat searchable list of pin targets.
//
// Delegates to repo.ListAllCategoriesFlat so every call site that
// needs "every category with parent chain" shares a single walk —
// previously healTagColors and this method duplicated the nested
// iteration, doubling the filesystem traffic on every startup.
func (a *App) ListAllCategories() ([]CategoryPath, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	flat, err := a.repo.ListAllCategoriesFlat()
	if err != nil {
		return nil, err
	}
	results := make([]CategoryPath, 0, len(flat))
	for _, f := range flat {
		results = append(results, CategoryPath{
			BrandSlug:           f.Brand.Slug,
			StreamSlug:          f.Stream.Slug,
			ProjectSlug:         f.Project.Slug,
			CategorySlug:        f.Category.Slug,
			BrandName:           f.Brand.Name,
			StreamName:          f.Stream.Name,
			ProjectName:         f.Project.Name,
			CategoryName:        f.Category.Name,
			BrandDescription:    f.Brand.Description,
			StreamDescription:   f.Stream.Description,
			ProjectDescription:  f.Project.Description,
			CategoryDescription: f.Category.Description,
			ProjectID:           f.Project.ID,
			CategoryID:          f.Category.ID,
			Breadcrumb:          f.Brand.Name + " / " + f.Stream.Name + " / " + f.Project.Name + " / " + f.Category.Name,
			AcceptedTypes:       f.Category.AcceptedTypes,
		})
	}
	return results, nil
}

// GetCardPinBreadcrumbs returns a CategoryPath for every pin the card has,
// enriched with full hierarchy names. Used by CardDetail to display the location indicator.
func (a *App) GetCardPinBreadcrumbs(cardID string) ([]CategoryPath, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	pins, err := a.repo.GetCardPins(cardID)
	if err != nil {
		return nil, err
	}
	if len(pins) == 0 {
		return nil, nil
	}
	all, err := a.ListAllCategories()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]CategoryPath, len(all))
	for _, cp := range all {
		byID[cp.CategoryID] = cp
	}
	var result []CategoryPath
	for _, pin := range pins {
		if cp, ok := byID[pin.CategoryID]; ok {
			cp.PinnedProjectID = pin.ProjectID // carry actual stored value so frontend can unpin correctly
			result = append(result, cp)
		}
	}
	return result, nil
}

// --- Moves ---

// MoveCardInCategory reorders a card within its current category.
func (a *App) MoveCardInCategory(cardID, projectID, categoryID string, newPosition int) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	if err := a.repo.MoveCardInCategory(cardID, projectID, categoryID, newPosition); err != nil {
		return err
	}
	if a.idx != nil {
		pins, err := a.repo.GetCardPins(cardID)
		if err == nil {
			a.logIdxErr("IndexPins", a.idx.IndexPins(cardID, pins))
		}
	}
	return nil
}

// MoveCardToCategory moves a card from one category to another.
func (a *App) MoveCardToCategory(cardID, projectID, fromCategoryID, toCategoryID string, newPosition int) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	if err := a.validateCardTypeForCategory(cardID, toCategoryID); err != nil {
		return err
	}
	if err := a.repo.MoveCardToCategory(cardID, projectID, fromCategoryID, toCategoryID, newPosition); err != nil {
		return err
	}
	if a.idx != nil {
		pins, err := a.repo.GetCardPins(cardID)
		if err == nil {
			a.logIdxErr("IndexPins", a.idx.IndexPins(cardID, pins))
		}
	}
	return nil
}

// --- Comments ---

// ListCardComments returns all comments attached to a card in chronological order.
func (a *App) ListCardComments(cardID string) ([]model.Comment, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cf, err := a.repo.LoadComments(cardID)
	if err != nil {
		return nil, err
	}
	return cf.Comments, nil
}

// AddCardComment appends a new comment to a card. The author defaults to the
// current profile display name when empty.
func (a *App) AddCardComment(cardID, author, text string) (*model.Comment, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	text = strings.TrimSpace(repo.SanitizeText(text))
	if text == "" {
		return nil, fmt.Errorf("comment text cannot be empty")
	}
	if author == "" {
		if profile, err := config.LoadProfile(); err == nil && profile.DisplayName != "" {
			author = profile.DisplayName
		} else {
			author = "You"
		}
	}
	comment, err := a.repo.AddCardComment(cardID, author, text, time.Time{})
	if err != nil {
		return nil, err
	}
	a.logActivity(cardID, "commented", "")
	return comment, nil
}

// UpdateCardComment edits an existing comment's text.
func (a *App) UpdateCardComment(cardID, commentID, text string) (*model.Comment, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	text = strings.TrimSpace(repo.SanitizeText(text))
	if text == "" {
		return nil, fmt.Errorf("comment text cannot be empty")
	}
	return a.repo.UpdateCardComment(cardID, commentID, text)
}

// DeleteCardComment removes a comment by ID.
func (a *App) DeleteCardComment(cardID, commentID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	return a.repo.DeleteCardComment(cardID, commentID)
}
