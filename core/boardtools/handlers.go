package boardtools

import (
	"encoding/json"
	"fmt"
	"strings"

	cardtools "bruv/core/runtime/tools"
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

func hListBrands(rt Board, _ map[string]any) (string, bool) {
	brands, err := rt.ListBrands()
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(brands)
}

func hListStreams(rt Board, a map[string]any) (string, bool) {
	brand := argStr(a, "brand")
	if brand == "" {
		return errResult("brand is required")
	}
	brandSlug, _, ok := cardtools.FindBrand(rt.ProjectService(), brand)
	if !ok {
		return errResult("brand %q not found", brand)
	}
	streams, err := rt.ListStreams(brandSlug)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(streams)
}

func hListProjects(rt Board, a map[string]any) (string, bool) {
	brand, stream := argStr(a, "brand"), argStr(a, "stream")
	if brand == "" || stream == "" {
		return errResult("brand and stream are required")
	}
	brandSlug, _, ok := cardtools.FindBrand(rt.ProjectService(), brand)
	if !ok {
		return errResult("brand %q not found", brand)
	}
	streamSlug, _, ok := cardtools.FindStream(rt.ProjectService(), brandSlug, stream)
	if !ok {
		return errResult("stream %q not found", stream)
	}
	projects, err := rt.ListProjects(brandSlug, streamSlug)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(projects)
}

func hListCategories(rt Board, a map[string]any) (string, bool) {
	brand, stream, project := argStr(a, "brand"), argStr(a, "stream"), argStr(a, "project")
	if brand == "" || stream == "" || project == "" {
		return errResult("brand, stream and project are required")
	}
	brandSlug, _, ok := cardtools.FindBrand(rt.ProjectService(), brand)
	if !ok {
		return errResult("brand %q not found", brand)
	}
	streamSlug, _, ok := cardtools.FindStream(rt.ProjectService(), brandSlug, stream)
	if !ok {
		return errResult("stream %q not found", stream)
	}
	projectSlug, _, ok := cardtools.FindProject(rt.ProjectService(), brandSlug, streamSlug, project)
	if !ok {
		return errResult("project %q not found", project)
	}
	cats, err := rt.ListCategories(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(cats)
}

func hListCardTypes(rt Board, _ map[string]any) (string, bool) {
	return jsonResult(rt.ListCardTypes())
}

func hGetCard(rt Board, a map[string]any) (string, bool) {
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

func hSearchCards(rt Board, a map[string]any) (string, bool) {
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

func hCreateBrand(rt Board, a map[string]any) (string, bool) {
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

func hCreateStream(rt Board, a map[string]any) (string, bool) {
	brand, name := argStr(a, "brand"), argStr(a, "name")
	if brand == "" || name == "" {
		return errResult("brand and name are required")
	}
	brandSlug, _, err := cardtools.EnsureBrand(rt.ProjectService(), brand)
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

func hCreateProject(rt Board, a map[string]any) (string, bool) {
	brand, stream, name := argStr(a, "brand"), argStr(a, "stream"), argStr(a, "name")
	if brand == "" || stream == "" || name == "" {
		return errResult("brand, stream and name are required")
	}
	brandSlug, _, err := cardtools.EnsureBrand(rt.ProjectService(), brand)
	if err != nil {
		return errResult("%v", err)
	}
	streamSlug, _, err := cardtools.EnsureStream(rt.ProjectService(), brandSlug, stream)
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

func hCreateCategory(rt Board, a map[string]any) (string, bool) {
	brand, stream, project, name := argStr(a, "brand"), argStr(a, "stream"), argStr(a, "project"), argStr(a, "name")
	if brand == "" || stream == "" || project == "" || name == "" {
		return errResult("brand, stream, project and name are required")
	}
	brandSlug, _, err := cardtools.EnsureBrand(rt.ProjectService(), brand)
	if err != nil {
		return errResult("%v", err)
	}
	streamSlug, _, err := cardtools.EnsureStream(rt.ProjectService(), brandSlug, stream)
	if err != nil {
		return errResult("%v", err)
	}
	projectSlug, _, err := cardtools.EnsureProject(rt.ProjectService(), brandSlug, streamSlug, project)
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

func hCreateCard(rt Board, a map[string]any) (string, bool) {
	// The type resolves against the catalog (id or label match, unknown
	// names created); an omitted type gets the built-in default.
	spec, err := cardtools.ParseCardSpec(a)
	if err != nil {
		return errResult("%v", err)
	}
	created, err := cardtools.CreateCard(rt.CardService(), rt.ProjectService(), rt.CatalogService(), spec)
	if err != nil {
		return errResult("%v", err)
	}
	pinnedTo := created.PinnedTo
	if pinnedTo == "" {
		pinnedTo = "inbox (unfiled)"
	}
	out := map[string]any{
		"card_id": created.Card.ID, "title": created.Card.Title, "type": created.Card.Type, "pinned_to": pinnedTo,
	}
	if created.TypeCreated {
		out["type_created"] = true
	}
	return jsonResult(out)
}

// --- Populate existing cards ---

func hAddCardBlocks(rt Board, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	if cardID == "" {
		return errResult("card_id is required")
	}
	blocks := cardtools.ParseBlocks(a["blocks"])
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

func hSetCardFields(rt Board, a map[string]any) (string, bool) {
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

func hAddCardTags(rt Board, a map[string]any) (string, bool) {
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
