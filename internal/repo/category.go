package repo

import (
	"bruv/internal/model"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
)

// CreateCategory creates a new Category within a Project.
func (r *Repository) CreateCategory(brandSlug, streamSlug, projectSlug, name string, position int) (*model.Category, error) {
	project, err := r.GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}

	slug := Slugify(name)
	if slug == "" {
		return nil, fmt.Errorf("invalid category name: %q", name)
	}

	catPath := r.categoryFilePath(brandSlug, streamSlug, projectSlug, slug)
	if fileExists(catPath) {
		return nil, fmt.Errorf("category %q already exists in project %q", name, projectSlug)
	}

	now := time.Now().UTC()
	category := &model.Category{
		ID:        uuid.New().String(),
		ProjectID: project.ID,
		Name:      name,
		Slug:      slug,
		Position:  position,
		CreatedAt: now,
		UpdatedAt: now,
	}

	catDir := r.categoriesPath(brandSlug, streamSlug, projectSlug)
	if err := os.MkdirAll(catDir, 0755); err != nil {
		return nil, fmt.Errorf("create categories directory: %w", err)
	}

	if err := writeJSON(catPath, category); err != nil {
		return nil, fmt.Errorf("write category: %w", err)
	}

	return category, nil
}

// GetCategory reads a Category by its slug.
func (r *Repository) GetCategory(brandSlug, streamSlug, projectSlug, categorySlug string) (*model.Category, error) {
	path := r.categoryFilePath(brandSlug, streamSlug, projectSlug, categorySlug)
	if !fileExists(path) {
		return nil, fmt.Errorf("category %q not found in project %q", categorySlug, projectSlug)
	}

	var category model.Category
	if err := readJSON(path, &category); err != nil {
		return nil, err
	}
	return &category, nil
}

// ListCategories returns all Categories within a Project, sorted by position.
func (r *Repository) ListCategories(brandSlug, streamSlug, projectSlug string) ([]model.Category, error) {
	slugs, err := listJSONFiles(r.categoriesPath(brandSlug, streamSlug, projectSlug))
	if err != nil {
		return nil, fmt.Errorf("list category files: %w", err)
	}

	categories := make([]model.Category, 0, len(slugs))
	for _, slug := range slugs {
		cat, err := r.GetCategory(brandSlug, streamSlug, projectSlug, slug)
		if err != nil {
			continue
		}
		categories = append(categories, *cat)
	}

	// Sort by position
	for i := 0; i < len(categories); i++ {
		for j := i + 1; j < len(categories); j++ {
			if categories[j].Position < categories[i].Position {
				categories[i], categories[j] = categories[j], categories[i]
			}
		}
	}

	return categories, nil
}

// UpdateCategory updates a Category's mutable fields.
func (r *Repository) UpdateCategory(brandSlug, streamSlug, projectSlug, categorySlug string, update func(*model.Category)) (*model.Category, error) {
	category, err := r.GetCategory(brandSlug, streamSlug, projectSlug, categorySlug)
	if err != nil {
		return nil, err
	}

	update(category)
	category.UpdatedAt = time.Now().UTC()

	if err := writeJSON(r.categoryFilePath(brandSlug, streamSlug, projectSlug, categorySlug), category); err != nil {
		return nil, fmt.Errorf("write category: %w", err)
	}
	return category, nil
}

// DeleteCategory removes a Category.
func (r *Repository) DeleteCategory(brandSlug, streamSlug, projectSlug, categorySlug string) error {
	path := r.categoryFilePath(brandSlug, streamSlug, projectSlug, categorySlug)
	if !fileExists(path) {
		return fmt.Errorf("category %q not found", categorySlug)
	}
	return os.Remove(path)
}
