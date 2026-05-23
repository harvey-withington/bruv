// Package project is the ProjectService — CRUD + move/copy/reorder for
// the four levels of BRUV's hierarchy: brand, stream, project, category.
// "Project" as the package name is a small lie of convenience; all four
// concepts live here because they share a repo layer, event topics, and
// delete-unpinning semantics.
package project

import (
	"bruv/internal/config"
	"bruv/internal/index"
	"bruv/internal/model"
	"bruv/internal/repo"
	"fmt"
	"log/slog"
)

// Deps is the narrow host contract for ProjectService.
type Deps interface {
	Repo() *repo.Repository
	Index() *index.Index
	// Publish announces a domain event. Every mutation below publishes
	// either "<level>:updated" (on create/rename/update) or
	// "<level>:deleted" (on delete), where level is brand, stream,
	// project, or category. Same-user-multi-device relies on this.
	Publish(topic string, payload any)
}

// Service exposes brand/stream/project/category operations.
type Service struct{ deps Deps }

// New constructs a ProjectService.
func New(deps Deps) *Service { return &Service{deps: deps} }

// idxRefresh is a no-op when no index is open; otherwise it runs the
// incremental refresh. Errors are logged and not returned — callers
// treat index freshness as best-effort.
func (s *Service) idxRefresh() {
	idx, r := s.deps.Index(), s.deps.Repo()
	if idx == nil || r == nil {
		return
	}
	if _, err := idx.IncrementalRefresh(r.Root); err != nil {
		slog.Warn("index incremental refresh failed", "err", err)
	}
}

// emit publishes a domain event with the entity (nil-safe).
func (s *Service) emit(topic string, payload any) { s.deps.Publish(topic, payload) }

// --- Brand ---

func (s *Service) CreateBrand(name string) (*model.Brand, error) {
	name = repo.SanitizeText(name)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	brand, err := r.CreateBrand(name)
	if err == nil {
		s.emit("brand:updated", brand)
	}
	return brand, err
}

func (s *Service) GetBrand(slug string) (*model.Brand, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.GetBrand(slug)
}

func (s *Service) ListBrands() ([]model.Brand, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.ListBrands()
}

func (s *Service) RenameBrand(slug, newName string) (*model.Brand, error) {
	newName = repo.SanitizeText(newName)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	brand, err := r.RenameBrand(slug, newName)
	if err != nil {
		return nil, err
	}
	s.idxRefresh()
	s.emit("brand:updated", brand)
	return brand, nil
}

func (s *Service) UpdateBrandDescription(slug, description string) (*model.Brand, error) {
	description = repo.SanitizeText(description)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	brand, err := r.UpdateBrandDescription(slug, description)
	if err == nil {
		s.emit("brand:updated", brand)
	}
	return brand, err
}

func (s *Service) UpdateBrandIcon(slug, icon string) (*model.Brand, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	brand, err := r.UpdateBrandIcon(slug, icon)
	if err == nil {
		s.emit("brand:updated", brand)
	}
	return brand, err
}

func (s *Service) DeleteBrand(slug string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	streams, _ := r.ListStreams(slug)
	for _, stream := range streams {
		projects, _ := r.ListProjects(slug, stream.Slug)
		for _, proj := range projects {
			cats, _ := r.ListCategories(slug, stream.Slug, proj.Slug)
			for _, cat := range cats {
				pins, _ := r.ListCardsInCategory(cat.ID)
				for _, p := range pins {
					_ = r.UnpinCard(p.CardID, p.CategoryID)
				}
			}
		}
	}
	err := r.DeleteBrand(slug)
	if err == nil {
		s.idxRefresh()
		s.emit("brand:deleted", map[string]any{"slug": slug})
	}
	return err
}

// --- Stream ---

func (s *Service) CreateStream(brandSlug, name string) (*model.Stream, error) {
	name = repo.SanitizeText(name)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	stream, err := r.CreateStream(brandSlug, name)
	if err == nil {
		s.emit("stream:updated", stream)
	}
	return stream, err
}

func (s *Service) ListStreams(brandSlug string) ([]model.Stream, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.ListStreams(brandSlug)
}

func (s *Service) RenameStream(brandSlug, streamSlug, newName string) (*model.Stream, error) {
	newName = repo.SanitizeText(newName)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	stream, err := r.RenameStream(brandSlug, streamSlug, newName)
	if err != nil {
		return nil, err
	}
	s.idxRefresh()
	s.emit("stream:updated", stream)
	return stream, nil
}

