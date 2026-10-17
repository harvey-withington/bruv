<script lang="ts">
  // "Available offline" switch in the card page footer. Marking downloads
  // the card now; unmarking a card with unsynced offline edits asks first.
  // On iOS in a browser tab it explains the home-screen requirement —
  // Safari would clear the cache after a week without a visit.
  import { CloudOff, Cloud } from 'lucide-svelte'
  import { offline, requestPersistentStorage, isIosBrowserTab } from '../lib/offline.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { t } from '../lib/i18n.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'

  let { cardId, title }: { cardId: string; title: string } = $props()

  const marked = $derived(offline.isMarked(cardId))
  let busy = $state(false)
  let confirmingUnmark = $state(false)

  async function toggle() {
    if (busy) return
    if (marked) {
      const status = offline.summary(cardId)?.status
      if (status && status !== 'synced') { confirmingUnmark = true; return }
      await unmark()
      return
    }
    busy = true
    try {
      await offline.mark(cardId)
      void requestPersistentStorage()
      showToast(t('offline.marked_toast', { title }), 'success')
      if (isIosBrowserTab()) showToast(t('offline.ios_home_screen'), 'info', 9000)
    } catch (e) {
      showToast(`${t('offline.mark_failed')}: ${e instanceof Error ? e.message : ''}`, 'error')
    } finally {
      busy = false
    }
  }

  async function unmark() {
    confirmingUnmark = false
    busy = true
    try {
      await offline.unmark(cardId)
      showToast(t('offline.unmarked_toast', { title }), 'success')
    } catch (e) {
      showToast(`${t('offline.unmark_failed')}: ${e instanceof Error ? e.message : ''}`, 'error')
    } finally {
      busy = false
    }
  }
</script>

{#if offline.scope}
  <button type="button" class="offline-link" class:on={marked} onclick={toggle} disabled={busy || offline.working} aria-pressed={marked}>
    {#if marked}<CloudOff size={14} />{:else}<Cloud size={14} />{/if}
    {busy ? t('common.working') : marked ? t('offline.toggle_on') : t('offline.toggle_off')}
  </button>
{/if}

{#if confirmingUnmark}
  <ConfirmDialog
    title={t('offline.toggle_on')}
    body={t('offline.unmark_confirm_unsynced')}
    confirmLabel={t('common.remove')}
    destructive
    onConfirm={unmark}
    onCancel={() => (confirmingUnmark = false)}
  />
{/if}

<style>
  .offline-link {
    background: transparent;
    border: none;
    color: var(--text-faint);
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    min-height: 40px;
  }
  .offline-link.on { color: var(--accent); }
  .offline-link:hover, .offline-link:focus-visible { background: var(--bg-elev-1); outline: none; }
  .offline-link:disabled { opacity: 0.6; cursor: default; }
</style>
