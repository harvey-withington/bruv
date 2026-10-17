<script lang="ts">
  // "Available offline" switch for one card (card dialog footer). Only
  // shown on a remote connection — the engine has no scope on Local.
  // Marking downloads the card now; unmarking a card whose offline edits
  // haven't synced asks first, since they'd be lost.
  import { CloudOff, Cloud } from 'lucide-svelte'
  import { offline, requestPersistentStorage } from '../lib/offline.svelte'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { t } from '../lib/i18n.svelte'
  import { errorDetail } from '@shared/errors'

  let { cardId, title }: { cardId: string; title: string } = $props()

  const marked = $derived(offline.isMarked(cardId))
  let busy = $state(false)

  async function toggle() {
    if (busy) return
    busy = true
    try {
      if (marked) {
        const status = offline.summary(cardId)?.status
        if (status && status !== 'synced' && !await showConfirm(t('offline.unmark_confirm_unsynced'))) return
        await offline.unmark(cardId)
        showToast(t('offline.unmarked_toast', { title }), 'success')
      } else {
        await offline.mark(cardId)
        void requestPersistentStorage()
        showToast(t('offline.marked_toast', { title }), 'success')
      }
    } catch (e) {
      showToast(`${t(marked ? 'offline.unmark_failed' : 'offline.mark_failed')}: ${errorDetail(e)}`, 'error')
    } finally {
      busy = false
    }
  }
</script>

{#if offline.scope}
  <button
    class="btn offline-toggle"
    class:on={marked}
    onclick={toggle}
    disabled={busy || offline.working}
    aria-pressed={marked}
    title={t('offline.toggle_tooltip')}
  >
    {#if marked}<CloudOff size={14} />{:else}<Cloud size={14} />{/if}
    {busy ? t('common.working') : marked ? t('offline.toggle_on') : t('offline.toggle_off')}
  </button>
{/if}

<style>
  .offline-toggle.on {
    color: var(--accent);
    border-color: var(--accent);
  }
</style>
