<script lang="ts">
  // "New card" while working offline: type a title, Enter (or the button)
  // makes the card on this device — in the Inbox, created at home on the
  // next sync — and hands it to the parent to open. Escape clears.
  import { Plus } from 'lucide-svelte'
  import type { Card } from '@shared/types'
  import { offline } from '../lib/offline.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { t } from '../lib/i18n.svelte'
  import { errorDetail } from '@shared/errors'

  let { onCreated }: { onCreated: (card: Card) => void } = $props()

  let title = $state('')
  let busy = $state(false)

  async function create() {
    const name = title.trim()
    if (!name || busy) return
    busy = true
    try {
      const card = await offline.createOffline(name)
      title = ''
      onCreated(card)
    } catch (e) {
      showToast(`${t('offline.new_card_failed')}: ${errorDetail(e)}`, 'error')
    } finally {
      busy = false
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.isComposing) return
    if (e.key === 'Enter') { e.preventDefault(); void create() }
    else if (e.key === 'Escape' && title) { e.preventDefault(); e.stopPropagation(); title = '' }
  }
</script>

<div class="new-card">
  <input
    class="new-card-input"
    bind:value={title}
    onkeydown={onKeydown}
    placeholder={t('offline.new_card_placeholder')}
    aria-label={t('offline.new_card_placeholder')}
  />
  <button class="btn primary" onclick={create} disabled={!title.trim() || busy}>
    <Plus size={14} /> {t('offline.new_card')}
  </button>
</div>

<style>
  .new-card {
    display: flex;
    gap: 0.5rem;
    max-width: 760px;
    margin: 0 auto 1rem;
  }
  .new-card-input {
    flex: 1;
    min-width: 0;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.45rem 0.65rem;
    color: var(--text-primary);
    font: inherit;
    font-size: 0.85rem;
    outline: none;
  }
  .new-card-input:focus { border-color: var(--accent); }
</style>
