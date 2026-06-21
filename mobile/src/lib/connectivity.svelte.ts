// Global connectivity state. Drives the full-screen "can't reach the
// server" overlay (ConnectionOverlay.svelte).
//
// Trigger: a *network-level* fetch failure (apiFetch throws a TypeError —
// offline, DNS, or the Tailscale tunnel down). Server-level errors (500s,
// RPC errors) are NOT connectivity problems and stay inline on the page.
//
// To avoid a false blur from a single flaky request, going offline is
// confirmed by a direct probe before the overlay shows. Recovery is both
// automatic (poll /version, and react to the browser's `online` event)
// and manual (the overlay's Retry button calls retryNow). On reconnect we
// re-run the active view's loader IN PLACE (no full reload) so in-flight
// work — a card being edited, a rename in progress — is never clobbered.
// Pages register a handler via onReconnect(); only the mounted page has
// one, so we refresh exactly what the user is looking at.
import { readEnrolment } from './auth'

export const connectivity = $state({
  /** True while the server is unreachable — the overlay is shown. */
  offline: false,
  /** True while a reconnect probe is in flight (drives the spinner). */
  checking: false,
})

let probeTimer: ReturnType<typeof setTimeout> | null = null
let suspected = false

const PROBE_INTERVAL_MS = 3000

// Handlers run when connectivity is restored — typically the mounted
// page's data loader, so the view it owns refreshes in place.
const reconnectHandlers = new Set<() => void>()

/** Register a callback to run when the server becomes reachable again.
 *  Returns an unsubscribe — call it from the component's onMount cleanup. */
export function onReconnect(handler: () => void): () => void {
  reconnectHandlers.add(handler)
  return () => reconnectHandlers.delete(handler)
}

/** Hit the unauthenticated /version endpoint. Resolves true iff the
 *  server answered — a clean reachability signal that needs no enrolment
 *  token and no repo context. */
async function pingServer(): Promise<boolean> {
  const enrolment = readEnrolment()
  if (!enrolment) return false
  try {
    const res = await fetch(`${enrolment.serverURL}/version`, { cache: 'no-store' })
    return res.ok
  } catch {
    return false
  }
}

/** Called by apiFetch when a request fails at the network level. Confirms
 *  with a probe before blurring so a single transient blip (server fine)
 *  doesn't flash the overlay. */
export function reportOffline(): void {
  if (connectivity.offline || suspected) return
  suspected = true
  void (async () => {
    const ok = await pingServer()
    suspected = false
    if (ok) {
      // Transient blip — the server is actually reachable, so don't blur.
      // Still run the reconnect handlers so any save that just failed and
      // queued a retry gets flushed (otherwise it'd be stranded, since no
      // overlay → no later recovery).
      runReconnectHandlers()
      return
    }
    connectivity.offline = true
    startProbing()
  })()
}

/** Proactive watchdog check when a request is taking too long. Unlike
 *  reportOffline it never runs reconnect handlers — the slow request is
 *  still in flight; we only blur if the server is genuinely unreachable. */
export async function checkReachable(): Promise<void> {
  if (connectivity.offline || suspected) return
  suspected = true
  const ok = await pingServer()
  suspected = false
  if (!ok) {
    connectivity.offline = true
    startProbing()
  }
}

function runReconnectHandlers(): void {
  for (const handler of reconnectHandlers) {
    try {
      handler()
    } catch (err) {
      console.error('reconnect handler failed:', err)
    }
  }
}

function startProbing(): void {
  if (probeTimer) return
  // The browser's `online` event means the device regained *a* network;
  // probe immediately (the server may now be reachable again).
  window.addEventListener('online', kick)
  scheduleProbe(PROBE_INTERVAL_MS)
}

function kick(): void {
  scheduleProbe(0)
}

function scheduleProbe(delay: number): void {
  if (probeTimer) clearTimeout(probeTimer)
  probeTimer = setTimeout(runProbe, delay)
}

async function runProbe(): Promise<void> {
  probeTimer = null
  connectivity.checking = true
  const ok = await pingServer()
  connectivity.checking = false
  if (ok) {
    recovered()
    return
  }
  scheduleProbe(PROBE_INTERVAL_MS)
}

/** Manual retry from the overlay button. Awaited so the button can show
 *  its spinner; on failure we fall back into the automatic poll. */
export async function retryNow(): Promise<void> {
  if (probeTimer) {
    clearTimeout(probeTimer)
    probeTimer = null
  }
  connectivity.checking = true
  const ok = await pingServer()
  connectivity.checking = false
  if (ok) recovered()
  else scheduleProbe(PROBE_INTERVAL_MS)
}

function recovered(): void {
  window.removeEventListener('online', kick)
  if (probeTimer) {
    clearTimeout(probeTimer)
    probeTimer = null
  }
  connectivity.offline = false
  // Refresh the active view in place (no reload) so in-flight edits and
  // navigation state survive. Each handler is the mounted page's loader.
  runReconnectHandlers()
}
