<script lang="ts">
  import { tick } from 'svelte'
  import { Plus } from 'lucide-svelte'
  import { repoRPC } from '../lib/auth'
  import { t } from '../lib/i18n.svelte'
  import type { Block, Card } from '@shared/types'
  import type { DeckTarget } from '../lib/capturePrefs'

  // "New deck" affordance for the deck picker: one tap opens a name
  // field, Create makes a card with an empty slide_deck block and hands
  // it back as the chosen target. Saves the user a trip to the desktop
  // just to have somewhere to put the first clip.

  let {
    disabled = false,
    onCreated,
    onError,
  }: {
    disabled?: boolean
    onCreated: (target: DeckTarget) => void
    onError: (message: string | null) => void
  } = $props()

  let open = $state(false)
  let name = $state('')
  let busy = $state(false)
  let nameEl = $state<HTMLInputElement | null>(null)

  async function start() {
    open = true
    await tick()
    nameEl?.focus()
  }

  async function create() {
    const deckName = name.trim()
    if (!deckName || busy) return
    busy = true
    onError(null)
    try {
      const card = await repoRPC<Card>('CreateCard', ['', deckName])
      const blockID = `blk-${crypto.randomUUID().slice(0, 8)}`
      const block: Block = {
        id: blockID,
        type: 'slide_deck',
        label: t('clip.deck_block_label'),
        key: '',
        value: { slides: [] },
      }
      await repoRPC('UpdateCardBlocks', [card.id, [block]])
      onCreated({ cardID: card.id, blockID, name: deckName })
    } catch (err) {
      onError(err instanceof Error ? err.message : t('clip.err_create_deck'))
    } finally {
      busy = false
    }
  }
</script>

{#if open}
  <div class="row">
    <input
      bind:this={nameEl}
      bind:value={name}
      type="text"
      placeholder={t('clip.new_deck_name')}
      aria-label={t('clip.new_deck_name')}
      disabled={busy || disabled}
      enterkeyhint="done"
    />
    <button type="button" class="ghost" onclick={() => (open = false)} disabled={busy}>
      {t('common.cancel')}
    </button>
    <button type="button" class="primary" onclick={create} disabled={busy || !name.trim()}>
      {t('clip.new_deck_create')}
    </button>
  </div>
{:else}
  <button type="button" class="new-btn" onclick={start} disabled={disabled || busy}>
    <Plus size={14} />
    {t('clip.new_deck')}
  </button>
{/if}

<style>
  .row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }

  input {
    flex: 1;
    min-width: 0;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    padding: 0.55rem 0.7rem;
    outline: none;
  }
  input:focus {
    border-color: var(--accent);
  }

  .new-btn,
  .ghost,
  .primary {
    font: inherit;
    font-size: 0.85rem;
    border-radius: 8px;
    cursor: pointer;
    padding: 0.55rem 0.9rem;
    border: 1px solid var(--border);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.35rem;
    touch-action: manipulation;
  }

  .new-btn {
    align-self: flex-start;
    background: transparent;
    border-style: dashed;
    color: var(--text-muted);
  }
  .new-btn:hover:not(:disabled),
  .new-btn:focus-visible:not(:disabled) {
    color: var(--text);
    border-color: var(--accent);
    outline: none;
  }

  .ghost {
    background: transparent;
    color: var(--text-muted);
  }

  .primary {
    background: var(--accent);
    color: var(--bg);
    border-color: transparent;
    font-weight: 600;
  }

  .new-btn:disabled,
  .ghost:disabled,
  .primary:disabled,
  input:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
