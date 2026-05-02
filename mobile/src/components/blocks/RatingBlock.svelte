<script lang="ts">
  import { Star } from 'lucide-svelte'
  import { t } from '../../lib/i18n.svelte'
  import type { Block } from '@shared/types'
  import { asNumber, withValue } from './narrow'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  const max = $derived(block.meta?.max ?? 5)
  const current = $derived(asNumber(block.value) ?? 0)

  function setRating(value: number) {
    // Tapping the currently-set star clears the rating.
    const next = current === value ? 0 : value
    onChange(withValue(block, next))
  }
</script>

<div class="rating" role="radiogroup" aria-label={t('block.rating.aria', { value: current, max })}>
  {#each Array(max) as _, i}
    {@const v = i + 1}
    <button
      type="button"
      class="star"
      class:filled={v <= current}
      onclick={() => setRating(v)}
      aria-label={t('block.rating.set_aria', { value: v })}
    >
      <Star size={26} />
    </button>
  {/each}
  {#if current > 0}
    <button type="button" class="clear" onclick={() => onChange(withValue(block, 0))}>
      {t('block.rating.clear')}
    </button>
  {/if}
</div>

<style>
  .rating {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    flex-wrap: wrap;
  }
  .star {
    background: transparent;
    border: none;
    color: var(--text-faint);
    padding: 0.35rem;
    cursor: pointer;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 44px;
    min-height: 44px;
  }
  .star:hover,
  .star:focus-visible {
    color: var(--accent);
    background: var(--bg-elev-1);
    outline: none;
  }
  .star.filled :global(svg) {
    fill: var(--accent);
    color: var(--accent);
  }
  .clear {
    margin-left: 0.5rem;
    background: transparent;
    border: none;
    color: var(--text-faint);
    font: inherit;
    font-size: 0.75rem;
    padding: 0.4rem 0.5rem;
    border-radius: 6px;
    cursor: pointer;
  }
  .clear:hover,
  .clear:focus-visible {
    color: var(--text-muted);
    background: var(--bg-elev-1);
    outline: none;
  }
</style>
