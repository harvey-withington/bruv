<script lang="ts">
  import Column from './Column.svelte'
  import CardDetail from './CardDetail.svelte'
  import { board, nav, tagColors, dnd } from '../lib/store.svelte'
  import { CreateCard, PinCard, CreateCategory, ListCategories, GetCard, ListCardIDsInCategory, GetTagColors, MoveCardInCategory, MoveCardToCategory, ReorderCategories, DeleteCategory, MoveCategoryCards } from '../lib/api'
  import { X, Plus } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'

  let addingCategory = $state(false)
  let newCategoryName = $state('')
  let selectedCardId = $state<string | null>(null)

  // Category delete confirmation state
  let deletingCategory = $state<{ id: string; slug: string; name: string; cardCount: number } | null>(null)
  let moveTargetId = $state<string>('')

  async function handleAddCard(categoryId: string) {
    // Find the column component's new title
    const input = document.querySelector(`#add-input-${categoryId}`) as HTMLInputElement
    const title = input?.value?.trim()
    if (!title) return

    try {
      // Create a card of type "task" by default, pinned to this category
      const card = await CreateCard('task', title)

      // Find the category to get its ID for pinning
      const cat = board.categories.find(c => c.id === categoryId)
      if (cat) {
        // Find the project to get its ID
        // For now, pin using the category's project association
        // The project ID is stored on the category in the backend
        await PinCard(card.id, cat.id, cat.id)
      }

      // Refresh the board
      await refreshBoard()
    } catch (e) {
      console.error('Failed to add card:', e)
    }
  }

  function handleCardClick(cardId: string) {
    selectedCardId = cardId
  }

  function closeCardDetail() {
    selectedCardId = null
  }

  function handleCardUpdated() {
    refreshBoard()
  }

  async function handleCardDrop(cardId: string, fromCategoryId: string, toCategoryId: string, toIndex: number) {
    try {
      if (fromCategoryId === toCategoryId) {
        // Reorder within the same column — optimistic update
        const col = board.categories.find(c => c.id === toCategoryId)
        if (col) {
          const fromIdx = col.cards.findIndex(c => c.id === cardId)
          if (fromIdx !== -1 && fromIdx !== toIndex) {
            const card = col.cards.splice(fromIdx, 1)[0]
            const insertIdx = toIndex > fromIdx ? toIndex - 1 : toIndex
            col.cards.splice(insertIdx, 0, card)
            // Persist all positions
            for (let i = 0; i < col.cards.length; i++) {
              await MoveCardInCategory(col.cards[i].id, toCategoryId, toCategoryId, i)
            }
          }
        }
      } else {
        // Move between columns — optimistic update
        const fromCol = board.categories.find(c => c.id === fromCategoryId)
        const toCol = board.categories.find(c => c.id === toCategoryId)
        if (fromCol && toCol) {
          const fromIdx = fromCol.cards.findIndex(c => c.id === cardId)
          if (fromIdx !== -1) {
            const card = fromCol.cards.splice(fromIdx, 1)[0]
            toCol.cards.splice(toIndex, 0, card)
            // Move the card on backend
            await MoveCardToCategory(cardId, fromCategoryId, fromCategoryId, toCategoryId, toIndex)
            // Re-persist positions in both columns
            for (let i = 0; i < fromCol.cards.length; i++) {
              await MoveCardInCategory(fromCol.cards[i].id, fromCategoryId, fromCategoryId, i)
            }
            for (let i = 0; i < toCol.cards.length; i++) {
              await MoveCardInCategory(toCol.cards[i].id, toCategoryId, toCategoryId, i)
            }
          }
        }
      }
    } catch (e) {
      console.error('Card drop failed:', e)
      await refreshBoard()
    }
  }

  function handleColumnsDragOver(e: DragEvent) {
    if (dnd.dragging?.type !== 'column') return
    e.preventDefault()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'

    const container = e.currentTarget as HTMLElement
    const cols = Array.from(container.querySelectorAll(':scope > .col-slot'))
    let idx = cols.length
    for (let i = 0; i < cols.length; i++) {
      const rect = cols[i].getBoundingClientRect()
      if (e.clientX < rect.left + rect.width / 2) {
        idx = i
        break
      }
    }
    dnd.overColumnIndex = idx
  }

  function handleColumnsDragLeave(e: DragEvent) {
    const related = e.relatedTarget as HTMLElement | null
    if (related && (e.currentTarget as HTMLElement).contains(related)) return
    dnd.overColumnIndex = null
  }

  async function handleColumnsDrop(e: DragEvent) {
    e.preventDefault()
    if (!dnd.dragging || dnd.dragging.type !== 'column') return
    const draggedId = dnd.dragging.categoryId
    const fromIdx = board.categories.findIndex(c => c.id === draggedId)
    let toIdx = dnd.overColumnIndex ?? board.categories.length
    if (fromIdx === -1) return

    // Adjust target if dragging forward
    if (toIdx > fromIdx) toIdx--
    if (fromIdx === toIdx) {
      dnd.dragging = null
      dnd.overColumnIndex = null
      return
    }

    const col = board.categories.splice(fromIdx, 1)[0]
    board.categories.splice(toIdx, 0, col)
    dnd.dragging = null
    dnd.overColumnIndex = null

    // Persist
    if (nav.brandSlug && nav.streamSlug && nav.projectSlug) {
      try {
        const orderedSlugs = board.categories.map(c => c.slug)
        await ReorderCategories(nav.brandSlug, nav.streamSlug, nav.projectSlug, orderedSlugs)
      } catch (e) {
        console.error('Column reorder failed:', e)
        await refreshBoard()
      }
    }
  }

  function handleDeleteCategoryRequest(categoryId: string, categorySlug: string, categoryName: string, cardCount: number) {
    deletingCategory = { id: categoryId, slug: categorySlug, name: categoryName, cardCount }
    // Default move target: first other category
    const other = board.categories.find(c => c.id !== categoryId)
    moveTargetId = other?.id || ''
  }

  function cancelDeleteCategory() {
    deletingCategory = null
    moveTargetId = ''
  }

  async function confirmDeleteCategory() {
    if (!deletingCategory || !nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return
    const { id, slug, cardCount } = deletingCategory
    try {
      // If cards exist and a move target is chosen, move them first
      if (cardCount > 0 && moveTargetId) {
        await MoveCategoryCards(nav.brandSlug, nav.streamSlug, nav.projectSlug, id, moveTargetId)
      }
      await DeleteCategory(nav.brandSlug, nav.streamSlug, nav.projectSlug, slug)
      deletingCategory = null
      moveTargetId = ''
      await refreshBoard()
    } catch (e) {
      console.error('Delete category failed:', e)
    }
  }

  async function handleAddCategory() {
    if (!newCategoryName.trim() || !nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return

    try {
      const position = board.categories.length
      await CreateCategory(nav.brandSlug, nav.streamSlug, nav.projectSlug, newCategoryName.trim(), position)
      newCategoryName = ''
      addingCategory = false
      await refreshBoard()
    } catch (e) {
      console.error('Failed to add category:', e)
    }
  }

  function handleCategoryKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') handleAddCategory()
    else if (e.key === 'Escape') { addingCategory = false; newCategoryName = '' }
  }

  async function refreshBoard() {
    if (!nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return
    board.loading = true
    try {
      // Refresh tag colors
      try { tagColors.map = await GetTagColors() || {} } catch { /* ignore */ }

      const cats = await ListCategories(nav.brandSlug, nav.streamSlug, nav.projectSlug) || []
      const populated = await Promise.all(cats.map(async (cat: any) => {
        let cardIds: string[] = []
        try {
          cardIds = await ListCardIDsInCategory(cat.id, cat.id) || []
        } catch { /* no cards pinned yet */ }

        const cards = await Promise.all(cardIds.map(async (id: string) => {
          try {
            const card = await GetCard(id)
            return {
              id: card.id,
              type: card.type,
              title: card.title,
              tags: card.tags || [],
              due_date: card.due_date,
              checklist_total: card.checklist?.length || 0,
              checklist_done: card.checklist?.filter((c: any) => c.done).length || 0,
            }
          } catch { return null }
        }))

        return {
          id: cat.id,
          name: cat.name,
          slug: cat.slug,
          position: cat.position,
          cards: cards.filter((c): c is NonNullable<typeof c> => c !== null),
        }
      }))
      board.categories = populated
    } catch {
      board.categories = []
    }
    board.loading = false
  }
</script>

<div class="board">
  {#if board.loading}
    <div class="loading">{t('app.loading')}</div>

  {:else if !nav.projectSlug}
    <div class="empty-board">
      <p class="empty-text">{t('app.no_project')}</p>
    </div>

  {:else}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="columns" ondragover={handleColumnsDragOver} ondragleave={handleColumnsDragLeave} ondrop={handleColumnsDrop}>
      {#each board.categories as category, colIdx (category.id)}
        {#if dnd.dragging?.type === 'column' && dnd.overColumnIndex === colIdx}
          <div class="col-drop-indicator"></div>
        {/if}
        <div class="col-slot">
          <Column
            {category}
            onCardClick={handleCardClick}
            onAddCard={handleAddCard}
            onCardDrop={handleCardDrop}
            onDeleteCategory={handleDeleteCategoryRequest}
          />
        </div>
      {/each}
      {#if dnd.dragging?.type === 'column' && (dnd.overColumnIndex ?? 0) >= board.categories.length}
        <div class="col-drop-indicator"></div>
      {/if}

      <div class="add-column">
        {#if addingCategory}
          <div class="add-column-form">
            <input
              type="text"
              bind:value={newCategoryName}
              onkeydown={handleCategoryKeydown}
              placeholder={t('board.category_placeholder')}
              class="add-column-input"
            />
            <div class="add-column-actions">
              <button class="btn-add" onclick={handleAddCategory}>{t('board.add_category')}</button>
              <button class="btn-cancel" onclick={() => { addingCategory = false; newCategoryName = '' }}><X size={14} /></button>
            </div>
          </div>
        {:else}
          <button class="add-column-btn" onclick={() => { addingCategory = true; setTimeout(() => (document.querySelector('.add-column-input') as HTMLElement)?.focus(), 0) }}>
            {t('board.add_category_long')}
          </button>
        {/if}
      </div>
    </div>
  {/if}
</div>

{#if deletingCategory}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div class="delete-overlay" onclick={cancelDeleteCategory}>
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="delete-dialog" onclick={(e: MouseEvent) => e.stopPropagation()}>
      <h3 class="delete-title">{t('board.delete_category_confirm', { name: deletingCategory.name })}</h3>

      {#if deletingCategory.cardCount === 0}
        <p class="delete-msg">{t('board.delete_category_empty')}</p>
        <div class="delete-actions">
          <button class="btn-ghost" onclick={cancelDeleteCategory}>{t('common.cancel')}</button>
          <button class="btn-danger" onclick={confirmDeleteCategory}>{t('board.delete_only')}</button>
        </div>
      {:else}
        <p class="delete-msg">{t('board.delete_category_has_cards', { count: deletingCategory.cardCount })}</p>
        <label class="move-label">
          <span>{t('board.move_cards_to')}</span>
          <select bind:value={moveTargetId}>
            {#each board.categories.filter(c => c.id !== deletingCategory?.id) as cat}
              <option value={cat.id}>{cat.name}</option>
            {/each}
          </select>
        </label>
        <div class="delete-actions">
          <button class="btn-ghost" onclick={cancelDeleteCategory}>{t('common.cancel')}</button>
          <button class="btn-danger" onclick={confirmDeleteCategory} disabled={!moveTargetId}>{t('board.delete_and_move')}</button>
        </div>
      {/if}
    </div>
  </div>
{/if}

{#if selectedCardId}
  <CardDetail
    cardId={selectedCardId}
    onClose={closeCardDetail}
    onUpdated={handleCardUpdated}
  />
{/if}

<style>
  .delete-overlay {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }

  .delete-dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 1.25rem;
    width: 380px;
    box-shadow: 0 8px 32px var(--shadow-lg);
  }

  .delete-title {
    margin: 0 0 0.75rem;
    font-size: 1rem;
    font-weight: 600;
    color: var(--text-primary);
  }

  .delete-msg {
    margin: 0 0 1rem;
    font-size: 0.85rem;
    color: var(--text-secondary);
  }

  .move-label {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    margin-bottom: 1rem;
    font-size: 0.85rem;
    color: var(--text-secondary);
  }

  .move-label select {
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    outline: none;
  }
  .move-label select:focus { border-color: var(--accent); }

  .delete-actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
  }

  .btn-ghost {
    padding: 0.4rem 0.85rem;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--text-secondary);
    font-size: 0.85rem;
    cursor: pointer;
  }
  .btn-ghost:hover { color: var(--text-primary); }

  .btn-danger {
    padding: 0.4rem 0.85rem;
    border: none;
    border-radius: 6px;
    background: var(--danger);
    color: #fff;
    font-size: 0.85rem;
    font-weight: 500;
    cursor: pointer;
  }
  .btn-danger:hover { background: var(--danger-light); }
  .btn-danger:disabled { opacity: 0.5; cursor: not-allowed; }

  .board {
    flex: 1;
    overflow-x: auto;
    overflow-y: hidden;
    padding: 1rem;
    height: 100vh;
  }

  .columns {
    display: flex;
    gap: 0.75rem;
    align-items: flex-start;
    height: 100%;
  }

  .col-slot {
    flex-shrink: 0;
  }

  .col-drop-indicator {
    width: 3px;
    min-height: 80px;
    align-self: stretch;
    background: var(--accent);
    border-radius: 2px;
    flex-shrink: 0;
  }

  .loading {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    color: var(--text-muted);
    font-size: 0.9rem;
  }

  .empty-board {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
  }

  .empty-text {
    color: var(--text-faint);
    font-size: 0.95rem;
  }

  .add-column {
    min-width: 272px;
  }

  .add-column-btn {
    width: 272px;
    padding: 0.6rem 0.75rem;
    background: var(--bg-subtle);
    border: none;
    border-radius: 10px;
    color: var(--text-secondary);
    font-size: 0.85rem;
    cursor: pointer;
    text-align: left;
    transition: background 0.15s;
  }
  .add-column-btn:hover {
    background: var(--bg-subtle-hover);
  }

  .add-column-form {
    width: 272px;
    background: var(--bg-surface);
    border-radius: 10px;
    padding: 0.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .add-column-input {
    width: 100%;
    padding: 0.5rem;
    border-radius: 6px;
    border: none;
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    outline: none;
    box-sizing: border-box;
  }
  .add-column-input:focus {
    box-shadow: 0 0 0 2px var(--accent);
  }

  .add-column-actions {
    display: flex;
    gap: 0.4rem;
    align-items: center;
  }

  .btn-add {
    padding: 0.35rem 0.75rem;
    border: none;
    border-radius: 4px;
    background: var(--accent);
    color: #fff;
    font-size: 0.8rem;
    cursor: pointer;
    font-weight: 500;
  }
  .btn-add:hover {
    background: var(--accent-hover);
  }

  .btn-cancel {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 1rem;
    padding: 0.2rem 0.4rem;
  }
  .btn-cancel:hover {
    color: var(--text-primary);
  }
</style>
