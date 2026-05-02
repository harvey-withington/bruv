<script lang="ts">
  import { Check } from 'lucide-svelte'
  import { t } from '../../lib/i18n.svelte'
  import type { Block } from '@shared/types'
  import { asStringArray, withValue } from './narrow'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  const options = $derived(block.meta?.options ?? [])
  const selected = $derived(asStringArray(block.value))

  function toggle(opt: string) {
    if (selected.includes(opt)) {
      onChange(withValue(block, selected.filter((s) => s !== opt)))
    } else {
      onChange(withValue(block, [...selected, opt]))
    }
  }
</script>

{#if options.length === 0}
  <p class="empty">{t('block.checkbox_group.no_options')}</p>
{:else}
  <ul class="options">
    {#each options as opt}
      {@const isSelected = selected.includes(opt)}
      <li>
        <button
          type="button"
          class="opt"
          class:selected={isSelected}
          onclick={() => toggle(opt)}
          aria-pressed={isSelected}
        >
          <span class="box" class:filled={isSelected} aria-hidden="true">
            {#if isSelected}
              <Check size={14} />
            {/if}
          </span>
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
  .box {
    width: 18px;
    height: 18px;
    border-radius: 4px;
    border: 2px solid var(--text-muted);
    background: transparent;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    box-sizing: border-box;
    color: #fff;
  }
  .box.filled {
    border-color: var(--accent);
    background: var(--accent);
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
