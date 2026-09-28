<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import type { BlockMeta } from '@shared/types'
  import { isoToDateInput, isoToLocalInput, localInputToIso } from '@shared/dateTimeInput'

  let {
    value,
    meta = {},
    onUpdate
  }: {
    value: string | null
    meta?: BlockMeta & { format?: string }
    onUpdate: (value: string | null) => void
  } = $props()

  // Block meta may specify `format: "date-time"` (e.g. agent tracking
  // fields like "Last Run At") to mean the block should carry a full
  // timestamp, not just a calendar date. Default is date-only.
  const isDateTime = $derived(meta?.format === 'date-time')

  // Storage is ISO-8601 (what the LLM produces and the backend persists);
  // the native inputs want local "YYYY-MM-DD" / "YYYY-MM-DDTHH:MM".
  // Date-only values are already valid ISO, so they pass through as-is.
  const inputValue = $derived(isDateTime ? isoToLocalInput(value) : isoToDateInput(value))

  function handleChange(e: Event) {
    const raw = (e.target as HTMLInputElement).value
    onUpdate(isDateTime ? localInputToIso(raw) : raw || null)
  }
</script>

<div class="date-block">
  {#if isDateTime}
    <input
      type="datetime-local"
      class="date-input"
      value={inputValue}
      onchange={handleChange}
    />
  {:else}
    <input
      type="date"
      class="date-input"
      value={inputValue}
      onchange={handleChange}
    />
  {/if}
  {#if !value}
    <span class="date-hint">{t('block.no_date')}</span>
  {/if}
</div>

<style>
  .date-block { display: flex; align-items: center; gap: 8px; }
  .date-input {
    padding: 0.4rem 0.6rem; border: 1px solid var(--border); border-radius: 6px;
    background: var(--bg-elevated); color: var(--text-primary); font-size: 0.85rem;
    outline: none; color-scheme: dark light;
  }
  :global([data-theme="dark"]) .date-input { color-scheme: dark; }
  :global([data-theme="light"]) .date-input { color-scheme: light; }
  .date-input:focus { border-color: var(--accent); }
  .date-hint { color: var(--text-muted); font-size: 0.85em; font-style: italic; }
</style>