func (s *Service) UpdateStreamDescription(brandSlug, streamSlug, description string) (*model.Stream, error) {
	description = repo.SanitizeText(description)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	stream, err := r.UpdateStreamDescription(brandSlug, streamSlug, description)
	if err == nil {
		s.emit("stream:updated", stream)
	}
	return stream, err
}

func (s *Service) UpdateStreamIcon(brandSlug, streamSlug, icon string) (*model.Stream, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	stream, err := r.UpdateStreamIcon(brandSlug, streamSlug, icon)
	if err == nil {
		s.emit("stream:updated", stream)
	}
	return stream, err
}

func (s *Service) DeleteStream(brandSlug, streamSlug string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	projects, _ := r.ListProjects(brandSlug, streamSlug)
	for _, proj := range projects {
		cats, _ := r.ListCategories(brandSlug, streamSlug, proj.Slug)
		for _, cat := range cats {
			pins, _ := r.ListCardsInCategory(cat.ID)
			for _, p := range pins {
				_ = r.UnpinCard(p.CardID, p.CategoryID)
			}
		}
	}
	err := r.DeleteStream(brandSlug, streamSlug)
	if err == nil {
		s.idxRefresh()
		s.emit("stream:deleted", map[string]any{"brandSlug": brandSlug, "streamSlug": streamSlug})
	}
	return err
}

// --- Project ---

func (s *Service) CreateProject(brandSlug, streamSlug, name string) (*model.Project, error) {
	name = repo.SanitizeText(name)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	project, err := r.CreateProject(brandSlug, streamSlug, name)
	if err != nil {
		return nil, err
	}
	prefs, _ := config.LoadPreferences()
	catName := prefs.DefaultCategoryName
	if catName == "" {
		catName = "Ideas"
	}
	r.CreateCategory(brandSlug, streamSlug, project.Slug, catName, 0)
	s.emit("project:updated", project)
	return project, nil
}

func (s *Service) ListProjects(brandSlug, streamSlug string) ([]model.Project, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.ListProjects(brandSlug, streamSlug)
}

func (s *Service) RenameProject(brandSlug, streamSlug, projectSlug, newName string) (*model.Project, error) {
	newName = repo.SanitizeText(newName)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	project, err := r.RenameProject(brandSlug, streamSlug, projectSlug, newName)
	if err != nil {
		return nil, err
	}
	s.idxRefresh()
	s.emit("project:updated", project)
	return project, nil
}

func (s *Service) UpdateProjectDescription(brandSlug, streamSlug, projectSlug, description string) (*model.Project, error) {
	description = repo.SanitizeText(description)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	project, err := r.UpdateProjectDescription(brandSlug, streamSlug, projectSlug, description)
	if err == nil {
		s.emit("project:updated", project)
	}
	return project, err
}

func (s *Service) UpdateProjectIcon(brandSlug, streamSlug, projectSlug, icon string) (*model.Project, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	project, err := r.UpdateProjectIcon(brandSlug, streamSlug, projectSlug, icon)
	if err == nil {
		s.emit("project:updated", project)
	}
	return project, err
}

func (s *Service) DeleteProject(brandSlug, streamSlug, projectSlug string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	cats, _ := r.ListCategories(brandSlug, streamSlug, projectSlug)
	for _, cat := range cats {
		pins, _ := r.ListCardsInCategory(cat.ID)
		for _, p := range pins {
			_ = r.UnpinCard(p.CardID, p.CategoryID)
		}
	}
	err := r.DeleteProject(brandSlug, streamSlug, projectSlug)
	if err == nil {
		s.idxRefresh()
		s.emit("project:deleted", map[string]any{
			"brandSlug": brandSlug, "streamSlug": streamSlug, "projectSlug": projectSlug,
		})
	}
	return err
}

func (s *Service) GetProjectMembers(brandSlug, streamSlug, projectSlug string) ([]model.ProjectMember, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.GetProjectMembers(brandSlug, streamSlug, projectSlug)
}

// --- Category ---

func (s *Service) CreateCategory(brandSlug, streamSlug, projectSlug, name string, position int) (*model.Category, error) {
	name = repo.SanitizeText(name)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cat, err := r.CreateCategory(brandSlug, streamSlug, projectSlug, name, position)
	if err == nil {
		s.emit("category:updated", cat)
	}
	return cat, err
}

