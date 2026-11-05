package boardtools

// Tools that change a project itself rather than a card: moving a card
// between its categories, the project's name and icon, its tag
// vocabulary and its categories. Once project chat's own copies, now the
// registry's, so every surface has them. A project is named by brand,
// stream and project (a scoped chat fills in its own); categories and
// tags by id or, for one made earlier in the same session, by name.

import (
	"fmt"
	"strings"

	cardtools "bruv/core/runtime/tools"
	"bruv/internal/model"
)

// projectRef is an existing project, by slugs.
type projectRef struct{ brand, stream, project, name string }

// existingProject resolves a call's brand/stream/project without creating
// anything.
func existingProject(rt Board, a map[string]any) (projectRef, error) {
	loc := cardtools.LocationArgs(a)
	if loc.Brand == "" || loc.Stream == "" || loc.Project == "" {
		return projectRef{}, fmt.Errorf("brand, stream and project are required")
	}
	ps := rt.ProjectService()
	brand, _, ok := cardtools.FindBrand(ps, loc.Brand)
	if !ok {
		return projectRef{}, fmt.Errorf("brand %q not found", loc.Brand)
	}
	stream, _, ok := cardtools.FindStream(ps, brand, loc.Stream)
	if !ok {
		return projectRef{}, fmt.Errorf("stream %q not found", loc.Stream)
	}
	project, name, ok := cardtools.FindProject(ps, brand, stream, loc.Project)
	if !ok {
		return projectRef{}, fmt.Errorf("project %q not found", loc.Project)
	}
	return projectRef{brand, stream, project, name}, nil
}

// findCategory finds one of the project's categories by id, else by name
// or slug (an id the model mixed up still gets the name's chance). A name
// two categories share is refused rather than guessed.
func findCategory(rt Board, p projectRef, id, name string) (*model.Category, error) {
	if id == "" && name == "" {
		return nil, fmt.Errorf("a category id or name is required")
	}
	cats, err := rt.ListCategories(p.brand, p.stream, p.project)
	if err != nil {
		return nil, err
	}
	for i := range cats {
		if id != "" && cats[i].ID == id {
			return &cats[i], nil
		}
	}
	var named []*model.Category
	for i := range cats {
		if name != "" && (strings.EqualFold(cats[i].Name, name) || strings.EqualFold(cats[i].Slug, name)) {
			named = append(named, &cats[i])
		}
	}
	switch len(named) {
	case 0:
		return nil, fmt.Errorf("no category %q in %s (see list_categories)", firstNonEmpty(name, id), p.name)
	case 1:
		return named[0], nil
	}
	ids := make([]string, len(named))
	for i, c := range named {
		ids[i] = c.ID
	}
	return nil, fmt.Errorf("%d categories in %s are called %q (ids %s); pass the category's id", len(named), p.name, name, strings.Join(ids, ", "))
}

// findTag finds a tag in the project's vocabulary by id, else by name.
func findTag(rt Board, p projectRef, id, name string) (*model.Label, error) {
	if id == "" && name == "" {
		return nil, fmt.Errorf("tag_id or tag_name is required")
	}
	labels, err := rt.CatalogService().GetProjectLabels(p.brand, p.stream, p.project)
	if err != nil {
		return nil, err
	}
	for i := range labels {
		if (id != "" && labels[i].ID == id) || (name != "" && strings.EqualFold(labels[i].Name, name)) {
			return &labels[i], nil
		}
	}
	return nil, fmt.Errorf("no tag %q in %s", firstNonEmpty(name, id), p.name)
}

// optStr is an optional string argument: its value, and whether it was given.
func optStr(a map[string]any, key string) (string, bool) {
	v, ok := a[key].(string)
	return v, ok
}

