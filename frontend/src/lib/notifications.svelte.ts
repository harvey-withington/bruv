import { GetNotifications, MarkNotificationRead, MarkAllNotificationsRead, ClearAllNotifications, DeleteNotification } from '@shared/api'
import { formatRelativeTime } from '@shared/relativeTime'
import { showToast } from './toast.svelte'
import { t } from './i18n.svelte'

export type AppNotification = {
  id: string
  title: string
  body: string
  source: string
  card_id?: string
  card_title?: string
  created_at: string
  read: boolean
}

export const notifications = $state<{ list: AppNotification[], unreadCount: number }>({
  list: [],
  unreadCount: 0,
})

function updateUnreadCount() {
  notifications.unreadCount = notifications.list.filter(n => !n.read).length
}

export async function loadNotifications() {
  try {
    const list = await GetNotifications()
    notifications.list = (list || []).slice(0, 50)
    updateUnreadCount()
  } catch {
    notifications.list = []
    notifications.unreadCount = 0
  }
}

export async function markRead(id: string) {
  try {
    await MarkNotificationRead(id)
    const item = notifications.list.find(n => n.id === id)
    if (item) item.read = true
    updateUnreadCount()
  } catch { /* ignore */ }
}

export async function markAllRead() {
  try {
    await MarkAllNotificationsRead()
    for (const n of notifications.list) n.read = true
    updateUnreadCount()
  } catch { /* ignore */ }
}

export async function clearAll() {
  try {
    await ClearAllNotifications()
    notifications.list = []
    notifications.unreadCount = 0
  } catch { /* ignore */ }
}

export async function dismissNotification(id: string) {
  try {
    await DeleteNotification(id)
    notifications.list = notifications.list.filter(n => n.id !== id)
    updateUnreadCount()
  } catch {
    showToast(t('notifications.dismiss_failed'), 'error')
  }
}

// Event payloads arrive as loose JSON whose field casing has drifted
// between Go publishers (snake_case vs PascalCase) — read both until
// the per-topic payload union lands.
export type NotificationPayload = Partial<Record<
  'id' | 'title' | 'Title' | 'body' | 'Body' | 'source' | 'Source' |
  'card_id' | 'CardID' | 'card_title' | 'CardTitle' | 'created_at' | 'CreatedAt',
  string
>>

export function handleNewNotification(data: NotificationPayload) {
  const n: AppNotification = {
    id: data.id || crypto.randomUUID().slice(0, 8),
    title: data.title || data.Title || '',
    body: data.body || data.Body || '',
    source: data.source || data.Source || 'agent',
    card_id: data.card_id || data.CardID,
    card_title: data.card_title || data.CardTitle,
    created_at: data.created_at || data.CreatedAt || new Date().toISOString(),
    read: false,
  }
  notifications.list = [n, ...notifications.list].slice(0, 50)
  updateUnreadCount()
  showToast(n.title, 'info', 5000)
}

export function timeAgo(iso: string): string {
  return formatRelativeTime(iso, t)
}