func (s *Service) ListCategories(brandSlug, streamSlug, projectSlug string) ([]model.Category, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.ListCategories(brandSlug, streamSlug, projectSlug)
}

func (s *Service) RenameCategory(brandSlug, streamSlug, projectSlug, categorySlug, newName string) (*model.Category, error) {
	newName = repo.SanitizeText(newName)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cat, err := r.RenameCategory(brandSlug, streamSlug, projectSlug, categorySlug, newName)
	if err != nil {
		return nil, err
	}
	s.idxRefresh()
	s.emit("category:updated", cat)
	return cat, nil
}

func (s *Service) UpdateCategoryDescription(brandSlug, streamSlug, projectSlug, categorySlug, description string) (*model.Category, error) {
	description = repo.SanitizeText(description)
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cat, err := r.UpdateCategoryDescription(brandSlug, streamSlug, projectSlug, categorySlug, description)
	if err == nil {
		s.emit("category:updated", cat)
	}
	return cat, err
}

func (s *Service) UpdateCategoryIcon(brandSlug, streamSlug, projectSlug, categorySlug, icon string) (*model.Category, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cat, err := r.UpdateCategoryIcon(brandSlug, streamSlug, projectSlug, categorySlug, icon)
	if err == nil {
		s.emit("category:updated", cat)
	}
	return cat, err
}

func (s *Service) DeleteCategory(brandSlug, streamSlug, projectSlug, categorySlug string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	cats, _ := r.ListCategories(brandSlug, streamSlug, projectSlug)
	if len(cats) <= 1 {
		return fmt.Errorf("cannot delete the last category in a project")
	}
	cat, err := r.GetCategory(brandSlug, streamSlug, projectSlug, categorySlug)
	if err == nil {
		pins, _ := r.ListCardsInCategory(cat.ID)
		for _, p := range pins {
			_ = r.UnpinCard(p.CardID, p.CategoryID)
		}
	}
	err = r.DeleteCategory(brandSlug, streamSlug, projectSlug, categorySlug)
	if err == nil {
		s.idxRefresh()
		s.emit("category:deleted", map[string]any{
			"brandSlug": brandSlug, "streamSlug": streamSlug,
			"projectSlug": projectSlug, "categorySlug": categorySlug,
		})
	}
	return err
}

func (s *Service) UpdateCategoryAcceptedTypes(brandSlug, streamSlug, projectSlug, categorySlug string, acceptedTypes []string) (*model.Category, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return r.UpdateCategory(brandSlug, streamSlug, projectSlug, categorySlug, func(c *model.Category) {
		c.AcceptedTypes = acceptedTypes
	})
}

// MoveCategoryCards moves every card pin from one category to another
// and deletes the source category. Re-indexes pins as it goes.
func (s *Service) MoveCategoryCards(brandSlug, streamSlug, projectSlug, fromCategoryID, toCategoryID string) error {
	r, idx := s.deps.Repo(), s.deps.Index()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	if idx == nil {
		return fmt.Errorf("no index available")
	}
	cardIDs, err := idx.ListCardIDsInCategory(fromCategoryID)
	if err != nil {
		return fmt.Errorf("list cards in category: %w", err)
	}
	for i, cardID := range cardIDs {
		if err := r.MoveCardToCategory(cardID, fromCategoryID, toCategoryID, i); err != nil {
			return fmt.Errorf("move card %s: %w", cardID, err)
		}
		if pins, err := r.GetCardPins(cardID); err == nil {
			if ierr := idx.IndexPins(cardID, pins); ierr != nil {
				slog.Warn("index pins failed", "card", cardID, "err", ierr)
			}
		}
	}
	return nil
}

// --- Move ---

func (s *Service) MoveProject(fromBrand, fromStream, projectSlug, toBrand, toStream string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	_, err := r.MoveProject(fromBrand, fromStream, projectSlug, toBrand, toStream)
	return err
}

func (s *Service) MoveStream(fromBrand, streamSlug, toBrand string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	_, err := r.MoveStream(fromBrand, streamSlug, toBrand)
	return err
}

// --- Copy ---
//
// Deep-copies need to duplicate cards in the source project's categories
// and re-pin them to the new categories. The helpers snapshotCatIDs +
// duplicateCardsForProject implement that pattern once for brand/stream/
// project copies.

