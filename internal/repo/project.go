package repo

import (
	"bruv/internal/model"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
)

// CreateProject creates a new Project within a Stream.
func (r *Repository) CreateProject(brandSlug, streamSlug, name string) (*model.Project, error) {
	stream, err := r.GetStream(brandSlug, streamSlug)
	if err != nil {
		return nil, err
	}

	brand, err := r.GetBrand(brandSlug)
	if err != nil {
		return nil, err
	}

	slug := Slugify(name)
	if slug == "" {
		return nil, fmt.Errorf("invalid project name: %q", name)
	}

	projectDir := r.projectPath(brandSlug, streamSlug, slug)
	if fileExists(projectDir) {
		return nil, fmt.Errorf("project %q already exists in stream %q", name, streamSlug)
	}

	// Count existing projects so the new one is appended at the end
	existingProjects, _ := r.ListProjects(brandSlug, streamSlug)
	position := len(existingProjects)

	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return nil, fmt.Errorf("create project directory: %w", err)
	}

	now := time.Now().UTC()
	project := &model.Project{
		ID:        uuid.New().String(),
		StreamID:  stream.ID,
		BrandID:   brand.ID,
		Name:      name,
		Slug:      slug,
		Position:  position,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := writeJSON(r.projectFilePath(brandSlug, streamSlug, slug), project); err != nil {
		os.RemoveAll(projectDir)
		return nil, fmt.Errorf("write project: %w", err)
	}

	// Create the categories subdirectory
	if err := os.MkdirAll(r.categoriesPath(brandSlug, streamSlug, slug), 0755); err != nil {
		return nil, fmt.Errorf("create categories directory: %w", err)
	}

	return project, nil
}

// GetProject reads a Project by its slug path.
func (r *Repository) GetProject(brandSlug, streamSlug, projectSlug string) (*model.Project, error) {
	path := r.projectFilePath(brandSlug, streamSlug, projectSlug)
	if !fileExists(path) {
		return nil, fmt.Errorf("project %q not found", projectSlug)
	}

	var project model.Project
	if err := readJSON(path, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// ListProjects returns all Projects within a Stream.
func (r *Repository) ListProjects(brandSlug, streamSlug string) ([]model.Project, error) {
	slugs, err := listSubdirs(r.projectsPath(brandSlug, streamSlug))
	if err != nil {
		return nil, fmt.Errorf("list project directories: %w", err)
	}

	projects := make([]model.Project, 0, len(slugs))
	for _, slug := range slugs {
		project, err := r.GetProject(brandSlug, streamSlug, slug)
		if err != nil {
			continue
		}
		projects = append(projects, *project)
	}

	// Sort by position
	for i := 0; i < len(projects); i++ {
		for j := i + 1; j < len(projects); j++ {
			if projects[j].Position < projects[i].Position {
				projects[i], projects[j] = projects[j], projects[i]
			}
		}
	}

	return projects, nil
}

// UpdateProject updates a Project's mutable fields.
func (r *Repository) UpdateProject(brandSlug, streamSlug, projectSlug string, update func(*model.Project)) (*model.Project, error) {
	project, err := r.GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}

	update(project)
	project.UpdatedAt = time.Now().UTC()

	if err := writeJSON(r.projectFilePath(brandSlug, streamSlug, projectSlug), project); err != nil {
		return nil, fmt.Errorf("write project: %w", err)
	}
	return project, nil
}

// ReorderProjects updates the position of all projects within a stream based on the given ordered slug list.
func (r *Repository) ReorderProjects(brandSlug, streamSlug string, orderedSlugs []string) error {
	for i, slug := range orderedSlugs {
		_, err := r.UpdateProject(brandSlug, streamSlug, slug, func(p *model.Project) {
			p.Position = i
		})
		if err != nil {
			return fmt.Errorf("reorder project %q: %w", slug, err)
		}
	}
	return nil
}

// DeleteProject removes a Project and all its contents.
func (r *Repository) DeleteProject(brandSlug, streamSlug, projectSlug string) error {
	projectDir := r.projectPath(brandSlug, streamSlug, projectSlug)
	if !fileExists(projectDir) {
		return fmt.Errorf("project %q not found", projectSlug)
	}
	return os.RemoveAll(projectDir)
}
