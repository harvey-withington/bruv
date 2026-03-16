package repo

import (
	"bruv/internal/model"
	"fmt"
	"os"
	"time"
)

// MoveProject moves a project directory from one stream to another (possibly across brands).
// It updates the project's BrandID and StreamID references.
func (r *Repository) MoveProject(fromBrand, fromStream, projectSlug, toBrand, toStream string) (*model.Project, error) {
	srcDir := r.projectPath(fromBrand, fromStream, projectSlug)
	if !fileExists(srcDir) {
		return nil, fmt.Errorf("source project %q not found", projectSlug)
	}

	// Verify destination stream exists
	dstStream, err := r.GetStream(toBrand, toStream)
	if err != nil {
		return nil, fmt.Errorf("destination stream: %w", err)
	}
	dstBrand, err := r.GetBrand(toBrand)
	if err != nil {
		return nil, fmt.Errorf("destination brand: %w", err)
	}

	dstDir := r.projectPath(toBrand, toStream, projectSlug)
	if fileExists(dstDir) {
		return nil, fmt.Errorf("project %q already exists in destination stream", projectSlug)
	}

	// Ensure destination projects directory exists
	if err := os.MkdirAll(r.projectsPath(toBrand, toStream), 0755); err != nil {
		return nil, fmt.Errorf("create destination projects dir: %w", err)
	}

	// Assign position at end of destination
	existingProjects, _ := r.ListProjects(toBrand, toStream)
	position := len(existingProjects)

	// Move directory
	if err := os.Rename(srcDir, dstDir); err != nil {
		return nil, fmt.Errorf("move project directory: %w", err)
	}

	// Update project metadata
	project, err := r.GetProject(toBrand, toStream, projectSlug)
	if err != nil {
		// Rollback
		os.Rename(dstDir, srcDir)
		return nil, fmt.Errorf("read moved project: %w", err)
	}

	project.BrandID = dstBrand.ID
	project.StreamID = dstStream.ID
	project.Position = position
	project.UpdatedAt = time.Now().UTC()

	if err := writeJSON(r.projectFilePath(toBrand, toStream, projectSlug), project); err != nil {
		os.Rename(dstDir, srcDir)
		return nil, fmt.Errorf("update moved project: %w", err)
	}

	return project, nil
}

// MoveStream moves a stream directory from one brand to another.
// It updates the stream's BrandID and all child projects' BrandID.
func (r *Repository) MoveStream(fromBrand, streamSlug, toBrand string) (*model.Stream, error) {
	srcDir := r.streamPath(fromBrand, streamSlug)
	if !fileExists(srcDir) {
		return nil, fmt.Errorf("source stream %q not found", streamSlug)
	}

	// Verify destination brand exists
	dstBrand, err := r.GetBrand(toBrand)
	if err != nil {
		return nil, fmt.Errorf("destination brand: %w", err)
	}

	dstDir := r.streamPath(toBrand, streamSlug)
	if fileExists(dstDir) {
		return nil, fmt.Errorf("stream %q already exists in destination brand", streamSlug)
	}

	// Ensure destination streams directory exists
	if err := os.MkdirAll(r.streamsPath(toBrand), 0755); err != nil {
		return nil, fmt.Errorf("create destination streams dir: %w", err)
	}

	// Assign position at end of destination
	existingStreams, _ := r.ListStreams(toBrand)
	position := len(existingStreams)

	// Move directory
	if err := os.Rename(srcDir, dstDir); err != nil {
		return nil, fmt.Errorf("move stream directory: %w", err)
	}

	// Update stream metadata
	stream, err := r.GetStream(toBrand, streamSlug)
	if err != nil {
		os.Rename(dstDir, srcDir)
		return nil, fmt.Errorf("read moved stream: %w", err)
	}

	stream.BrandID = dstBrand.ID
	stream.Position = position
	stream.UpdatedAt = time.Now().UTC()

	if err := writeJSON(r.streamFilePath(toBrand, streamSlug), stream); err != nil {
		os.Rename(dstDir, srcDir)
		return nil, fmt.Errorf("update moved stream: %w", err)
	}

	// Update all child projects' BrandID
	projects, _ := r.ListProjects(toBrand, streamSlug)
	for _, p := range projects {
		r.UpdateProject(toBrand, streamSlug, p.Slug, func(proj *model.Project) {
			proj.BrandID = dstBrand.ID
		})
	}

	return stream, nil
}
