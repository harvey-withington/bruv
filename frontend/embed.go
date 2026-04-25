// Package frontend exposes the built Svelte bundle as an embedded
// fs.FS for in-process static serving from bruv-server (and any other
// future Go binary that needs to ship the UI).
//
// The embed directive lives next to dist/ — that's the only directory
// in the module from which `//go:embed all:dist` is reachable without
// the forbidden ".." path element. Build order: the frontend must be
// built (`wails build` or `npm run build` inside frontend/) before
// `go build` so dist/ exists at compile time. A fresh checkout that
// hasn't run the frontend build yet will see a build error pointing
// at this file — that's the intended signal.
package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var assets embed.FS

// Assets returns an fs.FS rooted at the bundle root (so index.html
// lives at the top level). Callers can hand it directly to the
// static handler without further sub-pathing.
func Assets() fs.FS {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		// fs.Sub only errors on a malformed path, which "dist" isn't —
		// any non-nil error here is a programming bug, not a runtime
		// condition. Panic so it's loud during development.
		panic("frontend: fs.Sub(dist) failed: " + err.Error())
	}
	return sub
}
