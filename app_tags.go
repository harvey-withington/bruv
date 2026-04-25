package main

// Wails-bound forwarders for tags + labels. Domain logic lives in
// core/services/catalog.

import "bruv/internal/model"

func (a *App) GetTagColors() (map[string]string, error) { return a.catalog.GetTagColors() }

func (a *App) SetTagColor(tag, color string) (map[string]string, error) {
	return a.catalog.SetTagColor(tag, color)
}

func (a *App) AssignTagColor(tag string) (map[string]string, error) {
	return a.catalog.AssignTagColor(tag)
}

func (a *App) GetProjectLabels(brandSlug, streamSlug, projectSlug string) ([]model.Label, error) {
	return a.catalog.GetProjectLabels(brandSlug, streamSlug, projectSlug)
}

func (a *App) AddProjectLabel(brandSlug, streamSlug, projectSlug, name, color string) ([]model.Label, error) {
	return a.catalog.AddProjectLabel(brandSlug, streamSlug, projectSlug, name, color)
}

func (a *App) RemoveProjectLabel(brandSlug, streamSlug, projectSlug, labelID string) ([]model.Label, error) {
	return a.catalog.RemoveProjectLabel(brandSlug, streamSlug, projectSlug, labelID)
}

func (a *App) UpdateProjectLabel(brandSlug, streamSlug, projectSlug, labelID, name, color string) ([]model.Label, error) {
	return a.catalog.UpdateProjectLabel(brandSlug, streamSlug, projectSlug, labelID, name, color)
}

func (a *App) SetProjectLabelIcon(brandSlug, streamSlug, projectSlug, labelID, icon string) ([]model.Label, error) {
	return a.catalog.SetProjectLabelIcon(brandSlug, streamSlug, projectSlug, labelID, icon)
}

func (a *App) UpdateCardLabels(id string, labelIDs []string) (*model.Card, error) {
	return a.catalog.UpdateCardLabels(id, labelIDs)
}

// healTagColors forwards to the service so the repo-open hook in
// app.go doesn't need to know the service package.
func (a *App) healTagColors() { a.catalog.HealTagColors() }
