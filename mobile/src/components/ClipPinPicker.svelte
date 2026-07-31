<script lang="ts">
  import { Inbox, Layers, FolderTree, Check } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { DECK_PIN_SENTINEL } from '../lib/capturePrefs'
  import BottomSheet from './BottomSheet.svelte'
  import PinPicker from './PinPicker.svelte'

  // Where should a clipped card land? Three answers, in the order they
  // get used: the Inbox (triage later), wherever the deck card lives
  // (the common case once a deck target is set), or a category picked
  // from the full tree — which hands off to the shared PinPicker.

  let {
    current,
    hasDeck,
    onSelect,
    onClose,
  }: {
    /** Currently-selected categoryID: '' | DECK_PIN_SENTINEL | category id. */
    current: string
    hasDeck: boolean
    onSelect: (sel: { categoryID: string; categoryName: string }) => void
    onClose: () => void
  } = $props()

  let treeOpen = $state(false)
</script>

{#if treeOpen}
  <PinPicker
    onSelect={(sel) => onSelect({ categoryID: sel.category.id, categoryName: sel.category.name })}
    onClose={() => (treeOpen = false)}
  />
{:else}
  <BottomSheet title={t('clip.pin_title')} historyKey="clipPin" {onClose}>
    <ul>
      <li>
        <button
          type="button"
          class="row"
          class:selected={current === ''}
          onclick={() => onSelect({ categoryID: '', categoryName: '' })}
        >
          <Inbox size={16} />
          <span class="row-text">
            <span class="row-title">{t('clip.pin_inbox')}</span>
            <span class="row-sub">{t('clip.pin_inbox_sub')}</span>
          </span>
          {#if current === ''}<Check size={16} />{/if}
        </button>
      </li>

      {#if hasDeck}
        <li>
          <button
            type="button"
            class="row"
            class:selected={current === DECK_PIN_SENTINEL}
            onclick={() => onSelect({ categoryID: DECK_PIN_SENTINEL, categoryName: '' })}
          >
            <Layers size={16} />
            <span class="row-text">
              <span class="row-title">{t('clip.pin_with_deck')}</span>
              <span class="row-sub">{t('clip.pin_with_deck_sub')}</span>
            </span>
            {#if current === DECK_PIN_SENTINEL}<Check size={16} />{/if}
          </button>
        </li>
      {/if}

      <li>
        <button type="button" class="row" onclick={() => (treeOpen = true)}>
          <FolderTree size={16} />
          <span class="row-text">
            <span class="row-title">{t('clip.pin_category')}</span>
            <span class="row-sub">{t('clip.pin_category_sub')}</span>
          </span>
          <span class="chevron" aria-hidden="true">›</span>
        </button>
      </li>
    </ul>
  </BottomSheet>
{/if}

<style>
  ul {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 0.65rem;
    width: 100%;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0.7rem 0.85rem;
    color: var(--text);
    font: inherit;
    cursor: pointer;
    text-align: left;
    min-height: 52px;
    touch-action: manipulation;
  }
  .row:hover,
  .row:focus-visible {
    border-color: var(--accent);
    outline: none;
  }
  .row.selected {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, var(--bg));
  }

  .row-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }
  .row-title {
    font-size: 0.95rem;
    font-weight: 500;
  }
  .row-sub {
    font-size: 0.78rem;
    color: var(--text-muted);
  }

  .chevron {
    color: var(--text-faint);
    font-size: 1.1rem;
  }
</style>
