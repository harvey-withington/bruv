package boardtools

// Instructions is what an MCP client is told about the board tools when
// it connects (the initialize reply's "instructions"): every tool in one
// line, which to use when, and a worked example. A model whose client
// loads tools lazily (Claude Code's tool search) still knows what exists
// from it. TestInstructionsNameEveryTool keeps the index complete.

import "strings"

// Instructions for the board named repoName.
func Instructions(repoName string) string {
	return strings.ReplaceAll(instructions, "{board}", repoName)
}

const instructions = `You're connected to the "{board}" BRUV board. A board organises ideas as Brand → Stream → Project → Category (a column) → Card. Everything you do here stays within this one board.

A card has a title, a description (free text under the title, Markdown), a type, tags, a due date, and blocks: typed fields such as text, checklist, list, url, date, number and checkbox. Read a card with get_card before changing it; its blocks show their key, label, type and value.

Tools, one line each:
- Find: list_brands, list_streams, list_projects, list_categories (the hierarchy); list_cards (a project's board, by category); search_cards (full text); recent_cards (just changed); get_card (one card, all of it); list_card_types (the types you may use).
- Create: create_card (a card, with description, blocks, tags and filing in one call); create_brand, create_stream, create_project, create_category; create_card_type (only when the user asks for a new type).
- Change a card: set_card_title; set_card_description; set_card_type; set_card_due_date; add_card_tags / remove_card_tags; add_card_blocks (new blocks of any type, checklists included); add_checklist_items (more items on a checklist the card already has); set_card_fields (fill the card type's own fields by key); update_card (several of title, due date, tags and block values in one call).
- File: pin_card (into a category; a card can be in several); unpin_card; move_card (from one of a project's categories to another).
- Projects: update_project (name, description, icon); create_project_tag, update_project_tag, delete_project_tag (the project's tag vocabulary; tagging cards is add_card_tags); update_category, delete_category (only when asked).
- Attachments and comments: add_card_attachment, get_card_attachment; add_card_comment, list_card_comments.
- Agents: get_card_agent, configure_card_agent, run_card_agent.

Which tool when:
- New checklist: add_card_blocks with type "checklist" and its items. More items on an existing one: add_checklist_items. Ticking or rewriting items: update_card, passing the whole list back as {id, text, done} so the others keep their ids and ticks.
- The description: set_card_description. A block's value: update_card (or set_card_fields for the card type's own fields). New blocks: add_card_blocks.
- A new card with content: put it all in create_card (description, blocks, tags, filing) rather than creating it and adding to it after.
- Card types: only ones list_card_types returns; never make one up.

Results: a change answers with what happened. In a chat whose changes the user reviews first, it may come back "staged": not applied yet, and the user decides when your turn ends. Don't repeat a staged change, and don't build on it as if it were done; say what you suggested.

Example: a packing list for a trip card.
1. get_card {card_id}: no checklist yet.
2. add_card_blocks {card_id, blocks: [{type: "checklist", label: "Packing", value: ["Passport", "Charger"]}]}.
3. Later, more items: add_checklist_items {card_id, checklist: "Packing", items: ["Adapter"]}.
4. Tick Passport: get_card for the items' ids, then update_card {card_id, updates: [{key: "Packing", value: [{id, text: "Passport", done: true}, …the other items as they were]}]}.

Link a card in any text as [Card title](bruv:card:<card id>).`
