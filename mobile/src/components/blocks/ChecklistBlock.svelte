<script lang="ts">
  import { flushSync } from 'svelte'
  import { Square, CheckSquare, X, Plus, GripVertical } from 'lucide-svelte'
  import { t } from '../../lib/i18n.svelte'
  import type { Block, ChecklistItem } from '@shared/types'
  import { asChecklist, withValue, newID } from './narrow'
  import { dragSortable, type DragMoveDetail } from '../../lib/actions/dnd.svelte'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  const items = $derived(asChecklist(block.value))

  // Local drafts so typing doesn't fight upstream re-renders. Keyed
  // by item.id so reorder/delete don't shift drafts onto wrong rows.
  let drafts = $state<Record<string, string>>({})

  function commitItems(next: ChecklistItem[]) {
    onChange(withValue(block, next))
  }

  function toggle(id: string) {
    commitItems(items.map((it) => (it.id === id ? { ...it, done: !it.done } : it)))
  }

  function uncheckAll() {
    commitItems(items.map((it) => (it.done ? { ...it, done: false } : it)))
  }

  // Only surface "uncheck all" when there's something to uncheck —
  // the action is non-destructive (text + order preserved) so no
  // confirm is needed.
  const hasChecked = $derived(items.some((it) => it.done))

  function deleteItem(id: string) {
    commitItems(items.filter((it) => it.id !== id))
    delete drafts[id]
  }

  function commitText(id: string, next: string) {
    const cur = items.find((it) => it.id === id)
    if (!cur) return
    if (cur.text === next) return
    commitItems(items.map((it) => (it.id === id ? { ...it, text: next } : it)))
  }

  function addItem() {
    // A blur on the focused row may have just committed its edit (via
    // Enter, or by tapping "Add item"). That commit reaches us through
    // onChange → the parent's card.blocks → our `block` prop, which does
    // NOT propagate back synchronously. Flush it before reading `items`,
    // otherwise we'd append to a stale array and clobber the just-typed
    // text on the last row (it would save empty the first time).
    flushSync()
    const id = newID()
    commitItems([...items, { id, text: '', done: false }])
    // Defer focus until the new <input> mounts.
    queueMicrotask(() => {
      const el = document.querySelector<HTMLInputElement>(
        `[data-checklist-input="${id}"]`,
      )
      el?.focus()
    })
  }

  // Reorder via shared dragSortable action. The action mutates the DOM
  // during drag for visual feedback and fires onMove with the new
  // position once the user releases. We treat it as a single-list
  // reorder — cross-container moves don't apply to checklist items.
  function handleReorder(detail: DragMoveDetail) {
    const itemID = detail.cardID
    const fromIdx = items.findIndex((it) => it.id === itemID)
    if (fromIdx === -1) return
    const toIdx = Math.max(0, Math.min(detail.toPosition, items.length - 1))
    if (fromIdx === toIdx) return
    const next = [...items]
    const [moved] = next.splice(fromIdx, 1)
    next.splice(toIdx, 0, moved)
    commitItems(next)
  }
</script>

<ul
  class="list"
  data-drop-target="checklist-items"
  use:dragSortable={{
    onMove: handleReorder,
    rowSelector: '.row[data-checklist-id]',
    dropTargetSelector: '[data-drop-target="checklist-items"]',
    rowIdAttribute: 'data-checklist-id',
    handleSelector: '.drag-handle',
    restoreDOM: false,
  }}
