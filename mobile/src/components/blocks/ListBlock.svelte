<script lang="ts">
  import { flushSync } from 'svelte'
  import { X, Plus, GripVertical } from 'lucide-svelte'
  import { t } from '../../lib/i18n.svelte'
  import type { Block, ListItem } from '@shared/types'
  import { asList, withValue, newID } from './narrow'
  import { dragSortable, type DragMoveDetail } from '../../lib/actions/dnd.svelte'
  import EditableItemText from './EditableItemText.svelte'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  const items = $derived(asList(block.value))

  function commitItems(next: ListItem[]) {
    onChange(withValue(block, next))
  }

  function deleteItem(id: string) {
    commitItems(items.filter((it) => it.id !== id))
  }

  function commitText(id: string, next: string) {
    const cur = items.find((it) => it.id === id)
    if (!cur || cur.text === next) return
    commitItems(items.map((it) => (it.id === id ? { ...it, text: next } : it)))
  }

  function addItem() {
    // A pending commit from the just-edited row reaches us through onChange →
    // the parent's card.blocks → our `block` prop, which does NOT propagate
    // back synchronously. Flush it before reading `items`, otherwise we'd
    // append to a stale array and clobber the just-typed text on the last row.
    // The new blank row auto-enters edit mode and focuses itself.
    flushSync()
    commitItems([...items, { id: newID(), text: '' }])
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
      <EditableItemText
        text={item.text}
        autoEdit={item.text === ''}
        placeholder={t('block.list.placeholder')}
        onSave={(v) => commitText(item.id, v)}
        onEmpty={() => deleteItem(item.id)}
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