func hMoveCard(rt Board, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	if cardID == "" {
		return errResult("card_id is required")
	}
	p, err := existingProject(rt, a)
	if err != nil {
		return errResult("%v", err)
	}
	to, err := findCategory(rt, p, argStr(a, "to_category_id"), argStr(a, "to_category_name"))
	if err != nil {
		return errResult("%v", err)
	}
	from := argStr(a, "from_category_id")
	if from == "" {
		if from, err = currentCategory(rt, p, cardID); err != nil {
			return errResult("%v", err)
		}
	}
	if from == to.ID {
		return errResult("the card is already in %s", to.Name)
	}
	if err := rt.CardService().MoveToCategory(cardID, from, to.ID, 0); err != nil {
		return errResult("%v", err)
	}
	return jsonResult(map[string]any{"card_id": cardID, "moved_to": to.Name, "category_id": to.ID})
}

// currentCategory is the category of the project a card is pinned in
// (the first, if several).
func currentCategory(rt Board, p projectRef, cardID string) (string, error) {
	cats, err := rt.ListCategories(p.brand, p.stream, p.project)
	if err != nil {
		return "", err
	}
	for _, c := range cats {
		pins, _ := rt.Repo().ListCardsInCategory(c.ID)
		for _, pin := range pins {
			if pin.CardID == cardID {
				return c.ID, nil
			}
		}
	}
	return "", fmt.Errorf("card %s isn't in any category of %s; file it with pin_card", cardID, p.name)
}

func hUpdateProject(rt Board, a map[string]any) (string, bool) {
	p, err := existingProject(rt, a)
	if err != nil {
		return errResult("%v", err)
	}
	ps := rt.ProjectService()
	var changed []string
	// Description and icon first: a rename may change the slug.
	if v, ok := optStr(a, "description"); ok {
		if _, err := ps.UpdateProjectDescription(p.brand, p.stream, p.project, v); err != nil {
			return errResult("description: %v", err)
		}
		changed = append(changed, "description")
	}
	if v, ok := optStr(a, "icon"); ok {
		if _, err := ps.UpdateProjectIcon(p.brand, p.stream, p.project, v); err != nil {
			return errResult("icon: %v", err)
		}
		changed = append(changed, "icon")
	}
	if v := strings.TrimSpace(argStr(a, "name")); v != "" {
		if _, err := ps.RenameProject(p.brand, p.stream, p.project, v); err != nil {
			return errResult("name: %v", err)
		}
		changed = append(changed, "name")
	}
	if len(changed) == 0 {
		return errResult("nothing to change: pass name, description or icon")
	}
	return jsonResult(map[string]any{"project": p.name, "updated": changed})
}

func hCreateProjectTag(rt Board, a map[string]any) (string, bool) {
	name := strings.TrimSpace(argStr(a, "name"))
	if name == "" {
		return errResult("name is required")
	}
	p, err := existingProject(rt, a)
	if err != nil {
		return errResult("%v", err)
	}
	cat := rt.CatalogService()
	if _, err := cat.AddProjectLabel(p.brand, p.stream, p.project, name, argStr(a, "color")); err != nil {
		return errResult("%v", err)
	}
	tag, err := findTag(rt, p, "", name)
	if err != nil {
		return errResult("%v", err)
	}
	if icon := argStr(a, "icon"); icon != "" {
		if _, err := cat.SetProjectLabelIcon(p.brand, p.stream, p.project, tag.ID, icon); err != nil {
			return errResult("icon: %v", err)
		}
	}
	return jsonResult(map[string]any{"tag_id": tag.ID, "name": tag.Name, "color": tag.Color})
}

