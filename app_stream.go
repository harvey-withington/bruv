package main

// Wails-bound forwarders for stream CRUD. Domain logic lives in
// core/services/project.

import "bruv/internal/model"

func (a *App) CreateStream(brandSlug, name string) (*model.Stream, error) {
	return a.project.CreateStream(brandSlug, name)
}
func (a *App) ListStreams(brandSlug string) ([]model.Stream, error) {
	return a.project.ListStreams(brandSlug)
}
func (a *App) RenameStream(brandSlug, streamSlug, newName string) (*model.Stream, error) {
	return a.project.RenameStream(brandSlug, streamSlug, newName)
}
func (a *App) UpdateStreamDescription(brandSlug, streamSlug, description string) (*model.Stream, error) {
	return a.project.UpdateStreamDescription(brandSlug, streamSlug, description)
}
func (a *App) UpdateStreamIcon(brandSlug, streamSlug, icon string) (*model.Stream, error) {
	return a.project.UpdateStreamIcon(brandSlug, streamSlug, icon)
}
func (a *App) DeleteStream(brandSlug, streamSlug string) error {
	return a.project.DeleteStream(brandSlug, streamSlug)
}