>
  {#each items as item (item.id)}
    <li class="row" class:done={item.done} data-checklist-id={item.id}>
      <button
        type="button"
        class="drag-handle"
        aria-label={t('block.checklist.reorder')}
        title={t('block.checklist.reorder')}
      >
        <GripVertical size={16} />
      </button>
      <button
        type="button"
        class="check"
        onclick={() => toggle(item.id)}
        aria-pressed={item.done}
        aria-label={t('block.checkbox.toggle')}
      >
        {#if item.done}
          <CheckSquare size={20} />
        {:else}
          <Square size={20} />
        {/if}
      </button>
      <input
        class="text"
        type="text"
        data-checklist-input={item.id}
        value={drafts[item.id] ?? item.text}
        oninput={(e) => (drafts[item.id] = (e.currentTarget as HTMLInputElement).value)}
        onblur={(e) => {
          const v = (e.currentTarget as HTMLInputElement).value
          delete drafts[item.id]
          if (v.trim() === '') {
            // Don't leave an empty row behind — drop abandoned/blank items.
            deleteItem(item.id)
          } else {
            commitText(item.id, v)
          }
        }}
        onkeydown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            ;(e.currentTarget as HTMLInputElement).blur()
            addItem()
          } else if (e.key === 'Escape') {
            // Cancel the edit — and never let Escape bubble up to close the
            // card. Revert to the stored text, then blur: onblur removes a
            // blank new row, or keeps an existing row's text unchanged.
            e.preventDefault()
            e.stopPropagation()
            const input = e.currentTarget as HTMLInputElement
            delete drafts[item.id]
            input.value = item.text
            input.blur()
          }
        }}
      />
      <button
        type="button"
        class="del"
        onclick={() => deleteItem(item.id)}
        aria-label={t('block.checklist.delete')}
      >
        <X size={14} />
      </button>
    </li>
  {/each}
  <li class="row add-row">
    {#if hasChecked}
      <button
        type="button"
        class="uncheck-all"
        onclick={uncheckAll}
        aria-label={t('block.checklist.uncheck_all')}
        title={t('block.checklist.uncheck_all')}
      >
        <Square size={18} />
      </button>
    {/if}
    <button type="button" class="add" onclick={addItem}>
      <Plus size={14} />
      <span>{t('block.checklist.add')}</span>
    </button>
  </li>
</ul>

<style>
  .list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.2rem 0;
  }
  .row.done .text {
    text-decoration: line-through;
    color: var(--text-muted);
  }
  /* Dedicated grab area — keeps text-selection long-press on the
     <input> intact by giving the drag action its own surface to
     listen on (see dndhandle gate in dnd.svelte.ts). */
  .drag-handle {
    background: transparent;
    border: none;
    color: var(--text-faint);
    padding: 0.4rem 0.15rem;
    cursor: grab;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 24px;
    min-height: 36px;
    flex-shrink: 0;
    /* Block the browser's own pan gesture on this surface so the
       drag action can claim the press without scrolling underneath. */
    touch-action: none;
  }
  .drag-handle:hover,
  .drag-handle:focus-visible {
    color: var(--text-muted);
    background: var(--bg-elev-1);
    outline: none;
  }
  .drag-handle:active {
    cursor: grabbing;
  }
  .check {
    background: transparent;
    border: none;
    color: var(--text-muted);
    padding: 0.4rem;
    cursor: pointer;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 36px;
    min-height: 36px;
    flex-shrink: 0;
  }
  .check:hover,
  .check:focus-visible {
    color: var(--accent);
    background: var(--bg-elev-1);
    outline: none;
  }
  .row.done .check {
    color: var(--accent);
  }
  .text {
    flex: 1;
    min-width: 0;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    padding: 0.4rem 0.5rem;
  }
  .text:hover {
    border-color: var(--border);
  }
  .text:focus {
    outline: none;
    border-color: var(--accent);
    background: var(--bg-elev-1);
  }
  .del {
    background: transparent;
    border: none;
    color: var(--text-faint);
    padding: 0.45rem;
    cursor: pointer;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 36px;
    min-height: 36px;
    flex-shrink: 0;
  }
  .del:hover,
  .del:focus-visible {
    color: #ef4444;
    background: rgba(239, 68, 68, 0.1);
    outline: none;
  }
  .add-row {
    margin-top: 0.2rem;
    /* Indent past the drag-handle column so the uncheck-all icon
       (when present) and the "Add item" button both sit in the
       checkbox column above. */
    margin-left: 1.85rem;
    gap: 0.4rem;
  }
  .add {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    background: transparent;
    border: 1px dashed var(--border);
    color: var(--text-muted);
    font: inherit;
    font-size: 0.85rem;
    padding: 0.4rem 0.7rem;
    border-radius: 6px;
    cursor: pointer;
  }
  .add:hover,
  .add:focus-visible {
    color: var(--text);
    border-color: var(--text-muted);
    outline: none;
  }
  /* Sized + positioned to mirror the per-row .check button so the
     icon sits in the same column as the checkboxes above. */
  .uncheck-all {
    background: transparent;
    border: none;
    color: var(--text-muted);
    padding: 0.4rem;
    cursor: pointer;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 36px;
    min-height: 36px;
    flex-shrink: 0;
  }
  .uncheck-all:hover,
  .uncheck-all:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }

  /* Drag visual states (mirrors the BlockEditor pattern in CardPage). */
  .list :global(.dnd-source) {
    opacity: 0.35;
    transition: opacity 120ms ease;
  }
  :global(.list.dnd-target-active),
  .list :global(.dnd-target-active) {
    outline: 2px dashed var(--accent);
    outline-offset: 2px;
    border-radius: 8px;
    transition: outline-color 120ms ease;
  }
</style>
