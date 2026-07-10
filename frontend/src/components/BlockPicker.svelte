<script lang="ts">
  import { Plus, Type, ListChecks, List, Film, Link, Minus, ChevronDown, Hash, Calendar, Star, ToggleLeft, CircleDot, ImageIcon, ChartColumn, Bell, ClipboardList } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { floatingDropdown, clickOutside } from '../lib/actions'
  import type { Block } from '@shared/types'

  // "+ Add block" button with its type-picker dropdown. Owns the open
  // state, click-outside/Escape handling, and the block-type catalogue
  // (label + icon per type); the parent owns what "add" means (default
  // value, persist) via onAdd.

  let { onAdd }: { onAdd: (type: Block['type'], label: string) => void } = $props()

  let open = $state(false)
  let btnEl = $state<HTMLButtonElement | null>(null)

  function handleKeydown(e: KeyboardEvent) {
    if (!open) return
    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      open = false
    }
  }

  const BLOCK_OPTIONS: ReadonlyArray<{ type: Block['type']; label: string; icon: typeof Type }> = [
    { type: 'text',           label: t('block.text'),           icon: Type },
    { type: 'checklist',      label: t('block.checklist'),      icon: ListChecks },
    { type: 'list',           label: t('block.list'),           icon: List },
    { type: 'media',          label: t('block.media'),          icon: Film },
    { type: 'url',            label: t('block.url'),            icon: Link },
    { type: 'divider',        label: t('block.divider'),        icon: Minus },
    { type: 'select',         label: t('block.select'),         icon: ChevronDown },
    { type: 'number',         label: t('block.number'),         icon: Hash },
    { type: 'date',           label: t('block.date'),           icon: Calendar },
    { type: 'rating',         label: t('block.rating'),         icon: Star },
    { type: 'checkbox',       label: t('block.checkbox'),       icon: ToggleLeft },
    { type: 'radio',          label: t('block.radio'),          icon: CircleDot },
    { type: 'checkbox_group', label: t('block.checkbox_group'), icon: ListChecks },
    { type: 'image',          label: t('block.image'),          icon: ImageIcon },
    { type: 'progress',       label: t('block.progress'),       icon: ChartColumn },
    { type: 'alarm',          label: t('block.alarm'),          icon: Bell },
    { type: 'survey',         label: t('block.survey'),         icon: ClipboardList },
  ]
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="add-block-toolbar">
  <button class="add-block-btn" bind:this={btnEl} onclick={() => open = !open} title={t('tooltip.add_block')}>
    <Plus size={12} />
    <span>{t('tooltip.add_block')}</span>
  </button>
  {#if open && btnEl}
    <div
      class="dropdown-menu dropdown-menu--grid"
      use:floatingDropdown={{ trigger: btnEl }}
      use:clickOutside={{ onOutsideClick: () => open = false, exclude: [btnEl] }}
    >
      {#each BLOCK_OPTIONS as opt (opt.type)}
        <button class="dropdown-menu-item" onclick={() => { onAdd(opt.type, opt.label); open = false }} title={opt.label}>
          <opt.icon size={14} />
          <span>{opt.label}</span>
        </button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .add-block-toolbar {
    position: relative;
    display: flex;
    align-items: center;
    top: -1.45rem;
  }
  .add-block-btn {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    background: var(--success);
    border: 1px solid var(--success);
    border-radius: 4px;
    padding: 0.15rem 0.55rem 0.15rem 0.35rem;
    color: var(--on-color);
    font-size: 0.75rem;
    font-weight: 500;
    cursor: pointer;
    transition: background var(--duration-normal), transform var(--duration-fast);
  }
  .add-block-btn:hover,
  .add-block-btn:focus-visible {
    background: var(--success-hover);
    border-color: var(--success-hover);
  }

  /* Menu chrome lives in the shared .dropdown-menu / .dropdown-menu-item
     classes (style.css) — :global there too, since floatingDropdown
     re-parents the menu out of this component's scope boundary. */
</style>
