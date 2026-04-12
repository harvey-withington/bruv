<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import type { BlockMeta } from '../lib/types'

  let {
    value,
    meta = { options: [] },
    onUpdate
  }: {
    value: string
    meta: BlockMeta
    onUpdate: (value: string) => void
  } = $props()

  const options = $derived(meta?.options || [])
  const horizontal = $derived(meta?.orientation === 'horizontal')
</script>

<div class="radio-block" class:horizontal>
  {#each options as opt}
    <label class="radio-option">
      <input type="radio" name="radio-block" checked={value === opt} onchange={() => onUpdate(opt)} />
      <span class="radio-dot"></span>
      <span class="radio-label">{opt}</span>
    </label>
  {/each}
  {#if options.length === 0}
    <span class="radio-empty">{t('block.no_options')}</span>
  {/if}
</div>

<style>
  .radio-block { display: flex; flex-direction: column; gap: 4px; }
  .radio-block.horizontal { flex-direction: row; flex-wrap: wrap; gap: 4px 12px; }
  .radio-option { display: flex; align-items: center; gap: 5px; cursor: pointer; }
  .radio-option input { display: none; }
  .radio-dot {
    width: 12px; height: 12px; border-radius: 50%; border: 1.5px solid var(--border);
    position: relative; flex-shrink: 0; transition: border-color var(--duration-normal);
  }
  .radio-dot::after {
    content: ''; position: absolute; inset: 0; margin: auto;
    width: 4px; height: 4px; border-radius: 50%;
    background: var(--accent); transform: scale(0); transition: transform var(--duration-normal);
  }
  .radio-option input:checked + .radio-dot { border-color: var(--accent); }
  .radio-option input:checked + .radio-dot::after { transform: scale(1); }
  .radio-label { font-size: 0.8em; color: var(--text-primary); }
  .radio-empty { color: var(--text-muted); font-style: italic; font-size: 0.85em; }
</style>
