package repo

import (
	"bruv/internal/model"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Repository represents an open BRUV repository on disk.
type Repository struct {
	Root     string
	Manifest *model.Manifest

	// RunsDir is the server-side directory holding agent run history
	// (one <cardID>.json per card). Kept OUTSIDE the repo tree so
	// runs don't travel with the repo when users share via git /
	// Syncthing. Empty string = legacy merged-file storage (runs
	// embedded in the in-repo .agent.json). Set via SetRunsDir
	// after Open/Init; see internal/repo/agent.go for the split
	// rationale.
	RunsDir string

	// BeforeDirOp, when set, is invoked before any rename / delete of
	// a watched directory subtree (brand, stream, project). The hook
	// returns a cleanup closure run after the operation completes
	// (success or failure). The wiring layer uses this to detach the
	// fsnotify watcher from the target subtree and reattach the parent
	// afterwards — required on Windows, where ReadDirectoryChangesW
	// keeps a pending IRP on each watched dir handle and the OS
	// refuses to rename / remove a directory while IRPs are pending,
	// even with FILE_SHARE_DELETE.
	//
	// Optional. nil = no-op (used by tests / headless flows that
	// don't run a watcher). The hook MUST always return a non-nil
	// cleanup func when set, even on detach failure, so callers can
	// `defer cleanup()` unconditionally.
	BeforeDirOp DirOpHook
}

// DirOpHook is the signature for the BeforeDirOp callback.
type DirOpHook func(targetPath string) (cleanup func())

// withDirOp invokes BeforeDirOp if set and returns the cleanup
// closure (or a no-op when no hook is wired). Caller defers the
// returned func.
func (r *Repository) withDirOp(targetPath string) func() {
	if r.BeforeDirOp == nil {
		return func() {}
	}
	cleanup := r.BeforeDirOp(targetPath)
	if cleanup == nil {
		return func() {}
	}
	return cleanup
}

// Directory layout constants
const (
	// bruvDir holds derived state only (SQLite index, advisory lock,
	// future caches). Authoritative data lives at the repo root so it
	// travels when the repo is shared via git/Dropbox/Syncthing — the
	// .bruv/ folder is fully gitignored by EnsureSyncHygiene.
	bruvDir       = ".bruv"
	manifestFile  = "manifest.json"
	brandsDir     = "brands"
	cardsDir      = "cards"
	pinsDir       = "pins"
	typesDir      = "types"
	streamsDir    = "streams"
	projectsDir   = "projects"
	categoriesDir = "categories"
	activityDir   = "activity"
)

// Init creates a new BRUV repository inside a subfolder of basePath,
// named after the slugified repo name. Used by the desktop app's
// "create repo" flow where the user picks a parent directory.
func Init(basePath string, name string) (*Repository, error) {
	basePath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	slug := Slugify(name)
	if slug == "" {
		return nil, fmt.Errorf("invalid repository name %q", name)
	}
	return InitAt(filepath.Join(basePath, slug), name)
}

// InitAt creates a new BRUV repository at exactly the given root
// path (no slug rewriting). Used by the server-install flow where
// the operator picks the literal repo location.
func InitAt(root string, name string) (*Repository, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	if fileExists(filepath.Join(root, manifestFile)) || fileExists(filepath.Join(root, bruvDir, manifestFile)) {
		return nil, fmt.Errorf("repository already exists at %s", root)
	}

	dirs := []string{
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
		ID:        uuid.New().String(),
		Version:   "0.1.0",
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := writeJSON(filepath.Join(root, manifestFile), manifest); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	return &Repository{Root: root, Manifest: manifest}, nil
}

// InspectAt reads the manifest at the given path WITHOUT opening the
// repository (no lock acquisition, no ID backfill, no side effects).
// Returns (nil, nil) when the path is not a BRUV repo — callers use
// this to decide between Init and Open without round-tripping an
// "is this a repo" error string. Distinct return is reserved for
// genuine I/O / parse failures.
func InspectAt(root string) (*model.Manifest, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	mpath := filepath.Join(root, manifestFile)
	if !fileExists(mpath) {
		return nil, nil
	}
	var manifest model.Manifest
	if err := readJSON(mpath, &manifest); err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	return &manifest, nil
}

// Open opens an existing BRUV repository at the given root path.
func Open(root string) (*Repository, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	mpath := filepath.Join(root, manifestFile)
	if !fileExists(mpath) {
		return nil, fmt.Errorf("no BRUV repository found at %s", root)
	}

	var manifest model.Manifest
	if err := readJSON(mpath, &manifest); err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	// Backfill a stable ID on repos created before the field existed.
	// Persisted immediately so later opens are no-ops. The same repo
	// shared to another machine keeps its ID — repoID travels with the
	// data, personal state (chats, etc.) stays keyed by it locally.
	if manifest.ID == "" {
		manifest.ID = uuid.New().String()
		manifest.UpdatedAt = time.Now().UTC()
		if err := writeJSON(mpath, &manifest); err != nil {
			// Non-fatal: if we can't persist the ID, we still let the
			// repo open, but subsequent opens will regenerate. This is
			// rare enough to not block the user on it.
			return nil, fmt.Errorf("backfill manifest id: %w", err)
		}
	}

	return &Repository{Root: root, Manifest: &manifest}, nil
}

// UpdateManifestDescription sets or clears the repository description.
func (r *Repository) UpdateManifestDescription(description string) error {
	r.Manifest.Description = description
	r.Manifest.UpdatedAt = time.Now().UTC()
	if err := writeJSON(filepath.Join(r.Root, manifestFile), r.Manifest); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

// UpdateManifestName renames the repository. The new name is the
// portable identity stored in the manifest at the repo root, so it
// travels with the repo when shared via git/Syncthing/Dropbox. The
// per-machine registry (repos.json) holds its own copy of the name
// for picker display — call sites that touch the registry should
// also sync that label.
func (r *Repository) UpdateManifestName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	r.Manifest.Name = name
	r.Manifest.UpdatedAt = time.Now().UTC()
	if err := writeJSON(filepath.Join(r.Root, manifestFile), r.Manifest); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

// RewriteManifestName updates the manifest at the given path WITHOUT
// requiring an open Repository. Used by the desktop's rename flow to
// edit a Local repo's name from the picker even when that repo isn't
// currently the active one (no Runtime loaded).
func RewriteManifestName(path, name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	root, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	mpath := filepath.Join(root, manifestFile)
	var manifest model.Manifest
	if err := readJSON(mpath, &manifest); err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	manifest.Name = name
	manifest.UpdatedAt = time.Now().UTC()
	if err := writeJSON(mpath, &manifest); err != nil {
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

func (r *Repository) projectMembersFilePath(brandSlug, streamSlug, projectSlug string) string {
	return filepath.Join(r.projectPath(brandSlug, streamSlug, projectSlug), "members.json")
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

// cardTypesPath returns the location of the repo-scoped card types store.
// Lives at the repo root so it travels when the repo is shared.
// See internal/repo/card_types.go for the Load/Save API.
func (r *Repository) cardTypesPath() string {
	return filepath.Join(r.Root, "card_types.json")
}

func (r *Repository) pinsDirPath(cardID string) string {
	return filepath.Join(r.Root, pinsDir, cardID)
}

func (r *Repository) pinsFilePath(cardID string) string {
	return filepath.Join(r.pinsDirPath(cardID), "pins.json")
}
