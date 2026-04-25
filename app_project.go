package main

// Wails-bound forwarders for project CRUD. Domain logic lives in
// core/services/project.

import "bruv/internal/model"

func (a *App) CreateProject(brandSlug, streamSlug, name string) (*model.Project, error) {
	return a.project.CreateProject(brandSlug, streamSlug, name)
}
func (a *App) ListProjects(brandSlug, streamSlug string) ([]model.Project, error) {
	return a.project.ListProjects(brandSlug, streamSlug)
}
func (a *App) RenameProject(brandSlug, streamSlug, projectSlug, newName string) (*model.Project, error) {
	return a.project.RenameProject(brandSlug, streamSlug, projectSlug, newName)
}
func (a *App) UpdateProjectDescription(brandSlug, streamSlug, projectSlug, description string) (*model.Project, error) {
	return a.project.UpdateProjectDescription(brandSlug, streamSlug, projectSlug, description)
}
func (a *App) UpdateProjectIcon(brandSlug, streamSlug, projectSlug, icon string) (*model.Project, error) {
	return a.project.UpdateProjectIcon(brandSlug, streamSlug, projectSlug, icon)
}
func (a *App) DeleteProject(brandSlug, streamSlug, projectSlug string) error {
	return a.project.DeleteProject(brandSlug, streamSlug, projectSlug)
}
