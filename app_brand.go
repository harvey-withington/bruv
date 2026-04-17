package main

import (
	"bruv/internal/model"
	"bruv/internal/repo"
	"fmt"
)

// Brand CRUD — split from app.go to keep the main file scannable.
// Every method is a thin transport wrapper around repo; business
// rules (e.g. "unpin cards before delete") live here rather than in
// repo so the repo stays a pure persistence layer.

func (a *App) CreateBrand(name string) (*model.Brand, error) {
	name = repo.SanitizeText(name)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.CreateBrand(name)
}

func (a *App) GetBrand(slug string) (*model.Brand, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.GetBrand(slug)
}

func (a *App) ListBrands() ([]model.Brand, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.ListBrands()
}

func (a *App) RenameBrand(slug, newName string) (*model.Brand, error) {
	newName = repo.SanitizeText(newName)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	brand, err := a.repo.RenameBrand(slug, newName)
	if err != nil {
		return nil, err
	}
	if a.idx != nil {
		a.idxIncrementalRefresh()
	}
	return brand, nil
}

func (a *App) UpdateBrandDescription(slug, description string) (*model.Brand, error) {
	description = repo.SanitizeText(description)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateBrandDescription(slug, description)
}

func (a *App) UpdateBrandIcon(slug, icon string) (*model.Brand, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateBrandIcon(slug, icon)
}

func (a *App) DeleteBrand(slug string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	// Unpin cards from all streams/projects/categories in this brand
	streams, _ := a.repo.ListStreams(slug)
	for _, stream := range streams {
		projects, _ := a.repo.ListProjects(slug, stream.Slug)
		for _, proj := range projects {
			cats, _ := a.repo.ListCategories(slug, stream.Slug, proj.Slug)
			for _, cat := range cats {
				pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
				for _, p := range pins {
					_ = a.repo.UnpinCard(p.CardID, p.ProjectID, p.CategoryID)
				}
			}
		}
	}
	err := a.repo.DeleteBrand(slug)
	if err == nil && a.idx != nil {
		a.idxIncrementalRefresh()
	}
	return err
}
