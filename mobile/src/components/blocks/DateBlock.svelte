<script lang="ts">
  import type { Block } from '@shared/types'
  import { asString, withValue } from './narrow'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  // Block.meta.format = "date" (YYYY-MM-DD) | "date-time" (full ISO).
  // Default "date" matches model.go's documented default.
  const isDateTime = $derived(block.meta?.format === 'date-time')

  // Native input expects the local format ("YYYY-MM-DD" for date,
  // "YYYY-MM-DDTHH:MM" for datetime-local). Input.value is stored in
  // local time — conversion to/from ISO 8601 is intentional here:
  // the on-disk value is always normalised, the input always shows
  // local. Phone date pickers are touch-friendly out of the box.
  function toInputValue(raw: string): string {
    if (!raw) return ''
    const d = new Date(raw)
    if (Number.isNaN(d.getTime())) return ''
    if (isDateTime) {
      const yr = d.getFullYear().toString().padStart(4, '0')
      const mo = (d.getMonth() + 1).toString().padStart(2, '0')
      const dy = d.getDate().toString().padStart(2, '0')
      const hr = d.getHours().toString().padStart(2, '0')
      const mn = d.getMinutes().toString().padStart(2, '0')
      return `${yr}-${mo}-${dy}T${hr}:${mn}`
    }
    return d.toISOString().slice(0, 10)
  }

  function fromInputValue(s: string): string {
    if (!s) return ''
    if (isDateTime) {
      const d = new Date(s)
      if (Number.isNaN(d.getTime())) return s
      return d.toISOString()
    }
    // Plain date: keep as YYYY-MM-DD with no time/zone implied.
    return s
  }

  const inputVal = $derived(toInputValue(asString(block.value)))

  function handleInput(e: Event) {
    const next = (e.currentTarget as HTMLInputElement).value
    onChange(withValue(block, fromInputValue(next)))
  }
</script>

{#if isDateTime}
  <input type="datetime-local" class="date" value={inputVal} oninput={handleInput} />
{:else}
  <input type="date" class="date" value={inputVal} oninput={handleInput} />
{/if}

<style>
  .date {
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    padding: 0.55rem 0.7rem;
    color-scheme: dark light;
  }
  .date:focus {
    outline: none;
    border-color: var(--accent);
  }
</style>
