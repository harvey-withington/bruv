package main

import (
	"bruv/internal/model"
	"bruv/internal/repo"
	"fmt"
)

// Category CRUD — split from app.go. See app_brand.go for the rationale.

func (a *App) CreateCategory(brandSlug, streamSlug, projectSlug, name string, position int) (*model.Category, error) {
	name = repo.SanitizeText(name)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.CreateCategory(brandSlug, streamSlug, projectSlug, name, position)
}

func (a *App) ListCategories(brandSlug, streamSlug, projectSlug string) ([]model.Category, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.ListCategories(brandSlug, streamSlug, projectSlug)
}

func (a *App) RenameCategory(brandSlug, streamSlug, projectSlug, categorySlug, newName string) (*model.Category, error) {
	newName = repo.SanitizeText(newName)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cat, err := a.repo.RenameCategory(brandSlug, streamSlug, projectSlug, categorySlug, newName)
	if err != nil {
		return nil, err
	}
	if a.idx != nil {
		a.idxIncrementalRefresh()
	}
	return cat, nil
}

func (a *App) UpdateCategoryDescription(brandSlug, streamSlug, projectSlug, categorySlug, description string) (*model.Category, error) {
	description = repo.SanitizeText(description)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateCategoryDescription(brandSlug, streamSlug, projectSlug, categorySlug, description)
}

func (a *App) UpdateCategoryIcon(brandSlug, streamSlug, projectSlug, categorySlug, icon string) (*model.Category, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateCategoryIcon(brandSlug, streamSlug, projectSlug, categorySlug, icon)
}

func (a *App) DeleteCategory(brandSlug, streamSlug, projectSlug, categorySlug string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	// Prevent deleting the last category in a project
	cats, _ := a.repo.ListCategories(brandSlug, streamSlug, projectSlug)
	if len(cats) <= 1 {
		return fmt.Errorf("cannot delete the last category in a project")
	}

	// Unpin cards from this category so they become orphaned (inbox) instead of invisible
	cat, err := a.repo.GetCategory(brandSlug, streamSlug, projectSlug, categorySlug)
	if err == nil {
		pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
		for _, p := range pins {
			_ = a.repo.UnpinCard(p.CardID, p.ProjectID, p.CategoryID)
		}
	}

	err = a.repo.DeleteCategory(brandSlug, streamSlug, projectSlug, categorySlug)
	if err == nil && a.idx != nil {
		a.idxIncrementalRefresh()
	}
	return err
}

// UpdateCategoryAcceptedTypes sets which card types a category will accept.
// An empty or nil slice clears the restriction (all types accepted).
func (a *App) UpdateCategoryAcceptedTypes(brandSlug, streamSlug, projectSlug, categorySlug string, acceptedTypes []string) (*model.Category, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateCategory(brandSlug, streamSlug, projectSlug, categorySlug, func(c *model.Category) {
		c.AcceptedTypes = acceptedTypes
	})
}

// MoveCategoryCards moves all card pins from one category to another, then deletes the source category.
func (a *App) MoveCategoryCards(brandSlug, streamSlug, projectSlug, fromCategoryID, toCategoryID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	if a.idx == nil {
		return fmt.Errorf("no index available")
	}

	// Get all card IDs in the source category
	cardIDs, err := a.idx.ListCardIDsInCategory(fromCategoryID, fromCategoryID)
	if err != nil {
		return fmt.Errorf("list cards in category: %w", err)
	}

	// Move each card's pin to the target category
	for i, cardID := range cardIDs {
		if err := a.repo.MoveCardToCategory(cardID, fromCategoryID, fromCategoryID, toCategoryID, i); err != nil {
			return fmt.Errorf("move card %s: %w", cardID, err)
		}
		// Re-index pins
		if pins, err := a.repo.GetCardPins(cardID); err == nil {
			a.logIdxErr("IndexPins", a.idx.IndexPins(cardID, pins))
		}
	}

	return nil
}
