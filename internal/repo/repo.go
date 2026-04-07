package repo

import (
	"bruv/internal/model"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Repository represents an open BRUV repository on disk.
type Repository struct {
	Root     string
	Manifest *model.Manifest
}

// Directory layout constants
const (
	bruvDir       = ".bruv"
	manifestFile  = "manifest.json"
	brandsDir     = "brands"
	cardsDir      = "cards"
	pinsDir       = "pins"
	typesDir      = "types"
	streamsDir    = "streams"
	projectsDir   = "projects"
	categoriesDir = "categories"
)

// Init creates a new BRUV repository inside a subfolder of basePath,
// named after the slugified repo name.
func Init(basePath string, name string) (*Repository, error) {
	basePath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	slug := Slugify(name)
	if slug == "" {
		return nil, fmt.Errorf("invalid repository name %q", name)
	}
	root := filepath.Join(basePath, slug)

	metaDir := filepath.Join(root, bruvDir)
	if fileExists(filepath.Join(metaDir, manifestFile)) {
		return nil, fmt.Errorf("repository already exists at %s", root)
	}

	dirs := []string{
		metaDir,
		filepath.Join(root, brandsDir),
		filepath.Join(root, cardsDir),
		filepath.Join(root, pinsDir),
		filepath.Join(root, typesDir),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	now := time.Now().UTC()
	manifest := &model.Manifest{
		Version:   "0.1.0",
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mpath := filepath.Join(metaDir, manifestFile)
	if err := writeJSON(mpath, manifest); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	return &Repository{Root: root, Manifest: manifest}, nil
}

// Open opens an existing BRUV repository at the given root path.
func Open(root string) (*Repository, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	mpath := filepath.Join(root, bruvDir, manifestFile)
	if !fileExists(mpath) {
		return nil, fmt.Errorf("no BRUV repository found at %s", root)
	}

	var manifest model.Manifest
	if err := readJSON(mpath, &manifest); err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	return &Repository{Root: root, Manifest: &manifest}, nil
}

// UpdateManifestDescription sets or clears the repository description.
func (r *Repository) UpdateManifestDescription(description string) error {
	r.Manifest.Description = description
	r.Manifest.UpdatedAt = time.Now().UTC()
	mpath := filepath.Join(r.Root, bruvDir, manifestFile)
	if err := writeJSON(mpath, r.Manifest); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

// Path helpers

func (r *Repository) brandsPath() string {
	return filepath.Join(r.Root, brandsDir)
}

func (r *Repository) brandPath(slug string) string {
	return filepath.Join(r.Root, brandsDir, slug)
}

func (r *Repository) brandFilePath(slug string) string {
	return filepath.Join(r.brandPath(slug), "brand.json")
}

func (r *Repository) streamsPath(brandSlug string) string {
	return filepath.Join(r.brandPath(brandSlug), streamsDir)
}

func (r *Repository) streamPath(brandSlug, streamSlug string) string {
	return filepath.Join(r.streamsPath(brandSlug), streamSlug)
}

func (r *Repository) streamFilePath(brandSlug, streamSlug string) string {
	return filepath.Join(r.streamPath(brandSlug, streamSlug), "stream.json")
}

func (r *Repository) projectsPath(brandSlug, streamSlug string) string {
	return filepath.Join(r.streamPath(brandSlug, streamSlug), projectsDir)
}

func (r *Repository) projectPath(brandSlug, streamSlug, projectSlug string) string {
	return filepath.Join(r.projectsPath(brandSlug, streamSlug), projectSlug)
}

func (r *Repository) projectFilePath(brandSlug, streamSlug, projectSlug string) string {
	return filepath.Join(r.projectPath(brandSlug, streamSlug, projectSlug), "project.json")
}

func (r *Repository) categoriesPath(brandSlug, streamSlug, projectSlug string) string {
	return filepath.Join(r.projectPath(brandSlug, streamSlug, projectSlug), categoriesDir)
}

func (r *Repository) categoryFilePath(brandSlug, streamSlug, projectSlug, categorySlug string) string {
	return filepath.Join(r.categoriesPath(brandSlug, streamSlug, projectSlug), categorySlug+".json")
}

func (r *Repository) cardsPath() string {
	return filepath.Join(r.Root, cardsDir)
}

func (r *Repository) cardFilePath(id string) string {
	return filepath.Join(r.Root, cardsDir, id+".json")
}

func (r *Repository) chatFilePath(cardID string) string {
	return filepath.Join(r.Root, cardsDir, cardID+".messages.json")
}

func (r *Repository) pinsDirPath(cardID string) string {
	return filepath.Join(r.Root, pinsDir, cardID)
}

func (r *Repository) pinsFilePath(cardID string) string {
	return filepath.Join(r.pinsDirPath(cardID), "pins.json")
}
