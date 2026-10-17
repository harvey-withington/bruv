// Offline cards on the desktop: the shared engine (shared/offline) wired
// into the cloud adapter and the connection-loss handling.
//
// Only remote connections take part — the in-process Local backend can't
// drop. Every adapter RPC passes through the engine (setRpcInterceptor);
// with no vault attached it is a straight pass-through.

import { setRpcInterceptor, offlineScope, sendRpc, type TransportInfo } from '@shared/adapters/cloud'
import { OfflineEngine, OfflineUnavailableError } from '@shared/offline/engine.svelte'
import { defaultStorage } from '@shared/offline/storage'
import type { CardPin } from '@shared/types'
import { abandonParkedCalls, onRecovered, watchForReconnect, WorkingOfflineError, connectivity } from './connectivity.svelte'
import { isLocalActive } from './connections.svelte'
import { loadCardTypes, loadGlobalTagColors } from './store.svelte'
import { showToast } from './toast.svelte'
import { t } from './i18n.svelte'

export const offline = new OfflineEngine({
  storage: defaultStorage(),
  reads: {
    GetCardPinBreadcrumbs: 'card',
    GetCardPins: 'card',
    GetCardProjectContext: 'card',
    ListCardComments: 'card',
    GetAgentConfig: 'card',
    ListCardTypes: 'board',
    GetTagColors: 'board',
    GetProjectLabels: 'board',
    GetProjectMembers: 'board',
  },
  // The card's tag palette comes from the projects it's pinned in.
  prefetch: async (cardId, _card, fetch) => {
    await fetch('ListCardTypes', [])
    await fetch('GetTagColors', [])
    const pins = ((await fetch('GetCardPinBreadcrumbs', [cardId])) as CardPin[] | null) ?? []
    for (const p of pins) {
      await fetch('GetProjectLabels', [p.brandSlug, p.streamSlug, p.projectSlug]).catch(() => undefined)
      await fetch('GetProjectMembers', [p.brandSlug, p.streamSlug, p.projectSlug]).catch(() => undefined)
    }
  },
  isNetworkError: (e) => e instanceof TypeError || e instanceof WorkingOfflineError,
  onReport: (r) => {
    if (r.synced) showToast(t('offline.sync_report', { count: r.synced }), 'success')
    if (r.conflicts) showToast(t('offline.sync_report_conflicts', { count: r.conflicts }), 'error')
  },
})

export { OfflineUnavailableError }

/** The user is in the offline view (it outlives reconnecting, so a sync
 *  never swaps the screen out from under them). */
export const offlineView = $state({ open: false })

/** Route adapter RPCs through the engine. Call once at startup. */
export function installOffline(): void {
  setRpcInterceptor((method, params, send) => offline.handle(method, params, send))
  onRecovered(() => {
    if (offline.working) offline.setWorking(false)
    else void offline.afterReconnect()
  })
}

/** Point the engine at the active remote vault (boot, before any RPC). */
export async function attachOffline(info: TransportInfo): Promise<void> {
  const scope = isLocalActive() ? null : offlineScope(info)
  if (!scope) return
  try {
    await offline.attach(scope, sendRpc)
  } catch {
    // No offline storage on this device (private mode, quota): the app
    // works as before, just without offline cards.
  }
}

/** Switch to the device's copies: parked calls fail over to the cache,
 *  the offline view opens, and reconnect probing carries on. */
export function workOffline(): void {
  offline.setWorking(true)
  offlineView.open = true
  if (!connectivity.offline) watchForReconnect()
  abandonParkedCalls()
  // The card dialog reads these stores; offline they come from the cache.
  void loadCardTypes()
  void loadGlobalTagColors()
}

/** Make a batch of cards available offline (a column, before a trip). */
export async function markAllOffline(cardIds: string[]): Promise<void> {
  const todo = cardIds.filter((id) => !offline.isMarked(id))
  if (todo.length === 0) {
    showToast(t('offline.column_all_marked'), 'info')
    return
  }
  let failed = 0
  for (const id of todo) {
    try { await offline.mark(id) } catch { failed++ }
  }
  void requestPersistentStorage()
  if (todo.length - failed > 0) showToast(t('offline.column_marked', { count: todo.length - failed }), 'success')
  if (failed) showToast(t('offline.column_mark_failed', { count: failed }), 'error')
}

/** Ask the browser not to evict the cache under storage pressure. Best
 *  effort — browsers may decline, and nothing depends on the answer. */
export async function requestPersistentStorage(): Promise<void> {
  try {
    if (navigator.storage?.persisted && !(await navigator.storage.persisted())) await navigator.storage.persist?.()
  } catch { /* unsupported */ }
}
