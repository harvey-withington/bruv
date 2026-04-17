package main

import (
	"bruv/internal/config"
	"bruv/internal/model"
	"bruv/internal/repo"
	"fmt"
)

// Project CRUD — split from app.go. See app_brand.go for the rationale.

func (a *App) CreateProject(brandSlug, streamSlug, name string) (*model.Project, error) {
	name = repo.SanitizeText(name)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	project, err := a.repo.CreateProject(brandSlug, streamSlug, name)
	if err != nil {
		return nil, err
	}
	// Auto-create a default category so the project is immediately usable for pinning
	prefs, _ := config.LoadPreferences()
	catName := prefs.DefaultCategoryName
	if catName == "" {
		catName = "Ideas"
	}
	a.repo.CreateCategory(brandSlug, streamSlug, project.Slug, catName, 0)
	return project, nil
}

func (a *App) ListProjects(brandSlug, streamSlug string) ([]model.Project, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.ListProjects(brandSlug, streamSlug)
}

func (a *App) RenameProject(brandSlug, streamSlug, projectSlug, newName string) (*model.Project, error) {
	newName = repo.SanitizeText(newName)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	project, err := a.repo.RenameProject(brandSlug, streamSlug, projectSlug, newName)
	if err != nil {
		return nil, err
	}
	if a.idx != nil {
		a.idxIncrementalRefresh()
	}
	return project, nil
}

func (a *App) UpdateProjectDescription(brandSlug, streamSlug, projectSlug, description string) (*model.Project, error) {
	description = repo.SanitizeText(description)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateProjectDescription(brandSlug, streamSlug, projectSlug, description)
}

func (a *App) UpdateProjectIcon(brandSlug, streamSlug, projectSlug, icon string) (*model.Project, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateProjectIcon(brandSlug, streamSlug, projectSlug, icon)
}

func (a *App) DeleteProject(brandSlug, streamSlug, projectSlug string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}

	// Before deleting, unpin all cards from this project's categories
	// so they become orphaned (appear in inbox) instead of silently deleted.
	cats, _ := a.repo.ListCategories(brandSlug, streamSlug, projectSlug)
	for _, cat := range cats {
		pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
		for _, p := range pins {
			_ = a.repo.UnpinCard(p.CardID, p.ProjectID, p.CategoryID)
		}
	}

	err := a.repo.DeleteProject(brandSlug, streamSlug, projectSlug)
	if err == nil && a.idx != nil {
		a.idxIncrementalRefresh()
	}
	return err
}
