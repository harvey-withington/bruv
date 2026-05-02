<script lang="ts">
  import { CheckSquare, Square } from 'lucide-svelte'
  import { t } from '../../lib/i18n.svelte'
  import type { Block } from '@shared/types'
  import { asBool, withValue } from './narrow'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  const checked = $derived(asBool(block.value))

  function toggle() {
    onChange(withValue(block, !checked))
  }
</script>

<button type="button" class="row" onclick={toggle} aria-pressed={checked} aria-label={t('block.checkbox.toggle')}>
  <span class="icon" class:checked aria-hidden="true">
    {#if checked}
      <CheckSquare size={20} />
    {:else}
      <Square size={20} />
    {/if}
  </span>
  <span class="text">{block.label || ''}</span>
</button>

<style>
  .row {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 8px;
    padding: 0.5rem 0.6rem;
    color: var(--text);
    font: inherit;
    cursor: pointer;
    text-align: left;
    min-height: 44px;
    transition: background 120ms ease, border-color 120ms ease;
  }
  .row:hover,
  .row:focus-visible {
    background: var(--bg-elev-1);
    border-color: var(--border);
    outline: none;
  }
  .icon {
    color: var(--text-muted);
    display: inline-flex;
  }
  .icon.checked {
    color: var(--accent);
  }
  .text {
    font-size: 0.95rem;
  }
</style>
