// Package card is the CardService — the operational core of BRUV.
// CRUD, mutations, pins, moves, comments, checklist items, and the
// category-hierarchy lookups used by card navigation.
//
// Activity logging (who edited what, when) is handled via a callback
// on Deps because actor resolution (user vs LLM-agent) lives on the
// host — extracted cleanly when an activity service lands later.
//
// Template merging (applyTypeBlocks) delegates to the catalog service
// via Deps; the card service doesn't need to know schema specifics.
package card

import (
	"bruv/internal/config"
	"bruv/internal/index"
	"bruv/internal/model"
	"bruv/internal/repo"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Deps is the narrow host contract for CardService.
type Deps interface {
	Repo() *repo.Repository
	Index() *index.Index

	// ApplyTypeBlocks merges a type's template blocks into a card.
	// Implemented on the host via catalog.Service.ApplyTypeBlocks.
	ApplyTypeBlocks(cardID, cardType string)

	// LogActivity records a card mutation in the activity feed.
	// Implemented on the host (resolves user vs LLM actor context).
	LogActivity(cardID, action, field string)
	// LogActivityWithContext logs when the card's own state is
	// unreliable (e.g. after a delete) — callers pass a frozen
	// snapshot of title + breadcrumbs.
	LogActivityWithContext(cardID, action, field, cardTitle string, breadcrumbs []CategoryPath)

	// Publish announces a domain event. Every card mutation
	// publishes card:updated (with the full Card payload) so the
	// same-user-on-multiple-devices story works without polling.
	Publish(topic string, payload any)
}

// emitCardUpdated publishes a card:updated event carrying the full
// post-mutation card. Full-payload policy is load-bearing for the
// "edit on phone, see on laptop" case and future sync-based collab
// — clients merge the payload into local state rather than refetching.
func (s *Service) emitCardUpdated(card *model.Card) {
	if card == nil {
		return
	}
	s.deps.Publish("card:updated", map[string]any{
		"cardID": card.ID,
		"card":   card,
	})
}

// Service exposes card CRUD, mutations, pins, moves, and comments.
type Service struct{ deps Deps }

// New constructs a CardService.
func New(deps Deps) *Service { return &Service{deps: deps} }

// --- Types (aliased in main for stable Wails TS bindings) ---

// CardLocation describes where a card lives in brand/stream/project.
type CardLocation struct {
	BrandSlug   string `json:"brandSlug"`
	StreamSlug  string `json:"streamSlug"`
	ProjectSlug string `json:"projectSlug"`
}

// CategoryPath describes a category's full hierarchy position.
// Used by PinPicker and everywhere that needs to render a card's
// location with breadcrumbs.
type CategoryPath struct {
	BrandSlug           string   `json:"brandSlug"`
	StreamSlug          string   `json:"streamSlug"`
	ProjectSlug         string   `json:"projectSlug"`
	CategorySlug        string   `json:"categorySlug"`
	BrandName           string   `json:"brandName"`
	StreamName          string   `json:"streamName"`
	ProjectName         string   `json:"projectName"`
	CategoryName        string   `json:"categoryName"`
	BrandDescription    string   `json:"brandDescription,omitempty"`
	StreamDescription   string   `json:"streamDescription,omitempty"`
	ProjectDescription  string   `json:"projectDescription,omitempty"`
	CategoryDescription string   `json:"categoryDescription,omitempty"`
	ProjectID           string   `json:"projectId"`
	CategoryID          string   `json:"categoryId"`
	Breadcrumb          string   `json:"breadcrumb"`
	AcceptedTypes       []string `json:"acceptedTypes,omitempty"`
	PinnedProjectID     string   `json:"pinnedProjectId,omitempty"`
}

// --- Helpers ---

func (s *Service) logIdxErr(op string, err error) {
	if err == nil {
		return
	}
	slog.Warn("index update failed", "op", op, "err", err)
}

func (s *Service) idxRefresh() {
	idx, r := s.deps.Index(), s.deps.Repo()
	if idx == nil || r == nil {
		return
	}
	if _, err := idx.IncrementalRefresh(r.Root); err != nil {
		s.logIdxErr("IncrementalRefresh", err)
	}
}

// --- CRUD ---

func (s *Service) Create(cardType, title string) (*model.Card, error) {
	title = repo.SanitizeText(title)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := r.CreateCard(cardType, title)
	if err != nil {
		return nil, err
	}
	if idx := s.deps.Index(); idx != nil {
		s.logIdxErr("IndexCard", idx.IndexCard(card, time.Now(), ""))
	}
	s.deps.LogActivity(card.ID, model.ActivityCreated, "")
	if cardType != "" {
		s.deps.ApplyTypeBlocks(card.ID, cardType)
		if updated, err := r.GetCard(card.ID); err == nil {
			card = updated
		}
	}
	s.deps.Publish("card:created", map[string]any{"cardID": card.ID, "card": card})
	return card, nil
}

func (s *Service) Get(id string) (*model.Card, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.GetCard(id)
}

func (s *Service) List() ([]model.Card, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.ListCards()
}

func (s *Service) Duplicate(cardID, categoryID string) (*model.Card, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	newCard, err := r.DuplicateCard(cardID)
	if err != nil {
		return nil, err
	}
	if err := r.PinCard(newCard.ID, categoryID); err != nil {
		return nil, err
	}
	s.idxRefresh()
	return newCard, nil
}

func (s *Service) CopyCategory(brandSlug, streamSlug, projectSlug, categorySlug string) (*model.Category, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	srcCats, err := r.ListCategories(brandSlug, streamSlug, projectSlug)
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
	newCat, err := r.CreateCategory(brandSlug, streamSlug, projectSlug, srcCat.Name+" Copy", len(srcCats))
	if err != nil {
		return nil, err
	}
	if idx := s.deps.Index(); idx != nil {
		cardIDs, err := idx.ListCardIDsInCategory(srcCat.ID)
		if err == nil {
			for i, cardID := range cardIDs {
				newCard, err := r.DuplicateCard(cardID)
				if err != nil {
					continue
				}
				_ = r.PinCard(newCard.ID, newCat.ID)
				_ = r.MoveCardInCategory(newCard.ID, newCat.ID, i)
			}
		}
		s.idxRefresh()
	}
	return newCat, nil
}

func (s *Service) Delete(id string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	cardTitle := ""
	if card, err := r.GetCard(id); err == nil {
		cardTitle = card.Title
	}
	breadcrumbs, _ := s.GetPinBreadcrumbs(id)
	if err := r.DeleteCard(id); err != nil {
		return err
	}
	if err := config.DeleteChatFor(r.Manifest.ID, id); err != nil {
		// Non-fatal — the card is gone either way — but silently
		// swallowing it leaves orphaned chat files accumulating.
		slog.Warn("delete chat for removed card failed", "cardID", id, "err", err)
	}
	if idx := s.deps.Index(); idx != nil {
		s.logIdxErr("RemoveCard", idx.RemoveCard(id))
	}
	s.deps.LogActivityWithContext(id, model.ActivityDeleted, "", cardTitle, breadcrumbs)
	s.deps.Publish("card:deleted", map[string]any{"cardID": id})
	return nil
}

// --- Mutations ---

func (s *Service) UpdateTitle(id, title string) (*model.Card, error) {
	title = repo.SanitizeText(title)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := r.UpdateCard(id, func(c *model.Card) { c.Title = title })
	if err == nil {
		if idx := s.deps.Index(); idx != nil {
			s.logIdxErr("IndexCard", idx.IndexCard(card, time.Now(), idx.GetCardProjectContext(card.ID)))
		}
		s.deps.LogActivity(id, model.ActivityUpdatedTitle, "title")
		s.emitCardUpdated(card)
	}
	return card, err
}

func (s *Service) UpdateType(id, cardType string) (*model.Card, error) {
	cardType = repo.SanitizeText(cardType)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := r.UpdateCard(id, func(c *model.Card) { c.Type = cardType })
	if err != nil {
		return nil, err
	}
	if idx := s.deps.Index(); idx != nil {
		s.logIdxErr("IndexCard", idx.IndexCard(card, time.Now(), idx.GetCardProjectContext(card.ID)))
	}
	s.deps.LogActivity(id, model.ActivityUpdatedType, cardType)
	if cardType != "" {
		s.deps.ApplyTypeBlocks(id, cardType)
	}
	updated, readErr := r.GetCard(id)
	if readErr != nil {
		s.emitCardUpdated(card)
		return card, nil
	}
	s.emitCardUpdated(updated)
	return updated, nil
}

// UpdateDescription replaces the card's intrinsic description body
// (markdown-formatted free text). Sanitises before save and logs an
// activity entry only when the value actually changed.
func (s *Service) UpdateDescription(id, description string) (*model.Card, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	clean := repo.SanitizeText(description)

	var changed bool
	if prev, perr := r.GetCard(id); perr == nil && prev != nil {
		changed = prev.Description != clean
	} else {
		changed = clean != ""
	}

	card, err := r.UpdateCard(id, func(c *model.Card) { c.Description = clean })
	if err == nil {
		if idx := s.deps.Index(); idx != nil {
			s.logIdxErr("IndexCard", idx.IndexCard(card, time.Now(), idx.GetCardProjectContext(card.ID)))
		}
		if changed {
			s.deps.LogActivity(id, model.ActivityUpdatedField, "description")
		}
		s.emitCardUpdated(card)
	}
	return card, err
}

func (s *Service) UpdateBlocks(id string, blocks []model.Block) (*model.Card, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := r.UpdateCardBlocks(id, blocks)
	if err == nil {
		if idx := s.deps.Index(); idx != nil {
			s.logIdxErr("IndexCard", idx.IndexCard(card, time.Now(), idx.GetCardProjectContext(card.ID)))
		}
		s.deps.LogActivity(id, model.ActivityUpdatedField, "content")
		s.emitCardUpdated(card)
	}
	return card, err
}

func (s *Service) AddAttachment(cardID, name, data string) (*model.Card, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := r.AddCardAttachment(cardID, name, data)
	if err == nil {
		if idx := s.deps.Index(); idx != nil {
			s.logIdxErr("IndexCard", idx.IndexCard(card, time.Now(), idx.GetCardProjectContext(card.ID)))
		}
		s.emitCardUpdated(card)
	}
	return card, err
}

func (s *Service) RemoveAttachment(cardID, attachmentID string) (*model.Card, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := r.RemoveCardAttachment(cardID, attachmentID)
	if err == nil {
		if idx := s.deps.Index(); idx != nil {
			s.logIdxErr("IndexCard", idx.IndexCard(card, time.Now(), idx.GetCardProjectContext(card.ID)))
		}
		s.emitCardUpdated(card)
	}
	return card, err
}

func (s *Service) UpdateTags(id string, tags []string) (*model.Card, error) {
	// Sanitize into a copy — mutating the caller's slice in place is a
	// latent aliasing bug (the dispatcher may reuse the decoded args).
	clean := make([]string, len(tags))
	for i, t := range tags {
		clean[i] = repo.SanitizeText(t)
	}
	tags = clean
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := r.UpdateCard(id, func(c *model.Card) { c.Tags = tags })
	if err == nil {
		if idx := s.deps.Index(); idx != nil {
			s.logIdxErr("IndexCard", idx.IndexCard(card, time.Now(), idx.GetCardProjectContext(card.ID)))
		}
		s.syncTagsToAllPinnedProjects(id)
		s.deps.LogActivity(id, model.ActivityUpdatedTags, "tags")
		s.emitCardUpdated(card)
	}
	return card, err
}

// SetFolder binds (or, with nil, unbinds) the card's workspace folder.
// Pure binding — never creates, moves, or deletes files on disk.
func (s *Service) SetFolder(id string, folder *model.CardFolder) (*model.Card, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := r.UpdateCard(id, func(c *model.Card) {
		c.Folder = folder
	})
	if err == nil {
		s.deps.LogActivity(id, model.ActivityUpdatedField, "folder")
		s.emitCardUpdated(card)
	}
	return card, err
}

func (s *Service) UpdateDueDate(id, dueDate string) (*model.Card, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := r.UpdateCard(id, func(c *model.Card) {
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
		if idx := s.deps.Index(); idx != nil {
			s.logIdxErr("IndexCard", idx.IndexCard(card, time.Now(), idx.GetCardProjectContext(card.ID)))
		}
		s.deps.LogActivity(id, model.ActivityUpdatedDate, "due date")
		s.emitCardUpdated(card)
	}
	return card, err
}

// --- Category helpers (internal) ---

func (s *Service) getCategoryByID(categoryID string) (*model.Category, string, string, string, error) {
	r := s.deps.Repo()
	brands, _ := r.ListBrands()
	for _, b := range brands {
		streams, _ := r.ListStreams(b.Slug)
		for _, st := range streams {
			projects, _ := r.ListProjects(b.Slug, st.Slug)
			for _, p := range projects {
				cats, _ := r.ListCategories(b.Slug, st.Slug, p.Slug)
				for _, c := range cats {
					if c.ID == categoryID {
						return &c, b.Slug, st.Slug, p.Slug, nil
					}
				}
			}
		}
	}
	return nil, "", "", "", fmt.Errorf("category %q not found", categoryID)
}

func (s *Service) GetCategoryAcceptedTypes(categoryID string) ([]string, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cat, _, _, _, err := s.getCategoryByID(categoryID)
	if err != nil {
		return nil, err
	}
	return cat.AcceptedTypes, nil
}

func (s *Service) validateCardTypeForCategory(cardID, categoryID string) error {
	r := s.deps.Repo()
	card, err := r.GetCard(cardID)
	if err != nil {
		return err
	}
	cat, _, _, _, err := s.getCategoryByID(categoryID)
	if err != nil {
		return err
	}
	if !repo.CategoryAcceptsType(cat, card.Type) {
		return fmt.Errorf("category %q does not accept card type %q", cat.Name, card.Type)
	}
	return nil
}

// --- Pin / unpin ---

func (s *Service) Pin(cardID, categoryID string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	if err := s.validateCardTypeForCategory(cardID, categoryID); err != nil {
		return err
	}
	if err := r.PinCard(cardID, categoryID); err != nil {
		return err
	}
	if idx := s.deps.Index(); idx != nil {
		pins, err := r.GetCardPins(cardID)
		if err == nil {
			s.logIdxErr("IndexPins", idx.IndexPins(cardID, pins))
		}
	}
	s.syncCardTagsToProject(cardID, categoryID)
	s.deps.LogActivity(cardID, model.ActivityPinned, "")
	// Card fields themselves are unchanged, but the "Pinned in" rail on
	// open card views needs to redraw. Reusing card:updated (rather than
	// a separate card:pinned) keeps every mutation channelled through one
	// listener — the LLM-suggestion-accept path was silent without this.
	if card, err := r.GetCard(cardID); err == nil {
		s.emitCardUpdated(card)
	}
	return nil
}

func (s *Service) Unpin(cardID, categoryID string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	s.deps.LogActivity(cardID, model.ActivityUnpinned, "")
	if err := r.UnpinCard(cardID, categoryID); err != nil {
		return err
	}
	if idx := s.deps.Index(); idx != nil {
		pins, err := r.GetCardPins(cardID)
		if err == nil {
			s.logIdxErr("IndexPins", idx.IndexPins(cardID, pins))
		}
	}
	if card, err := r.GetCard(cardID); err == nil {
		s.emitCardUpdated(card)
	}
	return nil
}

func (s *Service) GetPins(cardID string) ([]model.Pin, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.GetCardPins(cardID)
}

func (s *Service) syncTagsToAllPinnedProjects(cardID string) {
	r := s.deps.Repo()
	card, err := r.GetCard(cardID)
	if err != nil || len(card.Tags) == 0 {
		return
	}
	pins, err := r.GetCardPins(cardID)
	if err != nil || len(pins) == 0 {
		return
	}
	seen := make(map[string]bool)
	for _, pin := range pins {
		if seen[pin.CategoryID] {
			continue
		}
		seen[pin.CategoryID] = true
		s.syncCardTagsToProject(cardID, pin.CategoryID)
	}
}

func (s *Service) syncCardTagsToProject(cardID, categoryID string) {
	r := s.deps.Repo()
	card, err := r.GetCard(cardID)
	if err != nil || len(card.Tags) == 0 {
		return
	}
	_, brandSlug, streamSlug, projectSlug, err := s.getCategoryByID(categoryID)
	if err != nil {
		return
	}
	labels, _ := r.GetProjectLabels(brandSlug, streamSlug, projectSlug)
	existing := make(map[string]bool, len(labels))
	for _, l := range labels {
		existing[strings.ToLower(l.Name)] = true
	}
	for _, tag := range card.Tags {
		if !existing[strings.ToLower(tag)] {
			r.AddProjectLabel(brandSlug, streamSlug, projectSlug, tag, "")
		}
	}
}

// --- Location lookups ---

func (s *Service) GetLocation(cardID string) (*CardLocation, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	pins, err := r.GetCardPins(cardID)
	if err != nil || len(pins) == 0 {
		return nil, fmt.Errorf("card %q has no pins", cardID)
	}
	targetCatID := pins[0].CategoryID

	brands, _ := r.ListBrands()
	for _, b := range brands {
		streams, _ := r.ListStreams(b.Slug)
		for _, st := range streams {
			projects, _ := r.ListProjects(b.Slug, st.Slug)
			for _, p := range projects {
				cats, _ := r.ListCategories(b.Slug, st.Slug, p.Slug)
				for _, c := range cats {
					if c.ID == targetCatID {
						return &CardLocation{BrandSlug: b.Slug, StreamSlug: st.Slug, ProjectSlug: p.Slug}, nil
					}
				}
			}
		}
	}
	return nil, fmt.Errorf("could not resolve location for card %q", cardID)
}

func (s *Service) GetProjectLocation(projectID string) (*CardLocation, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	brands, _ := r.ListBrands()
	for _, b := range brands {
		streams, _ := r.ListStreams(b.Slug)
		for _, st := range streams {
			projects, _ := r.ListProjects(b.Slug, st.Slug)
			for _, p := range projects {
				if p.ID == projectID {
					return &CardLocation{BrandSlug: b.Slug, StreamSlug: st.Slug, ProjectSlug: p.Slug}, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("could not resolve location for project %q", projectID)
}

func (s *Service) ListAllCategories() ([]CategoryPath, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	flat, err := r.ListAllCategoriesFlat()
	if err != nil {
		return nil, err
	}
	results := make([]CategoryPath, 0, len(flat))
	for _, f := range flat {
		results = append(results, CategoryPath{
			BrandSlug: f.Brand.Slug, StreamSlug: f.Stream.Slug,
			ProjectSlug: f.Project.Slug, CategorySlug: f.Category.Slug,
			BrandName: f.Brand.Name, StreamName: f.Stream.Name,
			ProjectName: f.Project.Name, CategoryName: f.Category.Name,
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

func (s *Service) GetPinBreadcrumbs(cardID string) ([]CategoryPath, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	pins, err := r.GetCardPins(cardID)
	if err != nil {
		return nil, err
	}
	if len(pins) == 0 {
		return nil, nil
	}
	all, err := s.ListAllCategories()
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
			cp.PinnedProjectID = pin.ProjectID
			result = append(result, cp)
		}
	}
	return result, nil
}

// --- Moves ---

func (s *Service) MoveInCategory(cardID, categoryID string, newPosition int) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	if err := r.MoveCardInCategory(cardID, categoryID, newPosition); err != nil {
		return err
	}
	if idx := s.deps.Index(); idx != nil {
		pins, err := r.GetCardPins(cardID)
		if err == nil {
			s.logIdxErr("IndexPins", idx.IndexPins(cardID, pins))
		}
	}
	return nil
}

func (s *Service) MoveToCategory(cardID, fromCategoryID, toCategoryID string, newPosition int) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	if err := s.validateCardTypeForCategory(cardID, toCategoryID); err != nil {
		return err
	}
	if err := r.MoveCardToCategory(cardID, fromCategoryID, toCategoryID, newPosition); err != nil {
		return err
	}
	if idx := s.deps.Index(); idx != nil {
		pins, err := r.GetCardPins(cardID)
		if err == nil {
			s.logIdxErr("IndexPins", idx.IndexPins(cardID, pins))
		}
	}
	return nil
}

// --- Comments ---

func (s *Service) ListComments(cardID string) ([]model.Comment, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cf, err := r.LoadComments(cardID)
	if err != nil {
		return nil, err
	}
	return cf.Comments, nil
}

func (s *Service) AddComment(cardID, author, text string) (*model.Comment, error) {
	r := s.deps.Repo()
	if r == nil {
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
	comment, err := r.AddCardComment(cardID, author, text, time.Time{})
	if err != nil {
		return nil, err
	}
	s.deps.LogActivity(cardID, "commented", "")
	return comment, nil
}

func (s *Service) UpdateComment(cardID, commentID, text string) (*model.Comment, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	text = strings.TrimSpace(repo.SanitizeText(text))
	if text == "" {
		return nil, fmt.Errorf("comment text cannot be empty")
	}
	return r.UpdateCardComment(cardID, commentID, text)
}

func (s *Service) DeleteComment(cardID, commentID string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	return r.DeleteCardComment(cardID, commentID)
}
