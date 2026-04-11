<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import type { BlockMeta } from '../lib/types'

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

  // The underlying HTML inputs require very specific formats:
  //   <input type="date">          YYYY-MM-DD
  //   <input type="datetime-local"> YYYY-MM-DDTHH:MM (NO seconds, NO tz)
  //
  // The storage format is ISO-8601 (possibly with timezone) because
  // that's what the LLM produces and what the backend persists. These
  // two functions convert between the two.

  function toInputValue(iso: string | null | undefined): string {
    if (!iso) return ''
    // Empty or already in the short form — pass through.
    if (isDateTime) {
      // datetime-local wants YYYY-MM-DDTHH:MM. Parse and reformat in
      // LOCAL time so the user sees the wall-clock time they'd expect.
      const d = new Date(iso)
      if (isNaN(d.getTime())) return ''
      const pad = (n: number) => String(n).padStart(2, '0')
      return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
    }
    // Date-only: take the first 10 chars if it's already ISO-ish,
    // otherwise parse and reformat.
    if (/^\d{4}-\d{2}-\d{2}/.test(iso)) {
      return iso.slice(0, 10)
    }
    const d = new Date(iso)
    if (isNaN(d.getTime())) return ''
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
  }

  function fromInputValue(raw: string): string | null {
    if (!raw) return null
    if (isDateTime) {
      // The input gave us YYYY-MM-DDTHH:MM in local time. Construct a
      // Date and emit as an ISO string so the stored value round-trips.
      const d = new Date(raw)
      if (isNaN(d.getTime())) return null
      return d.toISOString()
    }
    // Date-only: pass through as-is. YYYY-MM-DD is valid ISO.
    return raw
  }

  const inputValue = $derived(toInputValue(value))

  function handleChange(e: Event) {
    const target = e.target as HTMLInputElement
    onUpdate(fromInputValue(target.value))
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
