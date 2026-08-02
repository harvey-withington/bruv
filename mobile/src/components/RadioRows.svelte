<script module lang="ts">
  /** One choice in the list. `sub` is the quiet second line. */
  export type RadioRow = {
    key: string
    label: string
    sub?: string
  }
</script>

<script lang="ts">
  import { Check } from 'lucide-svelte'

  // A tap-sized list of mutually exclusive choices — the shape both the
  // Capture Options sheet (video rungs, image handling) and Settings →
  // Capture (video/image/ask modes) need. Rows are labels wrapping a real
  // radio input, so keyboard and screen readers get the semantics for
  // free while the thumb gets a 48px target.

  let {
    name,
    rows,
    value,
    onChange,
    disabled = false,
  }: {
    /** Radio group name — unique per list on the page. */
    name: string
    rows: RadioRow[]
    value: string
    onChange: (key: string) => void
    disabled?: boolean
  } = $props()
</script>

<div class="rows" role="radiogroup">
  {#each rows as row (row.key)}
    <label class="row" class:selected={row.key === value} class:disabled>
      <input
        type="radio"
        {name}
        value={row.key}
        checked={row.key === value}
        {disabled}
        onchange={() => onChange(row.key)}
      />
      <span class="text">
        <span class="label">{row.label}</span>
        {#if row.sub}<span class="sub">{row.sub}</span>{/if}
      </span>
      {#if row.key === value}
        <span class="tick" aria-hidden="true"><Check size={16} /></span>
      {/if}
    </label>
  {/each}
</div>

<style>
  .rows {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    min-height: 44px;
    padding: 0.5rem 0.7rem;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    cursor: pointer;
    touch-action: manipulation;
  }
  .row.selected {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 8%, var(--bg));
  }
  .row.disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }

  .row input {
    width: 18px;
    height: 18px;
    accent-color: var(--accent);
    flex-shrink: 0;
    margin: 0;
  }

  .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  .label {
    font-size: 0.9rem;
    color: var(--text);
  }

  .sub {
    font-size: 0.75rem;
    color: var(--text-muted);
  }

  .tick {
    color: var(--accent);
    display: inline-flex;
    flex-shrink: 0;
  }
</style>
