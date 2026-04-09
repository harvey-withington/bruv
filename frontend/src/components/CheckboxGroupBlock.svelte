<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import type { BlockMeta } from '../lib/types'

  let {
    value,
    meta = { options: [] },
    onUpdate
  }: {
    value: string[]
    meta: BlockMeta
    onUpdate: (value: string[]) => void
  } = $props()

  const options = $derived(meta?.options || [])
  const selected = $derived(Array.isArray(value) ? value : [])
  const horizontal = $derived(meta?.orientation === 'horizontal')

  function toggle(opt: string) {
    const arr = [...selected]
    const idx = arr.indexOf(opt)
    if (idx >= 0) arr.splice(idx, 1)
    else arr.push(opt)
    onUpdate(arr)
  }
</script>

<div class="checkbox-group-block" class:horizontal>
  {#each options as opt}
    <label class="checkbox-option">
      <input type="checkbox" checked={selected.includes(opt)} onchange={() => toggle(opt)} />
      <span class="check-box"></span>
      <span class="check-label">{opt}</span>
    </label>
  {/each}
  {#if options.length === 0}
    <span class="group-empty">{t('block.no_options')}</span>
  {/if}
</div>

<style>
  .checkbox-group-block { display: flex; flex-direction: column; gap: 4px; }
  .checkbox-group-block.horizontal { flex-direction: row; flex-wrap: wrap; gap: 4px 12px; }
  .checkbox-option { display: flex; align-items: center; gap: 5px; cursor: pointer; }
  .checkbox-option input { display: none; }
  .check-box {
    width: 12px; height: 12px; border-radius: 2px; border: 1.5px solid var(--border);
    position: relative; flex-shrink: 0; transition: background 0.15s, border-color 0.15s;
  }
  .checkbox-option input:checked + .check-box {
    background: var(--accent); border-color: var(--accent);
  }
  .check-box::after {
    content: ''; position: absolute; left: 3px; top: 0.5px;
    width: 3px; height: 6px; border: solid white; border-width: 0 1.5px 1.5px 0;
    transform: rotate(45deg) scale(0); transition: transform 0.15s;
  }
  .checkbox-option input:checked + .check-box::after { transform: rotate(45deg) scale(1); }
  .check-label { font-size: 0.8em; color: var(--text-primary); }
  .group-empty { color: var(--text-muted); font-style: italic; font-size: 0.85em; }
</style>
