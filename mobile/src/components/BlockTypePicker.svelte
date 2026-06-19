<script lang="ts">
  import { onMount } from 'svelte'
  import {
    X, Type, ListChecks, List, Film, Link, Minus, ChevronDown, Hash,
    Calendar, Star, ToggleLeft, CircleDot, Image as ImageIcon, ChartColumn,
    Bell, ClipboardList,
  } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'

  // Slide-up sheet listing every block type. Tap one to add it to the
  // card. Mirrors CardTypePicker's sheet scaffolding (backdrop, drag-to-
  // dismiss header, history push/pop) so the two feel identical. Icons
  // are imported directly (a fixed set) so they're type-checked.

  let { onPick, onClose }: { onPick: (type: string) => void; onClose: () => void } = $props()

  const BLOCK_TYPES = [
    { type: 'text', Icon: Type },
    { type: 'checklist', Icon: ListChecks },
    { type: 'list', Icon: List },
    { type: 'media', Icon: Film },
    { type: 'url', Icon: Link },
    { type: 'divider', Icon: Minus },
    { type: 'select', Icon: ChevronDown },
    { type: 'number', Icon: Hash },
    { type: 'date', Icon: Calendar },
    { type: 'rating', Icon: Star },
    { type: 'checkbox', Icon: ToggleLeft },
    { type: 'radio', Icon: CircleDot },
    { type: 'checkbox_group', Icon: ListChecks },
    { type: 'image', Icon: ImageIcon },
    { type: 'progress', Icon: ChartColumn },
    { type: 'alarm', Icon: Bell },
    { type: 'survey', Icon: ClipboardList },
  ]

  let dragStartY = 0
  let dragCurrentY = 0
  let dragging = $state(false)
  let translateY = $state(0)

  onMount(() => {
    history.pushState({ blockPicker: true }, '')
    const onPop = () => onClose()
    window.addEventListener('popstate', onPop)
    return () => {
      window.removeEventListener('popstate', onPop)
      if (history.state?.blockPicker) history.back()
    }
  })

  function onHeaderPointerDown(e: PointerEvent) {
    // Skip drag setup when the press lands on a button inside the header,
    // otherwise capturing the pointer swallows the close button's click.
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
  aria-label={t('block.add_title')}
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
    <span class="title">{t('block.add_title')}</span>
    <button type="button" class="icon-btn" onclick={onClose} aria-label={t('common.cancel')}>
      <X size={18} />
    </button>
  </div>

  <div class="grid">
    {#each BLOCK_TYPES as bt (bt.type)}
      {@const Icon = bt.Icon}
      <button type="button" class="tile" onclick={() => onPick(bt.type)}>
        <span class="tile-icon" aria-hidden="true">
          <Icon size={20} />
        </span>
        <span class="tile-label">{t('block.type.' + bt.type)}</span>
      </button>
    {/each}
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

  .grid {
    padding: 0.6rem;
    overflow-y: auto;
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 0.5rem;
  }
  @media (max-width: 360px) {
    .grid { grid-template-columns: repeat(2, 1fr); }
  }
  .tile {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.4rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0.85rem 0.4rem;
    color: var(--text);
    font: inherit;
    cursor: pointer;
    text-align: center;
    touch-action: manipulation;
    min-height: 76px;
  }
  .tile:hover,
  .tile:focus-visible {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 8%, var(--bg-elev-1));
    outline: none;
  }
  .tile-icon {
    color: var(--text-muted);
    display: inline-flex;
  }
  .tile:hover .tile-icon,
  .tile:focus-visible .tile-icon {
    color: var(--accent);
  }
  .tile-label {
    font-size: 0.8rem;
    font-weight: 500;
    line-height: 1.1;
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
