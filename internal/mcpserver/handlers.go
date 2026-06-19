package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"

	cardtools "bruv/core/runtime/tools"
	"bruv/core/supervisor"
)

// jsonResult marshals v to pretty JSON for the tool's text content.
func jsonResult(v any) (string, bool) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "error: " + err.Error(), true
	}
	return string(b), false
}

func errResult(format string, a ...any) (string, bool) {
	return "error: " + fmt.Sprintf(format, a...), true
}

// --- Discovery / read ---

func hListBrands(rt *supervisor.Runtime, _ map[string]any) (string, bool) {
	brands, err := rt.ListBrands()
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(brands)
}

func hListStreams(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	brand := argStr(a, "brand")
	if brand == "" {
		return errResult("brand is required")
	}
	brandSlug, _, ok := resolveBrandSlug(rt, brand)
	if !ok {
		return errResult("brand %q not found", brand)
	}
	streams, err := rt.ListStreams(brandSlug)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(streams)
}

func hListProjects(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	brand, stream := argStr(a, "brand"), argStr(a, "stream")
	if brand == "" || stream == "" {
		return errResult("brand and stream are required")
	}
	brandSlug, _, ok := resolveBrandSlug(rt, brand)
	if !ok {
		return errResult("brand %q not found", brand)
	}
	streamSlug, _, ok := resolveStreamSlug(rt, brandSlug, stream)
	if !ok {
		return errResult("stream %q not found", stream)
	}
	projects, err := rt.ListProjects(brandSlug, streamSlug)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(projects)
}

