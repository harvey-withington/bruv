package http

import (
	"io"
	"io/fs"
	nethttp "net/http"
	"path"
	"strings"
)

// staticHandler serves the embedded Svelte bundle with SPA fallback.
// Requests for files that don't exist (typical for client-routed
// URLs like /app/settings) fall back to index.html so the router
// inside the bundle can handle them.
//
// Caching: long-immutable Cache-Control on hashed asset paths
// (Vite output uses content hashes like `index-B8TfjPnY.js`), no
// cache on index.html. If a user is behind a caching proxy they
// can still grab the newest shell + cached hashed chunks correctly.
func staticHandler(assets fs.FS) nethttp.Handler {
	// The embed directive on main is `all:frontend/dist`, so the
	// filesystem rooted at "frontend/dist/..." — sub into the dist
	// dir so request paths map directly to files.
	subFS, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		// Rather than panic at server init, return a handler that
		// surfaces the misconfiguration on every request. Makes the
		// failure visible in development.
		return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
			nethttp.Error(w, "static assets misconfigured: "+err.Error(), nethttp.StatusInternalServerError)
		})
	}

	fileServer := nethttp.FileServer(nethttp.FS(subFS))

	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		clean := path.Clean("/" + r.URL.Path)
		if clean == "/" || clean == "." {
			clean = "/index.html"
		}
		// Trim the leading / for fs.FS lookups.
		name := strings.TrimPrefix(clean, "/")

		// If the path exists, serve it with hashed-asset caching when
		// applicable. If not, SPA fallback to index.html.
		if exists := fileExists(subFS, name); !exists {
			serveIndex(w, r, subFS)
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
