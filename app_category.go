package main

// Wails-bound forwarders for category CRUD. Domain logic lives in
// core/services/project.

import "bruv/internal/model"

func (a *App) CreateCategory(brandSlug, streamSlug, projectSlug, name string, position int) (*model.Category, error) {
	return a.project.CreateCategory(brandSlug, streamSlug, projectSlug, name, position)
}
func (a *App) ListCategories(brandSlug, streamSlug, projectSlug string) ([]model.Category, error) {
	return a.project.ListCategories(brandSlug, streamSlug, projectSlug)
}
func (a *App) RenameCategory(brandSlug, streamSlug, projectSlug, categorySlug, newName string) (*model.Category, error) {
	return a.project.RenameCategory(brandSlug, streamSlug, projectSlug, categorySlug, newName)
}
func (a *App) UpdateCategoryDescription(brandSlug, streamSlug, projectSlug, categorySlug, description string) (*model.Category, error) {
	return a.project.UpdateCategoryDescription(brandSlug, streamSlug, projectSlug, categorySlug, description)
}
func (a *App) UpdateCategoryIcon(brandSlug, streamSlug, projectSlug, categorySlug, icon string) (*model.Category, error) {
	return a.project.UpdateCategoryIcon(brandSlug, streamSlug, projectSlug, categorySlug, icon)
}
func (a *App) DeleteCategory(brandSlug, streamSlug, projectSlug, categorySlug string) error {
	return a.project.DeleteCategory(brandSlug, streamSlug, projectSlug, categorySlug)
}
func (a *App) UpdateCategoryAcceptedTypes(brandSlug, streamSlug, projectSlug, categorySlug string, acceptedTypes []string) (*model.Category, error) {
	return a.project.UpdateCategoryAcceptedTypes(brandSlug, streamSlug, projectSlug, categorySlug, acceptedTypes)
}
func (a *App) MoveCategoryCards(brandSlug, streamSlug, projectSlug, fromCategoryID, toCategoryID string) error {
	return a.project.MoveCategoryCards(brandSlug, streamSlug, projectSlug, fromCategoryID, toCategoryID)
}
