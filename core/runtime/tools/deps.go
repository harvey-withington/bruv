package tools

import (
	"bruv/core/services/card"
	"bruv/core/services/catalog"
	projectsvc "bruv/core/services/project"
	"bruv/internal/repo"
	"bruv/internal/schema"
)

// Deps is the narrow host contract Dispatcher requires. Matches the
// pattern from core/services/* — each service exposes only what the
// package actually touches, so the desktop App and the headless
// cmd/bruv-server can both satisfy it without leaking irrelevant
// state into the other.
//
// Service accessors return pointers rather than snapshotting because
// the open repo + its services change at runtime (open / close).
// The dispatcher calls these freshly each invocation.
type Deps interface {
	Repo() *repo.Repository
	Registry() *schema.Registry

	// Publish announces a domain event. Used by tool implementations
	// that write to a card to fire `card:updated` so other open views
	// (desktop Agent tab, future multi-device clients) re-fetch.
	Publish(topic string, payload any)

	// Service handles. Tool implementations route all card / project /
	// label mutations through these rather than touching repo directly
	// so the event + activity-log instrumentation each service does
	// is automatically inherited.
	Card() *card.Service
	Project() *projectsvc.Service
	Catalog() *catalog.Service
}

// Dispatcher is the tool-execution entry point. Construct once with
// a Deps implementation and reuse.
type Dispatcher struct {
	deps Deps
}

// New constructs a Dispatcher.
func New(deps Deps) *Dispatcher {
	return &Dispatcher{deps: deps}
}

// emitCardUpdated publishes a card:updated event with the card ID.
// Mirrors the main-package helper that tool implementations use to
// notify the frontend that an open CardDetail should re-fetch.
func (d *Dispatcher) emitCardUpdated(cardID string) {
	if cardID == "" {
		return
	}
	d.deps.Publish("card:updated", map[string]any{"cardID": cardID})
}
