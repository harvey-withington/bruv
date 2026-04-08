import { GetNotifications, MarkNotificationRead, MarkAllNotificationsRead, ClearAllNotifications } from './api'
import { showToast } from './toast.svelte'

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

export function handleNewNotification(data: any) {
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
  const diff = Date.now() - new Date(iso).getTime()
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}
