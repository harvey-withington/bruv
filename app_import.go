package main

// Import/export: Trello board ingestion and project export to disk.
// Both sit at the boundary where the `internal/importer` package meets
// the Wails-bound App surface — thin wrappers that coerce frontend
// inputs (file paths, JSON payloads, archive-mode strings) into
// importer options and refresh the SQLite index after a successful
// import.
//
// Extracted from app.go so adding new import sources (Notion, Linear,
// etc.) has an obvious home instead of lengthening the god-file.

import (
	"bruv/internal/importer"
	"fmt"
	"os"
	"strings"
)

// --- Trello Import ---

// ImportTrelloBoard reads a Trello JSON export from disk and creates a new
// project under the given brand/stream. archiveMode is one of
// "skip" | "archive" | "inline" — see importer.ArchiveMode.
func (a *App) ImportTrelloBoard(brandSlug, streamSlug, filePath, archiveMode string) (*importer.Result, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return a.importTrelloBytes(brandSlug, streamSlug, data, archiveMode)
}

// ImportTrelloBoardFromJSON accepts a Trello JSON export as a string payload
// (useful when the frontend drops a file via FileReader and never has access
// to the original path).
func (a *App) ImportTrelloBoardFromJSON(brandSlug, streamSlug, jsonContent, archiveMode string) (*importer.Result, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.importTrelloBytes(brandSlug, streamSlug, []byte(jsonContent), archiveMode)
}

func (a *App) importTrelloBytes(brandSlug, streamSlug string, data []byte, archiveMode string) (*importer.Result, error) {
	parsed, err := importer.ParseTrelloJSON(data)
	if err != nil {
		return nil, err
	}

	mode := importer.ArchiveSeparate
	switch strings.ToLower(archiveMode) {
	case "skip":
		mode = importer.ArchiveSkip
	case "inline":
		mode = importer.ArchiveInline
	case "", "archive", "separate":
		mode = importer.ArchiveSeparate
	}

	result, err := importer.ImportTrello(a.repo, brandSlug, streamSlug, parsed, importer.Options{Archive: mode})
	if err != nil {
		return nil, err
	}
	if a.idx != nil {
		a.idxIncrementalRefresh()
	}
	return result, nil
}

// --- Project Export ---

// ExportProjectToFile writes a project export to the given absolute path.
// Returns the byte count of the written file on success.
func (a *App) ExportProjectToFile(brandSlug, streamSlug, projectSlug, filePath string) (int, error) {
	if a.repo == nil {
		return 0, fmt.Errorf("no repository open")
	}
	data, err := importer.ExportProject(a.repo, brandSlug, streamSlug, projectSlug)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return 0, fmt.Errorf("write export: %w", err)
	}
	return len(data), nil
}
