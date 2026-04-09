<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { ChevronDown, X } from 'lucide-svelte'
  import { floatingDropdown } from '../lib/actions'
  import type { BlockMeta } from '../lib/types'

  let {
    value,
    meta = { options: [] },
    onUpdate,
  }: {
    value: string | string[]
    meta: BlockMeta
    onUpdate: (value: string | string[], meta?: BlockMeta) => void
  } = $props()

  let showOptions = $state(false)
  let triggerEl = $state<HTMLElement>(null!)

  const options = $derived(meta?.options || [])
  const isMulti = $derived(meta?.multi || false)
  const selected = $derived(
    isMulti
      ? (Array.isArray(value) ? value : value ? [value as string] : [])
      : (value as string || '')
  )

  function selectOption(opt: string) {
    if (isMulti) {
      const arr = Array.isArray(selected) ? [...selected] : []
      const idx = arr.indexOf(opt)
      if (idx >= 0) arr.splice(idx, 1)
      else arr.push(opt)
      onUpdate(arr)
    } else {
      onUpdate(opt)
      showOptions = false
    }
  }

  function handleClickOutside(e: MouseEvent) {
    const target = e.target as HTMLElement
    if (!target.closest('.select-dropdown') && !triggerEl?.contains(target)) {
      showOptions = false
    }
  }

  $effect(() => {
    if (showOptions) {
      document.addEventListener('mousedown', handleClickOutside)
      return () => document.removeEventListener('mousedown', handleClickOutside)
    }
  })
</script>

<div class="select-wrapper">
  {#if isMulti}
    <div class="multi-chips">
      {#each (Array.isArray(selected) ? selected : []) as sel}
        <span class="chip">
          {sel}
          <button class="chip-remove" onclick={() => selectOption(sel)}><X size={12} /></button>
        </span>
      {/each}
    </div>
  {/if}
  <div class="select-control" role="button" tabindex="0" bind:this={triggerEl} onclick={() => showOptions = !showOptions} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); showOptions = !showOptions } }}>
    <span class="select-value">
      {#if !isMulti && selected}
        {selected}
      {:else if !isMulti}
        <span class="placeholder">{t('block.select_placeholder')}</span>
      {/if}
    </span>
    <span class="select-controls">
      {#if (!isMulti && selected) || (isMulti && Array.isArray(selected) && selected.length > 0)}
        <button class="select-clear" onclick={(e) => { e.stopPropagation(); onUpdate(isMulti ? [] : '') }} title={t('common.cancel')}>
          <X size={14} />
        </button>
      {/if}
      <ChevronDown size={16} />
    </span>
  </div>
  {#if showOptions && triggerEl}
    <div class="select-dropdown" use:floatingDropdown={{ trigger: triggerEl, matchWidth: true }}>
      {#each options as opt}
        <button
          class="select-option"
          class:selected={isMulti ? (Array.isArray(selected) && selected.includes(opt)) : selected === opt}
          onclick={() => selectOption(opt)}
        >{opt}</button>
      {/each}
      {#if options.length === 0}
        <div class="select-empty">{t('block.no_options')}</div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .select-wrapper { display: flex; flex-direction: column; }
  .select-control {
    display: flex; align-items: center; justify-content: space-between;
    padding: 6px 10px; border: 1px solid var(--border); border-radius: 6px;
    cursor: pointer; background: var(--bg-surface);
    min-height: 34px;
  }
  .select-control:hover { border-color: var(--accent); }
  .placeholder { color: var(--text-muted); }
  .select-controls { display: flex; align-items: center; gap: 2px; flex-shrink: 0; }
  .select-clear {
    display: flex; align-items: center; justify-content: center;
    background: none; border: none; color: var(--text-muted); cursor: pointer;
    padding: 2px; border-radius: 3px;
  }
  .select-clear:hover { color: var(--text-primary); background: var(--bg-hover); }
  :global(.select-dropdown) {
    background: var(--bg-surface); border: 1px solid var(--border);
    border-radius: 6px; max-height: 200px; overflow-y: auto;
    box-shadow: 0 4px 12px rgba(0,0,0,0.2);
  }
  :global(.select-dropdown .select-option) {
    display: block; width: 100%; text-align: left; padding: 8px 12px;
    background: none; border: none; color: var(--text-primary); cursor: pointer;
  }
  :global(.select-dropdown .select-option:hover) { background: var(--bg-hover); }
  :global(.select-dropdown .select-option.selected) { background: var(--accent-bg); color: var(--accent); }
  :global(.select-dropdown .select-empty) { padding: 8px 12px; color: var(--text-muted); font-style: italic; }
  .multi-chips { display: flex; flex-wrap: wrap; gap: 4px; margin-bottom: 4px; }
  .chip {
    display: inline-flex; align-items: center; gap: 4px; padding: 2px 8px;
    background: var(--accent-bg); color: var(--accent); border-radius: 12px; font-size: 0.85em;
  }
  .chip-remove { background: none; border: none; color: var(--accent); cursor: pointer; padding: 0; }
</style>
