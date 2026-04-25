// Package repository is the RepositoryService — metadata, recent-repos,
// and import/export. The lifecycle operations (Init, Open, Close) stay
// on App because they coordinate scheduler + MCP registry + due-date
// scanner + index + tray, which are host-owned concerns. Those move
// under a Core composition root later, not here.
package repository

import (
	"bruv/internal/config"
	"bruv/internal/importer"
	"bruv/internal/index"
	"bruv/internal/repo"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Deps is the narrow host contract for RepositoryService.
type Deps interface {
	Repo() *repo.Repository
	Index() *index.Index
}

// Service exposes repo metadata, recents, and import/export.
type Service struct{ deps Deps }

// New constructs a RepositoryService.
func New(deps Deps) *Service { return &Service{deps: deps} }

// --- Recents ---

func (s *Service) ListRecentRepos() ([]config.RecentRepo, error) { return config.LoadRecent() }
func (s *Service) RemoveRecentRepo(path string) error            { return config.RemoveRecent(path) }

// --- Metadata ---

func (s *Service) GetDescription() (string, error) {
	r := s.deps.Repo()
	if r == nil {
		return "", fmt.Errorf("no repository open")
	}
	return r.Manifest.Description, nil
}

func (s *Service) UpdateDescription(description string) error {
	description = repo.SanitizeText(description)
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	return r.UpdateManifestDescription(description)
}

// --- Import / Export ---

func (s *Service) ImportTrelloBoard(brandSlug, streamSlug, filePath, archiveMode string) (*importer.Result, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return s.importTrelloBytes(brandSlug, streamSlug, data, archiveMode)
}

func (s *Service) ImportTrelloBoardFromJSON(brandSlug, streamSlug, jsonContent, archiveMode string) (*importer.Result, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return s.importTrelloBytes(brandSlug, streamSlug, []byte(jsonContent), archiveMode)
}

func (s *Service) importTrelloBytes(brandSlug, streamSlug string, data []byte, archiveMode string) (*importer.Result, error) {
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

	r := s.deps.Repo()
	result, err := importer.ImportTrello(r, brandSlug, streamSlug, parsed, importer.Options{Archive: mode})
	if err != nil {
		return nil, err
	}
	if idx := s.deps.Index(); idx != nil {
		if _, err := idx.IncrementalRefresh(r.Root); err != nil {
			slog.Warn("index refresh after import failed", "err", err)
		}
	}
	return result, nil
}

// ExportProjectToFile writes a project export to disk. Returns bytes
// written on success. Name distinct from catalog.ExportCardTypesToFile
// because Wails exposes both — the main package aliases appropriately.
func (s *Service) ExportProjectToFile(brandSlug, streamSlug, projectSlug, filePath string) (int, error) {
	r := s.deps.Repo()
	if r == nil {
		return 0, fmt.Errorf("no repository open")
	}
	data, err := importer.ExportProject(r, brandSlug, streamSlug, projectSlug)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return 0, fmt.Errorf("write export: %w", err)
	}
	return len(data), nil
}
