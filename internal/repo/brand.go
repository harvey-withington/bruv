package repo

import (
	"bruv/internal/model"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
)

// CreateBrand creates a new Brand in the repository.
func (r *Repository) CreateBrand(name string) (*model.Brand, error) {
	slug := Slugify(name)
	if slug == "" {
		return nil, fmt.Errorf("invalid brand name: %q", name)
	}

	brandDir := r.brandPath(slug)
	if fileExists(brandDir) {
		return nil, fmt.Errorf("brand %q already exists", name)
	}

	if err := os.MkdirAll(brandDir, 0755); err != nil {
		return nil, fmt.Errorf("create brand directory: %w", err)
	}

	now := time.Now().UTC()
	brand := &model.Brand{
		ID:        uuid.New().String(),
		Name:      name,
		Slug:      slug,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := writeJSON(r.brandFilePath(slug), brand); err != nil {
		os.RemoveAll(brandDir)
		return nil, fmt.Errorf("write brand: %w", err)
	}

	// Create the streams subdirectory
	if err := os.MkdirAll(r.streamsPath(slug), 0755); err != nil {
		return nil, fmt.Errorf("create streams directory: %w", err)
	}

	return brand, nil
}

// GetBrand reads a Brand by its slug.
func (r *Repository) GetBrand(slug string) (*model.Brand, error) {
	path := r.brandFilePath(slug)
	if !fileExists(path) {
		return nil, fmt.Errorf("brand %q not found", slug)
	}

	var brand model.Brand
	if err := readJSON(path, &brand); err != nil {
		return nil, err
	}
	return &brand, nil
}

// ListBrands returns all Brands in the repository.
func (r *Repository) ListBrands() ([]model.Brand, error) {
	slugs, err := listSubdirs(r.brandsPath())
	if err != nil {
		return nil, fmt.Errorf("list brand directories: %w", err)
	}

	brands := make([]model.Brand, 0, len(slugs))
	for _, slug := range slugs {
		brand, err := r.GetBrand(slug)
		if err != nil {
			continue // skip malformed brands
		}
		brands = append(brands, *brand)
	}
	return brands, nil
}

// UpdateBrand updates a Brand's mutable fields.
func (r *Repository) UpdateBrand(slug string, update func(*model.Brand)) (*model.Brand, error) {
	brand, err := r.GetBrand(slug)
	if err != nil {
		return nil, err
	}

	update(brand)
	brand.UpdatedAt = time.Now().UTC()

	if err := writeJSON(r.brandFilePath(slug), brand); err != nil {
		return nil, fmt.Errorf("write brand: %w", err)
	}
	return brand, nil
}

// DeleteBrand removes a Brand and all its contents from the repository.
func (r *Repository) DeleteBrand(slug string) error {
	brandDir := r.brandPath(slug)
	if !fileExists(brandDir) {
		return fmt.Errorf("brand %q not found", slug)
	}
	return os.RemoveAll(brandDir)
}
