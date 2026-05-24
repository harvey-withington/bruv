package http

import (
	"io"
	"io/fs"
	"mime"
	nethttp "net/http"
	"path"
	"strings"
)

// Go's default MIME registry doesn't know about .webmanifest, the file
// extension PWAs use for the Web App Manifest. Without this, the file
// server falls back to content sniffing and some browsers reject the
// manifest entirely. Register the spec'd type once at package load.
func init() {
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		// AddExtensionType only errors on a malformed type string, which
		// the constant above isn't — non-nil here is a programming bug.
		panic("transport/http: register .webmanifest MIME failed: " + err.Error())
	}
	if err := mime.AddExtensionType(".svg", "image/svg+xml"); err != nil {
		panic("transport/http: register .svg MIME failed: " + err.Error())
	}
}

// staticHandler serves an embedded Svelte bundle with SPA fallback.
// Requests for files that don't exist (typical for client-routed
// URLs like /app/settings) fall back to index.html so the router
// inside the bundle can handle them.
//
// The fs.FS is expected to be rooted at the bundle root — index.html
// at the top level, hashed assets under `assets/`. The bruv/frontend
// package's `Assets()` helper produces exactly that shape.
//
// Caching: long-immutable Cache-Control on hashed asset paths
// (Vite output uses content hashes like `index-B8TfjPnY.js`), no
// cache on index.html. If a user is behind a caching proxy they
// can still grab the newest shell + cached hashed chunks correctly.
func staticHandler(assets fs.FS) nethttp.Handler {
	fileServer := nethttp.FileServer(nethttp.FS(assets))

	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		clean := path.Clean("/" + r.URL.Path)
		if clean == "/" || clean == "." {
			clean = "/index.html"
		}
		// Trim the leading / for fs.FS lookups.
		name := strings.TrimPrefix(clean, "/")

		// If the path exists, serve it with hashed-asset caching when
		// applicable. If not, SPA fallback to index.html.
		if exists := fileExists(assets, name); !exists {
			serveIndex(w, r, assets)
			return
		}

		setCacheHeaders(w, name)
		fileServer.ServeHTTP(w, r)
	})
}

func fileExists(fsys fs.FS, name string) bool {
	f, err := fsys.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return !info.IsDir() // directories don't count; we want actual files
}

func serveIndex(w nethttp.ResponseWriter, r *nethttp.Request, fsys fs.FS) {
	f, err := fsys.Open("index.html")
	if err != nil {
		nethttp.Error(w, "index.html missing from bundle", nethttp.StatusInternalServerError)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.Copy(w, f)
}

func setCacheHeaders(w nethttp.ResponseWriter, name string) {
	// Vite asset paths look like `assets/index-<hash>.js`. The hash
	// makes them immutable — safe to cache forever.
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	// The shell HTML + anything un-hashed should not be cached so
	// users pick up new bundles on their next page load.
	w.Header().Set("Cache-Control", "no-cache")
}
