<script lang="ts">
  // "Accepted card types" for one category (slide-up sheet, matching
  // CardTypePicker's idiom: history entry + popstate close + drag-to-
  // dismiss header). Mobile counterpart of desktop Column.svelte's
  // Layers-button settings popover: a multi-select checklist of every
  // card type, plus an explicit "All types" state. Selection semantics
  // mirror the backend contract — an EMPTY list means unrestricted
  // (CategoryAcceptsType treats no accepted_types as accept-everything),
  // so "All types" simply clears the selection. Save persists via the
  // host (UpdateCategoryAcceptedTypes); Cancel/back/backdrop discards.
  import { onMount } from 'svelte'
  import { X, Check } from 'lucide-svelte'
  import { repoMeta } from '../lib/repoMeta.svelte'
  import { getCardTypeColor, getCardTypeTextColor } from '@shared/cardTypes'
  import { t } from '../lib/i18n.svelte'
  import DynamicIcon from './DynamicIcon.svelte'

  let {
    categoryName,
    current,
    onSave,
    onClose,
  }: {
    categoryName: string
    /** The category's current accepted_types; empty/undefined = all. */
    current: string[] | undefined
    /** Persist the new list ([] = unrestricted). Should throw on
     *  failure (after surfacing the error) so the sheet stays open. */
    onSave: (types: string[]) => Promise<void>
    onClose: () => void
  } = $props()

  // svelte-ignore state_referenced_locally
  let selected = $state<string[]>([...(current ?? [])])
  let saving = $state(false)

  const unrestricted = $derived(selected.length === 0)

  function toggleType(id: string) {
    selected = selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]
  }

  async function save() {
    if (saving) return
    saving = true
    try {
      await onSave(selected)
      onClose()
    } catch {
      // Host already surfaced the error; keep the sheet (and the
      // user's selection) open for a retry.
    } finally {
      saving = false
    }
  }

  onMount(() => {
    history.pushState({ catTypesSheet: true }, '')
    const onPop = () => onClose()
    window.addEventListener('popstate', onPop)
    return () => {
      window.removeEventListener('popstate', onPop)
      if (history.state?.catTypesSheet) history.back()
    }
  })

  let dragStartY = 0
  let dragCurrentY = 0
  let dragging = $state(false)
  let translateY = $state(0)

  function onHeaderPointerDown(e: PointerEvent) {
    if ((e.target as HTMLElement | null)?.closest('button')) return
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

<div
  class="sheet"
  role="dialog"
  aria-label={t('project.accepted_types_title')}
  style:transform={translateY > 0 ? `translateY(${translateY}px)` : undefined}
  style:transition={dragging ? 'none' : undefined}
>
  <div
    class="header"
    role="presentation"
    onpointerdown={onHeaderPointerDown}
    onpointermove={onHeaderPointerMove}
    onpointerup={onHeaderPointerUp}
    onpointercancel={onHeaderPointerUp}
  >
    <span class="grabber" aria-hidden="true"></span>
    <span class="title">{t('project.accepted_types_title')}</span>
    <button type="button" class="icon-btn" onclick={onClose} aria-label={t('common.cancel')}>
      <X size={18} />
    </button>
  </div>

  <div class="body">
    <p class="message">{t('project.accepted_types_sub', { name: categoryName })}</p>

    <ul class="types" aria-label={t('project.accepted_types_title')}>
      <li>
        <button
          type="button"
          class="type-row"
          class:selected={unrestricted}
          role="checkbox"
          aria-checked={unrestricted}
          onclick={() => (selected = [])}
        >
          <span class="swatch all-swatch" aria-hidden="true">*</span>
          <div class="type-text">
            <span class="type-label">{t('project.all_types')}</span>
            <span class="type-desc">{t('project.all_types_sub')}</span>
          </div>
          {#if unrestricted}
            <Check size={16} class="check-icon" />
          {/if}
        </button>
      </li>
      <li class="divider" role="presentation"></li>
      {#each repoMeta.cardTypes as type (type.id)}
        {@const checked = selected.includes(type.id)}
        <li>
          <button
            type="button"
            class="type-row"
            class:selected={checked}
            role="checkbox"
            aria-checked={checked}
            onclick={() => toggleType(type.id)}
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
            {#if checked}
              <Check size={16} class="check-icon" />
            {/if}
          </button>
        </li>
      {/each}
    </ul>
  </div>

  <div class="footer">
    <button type="button" class="btn-secondary" onclick={onClose}>{t('common.cancel')}</button>
    <button type="button" class="btn-primary" disabled={saving} onclick={save}>
      {saving ? t('common.working') : t('common.save')}
    </button>
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
    color: var(--text-muted);
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
  .divider {
    height: 1px;
    background: var(--border);
    margin: 0.2rem 0;
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
  .all-swatch {
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
    .sheet { transition: none; }
  }
</style>
