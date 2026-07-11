<script lang="ts">
  // Type-conflict step of the card-import flow: the target category
  // doesn't accept the imported card's type, so the user picks one of
  // the category's accepted types (or "no type" — Pin accepts typeless
  // cards everywhere) and chooses whether to merge that type's template
  // blocks into the imported blocks. Cancelling aborts the import with
  // nothing created (the pre-flight in shared/cardTransfer.ts runs
  // before any mutation).
  import { t } from '../lib/i18n.svelte'
  import { fade } from 'svelte/transition'
  import { focusTrap, portal } from '../lib/actions'
  import { FileJson, X } from 'lucide-svelte'
  import { cardTypes } from '../lib/store.svelte'
  import { getCardTypeColor, getCardTypeTextColor, getCardTypeLabel } from '@shared/cardTypes'
  import type { TypeConflictResolution } from '../lib/cardExport'

  let { cardType, categoryName, acceptedTypes, onResolve }: {
    /** The imported card's original (rejected) type. */
    cardType: string
    categoryName: string
    acceptedTypes: string[]
    /** null = cancel — the import creates nothing. */
    onResolve: (resolution: TypeConflictResolution | null) => void
  } = $props()

  // null = nothing picked yet; '' = import with no type.
  let chosen = $state<string | null>(null)
  let merge = $state(false)

  function typeLabel(id: string): string {
    return getCardTypeLabel(id, cardTypes.list) || id
  }

  function confirm() {
    if (chosen === null) return
    onResolve({ type: chosen, merge: chosen ? merge : false })
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.stopPropagation()
      onResolve(null)
    }
  }

  function handleOverlayClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onResolve(null)
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" role="presentation" use:portal onclick={handleOverlayClick} out:fade={{ duration: 150 }}>
  <div
    class="dialog"
    role="dialog"
    tabindex="-1"
    aria-label={t('card.import_confirm_title')}
    use:focusTrap
  >
    <div class="dialog-header">
      <h2><FileJson size={15} /> {t('card.import_confirm_title')}</h2>
      <button class="close-btn" onclick={() => onResolve(null)} title={t('common.close')}><X size={18} /></button>
    </div>

    <div class="dialog-body">
      <p class="message">{t('card.import_confirm_msg', { category: categoryName, type: typeLabel(cardType) })}</p>

      <div class="type-list" role="radiogroup" aria-label={t('card.import_confirm_pick')}>
        {#each acceptedTypes as typeId (typeId)}
          <button
            type="button"
            class="type-row"
            class:selected={chosen === typeId}
            role="radio"
            aria-checked={chosen === typeId}
            onclick={() => chosen = typeId}
          >
            <span
              class="type-badge"
              style="background: {getCardTypeColor(typeId, cardTypes.list)}; color: {getCardTypeTextColor(typeId)}"
            >{typeLabel(typeId)}</span>
          </button>
        {/each}
        <button
          type="button"
          class="type-row"
          class:selected={chosen === ''}
          role="radio"
          aria-checked={chosen === ''}
          onclick={() => chosen = ''}
        >
          <span class="type-badge none-badge">{t('card.type_none')}</span>
        </button>
      </div>

      {#if chosen}
        <div class="merge-section">
          <p class="merge-question">{t('card.import_merge_q', { type: typeLabel(chosen) })}</p>
          <label class="merge-option">
            <input type="radio" name="merge" checked={!merge} onchange={() => merge = false} />
            <span>{t('card.import_merge_keep')}</span>
          </label>
          <label class="merge-option">
            <input type="radio" name="merge" checked={merge} onchange={() => merge = true} />
            <span>{t('card.import_merge_do')}</span>
          </label>
          <p class="hint">{t('card.import_merge_hint')}</p>
        </div>
      {/if}
    </div>

    <div class="dialog-footer">
      <button class="btn-secondary" onclick={() => onResolve(null)}>{t('common.cancel')}</button>
      <button class="btn-primary" disabled={chosen === null} onclick={confirm}>{t('card.import_confirm_btn')}</button>
    </div>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 200;
    animation: fade-in var(--duration-normal) var(--ease-out);
  }
  .dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    width: 420px;
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px var(--shadow-lg);
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }
  .dialog-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
  }
  .dialog-header h2 {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font-size: 0.82rem;
    font-weight: 600;
    color: var(--text-strong);
    margin: 0;
  }
  .close-btn { background: none; border: none; cursor: pointer; color: var(--text-muted); padding: 0.25rem; line-height: 1; border-radius: 4px; }
  .close-btn:hover { color: var(--text-primary); }

  .dialog-body { padding: 1.25rem; overflow-y: auto; flex: 1; display: flex; flex-direction: column; gap: 0.85rem; min-height: 0; }
  .dialog-footer { padding: 0.75rem 1.25rem; border-top: 1px solid var(--border-muted); display: flex; justify-content: flex-end; gap: 0.5rem; }

  .message { margin: 0; font-size: 0.85rem; color: var(--text-body); line-height: 1.5; }

  .type-list { display: flex; flex-direction: column; gap: 0.3rem; }
  .type-row {
    display: flex;
    align-items: center;
    padding: 0.45rem 0.6rem;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    cursor: pointer;
    text-align: left;
  }
  .type-row:hover,
  .type-row:focus-visible { border-color: var(--accent); outline: none; }
  .type-row.selected {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, var(--bg-elevated));
  }
  .type-badge {
    font-size: 0.7rem;
    font-weight: 600;
    padding: 2px 10px;
    border-radius: 999px;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .none-badge { background: var(--bg-elevated); color: var(--text-muted); border: 1px dashed var(--border); }

  .merge-section { display: flex; flex-direction: column; gap: 0.4rem; }
  .merge-question { margin: 0; font-size: 0.85rem; color: var(--text-body); line-height: 1.5; }
  .merge-option {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    font-size: 0.85rem;
    color: var(--text-primary);
    cursor: pointer;
  }
  .merge-option input { cursor: pointer; }
  .hint { margin: 0; font-size: 0.7rem; color: var(--text-muted); }

  .btn-primary {
    background: var(--accent);
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 0.5rem 1rem;
    font-size: 0.85rem;
    font-weight: 500;
    cursor: pointer;
  }
  .btn-primary:hover { opacity: 0.9; }
  .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn-secondary {
    background: none;
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.5rem 1rem;
    font-size: 0.85rem;
    color: var(--text-primary);
    cursor: pointer;
  }
  .btn-secondary:hover { background: var(--bg-hover); }
</style>
