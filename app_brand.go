package main

// Wails-bound forwarders for brand CRUD. Domain logic lives in
// core/services/project.

import "bruv/internal/model"

func (a *App) CreateBrand(name string) (*model.Brand, error) { return a.project.CreateBrand(name) }
func (a *App) GetBrand(slug string) (*model.Brand, error)    { return a.project.GetBrand(slug) }
func (a *App) ListBrands() ([]model.Brand, error)            { return a.project.ListBrands() }
func (a *App) RenameBrand(slug, newName string) (*model.Brand, error) {
	return a.project.RenameBrand(slug, newName)
}
func (a *App) UpdateBrandDescription(slug, description string) (*model.Brand, error) {
	return a.project.UpdateBrandDescription(slug, description)
}
func (a *App) UpdateBrandIcon(slug, icon string) (*model.Brand, error) {
	return a.project.UpdateBrandIcon(slug, icon)
}
func (a *App) DeleteBrand(slug string) error { return a.project.DeleteBrand(slug) }
