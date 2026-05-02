<script lang="ts">
  import { t } from '../../lib/i18n.svelte'
  import type { Block } from '@shared/types'
  import { asString, withValue } from './narrow'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  const options = $derived(block.meta?.options ?? [])
  const selected = $derived(asString(block.value))

  function handleChange(e: Event) {
    const next = (e.currentTarget as HTMLSelectElement).value
    onChange(withValue(block, next))
  }
</script>

{#if options.length === 0}
  <p class="empty">{t('block.select.no_options')}</p>
{:else}
  <select class="select" value={selected} onchange={handleChange}>
    <option value="">{t('block.select.choose')}</option>
    {#each options as opt}
      <option value={opt}>{opt}</option>
    {/each}
  </select>
{/if}

<style>
  .select {
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    padding: 0.55rem 0.7rem;
    min-width: 12rem;
    color-scheme: dark light;
  }
  .select:focus {
    outline: none;
    border-color: var(--accent);
  }
  .empty {
    color: var(--text-faint);
    font-size: 0.85rem;
    margin: 0;
    font-style: italic;
  }
</style>
