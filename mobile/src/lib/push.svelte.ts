// Web Push subscribe / unsubscribe flow + status. Pairs with the Phase 3
// backend (internal/push/) — backend owns VAPID + subscription storage,
// this module owns the browser-side `PushManager.subscribe()` call and
// the per-device registration RPC.
//
// Lifecycle:
//   1. UI calls isSupported(); shows the toggle only if true.
//   2. UI calls getStatus(); reflects whether we're already subscribed.
//   3. UI toggle on  → enable() → asks the OS for notification
//      permission, calls PushManager.subscribe with the server's VAPID
//      public key, ships the resulting endpoint+keys to the backend's
//      RegisterPushSubscription RPC.
//   4. UI toggle off → disable() → calls
//      navigator.serviceWorker.getRegistration().pushManager
//      .getSubscription() then .unsubscribe(), and tells the backend
//      via UnregisterPushSubscription.
//
// Errors propagate so the caller can show a toast / inline message —
// the most common failures are "permission denied" (user said no) and
// "VAPID not configured" (server didn't generate keys, backend returns
// errPushNotConfigured).

import { repoRPC, machineRPC, readEnrolment } from './auth'

export type PushStatus = 'unsupported' | 'denied' | 'subscribed' | 'unsubscribed'

/** True when the browser exposes the APIs we need. False on iOS Safari
 *  for non-installed PWAs, on browsers without a service worker, etc. */
export function isSupported(): boolean {
  if (typeof window === 'undefined') return false
  return 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window
}

/** Current state of the push subscription against this device. Cheap —
 *  reads the SW registration's existing subscription. */
export async function getStatus(): Promise<PushStatus> {
  if (!isSupported()) return 'unsupported'
  if (Notification.permission === 'denied') return 'denied'
  const reg = await navigator.serviceWorker.ready
  const sub = await reg.pushManager.getSubscription()
  return sub ? 'subscribed' : 'unsubscribed'
}

/** Subscribe + register. Throws on permission denial, missing VAPID
 *  on the server, or any browser API failure. Returns the endpoint
 *  for diagnostic display. */
export async function enable(): Promise<string> {
  if (!isSupported()) throw new Error('Push notifications are not supported on this device.')

  const enrol = readEnrolment()
  if (!enrol?.deviceID) throw new Error('Device must be paired before enabling push.')

  const permission = await Notification.requestPermission()
  if (permission !== 'granted') {
    throw new Error('Notification permission was not granted.')
  }

  // GetVapidPublicKey — server-rendered, base64url. Empty string means
  // backend is configured but VAPID generation failed (unusual).
  const vapidKey = (await machineRPC<string>('GetVapidPublicKey')) ?? ''
  if (!vapidKey) {
    throw new Error('Server has no VAPID key — push is not configured on this server.')
  }

  const reg = await navigator.serviceWorker.ready
  // Drop any stale subscription before subscribing — push-manager will
  // happily return the existing subscription if it matches the same
  // applicationServerKey, but if the key rotated, the old sub is dead
  // and a fresh subscribe is the right move.
  const existing = await reg.pushManager.getSubscription()
  if (existing) {
    try { await existing.unsubscribe() } catch { /* best effort */ }
  }

  const sub = await reg.pushManager.subscribe({
    userVisibleOnly: true,
    // PushSubscriptionOptionsInit's applicationServerKey types as
    // `string | BufferSource | null` — Uint8Array satisfies BufferSource
    // at runtime but TypeScript's lib widens the buffer field, so a
    // single cast at the call site is the simplest fix.
    applicationServerKey: urlBase64ToUint8Array(vapidKey) as BufferSource,
  })

  const p256dh = arrayBufferToBase64Url(sub.getKey('p256dh'))
  const auth = arrayBufferToBase64Url(sub.getKey('auth'))

  await machineRPC('RegisterPushSubscription', [enrol.deviceID, sub.endpoint, p256dh, auth])

  // Touch repoRPC import so the bundler keeps it — used by
  // notification handlers in the SW (passed via fetch from there).
  void repoRPC
  return sub.endpoint
}

/** Unsubscribe + unregister. Best-effort on the unsubscribe (if it
 *  fails locally we still tell the backend to drop us). */
export async function disable(): Promise<void> {
  const enrol = readEnrolment()
  if (!isSupported() || !enrol?.deviceID) return

  const reg = await navigator.serviceWorker.ready
  const sub = await reg.pushManager.getSubscription()
  if (sub) {
    try { await sub.unsubscribe() } catch { /* keep going to backend */ }
  }
  try {
    await machineRPC('UnregisterPushSubscription', [enrol.deviceID])
  } catch (err) {
    console.warn('push: backend unregister failed', err)
  }
}

// --- base64url helpers (Web Push uses URL-safe base64) ---

function urlBase64ToUint8Array(base64Url: string): Uint8Array {
  const padding = '='.repeat((4 - (base64Url.length % 4)) % 4)
  const base64 = (base64Url + padding).replace(/-/g, '+').replace(/_/g, '/')
  const raw = atob(base64)
  const out = new Uint8Array(raw.length)
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i)
  return out
}

function arrayBufferToBase64Url(buffer: ArrayBuffer | null): string {
  if (!buffer) return ''
  const bytes = new Uint8Array(buffer)
  let str = ''
  for (let i = 0; i < bytes.length; i++) str += String.fromCharCode(bytes[i])
  return btoa(str).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}
