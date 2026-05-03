<script lang="ts">
  import { onMount } from 'svelte'
  import { X, Check } from 'lucide-svelte'
  import { repoMeta } from '../lib/repoMeta.svelte'
  import { getCardTypeColor, getCardTypeTextColor } from '@shared/cardTypes'
  import { t } from '../lib/i18n.svelte'
  import DynamicIcon from './DynamicIcon.svelte'

  // Slide-up sheet listing every available card type. Tap a type to
  // pick it; the host commits via UpdateCardType. Used from CardPage's
  // type badge.

  let {
    current,
    onPick,
    onClose,
  }: {
    current: string
    onPick: (typeID: string) => void
    onClose: () => void
  } = $props()

  let sheetEl = $state<HTMLElement | null>(null)
  let dragStartY = 0
  let dragCurrentY = 0
  let dragging = $state(false)
  let translateY = $state(0)

  onMount(() => {
    history.pushState({ typePicker: true }, '')
    const onPop = () => onClose()
    window.addEventListener('popstate', onPop)
    return () => {
      window.removeEventListener('popstate', onPop)
      if (history.state?.typePicker) history.back()
    }
  })

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

<aside
  class="sheet"
  bind:this={sheetEl}
  role="dialog"
  aria-label={t('card.choose_type')}
  style:transform={translateY > 0 ? `translateY(${translateY}px)` : undefined}
  style:transition={dragging ? 'none' : undefined}
>
  <header
    class="header"
    onpointerdown={onHeaderPointerDown}
    onpointermove={onHeaderPointerMove}
    onpointerup={onHeaderPointerUp}
    onpointercancel={onHeaderPointerUp}
  >
    <span class="grabber" aria-hidden="true"></span>
    <span class="title">{t('card.choose_type')}</span>
    <button type="button" class="icon-btn" onclick={onClose} aria-label={t('common.cancel')}>
      <X size={18} />
    </button>
  </header>

  <ul class="types">
    <li>
      <button
        type="button"
        class="type-row none-row"
        class:selected={!current}
        onclick={() => onPick('')}
      >
        <span class="swatch none-swatch" aria-hidden="true">—</span>
        <div class="type-text">
          <span class="type-label">{t('card.type_none')}</span>
          <span class="type-desc">{t('card.type_none_sub')}</span>
        </div>
        {#if !current}
          <Check size={16} class="check-icon" />
        {/if}
      </button>
    </li>
    {#each repoMeta.cardTypes as type (type.id)}
      <li>
        <button
          type="button"
          class="type-row"
          class:selected={current === type.id}
          onclick={() => onPick(type.id)}
        >
          <span
            class="swatch"
            style:background={getCardTypeColor(type.id, repoMeta.cardTypes)}
            style:color={getCardTypeTextColor(type.id)}
            aria-hidden="true"
          >
            {#if type.icon}
              <DynamicIcon name={type.icon} size={16} />
            {/if}
          </span>
          <div class="type-text">
            <span class="type-label">{type.label}</span>
            {#if type.description}
              <span class="type-desc">{type.description}</span>
            {/if}
          </div>
          {#if current === type.id}
            <Check size={16} class="check-icon" />
          {/if}
        </button>
      </li>
    {/each}
  </ul>
</aside>

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
    max-height: 80vh;
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

  .types {
    list-style: none;
    padding: 0.5rem;
    margin: 0;
    overflow-y: auto;
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
    padding: 0.6rem 0.85rem;
    color: var(--text);
    font: inherit;
    cursor: pointer;
    text-align: left;
    touch-action: manipulation;
    min-height: 56px;
  }
  .type-row:hover,
  .type-row:focus-visible {
    border-color: var(--accent);
    outline: none;
  }
  .type-row.selected {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, var(--bg-elev-1));
  }

  .swatch {
    width: 36px;
    height: 36px;
    border-radius: 8px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .none-swatch {
    background: var(--bg);
    border: 1px dashed var(--border);
    color: var(--text-faint);
    font-size: 1.2rem;
    font-weight: 600;
  }

  .type-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }
  .type-label {
    font-size: 0.95rem;
    font-weight: 500;
  }
  .type-desc {
    font-size: 0.78rem;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  :global(.check-icon) {
    color: var(--accent);
    flex-shrink: 0;
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
    .sheet { transition: none; }
  }
</style>