func hListCategories(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	brand, stream, project := argStr(a, "brand"), argStr(a, "stream"), argStr(a, "project")
	if brand == "" || stream == "" || project == "" {
		return errResult("brand, stream and project are required")
	}
	brandSlug, _, ok := resolveBrandSlug(rt, brand)
	if !ok {
		return errResult("brand %q not found", brand)
	}
	streamSlug, _, ok := resolveStreamSlug(rt, brandSlug, stream)
	if !ok {
		return errResult("stream %q not found", stream)
	}
	projectSlug, _, ok := resolveProjectSlug(rt, brandSlug, streamSlug, project)
	if !ok {
		return errResult("project %q not found", project)
	}
	cats, err := rt.ListCategories(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(cats)
}

func hListCardTypes(rt *supervisor.Runtime, _ map[string]any) (string, bool) {
	return jsonResult(rt.ListCardTypes())
}

func hGetCard(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	id := argStr(a, "card_id")
	if id == "" {
		return errResult("card_id is required")
	}
	card, err := rt.GetCard(id)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(card)
}

func hSearchCards(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	query := argStr(a, "query")
	if query == "" {
		return errResult("query is required")
	}
	limit := argInt(a, "limit", 20)
	if limit <= 0 {
		limit = 20
	}
	results, err := rt.SearchCards(query, limit)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(results)
}

// --- Create / capture ---

func hCreateBrand(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	name := argStr(a, "name")
	if name == "" {
		return errResult("name is required")
	}
	brand, err := rt.CreateBrand(name)
	if err != nil {
		return errResult("%v", err)
	}
	if desc := argStr(a, "description"); desc != "" {
		if updated, err := rt.UpdateBrandDescription(brand.Slug, desc); err == nil {
			brand = updated
		}
	}
	return jsonResult(brand)
}

func hCreateStream(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	brand, name := argStr(a, "brand"), argStr(a, "name")
	if brand == "" || name == "" {
		return errResult("brand and name are required")
	}
	brandSlug, _, err := ensureBrand(rt, brand)
	if err != nil {
		return errResult("%v", err)
	}
	stream, err := rt.CreateStream(brandSlug, name)
	if err != nil {
		return errResult("%v", err)
	}
	if desc := argStr(a, "description"); desc != "" {
		if updated, err := rt.UpdateStreamDescription(brandSlug, stream.Slug, desc); err == nil {
			stream = updated
		}
	}
	return jsonResult(stream)
}

func hCreateProject(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	brand, stream, name := argStr(a, "brand"), argStr(a, "stream"), argStr(a, "name")
	if brand == "" || stream == "" || name == "" {
		return errResult("brand, stream and name are required")
	}
	brandSlug, _, err := ensureBrand(rt, brand)
	if err != nil {
		return errResult("%v", err)
	}
	streamSlug, _, err := ensureStream(rt, brandSlug, stream)
	if err != nil {
		return errResult("%v", err)
	}
	project, err := rt.CreateProject(brandSlug, streamSlug, name)
	if err != nil {
		return errResult("%v", err)
	}
	if desc := argStr(a, "description"); desc != "" {
		if updated, err := rt.UpdateProjectDescription(brandSlug, streamSlug, project.Slug, desc); err == nil {
			project = updated
		}
	}
	return jsonResult(project)
}

func hCreateCategory(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	brand, stream, project, name := argStr(a, "brand"), argStr(a, "stream"), argStr(a, "project"), argStr(a, "name")
	if brand == "" || stream == "" || project == "" || name == "" {
		return errResult("brand, stream, project and name are required")
	}
	brandSlug, _, err := ensureBrand(rt, brand)
	if err != nil {
		return errResult("%v", err)
	}
	streamSlug, _, err := ensureStream(rt, brandSlug, stream)
	if err != nil {
		return errResult("%v", err)
	}
	projectSlug, _, err := ensureProject(rt, brandSlug, streamSlug, project)
	if err != nil {
		return errResult("%v", err)
	}
	cats, _ := rt.ListCategories(brandSlug, streamSlug, projectSlug)
	position := argInt(a, "position", len(cats))
	cat, err := rt.CreateCategory(brandSlug, streamSlug, projectSlug, name, position)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(cat)
}

func hCreateCard(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	title := argStr(a, "title")
	if title == "" {
		return errResult("title is required")
	}
	cardType := argStr(a, "card_type")
	if cardType == "" {
		cardType = "idea"
	}
	// CreateCard seeds the type's schema blocks automatically.
	card, err := rt.CreateCard(cardType, title)
	if err != nil {
		return errResult("%v", err)
	}
	cardID := card.ID

	// File into the hierarchy if requested — all-or-nothing so we never
	// half-resolve a location.
	brand, stream := argStr(a, "brand"), argStr(a, "stream")
	project, category := argStr(a, "project"), argStr(a, "category")
	anyHierarchy := brand != "" || stream != "" || project != "" || category != ""
	pinnedTo := ""
	if anyHierarchy {
		if brand == "" || stream == "" || project == "" || category == "" {
			return errResult("to file the card, provide all of brand, stream, project and category (or none to leave it in the inbox)")
		}
		catID, breadcrumb, err := resolveOrCreateHierarchy(rt, brand, stream, project, category)
		if err != nil {
			return errResult("%v", err)
		}
		if err := rt.PinCard(cardID, catID); err != nil {
			return errResult("pin card: %v", err)
		}
		pinnedTo = breadcrumb
	}

	if tags := argStrSlice(a, "tags"); len(tags) > 0 {
		if _, err := rt.UpdateCardTags(cardID, tags); err != nil {
			return errResult("set tags: %v", err)
		}
	}
	if desc := argStr(a, "description"); desc != "" {
		if _, err := rt.UpdateCardDescription(cardID, desc); err != nil {
			return errResult("set description: %v", err)
		}
	}
	if blocks := parseBlocks(a["blocks"]); len(blocks) > 0 {
		current, err := rt.GetCard(cardID)
		if err != nil {
			return errResult("reload card: %v", err)
		}
		current.Blocks = append(current.Blocks, blocks...)
		if _, err := rt.UpdateCardBlocks(cardID, current.Blocks); err != nil {
			return errResult("add blocks: %v", err)
		}
	}

	out := map[string]any{"card_id": cardID, "title": title, "type": card.Type}
	if pinnedTo != "" {
		out["pinned_to"] = pinnedTo
	} else {
		out["pinned_to"] = "inbox (unfiled)"
	}
	return jsonResult(out)
}

// --- Populate existing cards ---

func hAddCardBlocks(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	if cardID == "" {
		return errResult("card_id is required")
	}
	blocks := parseBlocks(a["blocks"])
	if len(blocks) == 0 {
		return errResult("blocks is required and must be a non-empty array")
	}
	current, err := rt.GetCard(cardID)
	if err != nil {
		return errResult("%v", err)
	}
	current.Blocks = append(current.Blocks, blocks...)
	if _, err := rt.UpdateCardBlocks(cardID, current.Blocks); err != nil {
		return errResult("%v", err)
	}
	return jsonResult(map[string]any{"card_id": cardID, "blocks_added": len(blocks)})
}

func hSetCardFields(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	if cardID == "" {
		return errResult("card_id is required")
	}
	fields, _ := a["fields"].(map[string]any)
	if len(fields) == 0 {
		return errResult("fields is required and must be a non-empty object")
	}
	card, err := rt.GetCard(cardID)
	if err != nil {
		return errResult("%v", err)
	}
	var updatedKeys []string
	for i := range card.Blocks {
		key := card.Blocks[i].Key
		if key == "" {
			continue
		}
		val, ok := fields[key]
		if !ok {
			continue
		}
		coerced, _ := cardtools.CoerceBlockValueForBlock(&card.Blocks[i], val)
		card.Blocks[i].Value = coerced
		updatedKeys = append(updatedKeys, key)
	}
	if len(updatedKeys) == 0 {
		var available []string
		for _, b := range card.Blocks {
			if b.Key != "" {
				available = append(available, b.Key)
			}
		}
		return errResult("no matching field keys. Available keys: %s", strings.Join(available, ", "))
	}
	if _, err := rt.UpdateCardBlocks(cardID, card.Blocks); err != nil {
		return errResult("%v", err)
	}
	return jsonResult(map[string]any{"card_id": cardID, "updated_fields": updatedKeys})
}

func hAddCardTags(rt *supervisor.Runtime, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	if cardID == "" {
		return errResult("card_id is required")
	}
	newTags := argStrSlice(a, "tags")
	if len(newTags) == 0 {
		return errResult("tags is required and must be a non-empty array")
	}
	card, err := rt.GetCard(cardID)
	if err != nil {
		return errResult("%v", err)
	}
	seen := make(map[string]bool, len(card.Tags))
	for _, t := range card.Tags {
		seen[strings.ToLower(t)] = true
	}
	merged := card.Tags
	var added []string
	for _, t := range newTags {
		if !seen[strings.ToLower(t)] {
			merged = append(merged, t)
			seen[strings.ToLower(t)] = true
			added = append(added, t)
		}
	}
	if len(added) > 0 {
		if _, err := rt.UpdateCardTags(cardID, merged); err != nil {
			return errResult("%v", err)
		}
	}
	return jsonResult(map[string]any{"card_id": cardID, "tags_added": added, "tags": merged})
}

// --- hierarchy resolution ---

func resolveBrandSlug(rt *supervisor.Runtime, nameOrSlug string) (slug, name string, ok bool) {
	brands, _ := rt.ListBrands()
	for _, b := range brands {
		if strings.EqualFold(b.Name, nameOrSlug) || strings.EqualFold(b.Slug, nameOrSlug) {
			return b.Slug, b.Name, true
		}
	}
	return "", "", false
}

func resolveStreamSlug(rt *supervisor.Runtime, brandSlug, nameOrSlug string) (slug, name string, ok bool) {
	streams, _ := rt.ListStreams(brandSlug)
	for _, s := range streams {
		if strings.EqualFold(s.Name, nameOrSlug) || strings.EqualFold(s.Slug, nameOrSlug) {
			return s.Slug, s.Name, true
		}
	}
	return "", "", false
}

func resolveProjectSlug(rt *supervisor.Runtime, brandSlug, streamSlug, nameOrSlug string) (slug, name string, ok bool) {
	projects, _ := rt.ListProjects(brandSlug, streamSlug)
	for _, p := range projects {
		if strings.EqualFold(p.Name, nameOrSlug) || strings.EqualFold(p.Slug, nameOrSlug) {
			return p.Slug, p.Name, true
		}
	}
	return "", "", false
}

func ensureBrand(rt *supervisor.Runtime, nameOrSlug string) (slug, name string, err error) {
	if s, n, ok := resolveBrandSlug(rt, nameOrSlug); ok {
		return s, n, nil
	}
	b, err := rt.CreateBrand(nameOrSlug)
	if err != nil {
		return "", "", fmt.Errorf("create brand %q: %w", nameOrSlug, err)
	}
	return b.Slug, b.Name, nil
}

func ensureStream(rt *supervisor.Runtime, brandSlug, nameOrSlug string) (slug, name string, err error) {
	if s, n, ok := resolveStreamSlug(rt, brandSlug, nameOrSlug); ok {
		return s, n, nil
	}
	s, err := rt.CreateStream(brandSlug, nameOrSlug)
	if err != nil {
		return "", "", fmt.Errorf("create stream %q: %w", nameOrSlug, err)
	}
	return s.Slug, s.Name, nil
}

func ensureProject(rt *supervisor.Runtime, brandSlug, streamSlug, nameOrSlug string) (slug, name string, err error) {
	if s, n, ok := resolveProjectSlug(rt, brandSlug, streamSlug, nameOrSlug); ok {
		return s, n, nil
	}
	p, err := rt.CreateProject(brandSlug, streamSlug, nameOrSlug)
	if err != nil {
		return "", "", fmt.Errorf("create project %q: %w", nameOrSlug, err)
	}
	return p.Slug, p.Name, nil
}

// resolveOrCreateHierarchy walks Brand → Stream → Project → Category,
// creating any level that doesn't already exist, and returns the leaf
// category id plus a human-readable breadcrumb.
func resolveOrCreateHierarchy(rt *supervisor.Runtime, brand, stream, project, category string) (catID, breadcrumb string, err error) {
	brandSlug, brandName, err := ensureBrand(rt, brand)
	if err != nil {
		return "", "", err
	}
	streamSlug, streamName, err := ensureStream(rt, brandSlug, stream)
	if err != nil {
		return "", "", err
	}
	projectSlug, projectName, err := ensureProject(rt, brandSlug, streamSlug, project)
	if err != nil {
		return "", "", err
	}

	cats, _ := rt.ListCategories(brandSlug, streamSlug, projectSlug)
	categoryName := category
	for _, c := range cats {
		if strings.EqualFold(c.Name, category) || strings.EqualFold(c.Slug, category) {
			catID = c.ID
			categoryName = c.Name
			break
		}
	}
	if catID == "" {
		c, err := rt.CreateCategory(brandSlug, streamSlug, projectSlug, category, len(cats))
		if err != nil {
			return "", "", fmt.Errorf("create category %q: %w", category, err)
		}
		catID = c.ID
		categoryName = c.Name
	}

	breadcrumb = strings.Join([]string{brandName, streamName, projectName, categoryName}, " / ")
	return catID, breadcrumb, nil
}
