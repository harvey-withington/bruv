<script lang="ts">
  import { Square, CheckSquare, Trash2, ArrowUpRight, GripVertical } from 'lucide-svelte'
  import EditableText from './EditableText.svelte'
  import { t } from '../lib/i18n.svelte'
  import { getContext } from 'svelte'
  import { EDIT_SCOPE_KEY, type EditScope } from '@shared/editScope'
  import { inlineEdit } from '../lib/actions'
  import { computeReorder, wouldReorder, DROP_END } from '../lib/reorder'

  type ChecklistItem = { id: string; text: string; done: boolean }

  let {
    items = [],
    placeholder = '',
    onUpdate,
    onPromote,
  }: {
    items?: ChecklistItem[]
    placeholder?: string
    onUpdate?: (items: ChecklistItem[]) => void
    onPromote?: (text: string) => void
  } = $props()

  let newText = $state('')
  let addInputEl = $state<HTMLInputElement | null>(null)

  const editScope = getContext<EditScope | undefined>(EDIT_SCOPE_KEY) ?? null

  function emit(updated: ChecklistItem[]) {
    onUpdate?.(updated)
  }

  function toggleItem(id: string) {
    emit(items.map(item => item.id === id ? { ...item, done: !item.done } : item))
  }

  function uncheckAll() {
    emit(items.map(item => item.done ? { ...item, done: false } : item))
  }

  // Surface the "uncheck all" affordance only when there's something to
  // uncheck. Reading the bar already tells the user the count visually;
  // the button gives them a one-click reset without confirm (it's
  // non-destructive — text and order are preserved, only `done` flips).
  const hasChecked = $derived(items.some(it => it.done))

  function removeItem(id: string) {
    emit(items.filter(item => item.id !== id))
  }

  function addItem() {
    const text = newText.trim()
    if (!text) return
    const id = `ck-${crypto.randomUUID().slice(0, 8)}`
    emit([...items, { id, text, done: false }])
    newText = ''
  }

  function saveItemText(id: string, text: string) {
    if (!text) return
    emit(items.map(item => item.id === id ? { ...item, text } : item))
  }

  function promoteItem(id: string) {
    const item = items.find(i => i.id === id)
    if (!item || !onPromote) return
    onPromote(item.text)
    // Mark as done after promotion
    emit(items.map(i => i.id === id ? { ...i, done: true } : i))
  }

  function focusDeleteButton(itemId: string) {
    setTimeout(() => {
      const row = document.querySelector(`.cl-item[data-item-id="${itemId}"]`)
      const removeBtn = row?.querySelector('.cl-remove') as HTMLElement
      removeBtn?.focus()
    }, 0)
  }

  // Serial add-input per the keyboard entry contract: Enter commits the
  // item and re-arms, Escape discards the draft and ends the entry,
  // blur keeps the draft uncommitted.
  function cancelAdd() {
    newText = ''
    addInputEl?.blur()
  }

  // --- Drag-to-reorder ---
  // Mirrors the BlockItem / OptionsEditorDialog pattern: ID-keyed
  // drop targets, drop indicator painted only when the drop would
  // actually change order, reorder via computeReorder so the array
  // mutation lives in one tested helper.
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

