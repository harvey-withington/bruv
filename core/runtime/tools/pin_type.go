package tools

import (
	"fmt"
	"strings"
)

// PinTypeConflict says why a card of cardType cannot be pinned to the
// category catID, or "" when it can. Categories may restrict the card
// types they accept; the card service enforces that at pin time, but by
// then a Suggest-mode batch has been staged and accepted and the refusal
// lands on the user. Checked while staging instead, the model hears the
// refusal in the same turn and picks another category or fixes the type.
//
// The message steers the model towards changing the card's type rather
// than the location (Harvey, 2026-09-25): the restriction says what the
// category is for, so a card that belongs there should take one of its
// types; only a card none of them describes goes elsewhere.
//
// An unknown catID is not a conflict here — the pin tool reports that on
// its own. An empty cardType matches every category.
func PinTypeConflict(allCats []CategoryPath, catID, cardType string) string {
	if catID == "" || cardType == "" {
		return ""
	}
	for _, c := range allCats {
		if c.CategoryID != catID {
			continue
		}
		if len(c.AcceptedTypes) == 0 {
			return ""
		}
		for _, t := range c.AcceptedTypes {
			if t == cardType {
				return ""
			}
		}
		return fmt.Sprintf("error: category %q only accepts card types [%s]; this card is type %q. Keep this location if it is the right one: call set_card_type with the accepted type that best describes the card, then suggest_pin here again. Only choose a different category if none of [%s] fits the card.",
			c.Breadcrumb, strings.Join(c.AcceptedTypes, ", "), cardType, strings.Join(c.AcceptedTypes, ", "))
	}
	return ""
}
