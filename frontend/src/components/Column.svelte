<script lang="ts">
  import CardItem from './CardItem.svelte'
  import { GripVertical, Trash2 } from 'lucide-svelte'
  import { dnd } from '../lib/store.svelte'
  import { t } from '../lib/i18n.svelte'

  type CardData = {
    id: string
    type: string
    title: string
    tags: string[]
    due_date: string | null
    checklist_total: number
    checklist_done: number
  }

  type CategoryData = {
    id: string
    name: string
    slug: string
    position: number
    cards: CardData[]
  }

  let { category, onCardClick, onAddCard, onCardDrop, onDeleteCategory, renaming, renamingName, onRenamingNameChange, onCommitRename, onCancelRename }: {
    category: CategoryData
    onCardClick?: (cardId: string) => void
    onAddCard?: (categoryId: string) => void
    onCardDrop?: (cardId: string, fromCategoryId: string, toCategoryId: string, toIndex: number) => void
    onDeleteCategory?: (categoryId: string, categorySlug: string, categoryName: string, cardCount: number) => void
    renaming?: boolean
    renamingName?: string
    onRenamingNameChange?: (value: string) => void
    onCommitRename?: () => void
    onCancelRename?: () => void
  } = $props()

  let renameInputEl = $state<HTMLInputElement | null>(null)

  $effect(() => {
    if (renaming && renameInputEl) {
      renameInputEl.focus()
      renameInputEl.select()
    }
  })


  // --- Card drop zone ---
  function handleCardDragOver(e: DragEvent) {
    if (dnd.dragging?.type !== 'card') return
    e.preventDefault()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
    dnd.overCategoryId = category.id

    // Calculate which card index we're hovering over
    const list = (e.currentTarget as HTMLElement)
    const cards = Array.from(list.querySelectorAll('.card-wrapper'))
    let idx = cards.length // default: append at end
    for (let i = 0; i < cards.length; i++) {
      const rect = cards[i].getBoundingClientRect()
      if (e.clientY < rect.top + rect.height / 2) {
        idx = i
        break
      }
    }
    dnd.overCardIndex = idx
  }

  function handleCardDragLeave(e: DragEvent) {
    // Only clear if actually leaving this column
    const related = e.relatedTarget as HTMLElement | null
    if (related && (e.currentTarget as HTMLElement).contains(related)) return
    if (dnd.overCategoryId === category.id) {
      dnd.overCategoryId = null
      dnd.overCardIndex = null
    }
  }

  function handleCardDropOnList(e: DragEvent) {
    e.preventDefault()
    if (dnd.dragging?.type !== 'card') return
    const { cardId, fromCategoryId } = dnd.dragging
    const toIndex = dnd.overCardIndex ?? category.cards.length
    onCardDrop?.(cardId, fromCategoryId, category.id, toIndex)
    dnd.dragging = null
    dnd.overCategoryId = null
    dnd.overCardIndex = null
  }

  // --- Column drag ---
  function handleColDragStart(e: DragEvent) {
    if (!e.dataTransfer) return
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', category.id)
    dnd.dragging = { type: 'column', categoryId: category.id }
  }

  function handleColDragEnd() {
    dnd.dragging = null
    dnd.overColumnIndex = null
  }

</script>

<div class="column">
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="column-header"
    draggable="true"
    ondragstart={handleColDragStart}
    ondragend={handleColDragEnd}
  >
    <span class="drag-handle" title={t('tooltip.drag_column')}><GripVertical size={14} /></span>
    {#if renaming}
      <input
        class="column-rename-input"
        bind:this={renameInputEl}
        value={renamingName}
        oninput={(e: Event) => onRenamingNameChange?.((e.target as HTMLInputElement).value)}
        onkeydown={(e: KeyboardEvent) => { if (e.key === 'Enter') onCommitRename?.(); if (e.key === 'Escape') onCancelRename?.() }}
        onblur={() => onCommitRename?.()}
      />
    {:else}
      <h3 class="column-title">{category.name}</h3>
      <span class="card-count">{category.cards.length}</span>
      <button
        class="col-delete-btn"
        title={t('tooltip.delete_category')}
        onclick={(e: MouseEvent) => { e.stopPropagation(); onDeleteCategory?.(category.id, category.slug, category.name, category.cards.length) }}
      ><Trash2 size={13} /></button>
    {/if}
  </div>

  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="card-list"
    ondragover={handleCardDragOver}
    ondragleave={handleCardDragLeave}
    ondrop={handleCardDropOnList}
  >
    {#each category.cards as card, i (card.id)}
      {#if dnd.dragging?.type === 'card' && dnd.overCategoryId === category.id && dnd.overCardIndex === i}
        <div class="drop-indicator"></div>
      {/if}
      <div class="card-wrapper">
        <CardItem {card} categoryId={category.id} onclick={() => onCardClick?.(card.id)} />
      </div>
    {/each}
    {#if dnd.dragging?.type === 'card' && dnd.overCategoryId === category.id && (dnd.overCardIndex ?? 0) >= category.cards.length}
      <div class="drop-indicator"></div>
    {/if}
  </div>

  <div class="column-footer">
    <button class="add-card-btn" onclick={() => onAddCard?.(category.id)} title={t('tooltip.add_card')}>
      {t('column.add_card_long')}
    </button>
  </div>
</div>

<style>
  .column {
    width: 272px;
    min-width: 272px;
    max-height: calc(100vh - 4rem);
    background: var(--bg-surface);
    border-radius: 10px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    transition: outline 0.15s;
  }

  .column-header {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.6rem 0.75rem;
    cursor: grab;
  }

  .drag-handle {
    color: var(--text-faint);
    display: flex;
    align-items: center;
    flex-shrink: 0;
  }

  .card-wrapper {
    width: 100%;
  }

  .drop-indicator {
    height: 3px;
    background: var(--accent);
    border-radius: 2px;
    margin: 0 0.25rem;
  }

  .column-rename-input {
    flex: 1;
    font-size: 0.85rem;
    font-weight: 600;
    background: var(--bg-elevated);
    border: 1px solid var(--accent);
    border-radius: 4px;
    color: var(--text-strong);
    padding: 0.2rem 0.4rem;
    outline: none;
    min-width: 0;
  }

  .column-title {
    margin: 0;
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--text-strong);
  }

  .card-count {
    font-size: 0.7rem;
    color: var(--text-muted);
    background: var(--bg-elevated);
    padding: 0.1rem 0.4rem;
    border-radius: 10px;
  }

  .col-delete-btn {
    background: none;
    border: none;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0.2rem;
    border-radius: 4px;
    display: flex;
    align-items: center;
    margin-left: auto;
    opacity: 0;
    transition: opacity 0.15s, color 0.15s;
  }
  .column-header:hover .col-delete-btn { opacity: 1; }
  .col-delete-btn:hover { color: var(--danger); }

  .card-list {
    flex: 1;
    overflow-y: auto;
    padding: 0 0.5rem;
    min-height: 40px;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .column-footer {
    padding: 0.5rem;
  }

  .add-card-btn {
    width: 100%;
    padding: 0.4rem 0.5rem;
    background: none;
    border: none;
    border-radius: 6px;
    color: var(--text-muted);
    font-size: 0.85rem;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s, color 0.1s;
  }

  .add-card-btn:hover {
    background: var(--bg-elevated);
    color: var(--text-body);
  }

</style>
