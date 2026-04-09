<script lang="ts">
  import { t } from '../lib/i18n.svelte'

  let {
    value,
    onUpdate
  }: {
    value: string | null
    onUpdate: (value: string | null) => void
  } = $props()

  function handleChange(e: Event) {
    const target = e.target as HTMLInputElement
    onUpdate(target.value || null)
  }
</script>

<div class="date-block">
  <input
    type="date"
    class="date-input"
    value={value || ''}
    onchange={handleChange}
  />
  {#if !value}
    <span class="date-hint">{t('block.no_date')}</span>
  {/if}
</div>

<style>
  .date-block { display: flex; align-items: center; gap: 8px; }
  .date-input {
    padding: 6px 10px; border: 1px solid var(--border); border-radius: 6px;
    background: var(--bg-surface); color: var(--text-primary); font-size: 0.95em;
  }
  .date-input:focus { border-color: var(--accent); outline: none; }
  .date-hint { color: var(--text-muted); font-size: 0.85em; font-style: italic; }
</style>
