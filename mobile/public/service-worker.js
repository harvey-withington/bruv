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
// flushed on next install. v9 is the alpha bundle: settings page,
// push notification mobile UI, in-app notification feed + bell badge,
// recently-updated shelf, activity log, search sheet, inbox bulk-
// select, card type picker, type refresh, and comments — every "ship
// for alpha" gap closed.
const CACHE_VERSION = 'v12'
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

// --- Web Push -----------------------------------------------------
//
// Push payload shape from the backend (internal/push/sender.go's
// Notification type):
//
//   { title: string, body?: string, url?: string, icon?: string, tag?: string }
//
// Title is required; everything else optional. URL is the path to
// navigate on tap (eg /m/c/<id>). Tag is a collapse key — same tag
// replaces an earlier notification rather than stacking.
//
// Permission is requested by the page, not the SW. By the time we
// receive a push event we already have permission; if we didn't,
// the push subscription wouldn't exist.

self.addEventListener('push', (event) => {
  let data = {}
  try {
    data = event.data ? event.data.json() : {}
  } catch (_) {
    // Non-JSON payload — fall through with an empty object.
  }
  const title = data.title || 'BRUV'
  const opts = {
    body: data.body || '',
    icon: data.icon || '/m/icon-192.png',
    badge: '/m/icon-192.png',
    tag: data.tag || undefined,
    data: { url: data.url || '/m/' },
    // Respect the user's preference for silent vs ringing pushes —
    // most BRUV agent pings are informational. Setting requireInteraction
    // false keeps notifications dismissible without user action.
    requireInteraction: false,
  }
  event.waitUntil(self.registration.showNotification(title, opts))
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const target = (event.notification.data && event.notification.data.url) || '/m/'
  event.waitUntil(
    (async () => {
      // If the PWA is already open, focus the existing window and
      // navigate it. Otherwise open a fresh one. clients.matchAll
      // returns every controlled tab/window in scope.
      const all = await clients.matchAll({ type: 'window', includeUncontrolled: true })
      for (const c of all) {
        // c.url may be a deep path; just need to find any in-scope client.
        if (c.url.includes('/m/')) {
          await c.focus()
          // navigate is a feature flag in some browsers — fall back to
          // a postMessage if the API is missing.
          if ('navigate' in c && typeof c.navigate === 'function') {
            try { await c.navigate(target) } catch (_) { /* best effort */ }
          } else {
            c.postMessage({ type: 'bruv:navigate', url: target })
          }
          return
        }
      }
      await clients.openWindow(target)
    })(),
  )
})
