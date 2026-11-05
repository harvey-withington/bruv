package boardtools

import "bruv/internal/mcp"

// projectDefs are the definitions of the project tools (handlers_project.go).
func projectDefs(board string) []mcp.Tool {
	project := func(props map[string]any, required ...string) map[string]any {
		props["brand"] = strProp("The project's brand (name or slug).")
		props["stream"] = strProp("The project's stream (name or slug).")
		props["project"] = strProp("The project (name or slug).")
		return obj(props, append([]string{"brand", "stream", "project"}, required...)...)
	}
	return []mcp.Tool{
		{
			Name: "move_card",
			Description: "Move a card from its category to another category of the same project in " + board + ". " +
				"Name the destination by to_category_id, or by to_category_name (for one created earlier in this conversation). " +
				"The source is the card's current category there unless from_category_id says otherwise. To file a card somewhere new, use pin_card.",
			InputSchema: project(map[string]any{
				"card_id":          strProp("The card's id."),
				"to_category_id":   strProp("The destination category's id (preferred)."),
				"to_category_name": strProp("The destination category's name, when its id isn't known."),
				"from_category_id": strProp("Optional: the category to move it out of."),
			}, "card_id"),
		},
		{
			Name:        "update_project",
			Description: "Change a project's name, description or icon in " + board + ". Only what you pass changes. It stays under its brand and stream.",
			InputSchema: project(map[string]any{
				"name":        strProp("New project name."),
				"description": strProp("New project description."),
				"icon":        strProp(iconDesc),
			}),
		},
		{
			Name: "create_project_tag",
			Description: "Add a tag to a project's tag vocabulary in " + board + " (the name is the string used on cards). " +
				"The colour comes from the palette unless given. To tag cards, use add_card_tags.",
			InputSchema: project(map[string]any{
				"name":  strProp("Tag name."),
				"color": strProp("Optional hex colour, e.g. #61bd4f."),
				"icon":  strProp("Optional " + iconDesc),
			}, "name"),
		},
		{
			Name: "update_project_tag",
			Description: "Rename a project tag, or change its colour or icon, in " + board + ". Name it by tag_id or tag_name. " +
				"Renaming here doesn't change the tag string on cards already using it.",
			InputSchema: project(map[string]any{
				"tag_id":   strProp("The tag's id (preferred)."),
				"tag_name": strProp("The tag's name, when its id isn't known."),
				"name":     strProp("New name."),
				"color":    strProp("New hex colour."),
				"icon":     strProp("New " + iconDesc),
			}),
		},
		{
			Name: "delete_project_tag",
			Description: "Remove a tag from a project's tag vocabulary in " + board + ". Name it by tag_id or tag_name. " +
				"Cards keep the tag string; remove it from them with remove_card_tags first if needed.",
			InputSchema: project(map[string]any{
				"tag_id":   strProp("The tag's id (preferred)."),
				"tag_name": strProp("The tag's name, when its id isn't known."),
			}),
		},
		{
			Name: "update_category",
			Description: "Change a category's name, description, icon or accepted card types in " + board + ". " +
				"Name it by category_id, or category_name (for one created earlier in this conversation). Only what you pass changes.",
			InputSchema: project(map[string]any{
				"category_id":    strProp("The category's id (preferred)."),
				"category_name":  strProp("The category's name, when its id isn't known."),
				"name":           strProp("New name."),
				"description":    strProp("New description."),
				"icon":           strProp("New " + iconDesc),
				"accepted_types": strArr("Card types (ids or labels, from list_card_types) the category accepts; an empty list accepts all."),
			}),
		},
		{
			Name: "delete_category",
			Description: "Delete a category from a project in " + board + "; its cards are unpinned from it (and go to the inbox if " +
				"it was their only place). A project keeps at least one category. Only when the user asks for it.",
			InputSchema: project(map[string]any{
				"category_id":   strProp("The category's id (preferred)."),
				"category_name": strProp("The category's name, when its id isn't known."),
			}),
		},
	}
}

const iconDesc = "icon: a Lucide icon name, or a data:image/…;base64 URL, optionally prefixed c:#rrggbb: to colour it; empty clears it."
