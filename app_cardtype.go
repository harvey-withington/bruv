package main

// Wails-bound forwarders for card types + templates + import/export.
// Domain logic lives in core/services/catalog.

import (
	"bruv/core/services/catalog"
	"bruv/internal/config"
	"bruv/internal/model"
)

// Exported type aliases preserve stable Wails TS bindings.
type CardTypeInfo = catalog.CardTypeInfo
type CardTypesExport = catalog.CardTypesExport
type CardTypesImportResult = catalog.CardTypesImportResult

// ListCardTypes returns all card types (built-in first, then user).
func (a *App) ListCardTypes() []CardTypeInfo { return a.catalog.ListCardTypes() }

func (a *App) ValidateCardFields(cardType string, fields map[string]any) []string {
	return a.catalog.ValidateCardFields(cardType, fields)
}

func (a *App) CreateUserCardType(label, color, description, aiHint, templateID string) (config.UserCardType, error) {
	return a.catalog.CreateUserCardType(label, color, description, aiHint, templateID)
}

func (a *App) UpdateUserCardType(id, label, color, description, aiHint, templateID string) (config.UserCardType, error) {
	return a.catalog.UpdateUserCardType(id, label, color, description, aiHint, templateID)
}

func (a *App) UpdateUserCardTypeIcon(id, icon string) (config.UserCardType, error) {
	return a.catalog.UpdateUserCardTypeIcon(id, icon)
}

func (a *App) DeleteUserCardType(id string) error { return a.catalog.DeleteUserCardType(id) }

func (a *App) UpdateBuiltinCardType(id, color, templateID string) error {
	return a.catalog.UpdateBuiltinCardType(id, color, templateID)
}

func (a *App) ListCardTemplates() ([]config.CardTemplate, error) {
	return a.catalog.ListCardTemplates()
}

func (a *App) CreateCardTemplate(name string, blocks []model.Block) (config.CardTemplate, error) {
	return a.catalog.CreateCardTemplate(name, blocks)
}

func (a *App) UpdateCardTemplate(id, name string, blocks []model.Block) (config.CardTemplate, error) {
	return a.catalog.UpdateCardTemplate(id, name, blocks)
}

func (a *App) DeleteCardTemplate(id string) error { return a.catalog.DeleteCardTemplate(id) }

func (a *App) RefreshTypeBlocks(cardID string) (*model.Card, error) {
	return a.catalog.RefreshTypeBlocks(cardID)
}

func (a *App) ExportCardTypesToFile(filePath string) error {
	return a.catalog.ExportCardTypesToFile(filePath)
}

func (a *App) ImportCardTypesFromFile(filePath, mode string) (CardTypesImportResult, error) {
	return a.catalog.ImportCardTypesFromFile(filePath, mode)
}

func (a *App) ImportCardTypesFromRepo(otherRepoPath, mode string) (CardTypesImportResult, error) {
	return a.catalog.ImportCardTypesFromRepo(otherRepoPath, mode)
}

// Internal helpers — App forwarders so app_card.go's creation flow
// doesn't need to import the catalog package directly. When the card
// service is extracted these go away.
func (a *App) applyTypeBlocks(cardID, cardType string) {
	a.catalog.ApplyTypeBlocks(cardID, cardType)
}

func (a *App) resolveTemplateBlocks(cardType string) []model.Block {
	return a.catalog.ResolveTemplateBlocks(cardType)
}
