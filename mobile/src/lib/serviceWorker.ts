// Register the mobile PWA's service worker. Kept tiny on purpose —
// the SW itself (in /public/service-worker.js) does the real work.
//
// The SW is only registered in production builds. In dev, Vite serves
// modules via HMR and an active SW would intercept them, hiding live
// edits. `import.meta.env.DEV` is true on `vite dev`, false on the
// embedded production bundle.

import { navigate } from './router.svelte'

type NavigateMessage = { type: 'bruv:navigate'; url: string }

function isNavigateMessage(msg: unknown): msg is NavigateMessage {
  return (
    !!msg &&
    typeof msg === 'object' &&
    (msg as { type?: unknown }).type === 'bruv:navigate' &&
    typeof (msg as { url?: unknown }).url === 'string'
  )
}

/**
 * A notification tap while the PWA is open: the SW asks this page to
 * route IN PLACE (a document navigation would drop CardPage's pending
 * saves and drafts). Routes through the router's navigate() — a synthetic
 * popstate would be read as Back by pages that layer Back = Escape — and
 * acknowledges over the message's reply port so the SW only falls back
 * to a full navigation when no page answers. Same-origin /m/ paths only.
 */
export function handleServiceWorkerMessage(ev: MessageEvent): void {
  const msg: unknown = ev.data
  if (!isNavigateMessage(msg)) return
  let target: URL
  try {
    target = new URL(msg.url, window.location.origin)
  } catch {
    return
  }
  if (target.origin !== window.location.origin) return
  if (target.pathname !== '/m' && !target.pathname.startsWith('/m/')) return
  // The router works within the /m scope.
  const path = (target.pathname.replace(/^\/m/, '') || '/') + target.search + target.hash
  navigate(path)
  ev.ports[0]?.postMessage({ type: 'bruv:navigated' })
}

export async function registerServiceWorker(): Promise<void> {
  if (!('serviceWorker' in navigator)) return
  if (import.meta.env.DEV) return

  // Listen before registering: a notification tap can arrive for a page
  // that's already controlled while registration is still settling.
  navigator.serviceWorker.addEventListener('message', handleServiceWorkerMessage)

  try {
    await navigator.serviceWorker.register('/m/service-worker.js', { scope: '/m/' })
  } catch (err) {
    // SW registration failures don't break the app — the user just
    // misses offline shell + future Web Push. Surface to the console
    // so a deploy issue is visible to anyone watching DevTools.
    console.warn('[bruv] service worker registration failed:', err)
  }
}