func hUpdateProjectTag(rt Board, a map[string]any) (string, bool) {
	p, err := existingProject(rt, a)
	if err != nil {
		return errResult("%v", err)
	}
	tag, err := findTag(rt, p, argStr(a, "tag_id"), argStr(a, "tag_name"))
	if err != nil {
		return errResult("%v", err)
	}
	cat := rt.CatalogService()
	var changed []string
	name, hasName := optStr(a, "name")
	color, hasColor := optStr(a, "color")
	if hasName || hasColor {
		// Empty keeps the current value.
		if _, err := cat.UpdateProjectLabel(p.brand, p.stream, p.project, tag.ID, name, color); err != nil {
			return errResult("%v", err)
		}
		if hasName {
			changed = append(changed, "name")
		}
		if hasColor {
			changed = append(changed, "color")
		}
	}
	if icon, ok := optStr(a, "icon"); ok {
		if _, err := cat.SetProjectLabelIcon(p.brand, p.stream, p.project, tag.ID, icon); err != nil {
			return errResult("icon: %v", err)
		}
		changed = append(changed, "icon")
	}
	if len(changed) == 0 {
		return errResult("nothing to change: pass name, color or icon")
	}
	return jsonResult(map[string]any{"tag_id": tag.ID, "updated": changed})
}

func hDeleteProjectTag(rt Board, a map[string]any) (string, bool) {
	p, err := existingProject(rt, a)
	if err != nil {
		return errResult("%v", err)
	}
	tag, err := findTag(rt, p, argStr(a, "tag_id"), argStr(a, "tag_name"))
	if err != nil {
		return errResult("%v", err)
	}
	if _, err := rt.CatalogService().RemoveProjectLabel(p.brand, p.stream, p.project, tag.ID); err != nil {
		return errResult("%v", err)
	}
	return jsonResult(map[string]any{"deleted_tag": tag.Name})
}

func hUpdateCategory(rt Board, a map[string]any) (string, bool) {
	p, err := existingProject(rt, a)
	if err != nil {
		return errResult("%v", err)
	}
	c, err := findCategory(rt, p, argStr(a, "category_id"), argStr(a, "category_name"))
	if err != nil {
		return errResult("%v", err)
	}
	ps := rt.ProjectService()
	var changed []string
	// Everything keyed by the slug before a rename changes it.
	if v, ok := optStr(a, "description"); ok {
		if _, err := ps.UpdateCategoryDescription(p.brand, p.stream, p.project, c.Slug, v); err != nil {
			return errResult("description: %v", err)
		}
		changed = append(changed, "description")
	}
	if v, ok := optStr(a, "icon"); ok {
		if _, err := ps.UpdateCategoryIcon(p.brand, p.stream, p.project, c.Slug, v); err != nil {
			return errResult("icon: %v", err)
		}
		changed = append(changed, "icon")
	}
	if _, ok := a["accepted_types"]; ok {
		types := make([]string, 0)
		for _, input := range argStrSlice(a, "accepted_types") {
			id, err := rt.CatalogService().ResolveType(input)
			if err != nil {
				return typeErrResult(err)
			}
			types = append(types, id)
		}
		if _, err := ps.UpdateCategoryAcceptedTypes(p.brand, p.stream, p.project, c.Slug, types); err != nil {
			return errResult("accepted_types: %v", err)
		}
		changed = append(changed, "accepted_types")
	}
	if v := strings.TrimSpace(argStr(a, "name")); v != "" {
		if _, err := ps.RenameCategory(p.brand, p.stream, p.project, c.Slug, v); err != nil {
			return errResult("name: %v", err)
		}
		changed = append(changed, "name")
	}
	if len(changed) == 0 {
		return errResult("nothing to change: pass name, description, icon or accepted_types")
	}
	return jsonResult(map[string]any{"category_id": c.ID, "updated": changed})
}

func hDeleteCategory(rt Board, a map[string]any) (string, bool) {
	p, err := existingProject(rt, a)
	if err != nil {
		return errResult("%v", err)
	}
	c, err := findCategory(rt, p, argStr(a, "category_id"), argStr(a, "category_name"))
	if err != nil {
		return errResult("%v", err)
	}
	if err := rt.ProjectService().DeleteCategory(p.brand, p.stream, p.project, c.Slug); err != nil {
		return errResult("%v", err)
	}
	return jsonResult(map[string]any{"deleted_category": c.Name})
}
