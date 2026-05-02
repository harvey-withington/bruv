// BRUV mobile service worker — Phase 1 minimal.
//
// Scope is /m/ (set by the registration call). Behaviour:
//
//   - Navigation requests:  network-first, fall back to cached /m/index.html.
//                           Lets the PWA shell open offline once visited.
//   - /m/assets/*:          cache-first. Vite hashes these filenames, so
//                           anything matching is immutable for its lifetime.
//   - Anything else:        passthrough (no caching). API calls under
//                           /repos/<id>/rpc, SSE under /repos/<id>/events,
//                           signed-URL attachments — none of those should
//                           ever be cached client-side.
//
// Versioning: bump CACHE_VERSION when the SW logic itself changes (rare).
// The shell-cache key incorporates the version, so old caches are dropped
// on activate.

// Bump CACHE_VERSION whenever the SW logic itself changes OR when a
// caching-policy bug shipped in the prior version needs old caches
// flushed on next install. v2 fixes a manifest-caching bug that was
// hiding share_target updates from Chrome.
const CACHE_VERSION = 'v2'
const SHELL_CACHE = `bruv-mobile-shell-${CACHE_VERSION}`
const ASSET_CACHE = `bruv-mobile-assets-${CACHE_VERSION}`
const SHELL_URL = '/m/'

self.addEventListener('install', (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(SHELL_CACHE)
      // Pre-warm the navigation cache with the shell. If the network is
      // unavailable on first install (rare but possible) we silently
      // skip — the runtime navigation handler will populate it later.
      try {
        await cache.add(SHELL_URL)
      } catch (_) {}
      await self.skipWaiting()
    })(),
  )
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const expected = new Set([SHELL_CACHE, ASSET_CACHE])
      const names = await caches.keys()
      await Promise.all(names.filter((n) => !expected.has(n)).map((n) => caches.delete(n)))
      await self.clients.claim()
    })(),
  )
})

self.addEventListener('fetch', (event) => {
  const req = event.request
  if (req.method !== 'GET') return // never cache mutations

  const url = new URL(req.url)
  if (url.origin !== self.location.origin) return // ignore cross-origin
  if (!url.pathname.startsWith('/m/')) return // outside scope — let it through

  // Navigation: HTML documents (top-level loads, deep links).
  if (req.mode === 'navigate') {
    event.respondWith(networkFirstShell(req))
    return
  }

  // Hashed bundle assets: cache-first, immutable.
  if (url.pathname.startsWith('/m/assets/')) {
    event.respondWith(cacheFirst(req, ASSET_CACHE))
    return
  }

  // Manifest: ALWAYS go to the network. The browser fetches this when
  // installing/updating a PWA, and stale cached values silently break
  // features like share_target (the WebAPK gets minted from whatever
  // the SW returns). No fallback to cache — better to fail fresh than
  // succeed stale.
  if (url.pathname === '/m/manifest.webmanifest') {
    event.respondWith(fetch(req))
    return
  }

  // Other static files in /m/ (icons, SVG): cache-first. These have
  // stable filenames and update rarely; CACHE_VERSION invalidates them
  // when needed.
  event.respondWith(cacheFirst(req, ASSET_CACHE))
})

async function networkFirstShell(req) {
  const cache = await caches.open(SHELL_CACHE)
  try {
    const fresh = await fetch(req)
    if (fresh.ok) cache.put(SHELL_URL, fresh.clone())
    return fresh
  } catch (_) {
    const cached = await cache.match(SHELL_URL)
    if (cached) return cached
    return new Response('Offline — shell not yet cached.', { status: 503 })
  }
}

async function cacheFirst(req, cacheName) {
  const cache = await caches.open(cacheName)
  const cached = await cache.match(req)
  if (cached) return cached
  const fresh = await fetch(req)
  if (fresh.ok) cache.put(req, fresh.clone())
  return fresh
}