func (s *Service) snapshotCatIDs(brand, stream, project string) map[string]string {
	r := s.deps.Repo()
	cats, _ := r.ListCategories(brand, stream, project)
	m := make(map[string]string, len(cats))
	for _, c := range cats {
		m[c.Slug] = c.ID
	}
	return m
}

func (s *Service) duplicateCardsForProject(oldCatIDs, newCatIDs map[string]string) {
	r, idx := s.deps.Repo(), s.deps.Index()
	if idx == nil || r == nil {
		return
	}
	for slug, oldCatID := range oldCatIDs {
		newCatID, ok := newCatIDs[slug]
		if !ok {
			continue
		}
		cardIDs, err := idx.ListCardIDsInCategory(oldCatID)
		if err != nil || len(cardIDs) == 0 {
			continue
		}
		for i, cardID := range cardIDs {
			newCard, err := r.DuplicateCard(cardID)
			if err != nil {
				continue
			}
			_ = r.PinCardAt(newCard.ID, newCatID, i)
		}
	}
}

func (s *Service) CopyBrand(brandSlug string) (*model.Brand, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	type projCatSnapshot struct {
		streamSlug, projectSlug string
		catIDs                  map[string]string
	}
	var snapshots []projCatSnapshot
	srcStreams, _ := r.ListStreams(brandSlug)
	for _, st := range srcStreams {
		projects, _ := r.ListProjects(brandSlug, st.Slug)
		for _, p := range projects {
			snapshots = append(snapshots, projCatSnapshot{
				streamSlug: st.Slug, projectSlug: p.Slug,
				catIDs: s.snapshotCatIDs(brandSlug, st.Slug, p.Slug),
			})
		}
	}
	result, err := r.CopyBrand(brandSlug)
	if err != nil {
		return nil, err
	}
	for _, snap := range snapshots {
		newCatIDs := s.snapshotCatIDs(result.Slug, snap.streamSlug, snap.projectSlug)
		s.duplicateCardsForProject(snap.catIDs, newCatIDs)
	}
	s.idxRefresh()
	return result, nil
}

func (s *Service) CopyStream(fromBrand, streamSlug, toBrand string) (*model.Stream, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	type projCatSnapshot struct {
		projectSlug string
		catIDs      map[string]string
	}
	var snapshots []projCatSnapshot
	srcProjects, _ := r.ListProjects(fromBrand, streamSlug)
	for _, p := range srcProjects {
		snapshots = append(snapshots, projCatSnapshot{
			projectSlug: p.Slug,
			catIDs:      s.snapshotCatIDs(fromBrand, streamSlug, p.Slug),
		})
	}
	result, err := r.CopyStream(fromBrand, streamSlug, toBrand)
	if err != nil {
		return nil, err
	}
	for _, snap := range snapshots {
		newCatIDs := s.snapshotCatIDs(toBrand, result.Slug, snap.projectSlug)
		s.duplicateCardsForProject(snap.catIDs, newCatIDs)
	}
	s.idxRefresh()
	return result, nil
}

func (s *Service) CopyProject(fromBrand, fromStream, projectSlug, toBrand, toStream string, position int) (*model.Project, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	oldCatIDs := s.snapshotCatIDs(fromBrand, fromStream, projectSlug)
	result, err := r.CopyProject(fromBrand, fromStream, projectSlug, toBrand, toStream, position)
	if err != nil {
		return nil, err
	}
	newCatIDs := s.snapshotCatIDs(toBrand, toStream, result.Slug)
	s.duplicateCardsForProject(oldCatIDs, newCatIDs)
	s.idxRefresh()
	return result, nil
}

// --- Reorder ---

func (s *Service) ReorderBrands(orderedSlugs []string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	return r.ReorderBrands(orderedSlugs)
}

func (s *Service) ReorderStreams(brandSlug string, orderedSlugs []string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	return r.ReorderStreams(brandSlug, orderedSlugs)
}

func (s *Service) ReorderProjects(brandSlug, streamSlug string, orderedSlugs []string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	return r.ReorderProjects(brandSlug, streamSlug, orderedSlugs)
}

func (s *Service) ReorderCategories(brandSlug, streamSlug, projectSlug string, orderedSlugs []string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	return r.ReorderCategories(brandSlug, streamSlug, projectSlug, orderedSlugs)
}
