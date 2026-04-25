package main

// Wails-bound forwarders for card CRUD, mutations, pins, moves,
// checklist, and comments. Domain logic lives in core/services/card.

import (
	"bruv/core/services/card"
	"bruv/internal/model"
)

// Type aliases preserve stable Wails TS bindings.
type CardLocation = card.CardLocation
type CategoryPath = card.CategoryPath

// --- CRUD ---

func (a *App) CreateCard(cardType, title string) (*model.Card, error) {
	return a.cardService.Create(cardType, title)
}
func (a *App) GetCard(id string) (*model.Card, error) { return a.cardService.Get(id) }
func (a *App) ListCards() ([]model.Card, error)       { return a.cardService.List() }

func (a *App) DuplicateCard(cardID, categoryID string) (*model.Card, error) {
	return a.cardService.Duplicate(cardID, categoryID)
}
func (a *App) CopyCategory(brandSlug, streamSlug, projectSlug, categorySlug string) (*model.Category, error) {
	return a.cardService.CopyCategory(brandSlug, streamSlug, projectSlug, categorySlug)
}
func (a *App) DeleteCard(id string) error { return a.cardService.Delete(id) }

// --- Mutations ---

func (a *App) UpdateCardTitle(id, title string) (*model.Card, error) {
	return a.cardService.UpdateTitle(id, title)
}
func (a *App) UpdateCardType(id, cardType string) (*model.Card, error) {
	return a.cardService.UpdateType(id, cardType)
}
func (a *App) UpdateCardFields(id string, fields map[string]any) (*model.Card, error) {
	return a.cardService.UpdateFields(id, fields)
}
func (a *App) UpdateCardBlocks(id string, blocks []model.Block) (*model.Card, error) {
	return a.cardService.UpdateBlocks(id, blocks)
}
func (a *App) AddCardAttachment(cardID, name, data string) (*model.Card, error) {
	return a.cardService.AddAttachment(cardID, name, data)
}
func (a *App) RemoveCardAttachment(cardID, attachmentID string) (*model.Card, error) {
	return a.cardService.RemoveAttachment(cardID, attachmentID)
}
func (a *App) UpdateCardTags(id string, tags []string) (*model.Card, error) {
	return a.cardService.UpdateTags(id, tags)
}
func (a *App) UpdateCardDueDate(id, dueDate string) (*model.Card, error) {
	return a.cardService.UpdateDueDate(id, dueDate)
}

// --- Checklist ---

func (a *App) AddChecklistItem(cardID, text string) (*model.Card, error) {
	return a.cardService.AddChecklistItem(cardID, text)
}
func (a *App) ToggleChecklistItem(cardID, itemID string) (*model.Card, error) {
	return a.cardService.ToggleChecklistItem(cardID, itemID)
}
func (a *App) RemoveChecklistItem(cardID, itemID string) (*model.Card, error) {
	return a.cardService.RemoveChecklistItem(cardID, itemID)
}

// --- Category helpers / pins / moves ---

func (a *App) GetCategoryAcceptedTypes(categoryID string) ([]string, error) {
	return a.cardService.GetCategoryAcceptedTypes(categoryID)
}
func (a *App) PinCard(cardID, projectID, categoryID string) error {
	return a.cardService.Pin(cardID, projectID, categoryID)
}
func (a *App) UnpinCard(cardID, projectID, categoryID string) error {
	return a.cardService.Unpin(cardID, projectID, categoryID)
}
func (a *App) GetCardPins(cardID string) ([]model.Pin, error) {
	return a.cardService.GetPins(cardID)
}
func (a *App) GetCardLocation(cardID string) (*CardLocation, error) {
	return a.cardService.GetLocation(cardID)
}
func (a *App) GetProjectLocation(projectID string) (*CardLocation, error) {
	return a.cardService.GetProjectLocation(projectID)
}
func (a *App) ListAllCategories() ([]CategoryPath, error) {
	return a.cardService.ListAllCategories()
}
func (a *App) GetCardPinBreadcrumbs(cardID string) ([]CategoryPath, error) {
	return a.cardService.GetPinBreadcrumbs(cardID)
}
func (a *App) MoveCardInCategory(cardID, projectID, categoryID string, newPosition int) error {
	return a.cardService.MoveInCategory(cardID, projectID, categoryID, newPosition)
}
func (a *App) MoveCardToCategory(cardID, projectID, fromCategoryID, toCategoryID string, newPosition int) error {
	return a.cardService.MoveToCategory(cardID, projectID, fromCategoryID, toCategoryID, newPosition)
}

// --- Comments ---

func (a *App) ListCardComments(cardID string) ([]model.Comment, error) {
	return a.cardService.ListComments(cardID)
}
func (a *App) AddCardComment(cardID, author, text string) (*model.Comment, error) {
	return a.cardService.AddComment(cardID, author, text)
}
func (a *App) UpdateCardComment(cardID, commentID, text string) (*model.Comment, error) {
	return a.cardService.UpdateComment(cardID, commentID, text)
}
func (a *App) DeleteCardComment(cardID, commentID string) error {
	return a.cardService.DeleteComment(cardID, commentID)
}
