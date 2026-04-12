<script lang="ts">
  import { notifications, markRead, markAllRead, clearAll, timeAgo } from '../lib/notifications.svelte'
  import { t } from '../lib/i18n.svelte'
  import { CheckCheck, Trash2, X } from 'lucide-svelte'

  let { onClose }: { onClose: () => void } = $props()

  function navigateToCard(cardId: string) {
    document.dispatchEvent(new CustomEvent('bruv:navigate', { detail: { type: 'card', id: cardId } }))
    onClose()
  }

  function handleItemClick(notif: typeof notifications.list[0]) {
    if (!notif.read) markRead(notif.id)
    if (notif.card_id) navigateToCard(notif.card_id)
  }

  function handleBackdropClick(e: MouseEvent) {
    if ((e.target as HTMLElement).classList.contains('notif-backdrop')) {
      onClose()
    }
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="notif-backdrop" role="presentation" onclick={handleBackdropClick}>
  <div class="notif-panel">
    <div class="notif-header">
      <span class="notif-title">{t('notifications.title')}</span>
      <div class="notif-actions">
        <button class="notif-action-btn" onclick={() => markAllRead()} title={t('notifications.mark_all_read')}>
          <CheckCheck size={14} />
        </button>
        <button class="notif-action-btn" onclick={() => clearAll()} title={t('notifications.clear_all')}>
          <Trash2 size={14} />
        </button>
        <button class="notif-action-btn" onclick={onClose}>
          <X size={14} />
        </button>
      </div>
    </div>
    <div class="notif-list">
      {#each notifications.list as notif (notif.id)}
        <button
          class="notif-item"
          class:unread={!notif.read}
          onclick={() => handleItemClick(notif)}
        >
          <div class="notif-item-content">
            <div class="notif-item-title">{notif.title}</div>
            {#if notif.body}
              <div class="notif-item-body">{notif.body}</div>
            {/if}
          </div>
          <div class="notif-item-meta">
            {#if notif.card_title}
              <span class="notif-card">{notif.card_title}</span>
            {/if}
            <span class="notif-time">{timeAgo(notif.created_at)}</span>
          </div>
        </button>
      {:else}
        <div class="notif-empty">{t('notifications.empty')}</div>
      {/each}
    </div>
  </div>
</div>

<style>
  .notif-backdrop {
    position: fixed;
    inset: 0;
    z-index: 9990;
  }

  .notif-panel {
    position: absolute;
    top: 44px;
    right: 12px;
    width: 380px;
    max-height: 480px;
    background: var(--bg-surface);
    border: 1px solid var(--border-muted);
    border-radius: 8px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.3);
    display: flex;
    flex-direction: column;
    overflow: hidden;
    z-index: 9991;
    animation: slide-down var(--duration-moderate) var(--ease-out);
    transform-origin: top right;
  }

  .notif-header {
    display: flex;
    align-items: center;
    padding: 0.6rem 0.75rem;
    border-bottom: 1px solid var(--border-muted);
    background: var(--bg-elevated);
    flex-shrink: 0;
  }

  .notif-title {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--text-body);
    flex: 1;
  }

  .notif-actions {
    display: flex;
    gap: 0.25rem;
  }

  .notif-action-btn {
    padding: 0.2rem;
    border: none;
    background: none;
    color: var(--text-muted);
    cursor: pointer;
    border-radius: 4px;
    display: flex;
    align-items: center;
    transition: color var(--duration-fast), background var(--duration-fast);
  }
  .notif-action-btn:hover {
    color: var(--text-body);
    background: var(--bg-hover);
  }

  .notif-list {
    overflow-y: auto;
    flex: 1;
  }

  .notif-item {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    padding: 0.5rem 0.75rem;
    width: 100%;
    text-align: left;
    background: none;
    border: none;
    border-bottom: 1px solid var(--border-muted);
    color: var(--text-body);
    cursor: pointer;
    transition: background var(--duration-fast);
  }
  .notif-item:last-child { border-bottom: none; }
  .notif-item:hover { background: var(--bg-hover); }
  .notif-item.unread {
    border-left: 3px solid var(--accent);
    background: color-mix(in srgb, var(--accent) 5%, transparent);
  }

  .notif-item-content {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  .notif-item-title {
    font-size: 0.8rem;
    font-weight: 500;
    line-height: 1.3;
  }
  .notif-item.unread .notif-item-title {
    font-weight: 600;
  }

  .notif-item-body {
    font-size: 0.73rem;
    color: var(--text-muted);
    line-height: 1.3;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .notif-item-meta {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.1rem;
  }

  .notif-card {
    font-size: 0.65rem;
    color: var(--accent);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 200px;
  }

  .notif-time {
    font-size: 0.65rem;
    color: var(--text-muted);
    margin-left: auto;
  }

  .notif-empty {
    padding: 2rem 1rem;
    text-align: center;
    font-size: 0.8rem;
    color: var(--text-muted);
  }
</style>
