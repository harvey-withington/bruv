<script lang="ts">
  import { Plus, Type, ListChecks, List, Film, Link, Minus, ChevronDown, Hash, Calendar, Star, ToggleLeft, CircleDot, ImageIcon, ChartColumn, Bell, ClipboardList } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { floatingDropdown } from '../lib/actions'
  import type { Block } from '@shared/types'

  // "+ Add block" button with its type-picker dropdown. Owns the open
  // state, click-outside handling, and the block-type catalogue (label
  // + icon per type); the parent owns what "add" means (default value,
  // persist) via onAdd.

  let { onAdd }: { onAdd: (type: Block['type'], label: string) => void } = $props()

  let open = $state(false)
  let btnEl = $state<HTMLButtonElement | null>(null)

  function handleWindowClick(e: MouseEvent) {
    if (!open) return
    const target = e.target as Node
    if (!btnEl?.contains(target) && !(target as HTMLElement).closest?.('.add-block-picker')) open = false
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

<svelte:window onclick={handleWindowClick} />

<div class="add-block-toolbar">
  <button class="add-block-btn" bind:this={btnEl} onclick={() => open = !open} title={t('tooltip.add_block')}>
    <Plus size={12} />
    <span>{t('tooltip.add_block')}</span>
  </button>
  {#if open && btnEl}
    <div class="add-block-picker" use:floatingDropdown={{ trigger: btnEl }}>
      {#each BLOCK_OPTIONS as opt (opt.type)}
        <button class="block-picker-item" onclick={() => { onAdd(opt.type, opt.label); open = false }} title={opt.label}>
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
    background: #22c55e;
    border: 1px solid #22c55e;
    border-radius: 4px;
    padding: 0.15rem 0.55rem 0.15rem 0.35rem;
    color: #fff;
    font-size: 0.75rem;
    font-weight: 500;
    cursor: pointer;
    transition: background var(--duration-normal), transform var(--duration-fast);
  }
  .add-block-btn:hover {
    background: #16a34a;
    border-color: #16a34a;
  }

  /* :global because floatingDropdown re-parents the dropdown out of
     this component's scope boundary. */
  :global(.add-block-picker) {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 4px 16px var(--shadow-lg);
    padding: 0.35rem;
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 2px;
    min-width: 220px;
  }
  :global(.add-block-picker .block-picker-item) {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.6rem;
    border: none;
    border-radius: 5px;
    background: transparent;
    color: var(--text-body);
    font-size: 0.8rem;
    cursor: pointer;
    white-space: nowrap;
  }
  :global(.add-block-picker .block-picker-item:hover) { background: var(--bg-elevated); color: var(--text-primary); }
</style>
