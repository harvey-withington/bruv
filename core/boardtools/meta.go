package boardtools

// Per-tool classification, shared by every surface: what a tool does to
// the board (Suggest-mode staging in-app, annotations over MCP) and how a
// project scope applies to its arguments (see native.go).

import "bruv/internal/mcp"

// effect is what a tool does to the board. The zero value is unclassified
// and fails safe: treated as a write, with no MCP annotations, so clients
// fall back to the spec's worst-case defaults.
type effect int

const (
	effectUnclassified effect = iota
	effectRead                // changes nothing
	effectAdd                 // only adds (a card, a comment); repeating adds again
	effectAddOnce             // only adds, and repeating changes nothing more (tags, pins)
	effectReplace             // overwrites or removes a value; repeating changes nothing more
	effectRunAgent            // starts an agent run, which can reach outside the board through the agent's own tools
)

// scopeRule says how a tool's arguments relate to a project scope.
type scopeRule int

const (
	scopeNone            scopeRule = iota // no card or location (list_card_types)
	scopeCard                             // card_id must be in scope
	scopeLocation                         // brand/stream/project[/category] default to, and must match, scope
	scopeCardAndLocation                  // both (pin_card, unpin_card)
	scopeResults                          // results filtered to in-scope cards (search_cards, recent_cards)
	scopeBoardOnly                        // board-level; not offered in a scoped session
)

type toolMeta struct {
	effect effect
	scope  scopeRule
}

// meta classifies every registered tool. The sync tests fail if a tool is
// registered without an entry here.
//
// Overwriting a value counts as destructive (ruling 2026-11-02): the spec
// reserves destructiveHint false for tools that only add, so set_* and
// update_card are effectReplace even though nothing is deleted outright.
var meta = map[string]toolMeta{
	"list_brands":          {effectRead, scopeBoardOnly},
	"list_streams":         {effectRead, scopeBoardOnly},
	"list_projects":        {effectRead, scopeBoardOnly},
	"list_categories":      {effectRead, scopeLocation},
	"list_card_types":      {effectRead, scopeNone},
	"get_card":             {effectRead, scopeCard},
	"search_cards":         {effectRead, scopeResults},
	"create_brand":         {effectAdd, scopeBoardOnly},
	"create_stream":        {effectAdd, scopeBoardOnly},
	"create_project":       {effectAdd, scopeBoardOnly},
	"create_category":      {effectAdd, scopeLocation},
	"create_card":          {effectAdd, scopeLocation},
	"create_card_type":     {effectAdd, scopeNone},
	"add_card_blocks":      {effectAdd, scopeCard},
	"add_checklist_items":  {effectAdd, scopeCard},
	"set_card_fields":      {effectReplace, scopeCard},
	"add_card_tags":        {effectAddOnce, scopeCard},
	"remove_card_tags":     {effectReplace, scopeCard},
	"set_card_title":       {effectReplace, scopeCard},
	"set_card_description": {effectReplace, scopeCard},
	"set_card_type":        {effectReplace, scopeCard},
	"set_card_due_date":    {effectReplace, scopeCard},
	"update_card":          {effectReplace, scopeCard},
	"add_card_attachment":  {effectAdd, scopeCard},
	"get_card_attachment":  {effectRead, scopeCard},
	"add_card_comment":     {effectAdd, scopeCard},
	"list_card_comments":   {effectRead, scopeCard},
	"pin_card":             {effectAddOnce, scopeCardAndLocation},
	"unpin_card":           {effectReplace, scopeCardAndLocation}, // unpinning the last pin takes the card off every board
	"list_cards":           {effectRead, scopeLocation},
	"recent_cards":         {effectRead, scopeResults},
	"get_card_agent":       {effectRead, scopeCard},
	"configure_card_agent": {effectReplace, scopeCard},
	"run_card_agent":       {effectRunAgent, scopeCard},
}

// IsWrite reports whether a tool changes the board (staged in Suggest mode).
func IsWrite(name string) bool { return meta[name].effect != effectRead }

// annotations renders a tool's effect as MCP tool annotations, every hint
// explicit so no client falls back to a default. Nil when unclassified.
func annotations(name string) *mcp.ToolAnnotations {
	e := meta[name].effect
	if e == effectUnclassified {
		return nil
	}
	readOnly := e == effectRead
	destructive := e == effectReplace || e == effectRunAgent
	idempotent := e == effectRead || e == effectAddOnce || e == effectReplace
	openWorld := e == effectRunAgent
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    &readOnly,
		DestructiveHint: &destructive,
		IdempotentHint:  &idempotent,
		OpenWorldHint:   &openWorld,
	}
}
