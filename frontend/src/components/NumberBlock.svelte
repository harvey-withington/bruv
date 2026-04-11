<script lang="ts">
  import type { BlockMeta } from '../lib/types'

  let {
    value,
    meta = {},
    onUpdate
  }: {
    value: number | null
    meta: BlockMeta
    onUpdate: (value: number | null) => void
  } = $props()

  // svelte-ignore state_referenced_locally
  let localValue = $state(value ?? 0)
  let debounceTimer: ReturnType<typeof setTimeout> | null = null

  function handleInput(e: Event) {
    const target = e.target as HTMLInputElement
    const num = target.valueAsNumber
    localValue = isNaN(num) ? 0 : num
    if (debounceTimer) clearTimeout(debounceTimer)
    debounceTimer = setTimeout(() => onUpdate(localValue), 300)
  }
</script>

<div class="number-block">
  <input
    type="number"
    class="number-input"
    value={localValue}
    min={meta?.min}
    max={meta?.max}
    oninput={handleInput}
  />
  {#if meta?.suffix}
    <span class="number-suffix">{meta.suffix}</span>
  {/if}
</div>

<style>
  .number-block { display: flex; align-items: center; gap: 6px; }
  .number-input {
    width: 120px; padding: 6px 10px; border: 1px solid var(--border); border-radius: 6px;
    background: var(--bg-surface); color: var(--text-primary); font-size: 0.95em;
  }
  .number-input:focus { border-color: var(--accent); outline: none; }
  .number-suffix { color: var(--text-muted); font-size: 0.9em; }
</style>
