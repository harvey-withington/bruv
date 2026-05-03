// Tiny path-based router for the mobile PWA.
//
// Why hand-rolled: we have a handful of routes and no need for nested
// layouts, route guards, or async-load splitting. A library would be
// more code than this.
//
// The Go static handler's SPA fallback serves /m/index.html for any
// unmapped path under /m/, so deep links (share_target landings, push
// notification taps) just work — the SW boots, the SPA reads
// window.location, and this router picks the right view.

const BASE = '/m/'

export type Route =
  | { name: 'home' }
  | { name: 'enrol' }
  | { name: 'repos' }
  | { name: 'inbox' }
  | { name: 'project'; brand: string; stream: string; project: string }
  | { name: 'category'; brand: string; stream: string; project: string; category: string }
  | { name: 'card'; id: string }
  | { name: 'share' }
  | { name: 'settings' }
  | { name: 'activity' }
  | { name: 'unknown'; path: string }

let _route = $state<Route>(parse(currentPath()))

function currentPath(): string {
  if (typeof window === 'undefined') return BASE
  return window.location.pathname
}

function parse(path: string): Route {
  // Strip the /m/ prefix; everything after is what we route on.
  const tail = path.startsWith(BASE) ? path.slice(BASE.length) : path.replace(/^\/+/, '')
  const segments = tail.split('?')[0].split('/').filter(Boolean)

  if (segments.length === 0) return { name: 'home' }

  switch (segments[0]) {
    case 'enrol':
      return { name: 'enrol' }
    case 'repos':
      return { name: 'repos' }
    case 'inbox':
      return { name: 'inbox' }
    case 'share':
      return { name: 'share' }
    case 'settings':
      return { name: 'settings' }
    case 'activity':
      return { name: 'activity' }
    case 'p':
      // /p/<brand>/<stream>/<project>          → project view
      // /p/<brand>/<stream>/<project>/<cat>    → focused single-category view
      if (segments.length === 4) {
        return {
          name: 'project',
          brand: decodeURIComponent(segments[1]),
          stream: decodeURIComponent(segments[2]),
          project: decodeURIComponent(segments[3]),
        }
      }
      if (segments.length === 5) {
        return {
          name: 'category',
          brand: decodeURIComponent(segments[1]),
          stream: decodeURIComponent(segments[2]),
          project: decodeURIComponent(segments[3]),
          category: decodeURIComponent(segments[4]),
        }
      }
      return { name: 'unknown', path }
    case 'c':
      // /c/<card-id>
      if (segments.length === 2) {
        return { name: 'card', id: decodeURIComponent(segments[1]) }
      }
      return { name: 'unknown', path }
  }

  return { name: 'unknown', path }
}

// Run a state mutation inside a View Transition when the browser
// supports it. The API captures before/after snapshots and morphs
// shared elements (any element with `view-transition-name`) between
// them. Browsers without the API just run the mutation immediately.
//
// Used by navigate(), replace(), and the popstate handler so SPA
// route changes get a transition for free; specific shared-element
// morphs (Category card → Card detail) are driven by view-transition-
// name CSS in the components themselves.
function withViewTransition(mutate: () => void): void {
  const doc = typeof document !== 'undefined' ? document : null
  // Type-cast: View Transitions API is recent and not yet in the
  // ambient DOM lib. No need to add full typings for one method call.
  const startVT = (doc as unknown as { startViewTransition?: (cb: () => void) => unknown })?.startViewTransition
  if (startVT) {
    startVT.call(doc, mutate)
  } else {
    mutate()
  }
}

if (typeof window !== 'undefined') {
  window.addEventListener('popstate', () => {
    withViewTransition(() => {
      _route = parse(currentPath())
    })
  })
}

export const route = {
  get current(): Route {
    return _route
  },
}

/**
 * Programmatic navigation. Pushes a new history entry and updates the
 * reactive route. Pass a path *within* the mobile scope (e.g. '/inbox')
 * — the BASE prefix is added automatically.
 */
export function navigate(to: string): void {
  const path = to.startsWith('/') ? to : `/${to}`
  const full = `${BASE.replace(/\/$/, '')}${path}`
  withViewTransition(() => {
    window.history.pushState({}, '', full)
    _route = parse(full)
  })
}

/**
 * Replace the current history entry instead of pushing — used after
 * enrolment when we don't want the user to be able to "back" into the
 * enrolment screen.
 */
export function replace(to: string): void {
  const path = to.startsWith('/') ? to : `/${to}`
  const full = `${BASE.replace(/\/$/, '')}${path}`
  withViewTransition(() => {
    window.history.replaceState({}, '', full)
    _route = parse(full)
  })
}

/**
 * Build the canonical URL for a project view. Encodes each slug so
 * unusual characters don't break the path.
 */
export function projectURL(brand: string, stream: string, project: string): string {
  return `/p/${encodeURIComponent(brand)}/${encodeURIComponent(stream)}/${encodeURIComponent(project)}`
}

/**
 * Build the canonical URL for a focused single-category view.
 */
export function categoryURL(brand: string, stream: string, project: string, category: string): string {
  return `/p/${encodeURIComponent(brand)}/${encodeURIComponent(stream)}/${encodeURIComponent(project)}/${encodeURIComponent(category)}`
}

/**
 * Build the canonical URL for a card view.
 */
export function cardURL(id: string): string {
  return `/c/${encodeURIComponent(id)}`
}
