<script lang="ts">
  import { Star } from 'lucide-svelte'
  import type { BlockMeta } from '../lib/types'

  let {
    value,
    meta = {},
    onUpdate
  }: {
    value: number
    meta: BlockMeta
    onUpdate: (value: number) => void
  } = $props()

  const maxStars = $derived(meta?.max || 5)
  let hoverIndex = $state(-1)

  function setRating(index: number) {
    const newVal = (value === index + 1) ? 0 : index + 1
    onUpdate(newVal)
  }
</script>

<div class="rating-block" role="group" onmouseleave={() => hoverIndex = -1}>
  {#each Array(maxStars) as _, i}
    <button
      class="star-btn"
      class:filled={i < (hoverIndex >= 0 ? hoverIndex + 1 : (value || 0))}
      onmouseenter={() => hoverIndex = i}
      onclick={() => setRating(i)}
      aria-label="Rate {i + 1} of {maxStars}"
    >
      <Star size={20} fill={i < (hoverIndex >= 0 ? hoverIndex + 1 : (value || 0)) ? 'currentColor' : 'none'} />
    </button>
  {/each}
  <span class="rating-value">{value || 0}/{maxStars}</span>
</div>

<style>
  .rating-block { display: flex; align-items: center; gap: 2px; }
  .star-btn {
    background: none; border: none; cursor: pointer; padding: 2px;
    color: var(--text-muted); transition: color var(--duration-normal), transform var(--duration-normal);
  }
  .star-btn.filled { color: var(--accent); }
  .star-btn:hover { transform: scale(1.2); }
  .rating-value { margin-left: 8px; font-size: 0.85em; color: var(--text-muted); }
</style>
