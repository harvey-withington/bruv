<script lang="ts">
  import Column from './Column.svelte'
  import CardDetail from './CardDetail.svelte'
  import { board, nav } from '../lib/store.svelte'
  import { CreateCard, PinCard, CreateCategory, ListCategories, GetCard, ListCardIDsInCategory } from '../lib/api'

  let addingCategory = $state(false)
  let newCategoryName = $state('')
  let selectedCardId = $state<string | null>(null)

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
    <div class="loading">Loading board…</div>

  {:else if !nav.projectSlug}
    <div class="empty-board">
      <p class="empty-text">Select a project from the sidebar to view its board.</p>
    </div>

  {:else}
    <div class="columns">
      {#each board.categories as category (category.id)}
        <Column
          {category}
          onCardClick={handleCardClick}
          onAddCard={handleAddCard}
        />
      {/each}

      <div class="add-column">
        {#if addingCategory}
          <div class="add-column-form">
            <input
              type="text"
              bind:value={newCategoryName}
              onkeydown={handleCategoryKeydown}
              placeholder="Enter list title..."
              class="add-column-input"
            />
            <div class="add-column-actions">
              <button class="btn-add" onclick={handleAddCategory}>Add list</button>
              <button class="btn-cancel" onclick={() => { addingCategory = false; newCategoryName = '' }}>✕</button>
            </div>
          </div>
        {:else}
          <button class="add-column-btn" onclick={() => { addingCategory = true; setTimeout(() => (document.querySelector('.add-column-input') as HTMLElement)?.focus(), 0) }}>
            + Add another list
          </button>
        {/if}
      </div>
    </div>
  {/if}
</div>

{#if selectedCardId}
  <CardDetail
    cardId={selectedCardId}
    onClose={closeCardDetail}
    onUpdated={handleCardUpdated}
  />
{/if}

<style>
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

  .loading {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    color: #71717a;
    font-size: 0.9rem;
  }

  .empty-board {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
  }

  .empty-text {
    color: #52525b;
    font-size: 0.95rem;
  }

  .add-column {
    min-width: 272px;
  }

  .add-column-btn {
    width: 272px;
    padding: 0.6rem 0.75rem;
    background: rgba(255, 255, 255, 0.05);
    border: none;
    border-radius: 10px;
    color: #a1a1aa;
    font-size: 0.85rem;
    cursor: pointer;
    text-align: left;
    transition: background 0.15s;
  }
  .add-column-btn:hover {
    background: rgba(255, 255, 255, 0.1);
  }

  .add-column-form {
    width: 272px;
    background: #1c1c1f;
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
    background: #27272a;
    color: #f5f5f5;
    font-size: 0.85rem;
    outline: none;
    box-sizing: border-box;
  }
  .add-column-input:focus {
    box-shadow: 0 0 0 2px #6366f1;
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
    background: #6366f1;
    color: #fff;
    font-size: 0.8rem;
    cursor: pointer;
    font-weight: 500;
  }
  .btn-add:hover {
    background: #4f46e5;
  }

  .btn-cancel {
    background: none;
    border: none;
    color: #71717a;
    cursor: pointer;
    font-size: 1rem;
    padding: 0.2rem 0.4rem;
  }
  .btn-cancel:hover {
    color: #f5f5f5;
  }
</style>
