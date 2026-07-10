<script lang="ts">
  import { untrack, getContext } from 'svelte'
  import { t } from '../../lib/i18n.svelte'
  import { EDIT_SCOPE_KEY, type EditScope } from '@shared/editScope'
  import type { Block } from '@shared/types'
  import { asNumber, withValue } from './narrow'
  import { draftEdit } from '../../lib/actions/draftEdit'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  // Local string draft so the input doesn't fight the user (e.g. when
  // they're mid-typing "1.5" the "1." intermediate value is invalid as
  // a number but valid as a draft). Committed explicitly per the
  // keyboard entry contract (draftEdit action: Enter/blur commit,
  // Escape reverts) rather than live per-keystroke.
  // untrack: seed once from the prop; the input owns the value.
  let draft = $state(untrack(() => formatInitial(block.value)))
  let lastSavedNumber = $state<number | null>(untrack(() => asNumber(block.value)))

  const editScope = getContext<EditScope | undefined>(EDIT_SCOPE_KEY) ?? null

  function formatInitial(v: unknown): string {
    const n = asNumber(v)
    return n === null ? '' : String(n)
  }

  function commitDraft() {
    const trimmed = draft.trim()
    if (trimmed === '') {
      if (lastSavedNumber !== null) {
        lastSavedNumber = null
        onChange(withValue(block, null))
      }
      return
    }
    const parsed = Number(trimmed)
    if (!Number.isFinite(parsed)) {
      // Unparseable leftovers (e.g. "1e") — revert rather than persist
      // garbage; the last committed number stays authoritative.
      revertDraft()
      return
    }
    if (parsed !== lastSavedNumber) {
      lastSavedNumber = parsed
      onChange(withValue(block, parsed))
    }
  }

  function revertDraft() {
    draft = lastSavedNumber === null ? '' : String(lastSavedNumber)
  }

  // Fresh block prop (re-keyed to a different block): re-seed.
  let seededID = untrack(() => block.id)
  $effect(() => {
    if (block.id === seededID) return
    seededID = block.id
    untrack(() => {
      lastSavedNumber = asNumber(block.value)
      draft = formatInitial(block.value)
    })
  })

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
    oninput={(e) => (draft = (e.currentTarget as HTMLInputElement).value)}
    enterkeyhint="done"
    use:draftEdit={{ onCommit: commitDraft, onCancel: revertDraft, scope: editScope }}
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
