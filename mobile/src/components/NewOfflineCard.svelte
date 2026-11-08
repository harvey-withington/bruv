<script lang="ts">
  // "New card" while working offline: type a title and tap Add — the card
  // is made on this phone (in the Inbox, created at home on the next sync)
  // and handed to the parent to open.
  import { Plus } from 'lucide-svelte'
  import type { Card } from '@shared/types'
  import { offline } from '../lib/offline.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { t } from '../lib/i18n.svelte'

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
      showToast(`${t('offline.new_card_failed')}: ${e instanceof Error ? e.message : ''}`, 'error')
    } finally {
      busy = false
    }
  }
</script>

<form class="new-card" onsubmit={(e) => { e.preventDefault(); void create() }}>
  <input
    class="new-card-input"
    bind:value={title}
    placeholder={t('offline.new_card_placeholder')}
    aria-label={t('offline.new_card_placeholder')}
    enterkeyhint="done"
  />
  <button type="submit" class="new-card-add" disabled={!title.trim() || busy} aria-label={t('offline.new_card')}>
    <Plus size={18} />
  </button>
</form>

<style>
  .new-card {
    display: flex;
    gap: 0.5rem;
    margin: 0 0 1rem;
  }
  .new-card-input {
    flex: 1;
    min-width: 0;
    min-height: 44px;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0 0.85rem;
    color: var(--text);
    font: inherit;
    font-size: 1rem;
    outline: none;
  }
  .new-card-input:focus { border-color: var(--accent); }
  .new-card-add {
    min-width: 44px;
    min-height: 44px;
    border-radius: 10px;
    border: 1px solid var(--accent);
    background: transparent;
    color: var(--accent);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
  }
  .new-card-add:disabled { opacity: 0.5; cursor: default; }
</style>
