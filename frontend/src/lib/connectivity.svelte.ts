// Runtime connection-loss handling for the desktop frontend.
//
// Only ever arms for REMOTE connections — the in-process Local backend
// can't drop, so isLocalActive() short-circuits everything here. When a
// remote backend becomes unreachable mid-session, the cloud adapter's
// RPC layer parks each in-flight call (see setNetworkResilience in
// cloud.ts); this module decides we're offline, drives the blur overlay,
// polls /healthz to recover, and releases the parked calls so they replay
// — preserving optimistic edits across the outage with no per-call-site
// changes.
import { setNetworkResilience } from '@shared/adapters/cloud'
import { probeBackend } from './repos.svelte'
import { isLocalActive } from './connections.svelte'

export const connectivity = $state({
  /** True while the remote backend is unreachable — the overlay shows. */
  offline: false,
  /** True while a reconnect probe is in flight (drives the spinner). */
  checking: false,
})

const PROBE_INTERVAL_MS = 3000

let probeTimer: ReturnType<typeof setTimeout> | null = null
let suspected = false
// Calls parked in the RPC layer awaiting reconnection. Released together
// on recovery (or immediately if a suspected drop turns out transient).
type Waiter = { resolve: () => void; reject: (e: Error) => void }
let waiters: Waiter[] = []

function releaseWaiters(): void {
  const pending = waiters
  waiters = []
  for (const w of pending) w.resolve()
}

/** Thrown out of every parked call when the user chooses to work offline:
 *  the offline layer answers what it can from the device instead. */
export class WorkingOfflineError extends Error {
  constructor() {
    super('working offline')
    this.name = 'WorkingOfflineError'
  }
}

/** Fail every parked call now (working offline) instead of waiting for
 *  the connection; probing carries on, so recovery is still noticed. */
export function abandonParkedCalls(): void {
  const pending = waiters
  waiters = []
  for (const w of pending) w.reject(new WorkingOfflineError())
}

const recoveryListeners = new Set<() => void>()
/** Run `fn` each time the remote comes back. Returns an unsubscribe. */
export function onRecovered(fn: () => void): () => void {
  recoveryListeners.add(fn)
  return () => recoveryListeners.delete(fn)
}

/** Treat the remote as down and poll for it — for a boot that found it
 *  unreachable and went on to work offline. */
export function watchForReconnect(): void {
  connectivity.offline = true
  startProbing()
}

// Confirm a reported failure with a direct probe before blurring — a
// single flaky request shouldn't flash the overlay. If the server is
// actually up, release the parked call so it replays immediately.
async function confirmOffline(): Promise<void> {
  if (connectivity.offline || suspected) return
  suspected = true
  const ok = await probeBackend()
  suspected = false
  if (ok) {
    releaseWaiters()
    return
  }
  connectivity.offline = true
  startProbing()
}

// Proactive check when a request is taking too long (watchdog). Unlike
// confirmOffline it never releases waiters — the slow request is still in
// flight; we only flip to offline if the server is genuinely unreachable.
async function checkReachable(): Promise<void> {
  if (connectivity.offline || suspected) return
  suspected = true
  const ok = await probeBackend()
  suspected = false
  if (!ok) {
    connectivity.offline = true
    startProbing()
  }
}

function startProbing(): void {
  if (probeTimer) return
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
  const ok = await probeBackend()
  connectivity.checking = false
  if (ok) {
    recovered()
    return
  }
  scheduleProbe(PROBE_INTERVAL_MS)
}

/** Manual retry from the overlay button — awaited so it can show a
 *  spinner; falls back into the automatic poll if still down. */
export async function retryNow(): Promise<void> {
  if (probeTimer) {
    clearTimeout(probeTimer)
    probeTimer = null
  }
  connectivity.checking = true
  const ok = await probeBackend()
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
  // Release every parked RPC so the views that stalled during the outage
  // resolve in place — no reload, optimistic edits intact.
  releaseWaiters()
  for (const fn of recoveryListeners) fn()
}

/** Wire the resilience strategy into the cloud adapter. Call once at
 *  startup, before any RPC. */
export function installResilience(): void {
  // The machine itself dropped its network: confirm right away rather
  // than waiting for a request to stall.
  window.addEventListener('offline', () => { if (!isLocalActive()) void checkReachable() })
  setNetworkResilience({
    shouldHandleOffline: () => !isLocalActive(),
    onNetworkFailure: () => void confirmOffline(),
    onSlowRequest: () => void checkReachable(),
    whenReconnected: () => new Promise<void>((resolve, reject) => waiters.push({ resolve, reject })),
  })
}
