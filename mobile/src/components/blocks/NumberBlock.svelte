<script lang="ts">
  import { untrack } from 'svelte'
  import { t } from '../../lib/i18n.svelte'
  import type { Block } from '@shared/types'
  import { asNumber, withValue } from './narrow'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  // Local string draft so the input doesn't fight the user (e.g. when
  // they're mid-typing "1.5" the "1." intermediate value is invalid as
  // a number but valid as a draft). Persist when the parsed number
  // changes, or on blur.
  // untrack: seed once from the prop; the input owns the value.
  let draft = $state(untrack(() => formatInitial(block.value)))
  let lastSavedNumber = $state<number | null>(untrack(() => asNumber(block.value)))

  function formatInitial(v: unknown): string {
    const n = asNumber(v)
    return n === null ? '' : String(n)
  }

  function handleInput(e: Event) {
    const next = (e.currentTarget as HTMLInputElement).value
    draft = next
    const trimmed = next.trim()
    if (trimmed === '') {
      if (lastSavedNumber !== null) {
        lastSavedNumber = null
        onChange(withValue(block, null))
      }
      return
    }
    const parsed = Number(trimmed)
    if (Number.isFinite(parsed) && parsed !== lastSavedNumber) {
      lastSavedNumber = parsed
      onChange(withValue(block, parsed))
    }
  }

  // External-edit sync (SSE → block replaced from outside). Re-seed
  // the draft when block.value changes externally AND the user isn't
  // mid-typing.
  $effect(() => {
    const nextNum = asNumber(block.value)
    if (nextNum === lastSavedNumber) return
    const draftNum = draft.trim() === '' ? null : Number(draft)
    if (draftNum !== lastSavedNumber) return // in-flight typing
    lastSavedNumber = nextNum
    draft = nextNum === null ? '' : String(nextNum)
  })
</script>

<div class="row">
  <input
    type="number"
    inputmode="decimal"
    class="num"
    placeholder={t('block.number.placeholder')}
    value={draft}
    oninput={handleInput}
    min={block.meta?.min}
    max={block.meta?.max}
  />
  {#if block.meta?.suffix}
    <span class="suffix">{block.meta.suffix}</span>
  {/if}
</div>

<style>
  .row {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
  }
  .num {
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    padding: 0.55rem 0.7rem;
    width: 9rem;
  }
  .num:focus {
    outline: none;
    border-color: var(--accent);
  }
  .suffix {
    color: var(--text-muted);
    font-size: 0.85rem;
  }
</style>