{#if items.length > 0}
  <div class="cl-bar-row">
    {#if hasChecked}
      <button class="cl-uncheck-all" onclick={uncheckAll} title={t('tooltip.uncheck_all')} aria-label={t('tooltip.uncheck_all')}>
        <Square size={16} />
      </button>
    {/if}
    <div class="cl-bar">
      <div
        class="cl-bar-fill"
        style="width: {(items.filter(c => c.done).length / items.length) * 100}%"
      ></div>
    </div>
  </div>
{/if}

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="cl-items"
  role="list"
  ondrop={handleDrop}
  ondragover={(e) => { if (draggingId !== null) e.preventDefault() }}
>
  {#each items as item, idx (item.id)}
    {#if draggingId !== null && dropBeforeId === item.id}
      <div class="cl-drop-indicator"></div>
    {/if}
    <div
      class="cl-item action-reveal-parent"
      class:done={item.done}
      class:cl-item-dragging={draggingId === item.id}
      data-item-id={item.id}
      role="listitem"
      ondragover={(e) => handleDragOver(e, item.id, idx)}
    >
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
      <span
        class="cl-drag-handle"
        draggable={true}
        ondragstart={(e) => handleDragStart(e, item.id)}
        ondragend={handleDragEnd}
        role="button"
        tabindex="-1"
        aria-label={t('tooltip.drag_checklist_item')}
        title={t('tooltip.drag_checklist_item')}
      ><GripVertical size={14} /></span>
      <button class="cl-checkbox" onclick={() => toggleItem(item.id)} title={t('tooltip.toggle_checklist')}>
        {#if item.done}<CheckSquare size={16} />{:else}<Square size={16} />{/if}
      </button>
      <EditableText
        value={item.text}
        inlineMarkdown
        class="cl-text"
        onSave={(text) => saveItemText(item.id, text)}
        onTab={() => focusDeleteButton(item.id)}
      />
      {#if onPromote}
        <button class="action-reveal cl-promote" onclick={() => promoteItem(item.id)} title={t('tooltip.promote_to_card')}><ArrowUpRight size={12} /></button>
      {/if}
      <button class="action-reveal action-reveal--danger cl-remove" onclick={() => removeItem(item.id)} title={t('tooltip.remove_checklist_item')}><Trash2 size={12} /></button>
    </div>
  {/each}
  {#if draggingId !== null && dropBeforeId === DROP_END}
    <div class="cl-drop-indicator"></div>
  {/if}
</div>

<div class="cl-add">
  <input
    type="text"
    bind:this={addInputEl}
    bind:value={newText}
    use:inlineEdit={{ serial: true, onCommit: addItem, onCancel: cancelAdd, scope: editScope }}
    placeholder={placeholder || t('card.checklist_placeholder')}
    class="cl-add-input"
  />
  <button class="cl-add-btn" onclick={addItem}>{t('common.add')}</button>
</div>

<style>
  .cl-bar-row {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    /* Indent past the drag-handle column (16px) + row gap (0.35rem)
       so the uncheck-all icon's left edge lines up with the row
       checkbox's left edge below. */
    padding-left: calc(16px + 0.35rem);
    margin-bottom: 0.4rem;
  }
  .cl-bar {
    flex: 1;
    height: 4px;
    background: var(--bg-elevated);
    border-radius: 2px;
    overflow: hidden;
  }
  .cl-bar-fill {
    height: 100%;
    background: var(--success);
    border-radius: 2px;
    transition: width var(--duration-moderate);
  }
  /* Sized to match the row checkbox so the icon lines up in the
     same column visually (.cl-bar-row's padding-left places the
     icon's left edge where the row checkbox's left edge is). */
  .cl-uncheck-all {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0;
    border-radius: 3px;
    width: 16px;
    height: 16px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .cl-uncheck-all:hover,
  .cl-uncheck-all:focus-visible {
    color: var(--text);
    background: var(--bg-elevated);
    outline: none;
  }

  .cl-items {
    display: flex;
    flex-direction: column;
  }

  .cl-item {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 2px 0;
    border-radius: 4px;
  }
  .cl-item.done .cl-checkbox { color: var(--success); }
  .cl-item.done :global(.cl-text) { text-decoration: line-through; color: var(--text-faint); }
  .cl-item-dragging { opacity: 0.4; }

  /* Drag handle: revealed on row hover via action-reveal-parent so
     it doesn't add visual noise when not interacting. Sits in its
     own column so cursor doesn't fight with the EditableText
     surface. */
  .cl-drag-handle {
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
  .cl-item:hover .cl-drag-handle,
  .cl-drag-handle:focus-visible {
    opacity: 1;
  }
  .cl-drag-handle:active {
    cursor: grabbing;
  }

  .cl-drop-indicator {
    height: 2px;
    background: var(--accent);
    border-radius: 1px;
    margin: 1px 0;
  }

  .cl-checkbox {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 1rem;
    padding: 0;
  }

  :global(.cl-text) {
    flex: 1;
    font-size: 0.85rem;
    color: var(--text-body);
  }

  .cl-promote {
    font-size: 0.75rem;
    color: var(--accent);
  }

  .cl-remove {
    font-size: 0.75rem;
  }

  .cl-add {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.4rem;
  }

  .cl-add-input {
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
  .cl-add-input:focus { border-color: var(--accent); }

  .cl-add-btn {
    padding: 0.3rem 0.6rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-secondary);
    font-size: 0.8rem;
    cursor: pointer;
    white-space: nowrap;
  }
  .cl-add-btn:hover {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
  }
</style>
