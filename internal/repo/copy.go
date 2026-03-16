package repo

import (
	"bruv/internal/model"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// copyDirRecursive copies a directory tree from src to dst.
func copyDirRecursive(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDirRecursive(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// CopyProject deep-copies a project (with all categories and cards) into the target stream.
// All entity IDs are regenerated. The copy gets " Copy" appended to its name.
func (r *Repository) CopyProject(fromBrand, fromStream, projectSlug, toBrand, toStream string) (*model.Project, error) {
	srcProject, err := r.GetProject(fromBrand, fromStream, projectSlug)
	if err != nil {
		return nil, fmt.Errorf("source project: %w", err)
	}

	dstBrand, err := r.GetBrand(toBrand)
	if err != nil {
		return nil, fmt.Errorf("destination brand: %w", err)
	}
	dstStream, err := r.GetStream(toBrand, toStream)
	if err != nil {
		return nil, fmt.Errorf("destination stream: %w", err)
	}

	// Generate unique name/slug
	copyName := srcProject.Name + " Copy"
	copySlug := Slugify(copyName)
	existingProjects, _ := r.ListProjects(toBrand, toStream)
	slugs := make(map[string]bool)
	for _, p := range existingProjects {
		slugs[p.Slug] = true
	}
	if slugs[copySlug] {
		for i := 2; ; i++ {
			candidate := fmt.Sprintf("%s Copy %d", srcProject.Name, i)
			cs := Slugify(candidate)
			if !slugs[cs] {
				copyName = candidate
				copySlug = cs
				break
			}
		}
	}

	srcDir := r.projectPath(fromBrand, fromStream, projectSlug)
	dstDir := r.projectPath(toBrand, toStream, copySlug)

	// Deep-copy the directory tree
	if err := copyDirRecursive(srcDir, dstDir); err != nil {
		os.RemoveAll(dstDir)
		return nil, fmt.Errorf("copy project directory: %w", err)
	}

	// Update the project.json with new identity
	now := time.Now().UTC()
	position := len(existingProjects)

	newProject := &model.Project{
		ID:        uuid.New().String(),
		StreamID:  dstStream.ID,
		BrandID:   dstBrand.ID,
		Name:      copyName,
		Slug:      copySlug,
		Position:  position,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := writeJSON(r.projectFilePath(toBrand, toStream, copySlug), newProject); err != nil {
		os.RemoveAll(dstDir)
		return nil, fmt.Errorf("write copied project: %w", err)
	}

	// Regenerate category IDs
	cats, _ := r.ListCategories(toBrand, toStream, copySlug)
	for _, cat := range cats {
		r.UpdateCategory(toBrand, toStream, copySlug, cat.Slug, func(c *model.Category) {
			c.ID = uuid.New().String()
			c.ProjectID = newProject.ID
		})
	}

	return newProject, nil
}

// CopyStream deep-copies a stream (with all projects and categories) into the target brand.
func (r *Repository) CopyStream(fromBrand, streamSlug, toBrand string) (*model.Stream, error) {
	srcStream, err := r.GetStream(fromBrand, streamSlug)
	if err != nil {
		return nil, fmt.Errorf("source stream: %w", err)
	}

	dstBrand, err := r.GetBrand(toBrand)
	if err != nil {
		return nil, fmt.Errorf("destination brand: %w", err)
	}

	// Generate unique name/slug
	copyName := srcStream.Name + " Copy"
	copySlug := Slugify(copyName)
	existingStreams, _ := r.ListStreams(toBrand)
	slugs := make(map[string]bool)
	for _, s := range existingStreams {
		slugs[s.Slug] = true
	}
	if slugs[copySlug] {
		for i := 2; ; i++ {
			candidate := fmt.Sprintf("%s Copy %d", srcStream.Name, i)
			cs := Slugify(candidate)
			if !slugs[cs] {
				copyName = candidate
				copySlug = cs
				break
			}
		}
	}

	srcDir := r.streamPath(fromBrand, streamSlug)
	dstDir := r.streamPath(toBrand, copySlug)

	if err := copyDirRecursive(srcDir, dstDir); err != nil {
		os.RemoveAll(dstDir)
		return nil, fmt.Errorf("copy stream directory: %w", err)
	}

	now := time.Now().UTC()
	position := len(existingStreams)

	newStream := &model.Stream{
		ID:        uuid.New().String(),
		BrandID:   dstBrand.ID,
		Name:      copyName,
		Slug:      copySlug,
		Position:  position,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := writeJSON(r.streamFilePath(toBrand, copySlug), newStream); err != nil {
		os.RemoveAll(dstDir)
		return nil, fmt.Errorf("write copied stream: %w", err)
	}

	// Regenerate IDs in all child projects and categories
	projects, _ := r.ListProjects(toBrand, copySlug)
	for _, p := range projects {
		newProjID := uuid.New().String()
		r.UpdateProject(toBrand, copySlug, p.Slug, func(proj *model.Project) {
			proj.ID = newProjID
			proj.StreamID = newStream.ID
			proj.BrandID = dstBrand.ID
		})
		cats, _ := r.ListCategories(toBrand, copySlug, p.Slug)
		for _, cat := range cats {
			r.UpdateCategory(toBrand, copySlug, p.Slug, cat.Slug, func(c *model.Category) {
				c.ID = uuid.New().String()
				c.ProjectID = newProjID
			})
		}
	}

	return newStream, nil
}

// CopyBrand deep-copies a brand (with all streams, projects, and categories).
func (r *Repository) CopyBrand(brandSlug string) (*model.Brand, error) {
	srcBrand, err := r.GetBrand(brandSlug)
	if err != nil {
		return nil, fmt.Errorf("source brand: %w", err)
	}

	// Generate unique name/slug
	copyName := srcBrand.Name + " Copy"
	copySlug := Slugify(copyName)
	existingBrands, _ := r.ListBrands()
	slugs := make(map[string]bool)
	for _, b := range existingBrands {
		slugs[b.Slug] = true
	}
	if slugs[copySlug] {
		for i := 2; ; i++ {
			candidate := fmt.Sprintf("%s Copy %d", srcBrand.Name, i)
			cs := Slugify(candidate)
			if !slugs[cs] {
				copyName = candidate
				copySlug = cs
				break
			}
		}
	}

	srcDir := r.brandPath(brandSlug)
	dstDir := r.brandPath(copySlug)

	if err := copyDirRecursive(srcDir, dstDir); err != nil {
		os.RemoveAll(dstDir)
		return nil, fmt.Errorf("copy brand directory: %w", err)
	}

	now := time.Now().UTC()
	position := len(existingBrands)

	newBrand := &model.Brand{
		ID:        uuid.New().String(),
		Name:      copyName,
		Slug:      copySlug,
		Position:  position,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := writeJSON(r.brandFilePath(copySlug), newBrand); err != nil {
		os.RemoveAll(dstDir)
		return nil, fmt.Errorf("write copied brand: %w", err)
	}

	// Regenerate IDs in all child streams, projects, and categories
	streams, _ := r.ListStreams(copySlug)
	for _, s := range streams {
		newStreamID := uuid.New().String()
		r.UpdateStream(copySlug, s.Slug, func(st *model.Stream) {
			st.ID = newStreamID
			st.BrandID = newBrand.ID
		})
		projects, _ := r.ListProjects(copySlug, s.Slug)
		for _, p := range projects {
			newProjID := uuid.New().String()
			r.UpdateProject(copySlug, s.Slug, p.Slug, func(proj *model.Project) {
				proj.ID = newProjID
				proj.StreamID = newStreamID
				proj.BrandID = newBrand.ID
			})
			cats, _ := r.ListCategories(copySlug, s.Slug, p.Slug)
			for _, cat := range cats {
				r.UpdateCategory(copySlug, s.Slug, p.Slug, cat.Slug, func(c *model.Category) {
					c.ID = uuid.New().String()
					c.ProjectID = newProjID
				})
			}
		}
	}

	return newBrand, nil
}
