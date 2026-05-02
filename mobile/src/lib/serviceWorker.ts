// Register the mobile PWA's service worker. Kept tiny on purpose —
// the SW itself (in /public/service-worker.js) does the real work.
//
// The SW is only registered in production builds. In dev, Vite serves
// modules via HMR and an active SW would intercept them, hiding live
// edits. `import.meta.env.DEV` is true on `vite dev`, false on the
// embedded production bundle.

export async function registerServiceWorker(): Promise<void> {
  if (!('serviceWorker' in navigator)) return
  if (import.meta.env.DEV) return

  try {
    await navigator.serviceWorker.register('/m/service-worker.js', { scope: '/m/' })
  } catch (err) {
    // SW registration failures don't break the app — the user just
    // misses offline shell + future Web Push. Surface to the console
    // so a deploy issue is visible to anyone watching DevTools.
    console.warn('[bruv] service worker registration failed:', err)
  }
}
