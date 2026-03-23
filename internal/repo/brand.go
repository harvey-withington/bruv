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
	baseSlug := Slugify(name)
	if baseSlug == "" {
		return nil, fmt.Errorf("invalid brand name: %q", name)
	}
	slug := uniqueSlug(baseSlug, func(s string) bool { return fileExists(r.brandPath(s)) })

	brandDir := r.brandPath(slug)

	// Count existing brands so the new one is appended at the end
	existing, _ := r.ListBrands()
	position := len(existing)

	if err := os.MkdirAll(brandDir, 0755); err != nil {
		return nil, fmt.Errorf("create brand directory: %w", err)
	}

	now := time.Now().UTC()
	brand := &model.Brand{
		ID:        uuid.New().String(),
		Name:      name,
		Slug:      slug,
		Position:  position,
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

	// Sort by position
	for i := 0; i < len(brands); i++ {
		for j := i + 1; j < len(brands); j++ {
			if brands[j].Position < brands[i].Position {
				brands[i], brands[j] = brands[j], brands[i]
			}
		}
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

// ReorderBrands updates the position of all brands based on the given ordered slug list.
func (r *Repository) ReorderBrands(orderedSlugs []string) error {
	for i, slug := range orderedSlugs {
		_, err := r.UpdateBrand(slug, func(b *model.Brand) {
			b.Position = i
		})
		if err != nil {
			return fmt.Errorf("reorder brand %q: %w", slug, err)
		}
	}
	return nil
}

// RenameBrand renames a Brand and moves its directory if the slug changes.
func (r *Repository) RenameBrand(slug, newName string) (*model.Brand, error) {
	brand, err := r.GetBrand(slug)
	if err != nil {
		return nil, err
	}

	newSlug := Slugify(newName)
	if newSlug == "" {
		return nil, fmt.Errorf("invalid brand name: %q", newName)
	}

	brand.Name = newName
	brand.UpdatedAt = time.Now().UTC()

	if newSlug != slug {
		newSlug = uniqueSlug(newSlug, func(s string) bool { return s != slug && fileExists(r.brandPath(s)) })
		brand.Slug = newSlug
		if err := os.Rename(r.brandPath(slug), r.brandPath(newSlug)); err != nil {
			return nil, fmt.Errorf("rename brand directory: %w", err)
		}
	}

	if err := writeJSON(r.brandFilePath(brand.Slug), brand); err != nil {
		return nil, fmt.Errorf("write brand: %w", err)
	}
	return brand, nil
}

// UpdateBrandDescription sets or clears the description on a Brand.
func (r *Repository) UpdateBrandDescription(slug, description string) (*model.Brand, error) {
	brand, err := r.GetBrand(slug)
	if err != nil {
		return nil, err
	}
	brand.Description = description
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
