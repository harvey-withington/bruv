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

// copyHierarchyDir copies a brand/stream/project directory tree, leaving
// out every project's workspace/ folder: workspace.json carries the
// workspace's identity (ID, claim, git-serve state), and a clone of it
// would make two projects resolve to one workspace.
func copyHierarchyDir(src, dst string) error {
	return copyDirRecursive(src, dst, isProjectWorkspaceDir)
}

// isProjectWorkspaceDir reports whether path is a project's workspace
// folder — named workspace/ AND sitting next to a project.json, so a
// stream or project that happens to be slugged "workspace" still copies.
func isProjectWorkspaceDir(path string) bool {
	return filepath.Base(path) == workspaceDir && fileExists(filepath.Join(filepath.Dir(path), "project.json"))
}

// copyDirRecursive copies a directory tree from src to dst. Directories
// for which skipDir returns true are left out (nil = copy everything).
func copyDirRecursive(src, dst string, skipDir func(path string) bool) error {
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
			if skipDir != nil && skipDir(srcPath) {
				continue
			}
			if err := copyDirRecursive(srcPath, dstPath, skipDir); err != nil {
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
func (r *Repository) CopyProject(fromBrand, fromStream, projectSlug, toBrand, toStream string, position int) (*model.Project, error) {
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
	if err := copyHierarchyDir(srcDir, dstDir); err != nil {
		os.RemoveAll(dstDir)
		return nil, fmt.Errorf("copy project directory: %w", err)
	}

	// Convert array index to actual position value (positions may be non-contiguous)
	if position < 0 || position > len(existingProjects) {
		position = len(existingProjects)
	}
	var insertPos int
	if position >= len(existingProjects) {
		if len(existingProjects) > 0 {
			insertPos = existingProjects[len(existingProjects)-1].Position + 1
		}
	} else {
		insertPos = existingProjects[position].Position
	}
	for _, p := range existingProjects {
		if p.Position >= insertPos {
			r.UpdateProject(toBrand, toStream, p.Slug, func(proj *model.Project) {
				proj.Position = proj.Position + 1
			})
		}
	}

	// Update the project.json with new identity
	now := time.Now().UTC()

	newProject := &model.Project{
		ID:          uuid.New().String(),
		StreamID:    dstStream.ID,
		BrandID:     dstBrand.ID,
		Name:        copyName,
		Slug:        copySlug,
		Description: srcProject.Description,
		Icon:        srcProject.Icon,
		Position:    insertPos,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := writeJSON(r.projectFilePath(toBrand, toStream, copySlug), newProject); err != nil {
		os.RemoveAll(dstDir)
		return nil, fmt.Errorf("write copied project: %w", err)
	}

	if err := r.regenerateCategoryIDs(toBrand, toStream, copySlug, newProject.ID); err != nil {
		os.RemoveAll(dstDir)
		return nil, err
	}

	return newProject, nil
}

// The regenerate* helpers give every entity under a freshly copied tree a
// new ID. Any failure is returned and the caller removes the whole copy:
// a category left with its source's ID shows the source's pins on both
// boards.

func (r *Repository) regenerateCategoryIDs(brandSlug, streamSlug, projectSlug, projectID string) error {
	cats, err := r.ListCategories(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return fmt.Errorf("list copied categories: %w", err)
	}
	for _, cat := range cats {
		if _, err := r.UpdateCategory(brandSlug, streamSlug, projectSlug, cat.Slug, func(c *model.Category) {
			c.ID = uuid.New().String()
			c.ProjectID = projectID
		}); err != nil {
			return fmt.Errorf("regenerate category %q ID: %w", cat.Slug, err)
		}
	}
	return nil
}

func (r *Repository) regenerateProjectIDs(brandSlug, streamSlug, streamID, brandID string) error {
	projects, err := r.ListProjects(brandSlug, streamSlug)
	if err != nil {
		return fmt.Errorf("list copied projects: %w", err)
	}
	for _, p := range projects {
		newProjID := uuid.New().String()
		if _, err := r.UpdateProject(brandSlug, streamSlug, p.Slug, func(proj *model.Project) {
			proj.ID = newProjID
			proj.StreamID = streamID
			proj.BrandID = brandID
		}); err != nil {
			return fmt.Errorf("regenerate project %q ID: %w", p.Slug, err)
		}
		if err := r.regenerateCategoryIDs(brandSlug, streamSlug, p.Slug, newProjID); err != nil {
			return err
		}
	}
	return nil
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

	if err := copyHierarchyDir(srcDir, dstDir); err != nil {
		os.RemoveAll(dstDir)
		return nil, fmt.Errorf("copy stream directory: %w", err)
	}

	now := time.Now().UTC()
	position := len(existingStreams)

	newStream := &model.Stream{
		ID:          uuid.New().String(),
		BrandID:     dstBrand.ID,
		Name:        copyName,
		Slug:        copySlug,
		Description: srcStream.Description,
		Icon:        srcStream.Icon,
		Position:    position,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := writeJSON(r.streamFilePath(toBrand, copySlug), newStream); err != nil {
		os.RemoveAll(dstDir)
		return nil, fmt.Errorf("write copied stream: %w", err)
	}

	// Regenerate IDs in all child projects and categories
	if err := r.regenerateProjectIDs(toBrand, copySlug, newStream.ID, dstBrand.ID); err != nil {
		os.RemoveAll(dstDir)
		return nil, err
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

	if err := copyHierarchyDir(srcDir, dstDir); err != nil {
		os.RemoveAll(dstDir)
		return nil, fmt.Errorf("copy brand directory: %w", err)
	}

	now := time.Now().UTC()
	position := len(existingBrands)

	newBrand := &model.Brand{
		ID:           uuid.New().String(),
		Name:         copyName,
		Slug:         copySlug,
		Description:  srcBrand.Description,
		Icon:         srcBrand.Icon,
		Logo:         srcBrand.Logo,
		Website:      srcBrand.Website,
		SystemPrompt: srcBrand.SystemPrompt,
		Position:     position,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := writeJSON(r.brandFilePath(copySlug), newBrand); err != nil {
		os.RemoveAll(dstDir)
		return nil, fmt.Errorf("write copied brand: %w", err)
	}

	// Regenerate IDs in all child streams, projects, and categories
	if err := r.regenerateStreamIDs(copySlug, newBrand.ID); err != nil {
		os.RemoveAll(dstDir)
		return nil, err
	}

	return newBrand, nil
}

func (r *Repository) regenerateStreamIDs(brandSlug, brandID string) error {
	streams, err := r.ListStreams(brandSlug)
	if err != nil {
		return fmt.Errorf("list copied streams: %w", err)
	}
	for _, s := range streams {
		newStreamID := uuid.New().String()
		if _, err := r.UpdateStream(brandSlug, s.Slug, func(st *model.Stream) {
			st.ID = newStreamID
			st.BrandID = brandID
		}); err != nil {
			return fmt.Errorf("regenerate stream %q ID: %w", s.Slug, err)
		}
		if err := r.regenerateProjectIDs(brandSlug, s.Slug, newStreamID, brandID); err != nil {
			return err
		}
	}
	return nil
}
