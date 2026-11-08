// Wording for offline cards, shared by both surfaces. shared/ can't import
// a surface's i18n module, so callers pass `t`; the keys live under
// offline.* in both surfaces' en.json.

import type { TranslateFn } from '../relativeTime'
import { formatRelativeTime } from '../relativeTime'
import type { OfflineCardSummary } from './engine.svelte'
import type { OfflineConflict } from './merge'
import type { OfflinePlace } from './storage'

/** One line saying where a card stands: "Synced 5m ago", "Needs your choice"… */
export function offlineStatusText(s: OfflineCardSummary, t: TranslateFn, workingOffline: boolean): string {
  switch (s.status) {
    case 'conflict': return t('offline.status_conflict')
    case 'error': return t('offline.status_error')
    case 'pending':
      if (s.created) return t('offline.status_new')
      return workingOffline ? t('offline.status_saved_here') : t('offline.status_pending')
    case 'synced': {
      const at = s.syncedAt ?? s.fetchedAt
      return t('offline.status_synced', { when: formatRelativeTime(new Date(at).toISOString(), t) })
    }
  }
}

/** "Home › Blog › Writing › Drafts", one line per pin, "Inbox" when
 *  unfiled, '' when not known yet (show nothing rather than a wrong
 *  "Inbox"). */
export function placesText(places: OfflinePlace[] | null, t: TranslateFn): string {
  if (!places) return ''
  if (places.length === 0) return t('offline.place_inbox')
  return places
    .map((p) => [p.brand, p.stream, p.project, p.category].filter(Boolean).join(' › '))
    .join('\n')
}

/** "Title", "Due date", "Field “Notes”"… */
export function conflictLabel(c: OfflineConflict, t: TranslateFn): string {
  switch (c.kind) {
    case 'title': return t('offline.conflict_title')
    case 'description': return t('offline.conflict_description')
    case 'due_date': return t('offline.conflict_due_date')
    case 'block': return t('offline.conflict_block', { label: c.label || t('offline.conflict_unnamed') })
  }
}

/** A conflict side as text to show. Blocks show their value, not their JSON wrapper. */
export function conflictValueText(c: OfflineConflict, side: 'mine' | 'theirs', t: TranslateFn): string {
  const v = c[side]
  if (v === null || v === undefined) return c.kind === 'block' ? t('offline.value_deleted') : t('offline.value_empty')
  if (c.kind === 'due_date') return String(v).slice(0, 10)
  if (c.kind === 'block') return blockValueText((v as { value?: unknown }).value, t)
  const s = String(v)
  return s.trim() ? s : t('offline.value_empty')
}

function blockValueText(value: unknown, t: TranslateFn): string {
  if (value === null || value === undefined || value === '') return t('offline.value_empty')
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (Array.isArray(value)) {
    return value.map((item) => {
      if (item && typeof item === 'object' && 'text' in item) {
        const it = item as { text: string; done?: boolean }
        return (it.done === undefined ? '• ' : it.done ? '☑ ' : '☐ ') + it.text
      }
      return typeof item === 'string' ? '• ' + item : JSON.stringify(item)
    }).join('\n')
  }
  return JSON.stringify(value, null, 2)
}

/** "1.2 MB" */
export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

/** How loud a card's offline mark is: quiet when synced, accent with edits
 *  waiting, a warning colour when it needs the user (conflict / failed). */
export type OfflineMarkTone = 'quiet' | 'pending' | 'attention'

export function offlineMarkTone(s: OfflineCardSummary): OfflineMarkTone {
  if (s.status === 'conflict' || s.status === 'error') return 'attention'
  return s.status === 'pending' ? 'pending' : 'quiet'
}

/** The mark's tooltip / accessible name: "Available offline · Synced 5m ago". */
export function offlineMarkLabel(s: OfflineCardSummary, t: TranslateFn, workingOffline: boolean): string {
  return t('offline.mark_label', { status: offlineStatusText(s, t, workingOffline) })
}
