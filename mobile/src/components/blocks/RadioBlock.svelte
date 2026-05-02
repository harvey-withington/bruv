<script lang="ts">
  import { t } from '../../lib/i18n.svelte'
  import type { Block } from '@shared/types'
  import { asString, withValue } from './narrow'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  const options = $derived(block.meta?.options ?? [])
  const selected = $derived(asString(block.value))
</script>

{#if options.length === 0}
  <p class="empty">{t('block.radio.no_options')}</p>
{:else}
  <ul class="options">
    {#each options as opt}
      {@const isSelected = opt === selected}
      <li>
        <button
          type="button"
          class="opt"
          class:selected={isSelected}
          onclick={() => onChange(withValue(block, isSelected ? '' : opt))}
          aria-pressed={isSelected}
        >
          <span class="dot" class:filled={isSelected} aria-hidden="true"></span>
          <span class="label">{opt}</span>
        </button>
      </li>
    {/each}
  </ul>
{/if}

<style>
  .options {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }
  .opt {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    width: 100%;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 8px;
    padding: 0.55rem 0.7rem;
    color: var(--text);
    font: inherit;
    cursor: pointer;
    text-align: left;
    min-height: 44px;
  }
  .opt:hover,
  .opt:focus-visible {
    background: var(--bg-elev-1);
    border-color: var(--border);
    outline: none;
  }
  .opt.selected {
    border-color: var(--accent);
  }
  .dot {
    width: 16px;
    height: 16px;
    border-radius: 50%;
    border: 2px solid var(--text-muted);
    background: transparent;
    flex-shrink: 0;
    box-sizing: border-box;
  }
  .dot.filled {
    border-color: var(--accent);
    background: var(--accent);
    box-shadow: inset 0 0 0 3px var(--bg);
  }
  .label {
    font-size: 0.95rem;
  }
  .empty {
    color: var(--text-faint);
    font-size: 0.85rem;
    margin: 0;
    font-style: italic;
  }
</style>
