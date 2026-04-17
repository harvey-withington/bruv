package main

import (
	"bruv/internal/model"
	"bruv/internal/repo"
	"fmt"
)

// Stream CRUD — split from app.go. See app_brand.go for the rationale.

func (a *App) CreateStream(brandSlug, name string) (*model.Stream, error) {
	name = repo.SanitizeText(name)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.CreateStream(brandSlug, name)
}

func (a *App) ListStreams(brandSlug string) ([]model.Stream, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.ListStreams(brandSlug)
}

func (a *App) RenameStream(brandSlug, streamSlug, newName string) (*model.Stream, error) {
	newName = repo.SanitizeText(newName)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	stream, err := a.repo.RenameStream(brandSlug, streamSlug, newName)
	if err != nil {
		return nil, err
	}
	if a.idx != nil {
		a.idxIncrementalRefresh()
	}
	return stream, nil
}

func (a *App) UpdateStreamDescription(brandSlug, streamSlug, description string) (*model.Stream, error) {
	description = repo.SanitizeText(description)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateStreamDescription(brandSlug, streamSlug, description)
}

func (a *App) UpdateStreamIcon(brandSlug, streamSlug, icon string) (*model.Stream, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateStreamIcon(brandSlug, streamSlug, icon)
}

func (a *App) DeleteStream(brandSlug, streamSlug string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	// Unpin cards from all projects/categories in this stream
	projects, _ := a.repo.ListProjects(brandSlug, streamSlug)
	for _, proj := range projects {
		cats, _ := a.repo.ListCategories(brandSlug, streamSlug, proj.Slug)
		for _, cat := range cats {
			pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
			for _, p := range pins {
				_ = a.repo.UnpinCard(p.CardID, p.ProjectID, p.CategoryID)
			}
		}
	}
	err := a.repo.DeleteStream(brandSlug, streamSlug)
	if err == nil && a.idx != nil {
		a.idxIncrementalRefresh()
	}
	return err
}
