package main

// Wails-bound forwarders for Trello import and project export.
// Domain logic lives in core/services/repository.

import "bruv/internal/importer"

func (a *App) ImportTrelloBoard(brandSlug, streamSlug, filePath, archiveMode string) (*importer.Result, error) {
	return a.repoService.ImportTrelloBoard(brandSlug, streamSlug, filePath, archiveMode)
}

func (a *App) ImportTrelloBoardFromJSON(brandSlug, streamSlug, jsonContent, archiveMode string) (*importer.Result, error) {
	return a.repoService.ImportTrelloBoardFromJSON(brandSlug, streamSlug, jsonContent, archiveMode)
}

// ExportProjectToFile writes a project export to the given absolute path.
// Returns the byte count written. Distinct from ExportCardTypesToFile
// which lives on the catalog service.
func (a *App) ExportProjectToFile(brandSlug, streamSlug, projectSlug, filePath string) (int, error) {
	return a.repoService.ExportProjectToFile(brandSlug, streamSlug, projectSlug, filePath)
}
