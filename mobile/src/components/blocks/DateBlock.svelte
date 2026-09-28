<script lang="ts">
  import type { Block } from '@shared/types'
  import { isoToDateInput, isoToLocalInput, localInputToIso } from '@shared/dateTimeInput'
  import { asString, withValue } from './narrow'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  // Block.meta.format = "date" (YYYY-MM-DD) | "date-time" (full ISO).
  // Default "date" matches model.go's documented default.
  const isDateTime = $derived(block.meta?.format === 'date-time')

  // Native input expects the local format ("YYYY-MM-DD" for date,
  // "YYYY-MM-DDTHH:MM" for datetime-local); the on-disk value is always
  // ISO 8601. Plain dates pass through with no time/zone implied.
  // Phone date pickers are touch-friendly out of the box.
  const inputVal = $derived(isDateTime ? isoToLocalInput(asString(block.value)) : isoToDateInput(asString(block.value)))

  function handleInput(e: Event) {
    const next = (e.currentTarget as HTMLInputElement).value
    onChange(withValue(block, isDateTime ? (localInputToIso(next) ?? '') : next))
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
