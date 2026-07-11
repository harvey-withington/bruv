<script lang="ts">
  // Type-conflict step of the card-import flow (slide-up sheet, matching
  // CardTypePicker's idiom): the target category doesn't accept the
  // imported card's type, so the user picks one of the accepted types
  // (or "no type" — Pin accepts typeless cards everywhere) and chooses
  // whether to merge that type's template blocks into the imported
  // blocks. Closing/cancelling aborts the import with nothing created
  // (the pre-flight in shared/cardTransfer.ts runs before any mutation).
  import { onMount } from 'svelte'
  import { X, Check } from 'lucide-svelte'
  import { repoMeta } from '../lib/repoMeta.svelte'
  import { getCardTypeColor, getCardTypeTextColor, getCardTypeLabel } from '@shared/cardTypes'
  import { t } from '../lib/i18n.svelte'
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
    return getCardTypeLabel(id, repoMeta.cardTypes) || id
  }

  function confirm() {
    if (chosen === null) return
    onResolve({ type: chosen, merge: chosen ? merge : false })
  }

  onMount(() => {
    history.pushState({ importConfirm: true }, '')
    const onPop = () => onResolve(null)
    window.addEventListener('popstate', onPop)
    return () => {
      window.removeEventListener('popstate', onPop)
      if (history.state?.importConfirm) history.back()
    }
  })
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="backdrop" onclick={() => onResolve(null)}></div>

<div class="sheet" role="dialog" aria-label={t('card.import_confirm_title')}>
  <div class="header">
    <span class="grabber" aria-hidden="true"></span>
    <span class="title">{t('card.import_confirm_title')}</span>
    <button type="button" class="icon-btn" onclick={() => onResolve(null)} aria-label={t('common.cancel')}>
      <X size={18} />
    </button>
  </div>

  <div class="body">
    <p class="message">{t('card.import_confirm_msg', { category: categoryName, type: typeLabel(cardType) })}</p>

    <ul class="types" role="radiogroup" aria-label={t('card.import_confirm_pick')}>
      {#each acceptedTypes as typeId (typeId)}
        <li>
          <button
            type="button"
            class="type-row"
            class:selected={chosen === typeId}
            role="radio"
            aria-checked={chosen === typeId}
            onclick={() => chosen = typeId}
          >
            <span
              class="swatch"
              style:background={getCardTypeColor(typeId, repoMeta.cardTypes)}
              style:color={getCardTypeTextColor(typeId)}
              aria-hidden="true"
            ></span>
            <span class="type-label">{typeLabel(typeId)}</span>
            {#if chosen === typeId}
              <Check size={16} class="check-icon" />
            {/if}
          </button>
        </li>
      {/each}
      <li>
        <button
          type="button"
          class="type-row"
          class:selected={chosen === ''}
          role="radio"
          aria-checked={chosen === ''}
          onclick={() => chosen = ''}
        >
          <span class="swatch none-swatch" aria-hidden="true">—</span>
          <span class="type-label">{t('card.type_none')}</span>
          {#if chosen === ''}
            <Check size={16} class="check-icon" />
          {/if}
        </button>
      </li>
    </ul>

    {#if chosen}
      <div class="merge-section">
        <p class="merge-question">{t('card.import_merge_q', { type: typeLabel(chosen) })}</p>
        <label class="merge-option">
          <input type="radio" name="import-merge" checked={!merge} onchange={() => merge = false} />
          <span>{t('card.import_merge_keep')}</span>
        </label>
        <label class="merge-option">
          <input type="radio" name="import-merge" checked={merge} onchange={() => merge = true} />
          <span>{t('card.import_merge_do')}</span>
        </label>
        <p class="hint">{t('card.import_merge_hint')}</p>
      </div>
    {/if}
  </div>

  <div class="footer">
    <button type="button" class="btn-secondary" onclick={() => onResolve(null)}>{t('common.cancel')}</button>
    <button type="button" class="btn-primary" disabled={chosen === null} onclick={confirm}>{t('card.import_confirm_btn')}</button>
  </div>
</div>

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
    max-height: 85vh;
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
  }
  .header {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.75rem 0.85rem 0.6rem;
    border-bottom: 1px solid var(--border);
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

  .body {
    padding: 0.75rem 0.85rem;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.65rem;
  }
  .message {
    margin: 0;
    font-size: 0.88rem;
    color: var(--text);
    line-height: 1.45;
  }

  .types {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }
  .type-row {
    display: flex;
    align-items: center;
    gap: 0.65rem;
    width: 100%;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0.55rem 0.85rem;
    color: var(--text);
    font: inherit;
    cursor: pointer;
    text-align: left;
    touch-action: manipulation;
    min-height: 48px;
  }
  .type-row:focus-visible { border-color: var(--accent); outline: none; }
  .type-row.selected {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, var(--bg-elev-1));
  }
  .swatch {
    width: 24px;
    height: 24px;
    border-radius: 6px;
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .none-swatch {
    background: var(--bg);
    border: 1px dashed var(--border);
    color: var(--text-faint);
    font-size: 1rem;
    font-weight: 600;
  }
  .type-label {
    flex: 1;
    min-width: 0;
    font-size: 0.92rem;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  :global(.check-icon) {
    color: var(--accent);
    flex-shrink: 0;
  }

  .merge-section { display: flex; flex-direction: column; gap: 0.45rem; }
  .merge-question {
    margin: 0;
    font-size: 0.88rem;
    color: var(--text);
    line-height: 1.45;
  }
  .merge-option {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.88rem;
    color: var(--text);
    min-height: 40px;
  }
  .hint { margin: 0; font-size: 0.75rem; color: var(--text-muted); }

  .footer {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    padding: 0.6rem 0.85rem;
    border-top: 1px solid var(--border);
  }
  .btn-primary {
    background: var(--accent);
    color: #fff;
    border: none;
    border-radius: 8px;
    padding: 0.55rem 1.1rem;
    font-size: 0.9rem;
    font-weight: 500;
    cursor: pointer;
    min-height: 42px;
  }
  .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn-secondary {
    background: none;
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.55rem 1.1rem;
    font-size: 0.9rem;
    color: var(--text);
    cursor: pointer;
    min-height: 42px;
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
    .backdrop, .sheet { animation: fade-in 120ms ease forwards; }
  }
</style>
