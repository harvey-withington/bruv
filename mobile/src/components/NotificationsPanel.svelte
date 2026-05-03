<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { X, Trash2, CheckCheck, Bell } from 'lucide-svelte'
  import { machineRPC, repoRPC } from '../lib/auth'
  import { navigate, cardURL } from '../lib/router.svelte'
  import { onEvent } from '../lib/events.svelte'
  import { t } from '../lib/i18n.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import type { AppNotification } from '@shared/types'

  // Slide-up panel from the home top-bar bell. Shows the notification
  // feed (GetNotifications), with unread highlight, mark-all-read,
  // clear-all, and tap-to-navigate-to-card. Reuses the chat-sheet
  // dismissal pattern (backdrop tap / swipe-down on header / back).

  let { onClose }: { onClose: () => void } = $props()

  let items = $state<AppNotification[]>([])
  let loading = $state(true)
  let errorMsg = $state<string | null>(null)
  let confirmingClear = $state(false)

  let sheetEl = $state<HTMLElement | null>(null)
  let dragStartY = 0
  let dragCurrentY = 0
  let dragging = $state(false)
  let translateY = $state(0)

  async function reload() {
    try {
      items = (await machineRPC<AppNotification[]>('GetNotifications')) ?? []
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('notifications.err_load')
    } finally {
      loading = false
    }
  }

  onMount(() => {
    void reload()
    history.pushState({ notifications: true }, '')
    const onPop = () => onClose()
    window.addEventListener('popstate', onPop)
    return () => {
      window.removeEventListener('popstate', onPop)
      if (history.state?.notifications) history.back()
    }
  })

  // Live: a new notification arriving updates the feed in place.
  const unsub = onEvent((ev) => {
    if (ev.topic === 'notification:new') void reload()
  })
  onDestroy(unsub)

  async function tapItem(n: AppNotification) {
    if (!n.read) {
      try {
        // Mark/clear write through the per-repo notify service (notifications
        // are stored per-repo, even though the read API is exposed on the
        // machine surface for boot-time tray badge population). repoRPC
        // routes this correctly; machineRPC would 404.
        await repoRPC('MarkNotificationRead', [n.id])
        items = items.map((x) => (x.id === n.id ? { ...x, read: true } : x))
      } catch {
        /* non-fatal — navigate anyway */
      }
    }
    if (n.card_id) {
      onClose()
      navigate(cardURL(n.card_id))
    }
  }

  async function markAllRead() {
    try {
      await repoRPC('MarkAllNotificationsRead', [])
      items = items.map((x) => ({ ...x, read: true }))
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('notifications.err_load')
    }
  }

  async function clearAll() {
    confirmingClear = false
    try {
      await repoRPC('ClearAllNotifications', [])
      items = []
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('notifications.err_load')
    }
  }

  function formatTime(iso: string): string {
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return ''
    const now = Date.now()
    const ms = now - d.getTime()
    const min = 60_000, hr = 60 * min, day = 24 * hr
    if (ms < min) return t('activity.now')
    if (ms < hr) return t('activity.minutes_ago', { n: Math.floor(ms / min) })
    if (ms < day) return t('activity.hours_ago', { n: Math.floor(ms / hr) })
    if (ms < 7 * day) return t('activity.days_ago', { n: Math.floor(ms / day) })
    return d.toLocaleDateString()
  }

  function onHeaderPointerDown(e: PointerEvent) {
    dragStartY = e.clientY
    dragCurrentY = e.clientY
    dragging = true
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  }
  function onHeaderPointerMove(e: PointerEvent) {
    if (!dragging) return
    dragCurrentY = e.clientY
    translateY = Math.max(0, dragCurrentY - dragStartY)
  }
  function onHeaderPointerUp() {
    if (!dragging) return
    dragging = false
    const dy = dragCurrentY - dragStartY
    if (dy > window.innerHeight * 0.4) {
      translateY = window.innerHeight
      setTimeout(onClose, 180)
    } else {
      translateY = 0
    }
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="backdrop" onclick={onClose}></div>

<div
  class="sheet"
  bind:this={sheetEl}
  role="dialog"
  aria-label={t('notifications.title')}
  style:transform={translateY > 0 ? `translateY(${translateY}px)` : undefined}
  style:transition={dragging ? 'none' : undefined}
>
  <div
    class="header"
    role="presentation"
    onpointerdown={onHeaderPointerDown}
    onpointermove={onHeaderPointerMove}
    onpointerup={onHeaderPointerUp}
    onpointercancel={onHeaderPointerUp}
  >
    <span class="grabber" aria-hidden="true"></span>
    <span class="title">
      {t('notifications.title')}
      {#if items.length > 0}<span class="count"> ({items.length})</span>{/if}
    </span>
    {#if items.some((x) => !x.read)}
      <button type="button" class="icon-btn" onclick={markAllRead} aria-label={t('notifications.mark_all_read')} title={t('notifications.mark_all_read')}>
        <CheckCheck size={16} />
      </button>
    {/if}
    {#if items.length > 0}
      <button type="button" class="icon-btn" onclick={() => (confirmingClear = true)} aria-label={t('notifications.clear_all')} title={t('notifications.clear_all')}>
        <Trash2 size={16} />
      </button>
    {/if}
    <button type="button" class="icon-btn" onclick={onClose} aria-label={t('common.cancel')}>
      <X size={18} />
    </button>
  </div>

  <div class="body">
    {#if loading && items.length === 0}
      <p class="status">{t('common.loading')}</p>
    {:else if errorMsg && items.length === 0}
      <p class="error">{errorMsg}</p>
    {:else if items.length === 0}
      <div class="empty">
        <Bell size={28} />
        <p>{t('notifications.empty')}</p>
      </div>
    {:else}
      <ul class="list">
        {#each items as n (n.id)}
          <li>
            <button type="button" class="item" class:unread={!n.read} onclick={() => tapItem(n)}>
              <div class="item-text">
                <span class="item-title">{n.title}</span>
                {#if n.body}<span class="item-body">{n.body}</span>{/if}
                {#if n.card_title}<span class="item-context">{n.card_title}</span>{/if}
              </div>
              <span class="time">{formatTime(n.created_at)}</span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

{#if confirmingClear}
  <ConfirmDialog
    title={t('notifications.clear_all')}
    body={t('notifications.clear_confirm')}
    confirmLabel={t('notifications.clear_all')}
    destructive
    onConfirm={clearAll}
    onCancel={() => (confirmingClear = false)}
  />
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    z-index: 60;
    animation: fade-in 200ms ease forwards;
  }
  .sheet {
    position: fixed;
    left: 0; right: 0; bottom: 0;
    height: 80vh;
    background: var(--bg);
    border-top-left-radius: 16px;
    border-top-right-radius: 16px;
    border-top: 1px solid var(--border);
    box-shadow: 0 -10px 30px rgba(0, 0, 0, 0.35);
    z-index: 61;
    display: flex;
    flex-direction: column;
    animation: slide-up 220ms cubic-bezier(0.16, 1, 0.3, 1) forwards;
    padding-bottom: env(safe-area-inset-bottom);
    transition: transform 180ms cubic-bezier(0.16, 1, 0.3, 1);
  }
  .header {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.75rem 0.85rem 0.6rem;
    border-bottom: 1px solid var(--border);
    cursor: grab;
    touch-action: none;
  }
  .grabber {
    position: absolute;
    top: 6px; left: 50%;
    transform: translateX(-50%);
    width: 36px; height: 4px;
    border-radius: 2px;
    background: var(--text-faint);
    opacity: 0.5;
  }
  .title {
    flex: 1;
    margin-top: 0.4rem;
    font-weight: 600;
    color: var(--text);
    font-size: 0.95rem;
  }
  .count { color: var(--text-faint); font-weight: 400; }
  .icon-btn {
    margin-top: 0.4rem;
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.4rem;
    border-radius: 6px;
    min-width: 36px;
    min-height: 36px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .icon-btn:hover,
  .icon-btn:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }

  .body {
    flex: 1;
    overflow-y: auto;
    padding: 0.5rem 0.85rem 1rem;
  }

  .list {
    list-style: none;
    padding: 0; margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }
  .item {
    display: flex;
    align-items: flex-start;
    gap: 0.65rem;
    width: 100%;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0.7rem 0.85rem;
    color: var(--text);
    font: inherit;
    cursor: pointer;
    text-align: left;
    touch-action: manipulation;
  }
  .item:hover,
  .item:focus-visible {
    border-color: var(--accent);
    outline: none;
  }
  .item.unread {
    border-left: 3px solid var(--accent);
    padding-left: calc(0.85rem - 2px);
  }
  .item-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }
  .item-title {
    font-weight: 600;
    font-size: 0.9rem;
  }
  .item-body {
    font-size: 0.82rem;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .item-context {
    font-size: 0.72rem;
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .time {
    font-size: 0.7rem;
    color: var(--text-faint);
    flex-shrink: 0;
    margin-top: 4px;
  }

  .empty {
    text-align: center;
    color: var(--text-faint);
    margin: 3rem 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.6rem;
  }
  .empty p { margin: 0; font-size: 0.9rem; }

  .status {
    color: var(--text-muted);
    text-align: center;
    margin: 2rem 0;
  }
  .error {
    margin: 2rem 0;
    padding: 1rem;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 8px;
    color: #fca5a5;
    text-align: center;
  }

  @keyframes fade-in {
    from { opacity: 0; }
    to { opacity: 1; }
  }
  @keyframes slide-up {
    from { transform: translateY(100%); }
    to { transform: translateY(0); }
  }

  @media (prefers-reduced-motion: reduce) {
    .backdrop, .sheet {
      animation: fade-in 120ms ease forwards;
    }
    .sheet { transition: none; }
  }
</style>
