package repo

import (
	"bruv/internal/model"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
)

// CreateStream creates a new Stream within a Brand.
func (r *Repository) CreateStream(brandSlug, name string) (*model.Stream, error) {
	// Verify brand exists
	brand, err := r.GetBrand(brandSlug)
	if err != nil {
		return nil, err
	}

	baseSlug := Slugify(name)
	if baseSlug == "" {
		return nil, fmt.Errorf("invalid stream name: %q", name)
	}
	slug := uniqueSlug(baseSlug, func(s string) bool { return fileExists(r.streamPath(brandSlug, s)) })

	streamDir := r.streamPath(brandSlug, slug)

	// Count existing streams so the new one is appended at the end
	existingStreams, _ := r.ListStreams(brandSlug)
	position := len(existingStreams)

	if err := os.MkdirAll(streamDir, 0755); err != nil {
		return nil, fmt.Errorf("create stream directory: %w", err)
	}

	now := time.Now().UTC()
	stream := &model.Stream{
		ID:        uuid.New().String(),
		BrandID:   brand.ID,
		Name:      name,
		Slug:      slug,
		Position:  position,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := writeJSON(r.streamFilePath(brandSlug, slug), stream); err != nil {
		os.RemoveAll(streamDir)
		return nil, fmt.Errorf("write stream: %w", err)
	}

	// Create the projects subdirectory
	if err := os.MkdirAll(r.projectsPath(brandSlug, slug), 0755); err != nil {
		return nil, fmt.Errorf("create projects directory: %w", err)
	}

	return stream, nil
}

// GetStream reads a Stream by brand and stream slug.
func (r *Repository) GetStream(brandSlug, streamSlug string) (*model.Stream, error) {
	path := r.streamFilePath(brandSlug, streamSlug)
	if !fileExists(path) {
		return nil, fmt.Errorf("stream %q not found in brand %q", streamSlug, brandSlug)
	}

	var stream model.Stream
	if err := readJSON(path, &stream); err != nil {
		return nil, err
	}
	return &stream, nil
}

// ListStreams returns all Streams within a Brand.
func (r *Repository) ListStreams(brandSlug string) ([]model.Stream, error) {
	slugs, err := listSubdirs(r.streamsPath(brandSlug))
	if err != nil {
		return nil, fmt.Errorf("list stream directories: %w", err)
	}

	streams := make([]model.Stream, 0, len(slugs))
	for _, slug := range slugs {
		stream, err := r.GetStream(brandSlug, slug)
		if err != nil {
			continue
		}
		streams = append(streams, *stream)
	}

	// Sort by position
	for i := 0; i < len(streams); i++ {
		for j := i + 1; j < len(streams); j++ {
			if streams[j].Position < streams[i].Position {
				streams[i], streams[j] = streams[j], streams[i]
			}
		}
	}

	return streams, nil
}

// UpdateStream updates a Stream's mutable fields.
func (r *Repository) UpdateStream(brandSlug, streamSlug string, update func(*model.Stream)) (*model.Stream, error) {
	stream, err := r.GetStream(brandSlug, streamSlug)
	if err != nil {
		return nil, err
	}

	update(stream)
	stream.UpdatedAt = time.Now().UTC()

	if err := writeJSON(r.streamFilePath(brandSlug, streamSlug), stream); err != nil {
		return nil, fmt.Errorf("write stream: %w", err)
	}
	return stream, nil
}

// ReorderStreams updates the position of all streams within a brand based on the given ordered slug list.
func (r *Repository) ReorderStreams(brandSlug string, orderedSlugs []string) error {
	for i, slug := range orderedSlugs {
		_, err := r.UpdateStream(brandSlug, slug, func(s *model.Stream) {
			s.Position = i
		})
		if err != nil {
			return fmt.Errorf("reorder stream %q: %w", slug, err)
		}
	}
	return nil
}

// RenameStream renames a Stream and moves its directory if the slug changes.
func (r *Repository) RenameStream(brandSlug, streamSlug, newName string) (*model.Stream, error) {
	stream, err := r.GetStream(brandSlug, streamSlug)
	if err != nil {
		return nil, err
	}

	newSlug := Slugify(newName)
	if newSlug == "" {
		return nil, fmt.Errorf("invalid stream name: %q", newName)
	}

	stream.Name = newName
	stream.UpdatedAt = time.Now().UTC()

	if newSlug != streamSlug {
		newSlug = uniqueSlug(newSlug, func(s string) bool { return s != streamSlug && fileExists(r.streamPath(brandSlug, s)) })
		stream.Slug = newSlug
		oldDir := r.streamPath(brandSlug, streamSlug)
		cleanup := r.withDirOp(oldDir)
		err := os.Rename(oldDir, r.streamPath(brandSlug, newSlug))
		cleanup()
		if err != nil {
			return nil, fmt.Errorf("rename stream directory: %w", err)
		}
	}

	if err := writeJSON(r.streamFilePath(brandSlug, stream.Slug), stream); err != nil {
		return nil, fmt.Errorf("write stream: %w", err)
	}
	return stream, nil
}

// UpdateStreamDescription sets or clears the description on a Stream.
func (r *Repository) UpdateStreamDescription(brandSlug, streamSlug, description string) (*model.Stream, error) {
	stream, err := r.GetStream(brandSlug, streamSlug)
	if err != nil {
		return nil, err
	}
	stream.Description = description
	stream.UpdatedAt = time.Now().UTC()
	if err := writeJSON(r.streamFilePath(brandSlug, streamSlug), stream); err != nil {
		return nil, fmt.Errorf("write stream: %w", err)
	}
	return stream, nil
}

// UpdateStreamIcon sets or clears the icon on a Stream.
func (r *Repository) UpdateStreamIcon(brandSlug, streamSlug, icon string) (*model.Stream, error) {
	stream, err := r.GetStream(brandSlug, streamSlug)
	if err != nil {
		return nil, err
	}
	stream.Icon = icon
	stream.UpdatedAt = time.Now().UTC()
	if err := writeJSON(r.streamFilePath(brandSlug, streamSlug), stream); err != nil {
		return nil, fmt.Errorf("write stream: %w", err)
	}
	return stream, nil
}

// DeleteStream removes a Stream and all its contents.
func (r *Repository) DeleteStream(brandSlug, streamSlug string) error {
	streamDir := r.streamPath(brandSlug, streamSlug)
	if !fileExists(streamDir) {
		return fmt.Errorf("stream %q not found in brand %q", streamSlug, brandSlug)
	}
	cleanup := r.withDirOp(streamDir)
	defer cleanup()
	return os.RemoveAll(streamDir)
}
