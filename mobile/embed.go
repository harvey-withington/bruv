// Package mobile exposes the built mobile PWA bundle as an embedded
// fs.FS for in-process static serving, mirroring frontend/embed.go.
//
// The embed directive lives next to dist/. Build order: the mobile app
// must be built (`npm run build` inside mobile/) before `go build` so
// dist/ exists at compile time. A fresh checkout that hasn't run the
// mobile build yet will see a build error pointing at this file —
// that's the intended signal.
package mobile

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var assets embed.FS

// Assets returns an fs.FS rooted at the bundle root (so index.html
// lives at the top level). The transport's static handler strips the
// /m/ prefix before looking files up here.
func Assets() fs.FS {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		// fs.Sub only errors on a malformed path, which "dist" isn't —
		// any non-nil error here is a programming bug, not a runtime
		// condition. Panic so it's loud during development.
		panic("mobile: fs.Sub(dist) failed: " + err.Error())
	}
	return sub
}
