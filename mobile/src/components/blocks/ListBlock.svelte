<script lang="ts">
  import { flushSync } from 'svelte'
  import { X, Plus, GripVertical } from 'lucide-svelte'
  import { t } from '../../lib/i18n.svelte'
  import type { Block, ListItem } from '@shared/types'
  import { asList, withValue, newID } from './narrow'
  import { dragSortable, type DragMoveDetail } from '../../lib/actions/dnd.svelte'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  const items = $derived(asList(block.value))

  let drafts = $state<Record<string, string>>({})

  function commitItems(next: ListItem[]) {
    onChange(withValue(block, next))
  }

  function deleteItem(id: string) {
    commitItems(items.filter((it) => it.id !== id))
    delete drafts[id]
  }

  function commitText(id: string, next: string) {
    const cur = items.find((it) => it.id === id)
    if (!cur || cur.text === next) return
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
    commitItems([...items, { id, text: '' }])
    queueMicrotask(() => {
      const el = document.querySelector<HTMLInputElement>(`[data-list-input="${id}"]`)
      el?.focus()
    })
  }

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
  data-drop-target="list-items"
  use:dragSortable={{
    onMove: handleReorder,
    rowSelector: '.row[data-list-id]',
    dropTargetSelector: '[data-drop-target="list-items"]',
    rowIdAttribute: 'data-list-id',
    handleSelector: '.drag-handle',
    restoreDOM: false,
  }}
>
  {#each items as item (item.id)}
    <li class="row" data-list-id={item.id}>
      <button
        type="button"
        class="drag-handle"
        aria-label={t('block.list.reorder')}
        title={t('block.list.reorder')}
      >
        <GripVertical size={16} />
      </button>
      <span class="bullet" aria-hidden="true">•</span>
      <input
        class="text"
        type="text"
        data-list-input={item.id}
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
        aria-label={t('block.list.delete')}
      >
        <X size={14} />
      </button>
    </li>
  {/each}
  <li class="row add-row">
    <button type="button" class="add" onclick={addItem}>
      <Plus size={14} />
      <span>{t('block.list.add')}</span>
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
  .bullet {
    color: var(--text-muted);
    width: 0.75rem;
    text-align: center;
    flex-shrink: 0;
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
    /* Sit under the bullet column (skip the drag-handle column). */
    margin-left: 1.85rem;
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
