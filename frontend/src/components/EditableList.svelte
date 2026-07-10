<script lang="ts">
  import { Trash2, GripVertical } from 'lucide-svelte'
  import EditableText from './EditableText.svelte'
  import { t } from '../lib/i18n.svelte'
  import { getContext } from 'svelte'
  import { EDIT_SCOPE_KEY, type EditScope } from '@shared/editScope'
  import { inlineEdit } from '../lib/actions'
  import { computeReorder, wouldReorder, DROP_END } from '../lib/reorder'

  type ListItem = { id: string; text: string }

  let {
    items = [],
    placeholder = '',
    onUpdate,
  }: {
    items?: ListItem[]
    placeholder?: string
    onUpdate?: (items: ListItem[]) => void
  } = $props()

  let newText = $state('')
  let addInputEl = $state<HTMLInputElement | null>(null)

  const editScope = getContext<EditScope | undefined>(EDIT_SCOPE_KEY) ?? null

  function emit(updated: ListItem[]) {
    onUpdate?.(updated)
  }

  function removeItem(id: string) {
    emit(items.filter(item => item.id !== id))
  }

  function addItem() {
    const text = newText.trim()
    if (!text) return
    const id = `li-${crypto.randomUUID().slice(0, 8)}`
    emit([...items, { id, text }])
    newText = ''
  }

  function saveItemText(id: string, text: string) {
    if (!text) return
    emit(items.map(item => item.id === id ? { ...item, text } : item))
  }

  function focusDeleteButton(itemId: string) {
    setTimeout(() => {
      const row = document.querySelector(`.li-item[data-item-id="${itemId}"]`)
      const removeBtn = row?.querySelector('.li-remove') as HTMLElement
      removeBtn?.focus()
    }, 0)
  }

  // Serial add-input per the keyboard entry contract (see
  // EditableChecklist for the same pattern).
  function cancelAdd() {
    newText = ''
    addInputEl?.blur()
  }

  // --- Drag-to-reorder (HTML5 native, mirrors EditableChecklist) ---
  let draggingId = $state<string | null>(null)
  let dropBeforeId = $state<string | typeof DROP_END | null>(null)

  function handleDragStart(e: DragEvent, id: string) {
    draggingId = id
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move'
      e.dataTransfer.setData('text/plain', id)
    }
  }

  function handleDragOver(e: DragEvent, overId: string, idx: number) {
    if (draggingId === null) return
    e.preventDefault()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const midY = rect.top + rect.height / 2
    let candidate: string | typeof DROP_END
    if (e.clientY < midY) {
      candidate = overId
    } else {
      const next = items[idx + 1]
      candidate = next ? next.id : DROP_END
    }
    dropBeforeId = wouldReorder(items, draggingId, candidate, 'move') ? candidate : null
  }

  function handleDragEnd() {
    draggingId = null
    dropBeforeId = null
  }

  function handleDrop(e: DragEvent) {
    e.preventDefault()
    if (draggingId === null || dropBeforeId === null) {
      handleDragEnd()
      return
    }
    const reordered = computeReorder(items, draggingId, dropBeforeId, { mode: 'move' })
    handleDragEnd()
    if (reordered !== items) emit(reordered)
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="li-items"
  role="list"
  ondrop={handleDrop}
  ondragover={(e) => { if (draggingId !== null) e.preventDefault() }}
>
  {#each items as item, idx (item.id)}
    {#if draggingId !== null && dropBeforeId === item.id}
      <div class="li-drop-indicator"></div>
    {/if}
    <div
      class="li-item action-reveal-parent"
      class:li-item-dragging={draggingId === item.id}
      data-item-id={item.id}
      role="listitem"
      ondragover={(e) => handleDragOver(e, item.id, idx)}
    >
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
      <span
        class="li-drag-handle"
        draggable={true}
        ondragstart={(e) => handleDragStart(e, item.id)}
        ondragend={handleDragEnd}
        role="button"
        tabindex="-1"
        aria-label={t('tooltip.drag_list_item')}
        title={t('tooltip.drag_list_item')}
      ><GripVertical size={14} /></span>
      <span class="li-bullet">&#8226;</span>
      <EditableText
        value={item.text}
        inlineMarkdown
        class="li-text"
        onSave={(text) => saveItemText(item.id, text)}
        onTab={() => focusDeleteButton(item.id)}
      />
      <button class="action-reveal action-reveal--danger li-remove" onclick={() => removeItem(item.id)} title={t('tooltip.remove_checklist_item')}><Trash2 size={12} /></button>
    </div>
  {/each}
  {#if draggingId !== null && dropBeforeId === DROP_END}
    <div class="li-drop-indicator"></div>
  {/if}
</div>

<div class="li-add">
  <input
    type="text"
    bind:this={addInputEl}
    bind:value={newText}
    use:inlineEdit={{ serial: true, onCommit: addItem, onCancel: cancelAdd, scope: editScope }}
    placeholder={placeholder || t('block.list_placeholder')}
    class="li-add-input"
  />
  <button class="li-add-btn" onclick={addItem}>{t('block.list_add')}</button>
</div>

<style>
  .li-items {
    display: flex;
    flex-direction: column;
  }

  .li-item {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 2px 0;
    border-radius: 4px;
  }
  .li-item-dragging { opacity: 0.4; }

  /* Drag handle hidden until row hover so the column stays clean
     when the user is just reading. */
  .li-drag-handle {
    color: var(--text-faint);
    cursor: grab;
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 16px;
    height: 16px;
    opacity: 0;
    transition: opacity var(--duration-fast) var(--ease-out);
  }
  .li-item:hover .li-drag-handle,
  .li-drag-handle:focus-visible {
    opacity: 1;
  }
  .li-drag-handle:active {
    cursor: grabbing;
  }

  .li-drop-indicator {
    height: 2px;
    background: var(--accent);
    border-radius: 1px;
    margin: 1px 0;
  }

  .li-bullet {
    color: var(--text-muted);
    font-size: 1rem;
    line-height: 1;
    flex-shrink: 0;
    width: 16px;
    text-align: center;
  }

  :global(.li-text) {
    flex: 1;
    font-size: 0.85rem;
    color: var(--text-body);
  }

  .li-remove {
    font-size: 0.75rem;
  }

  .li-add {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.4rem;
  }

  .li-add-input {
    flex: 1;
    padding: 0.3rem 0.5rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    font-family: inherit;
    outline: none;
  }
  .li-add-input:focus { border-color: var(--accent); }

  .li-add-btn {
    padding: 0.3rem 0.6rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-secondary);
    font-size: 0.8rem;
    cursor: pointer;
    white-space: nowrap;
  }
  .li-add-btn:hover {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
  }
</style>
