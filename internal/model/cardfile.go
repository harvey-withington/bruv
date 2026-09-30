package model

import "strings"

// CardFileExt is the extension of a card's own file, cards/<id>.json.
const CardFileExt = ".json"

// CardIDFromFileName reports whether name (a bare file name inside the
// cards directory) is a card file, and if so returns the card ID.
//
// The cards directory also holds per-card sidecars that share the .json
// extension — <id>.comments.json, <id>.agent.json, <id>.messages.json —
// plus atomic-write temps (<id>.json.tmp) and sync-tool artefacts such as
// <id>.sync-conflict-….json or hidden .syncthing.* files. Card IDs never
// contain a dot, so a card file is exactly "<id>.json" with no further dot
// (and no path separator) in <id>. Every enumeration of cards must go
// through this one predicate so the repo and the index agree on what a
// card is.
func CardIDFromFileName(name string) (id string, ok bool) {
	id, ok = strings.CutSuffix(name, CardFileExt)
	if !ok || id == "" || strings.ContainsAny(id, `./\`) {
		return "", false
	}
	return id, true
}
