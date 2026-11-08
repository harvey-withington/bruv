// Offline cards on the PWA: the shared engine (shared/offline) wired into
// repoRPC and the connectivity overlay.
//
// The cache lives in this origin's IndexedDB, keyed by server + vault, so
// the Tailscale hostname and the LAN address each keep their own. iOS
// only keeps it for a home-screen app — see isIosBrowserTab.

import { OfflineEngine } from '@shared/offline/engine.svelte'
import { defaultStorage } from '@shared/offline/storage'
import { NetworkError, readActiveRepoID, readEnrolment, sendRepoRPC, setRepoRpcInterceptor } from './auth'
import { onReconnect } from './connectivity.svelte'
import { navigate } from './router.svelte'
import { loadRepoMeta } from './repoMeta.svelte'
import { showToast } from './toast.svelte'
import { t } from './i18n.svelte'

type Location = { brandSlug: string; streamSlug: string; projectSlug: string } | null

export const offline = new OfflineEngine({
  storage: defaultStorage(),
  reads: {
    GetCardPinBreadcrumbs: 'card',
    GetCardLocation: 'card',
    ListCardComments: 'card',
    ListCardTypes: 'board',
    GetTagColors: 'board',
    GetProjectLabels: 'board',
  },
  prefetch: async (cardId, _card, fetch) => {
    await fetch('ListCardTypes', [])
    await fetch('GetTagColors', [])
    const loc = (await fetch('GetCardLocation', [cardId]).catch(() => null)) as Location
    if (loc?.brandSlug && loc.streamSlug && loc.projectSlug) {
      await fetch('GetProjectLabels', [loc.brandSlug, loc.streamSlug, loc.projectSlug]).catch(() => undefined)
    }
  },
  isNetworkError: (e) => e instanceof NetworkError || e instanceof TypeError,
  // Travel documents, scans, photos: the bytes through a fresh signed URL.
  // A raw fetch fails with TypeError (not NetworkError) when unreachable.
  fetchFile: async (cardId, attachmentId) => {
    const path = (await sendRepoRPC('SignAttachmentURL', [cardId, attachmentId])) as string
    const enrol = readEnrolment()
    const res = await fetch(enrol ? `${enrol.serverURL}${path}` : path)
    if (!res.ok) throw new Error(`attachment download failed: HTTP ${res.status}`)
    return res.blob()
  },
  onReport: (r) => {
    if (r.synced) showToast(t('offline.sync_report', { count: r.synced }), 'success')
    if (r.conflicts) showToast(t('offline.sync_report_conflicts', { count: r.conflicts }), 'error')
  },
})

let installed = false

/** Route repo RPCs through the engine and attach it to the active vault.
 *  Safe to call again after the vault changes (re-attaches). */
export async function initOffline(): Promise<void> {
  if (!installed) {
    installed = true
    setRepoRpcInterceptor((method, params, send) => offline.handle(method, params, send))
    onReconnect(() => {
      if (offline.working) offline.setWorking(false)
      else void offline.afterReconnect()
    })
  }
  const enrolment = readEnrolment()
  const repoID = readActiveRepoID()
  if (!enrolment || !repoID) return
  try {
    await offline.attach(`${enrolment.serverURL}|${repoID}`, sendRepoRPC)
  } catch {
    // No usable offline storage here (private tab, quota): the app works
    // as before, without offline cards.
  }
}

/** Switch to the device's copies and show them. */
export function workOffline(): void {
  offline.setWorking(true)
  void loadRepoMeta() // type badges + tag colours, from the cache
  navigate('/offline')
}

/** Ask the browser not to evict the cache under storage pressure. */
export async function requestPersistentStorage(): Promise<void> {
  try {
    if (navigator.storage?.persisted && !(await navigator.storage.persisted())) await navigator.storage.persist?.()
  } catch { /* unsupported */ }
}

/**
 * True on iOS when BRUV runs in a browser tab rather than from the home
 * screen. Safari clears a site's storage after about a week without a
 * visit; home-screen apps are exempt — so offline cards only last there.
 * (Every iOS browser runs Apple's engine, Chrome included.)
 */
export function isIosBrowserTab(): boolean {
  const ua = navigator.userAgent || ''
  const ios = /iPhone|iPad|iPod/i.test(ua) || (/Macintosh/i.test(ua) && navigator.maxTouchPoints > 1)
  if (!ios) return false
  const standalone = window.matchMedia?.('(display-mode: standalone)').matches || (navigator as Navigator & { standalone?: boolean }).standalone === true
  return !standalone
}
