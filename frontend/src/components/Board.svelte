<script lang="ts">
  import { onMount } from 'svelte'
  import Column from './Column.svelte'
  import CardDetail from './CardDetail.svelte'
  import InboxView from './InboxView.svelte'
  import AgentsPage from './AgentsPage.svelte'
  import LottiePlayer from './LottiePlayer.svelte'
  import loadingAnimation from '../lib/animations/loading.lottie?url'
  import { board, nav, dnd, boardSearch, boardSearchFilters, loadBoard } from '../lib/store.svelte'
  import { CreateCard, PinCard, CreateCategory, RenameCategory, GetCard, MoveCardInCategory, MoveCardToCategory, ReorderCategories, DeleteCategory, DeleteCard, MoveCategoryCards, DuplicateCard, CopyCategory } from '@shared/api'
  import { t } from '../lib/i18n.svelte'
  import { focusTrap } from '../lib/actions'

  let renamingCategorySlug = $state<string | null>(null)
  let renamingCategoryName = $state('')
  let renamingCategoryOriginal = $state('')
  let renameCatCancelled = $state(false)
  let renameCatIsNew = $state(false)
  let selectedCardId = $state<string | null>(null)
  let selectedCategoryId = $state<string | null>(null)
  let selectedCategoryName = $state<string | null>(null)
  let autoEditTitle = $state(false)

  // Close board's card dialog when navigating via internal links
  onMount(() => {
    function handleClose() { selectedCardId = null; selectedCategoryId = null; selectedCategoryName = null; autoEditTitle = false }
    function handleBoardChanged() { refreshBoard() }
    document.addEventListener('bruv:close-card-detail', handleClose)
    document.addEventListener('bruv:board-changed', handleBoardChanged)
    return () => {
      document.removeEventListener('bruv:close-card-detail', handleClose)
      document.removeEventListener('bruv:board-changed', handleBoardChanged)
    }
  })

  // Category delete confirmation state
  let deletingCategory = $state<{ id: string; slug: string; name: string; cardCount: number } | null>(null)
  let moveTargetId = $state<string>('')
  let moveCards = $state(true)

  async function handleAddCard(categoryId: string) {
    try {
      const cat = board.categories.find(c => c.id === categoryId)
      // If the category restricts types, use the sole type or start untyped
      let cardType = ''
      if (cat?.accepted_types?.length === 1) {
        cardType = cat.accepted_types[0]
      }

      const card = await CreateCard(cardType, t('default.card_name'))

      if (cat) {
        await PinCard(card.id, cat.id)
      }

      await refreshBoard()

      // Auto-open the card detail modal so user can rename
      selectedCardId = card.id
      selectedCategoryId = categoryId
      selectedCategoryName = cat?.name || null
      autoEditTitle = true
    } catch (e) {
      console.error('Failed to add card:', e)
    }
  }

  function handleCardClick(cardId: string, categoryId?: string) {
    selectedCardId = cardId
    selectedCategoryId = categoryId || null
    selectedCategoryName = categoryId
      ? (board.categories.find(c => c.id === categoryId)?.name || null)
      : null
  }

  async function closeCardDetail(opts?: { escaped?: boolean }) {
    const cardId = selectedCardId
    const wasAutoEdit = autoEditTitle
    selectedCardId = null
    selectedCategoryId = null
    selectedCategoryName = null
    autoEditTitle = false
    // Only delete unnamed new card if user pressed ESC without editing the name
    if (opts?.escaped && wasAutoEdit && cardId) {
      try {
        const card = await GetCard(cardId)
        if (card.title === t('default.card_name')) {
          await DeleteCard(cardId)
        }
      } catch (e) { console.error('Cleanup card:', e) }
    }
    // Refresh the view — inbox needs its own event since refreshBoard() is a no-op there
    if (nav.inboxMode) {
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
    } else {
      refreshBoard()
    }
  }

  function handleCardUpdated() {
    autoEditTitle = false
    if (nav.inboxMode) {
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
    } else {
      refreshBoard()
    }
  }

  function handleCardPinned() {
    if (nav.inboxMode) {
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
    } else {
      refreshBoard()
    }
  }

  async function handleNewIdea() {
    try {
      const card = await CreateCard('', t('default.card_name'))
      // Open the card for editing — don't refresh inbox yet to avoid race
      selectedCardId = card.id
      autoEditTitle = true
    } catch (e) {
      console.error('Failed to create idea:', e)
    }
  }

  async function handleCardDrop(cardId: string, fromCategoryId: string, toCategoryId: string, toIndex: number, copy?: boolean) {
    // Check type restriction on target category
    const targetCat = board.categories.find(c => c.id === toCategoryId)
    if (targetCat?.accepted_types && targetCat.accepted_types.length > 0) {
      const sourceCard = board.categories.flatMap(c => c.cards).find(c => c.id === cardId)
      if (sourceCard?.type && !targetCat.accepted_types.includes(sourceCard.type)) {
        console.warn(`Card type "${sourceCard.type}" not accepted by category "${targetCat.name}"`)
        return
      }
    }
    try {
      if (copy) {
        // Ctrl+drop: duplicate the card into the target category
        await DuplicateCard(cardId, toCategoryId)
        await refreshBoard()
      } else if (fromCategoryId === toCategoryId) {
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
              await MoveCardInCategory(col.cards[i].id, toCategoryId, i)
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
            await MoveCardToCategory(cardId, fromCategoryId, toCategoryId, toIndex)
            // Re-persist positions in source column
            for (let i = 0; i < fromCol.cards.length; i++) {
              try { await MoveCardInCategory(fromCol.cards[i].id, fromCategoryId, i) } catch { /* skip */ }
            }
            // Re-persist positions in target column
            for (let i = 0; i < toCol.cards.length; i++) {
              try { await MoveCardInCategory(toCol.cards[i].id, toCategoryId, i) } catch { /* skip */ }
            }
          }
        }
        // Always refresh after cross-column move to ensure UI matches backend
        await refreshBoard()
      }
    } catch (e) {
      console.error('Card drop failed:', e)
      await refreshBoard()
    }
  }

  function handleColumnsDragOver(e: DragEvent) {
    if (dnd.dragging?.type !== 'column') return
    e.preventDefault()
    if (e.dataTransfer) e.dataTransfer.dropEffect = e.ctrlKey ? 'copy' : 'move'
    dnd.copyMode = e.ctrlKey

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
    if (fromIdx === -1) return

    const copy = e.ctrlKey
    let toIdx = dnd.overColumnIndex ?? board.categories.length
    dnd.dragging = null
    dnd.overColumnIndex = null

    if (copy) {
      // Ctrl+drop: duplicate the entire column with all its cards
      const col = board.categories[fromIdx]
      if (nav.brandSlug && nav.streamSlug && nav.projectSlug) {
        try {
          await CopyCategory(nav.brandSlug, nav.streamSlug, nav.projectSlug, col.slug)
          await refreshBoard()
          // Move the new copy (appended at end) to the drop position
          if (board.categories.length > 1) {
            const newCol = board.categories.splice(board.categories.length - 1, 1)[0]
            board.categories.splice(toIdx, 0, newCol)
            const orderedSlugs = board.categories.map(c => c.slug)
            await ReorderCategories(nav.brandSlug, nav.streamSlug, nav.projectSlug, orderedSlugs)
          }
        } catch (e) {
          console.error('Column copy failed:', e)
        }
      }
      return
    }

    // Adjust target if dragging forward
    if (toIdx > fromIdx) toIdx--
    if (fromIdx === toIdx) return

    const col = board.categories.splice(fromIdx, 1)[0]
    board.categories.splice(toIdx, 0, col)

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

  async function handleDeleteCategoryRequest(categoryId: string, categorySlug: string, categoryName: string, cardCount: number) {
    if (cardCount === 0) {
      // Empty category — delete immediately without confirmation
      if (!nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return
      try {
        await DeleteCategory(nav.brandSlug, nav.streamSlug, nav.projectSlug, categorySlug)
        await refreshBoard()
      } catch (e) { console.error('Delete category failed:', e) }
      return
    }
    deletingCategory = { id: categoryId, slug: categorySlug, name: categoryName, cardCount }
    // Default move target: Inbox (orphan cards)
    moveTargetId = '__inbox__'
  }

  function cancelDeleteCategory() {
    deletingCategory = null
    moveTargetId = ''
  }

  async function confirmDeleteCategory() {
    if (!deletingCategory || !nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return
    const { id, slug, cardCount } = deletingCategory
    try {
      if (cardCount > 0) {
        if (moveCards && moveTargetId && moveTargetId !== '__inbox__') {
          // Move cards to the selected category
          await MoveCategoryCards(nav.brandSlug, nav.streamSlug, nav.projectSlug, id, moveTargetId)
        } else if (!moveCards) {
          // Delete all cards in this category
          const cat = board.categories.find(c => c.id === id)
          if (cat) {
            for (const card of cat.cards) {
              await DeleteCard(card.id)
            }
          }
        }
        // moveCards && __inbox__: DeleteCategory already unpins cards to inbox
      }
      await DeleteCategory(nav.brandSlug, nav.streamSlug, nav.projectSlug, slug)
      if (moveCards && moveTargetId === '__inbox__') {
        document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
      }
      deletingCategory = null
      moveTargetId = ''
      moveCards = true
      await refreshBoard()
    } catch (e) {
      console.error('Delete category failed:', e)
    }
  }

  async function handleAddCategory() {
    if (!nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return
    try {
      const existingNames = board.categories.map(c => c.name)
      const baseName = t('default.category_name')
      let name = baseName
      const lower = existingNames.map(n => n.toLowerCase())
      if (lower.includes(name.toLowerCase())) {
        for (let i = 2; ; i++) {
          name = `${baseName} ${i}`
          if (!lower.includes(name.toLowerCase())) break
        }
      }
      const position = board.categories.length
      const created = await CreateCategory(nav.brandSlug, nav.streamSlug, nav.projectSlug, name, position)
      await refreshBoard()
      renameCatCancelled = false
      renameCatIsNew = true
      renamingCategorySlug = created.slug
      renamingCategoryName = created.name
      renamingCategoryOriginal = created.name
    } catch (e) {
      console.error('Failed to add category:', e)
    }
  }

  async function commitRenameCategory(slug: string) {
    if (renameCatCancelled || renamingCategorySlug === null) return
    const name = renamingCategoryName.trim()
    renamingCategorySlug = null
    if (!name || !nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return
    try {
      await RenameCategory(nav.brandSlug, nav.streamSlug, nav.projectSlug, slug, name)
      await refreshBoard()
    } catch (e) { console.error('RenameCategory:', e) }
  }

  function startRenameCategory(slug: string, name: string) {
    renameCatCancelled = false
    renameCatIsNew = false
    renamingCategorySlug = slug
    renamingCategoryName = name
    renamingCategoryOriginal = name
  }

  async function cancelRenameCategory(slug: string) {
    const unchanged = renamingCategoryName.trim() === renamingCategoryOriginal
    renameCatCancelled = true
    renamingCategorySlug = null
    if (renameCatIsNew && unchanged && nav.brandSlug && nav.streamSlug && nav.projectSlug) {
      try {
        await DeleteCategory(nav.brandSlug, nav.streamSlug, nav.projectSlug, slug)
        await refreshBoard()
      } catch (e) { console.error('DeleteCategory:', e) }
    }
    renameCatIsNew = false
  }

  async function refreshBoard() {
    if (!nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return
    // Silent: keep the current cards visible while the fetch runs so
    // minor edits (checklist toggle, tag add) don't flash the board's
    // loading state. First-load and project switches still show it.
    await loadBoard(nav.brandSlug, nav.streamSlug, nav.projectSlug, { silent: true })
    // matchingIds are kept current by the $effect below
  }

  // Client-side board search — reacts to query, filters, and board data
  $effect(() => {
    const q = boardSearch.query.trim().toLowerCase()
    if (!q) {
      boardSearch.matchingIds = new Set()
      return
    }
    const ids = new Set<string>()
    for (const cat of board.categories) {
      for (const card of cat.cards) {
        const matchTitle = boardSearchFilters.title && card.title.toLowerCase().includes(q)
        const matchType = boardSearchFilters.type && card.type.toLowerCase().includes(q)
        const matchTags = boardSearchFilters.tags && card.tags.some(tag => tag.toLowerCase().includes(q))
        if (matchTitle || matchType || matchTags) ids.add(card.id)
      }
    }
    boardSearch.matchingIds = ids
  })
</script>

<div class="board">
  {#if board.loading}
    <div class="loading">
      <LottiePlayer
        src={loadingAnimation}
        ariaLabel={t('app.loading')}
        fallback={t('app.loading')}
        size={280}
      />
    </div>

  {:else if nav.inboxMode}
    <InboxView onNewIdea={handleNewIdea} onCardClick={handleCardClick} />

  {:else if nav.agentsMode}
    <AgentsPage onCardClick={handleCardClick} />

  {:else if !nav.projectSlug}
    <div class="empty-board">
      <p class="empty-text">{t('app.no_project')}</p>
    </div>

  {:else}
    <div class="columns" role="list" ondragover={handleColumnsDragOver} ondragleave={handleColumnsDragLeave} ondrop={handleColumnsDrop}>
      {#each board.categories as category, colIdx (category.id)}
        {#if dnd.dragging?.type === 'column' && dnd.overColumnIndex === colIdx}
          <div class="col-drop-indicator" class:copy={dnd.copyMode}></div>
        {/if}
        <div class="col-slot">
          <Column
            {category}
            brandSlug={nav.brandSlug || undefined}
            streamSlug={nav.streamSlug || undefined}
            projectSlug={nav.projectSlug || undefined}
            onCardClick={handleCardClick}
            onAddCard={handleAddCard}
            onCardDrop={handleCardDrop}
            onDeleteCategory={handleDeleteCategoryRequest}
            onStartRename={startRenameCategory}
            renaming={renamingCategorySlug === category.slug}
            renamingName={renamingCategoryName}
            onRenamingNameChange={(v) => renamingCategoryName = v}
            onCommitRename={() => commitRenameCategory(category.slug)}
            onCancelRename={() => cancelRenameCategory(category.slug)}
            onCategoryUpdated={refreshBoard}
            onAcceptedTypesChanged={(categoryId, acceptedTypes) => {
              const cat = board.categories.find(c => c.id === categoryId)
              if (cat) cat.accepted_types = acceptedTypes
            }}
          />
        </div>
      {/each}
      {#if dnd.dragging?.type === 'column' && (dnd.overColumnIndex ?? 0) >= board.categories.length}
        <div class="col-drop-indicator" class:copy={dnd.copyMode}></div>
      {/if}

      <div class="add-column">
        <button class="add-column-btn" onclick={handleAddCategory} title={t('tooltip.add_category')}>
          {t('board.add_category_long')}
        </button>
      </div>
    </div>
  {/if}
</div>

{#if deletingCategory}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="delete-overlay" role="presentation" onclick={cancelDeleteCategory}>
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="delete-dialog" role="dialog" tabindex="-1" onclick={(e: MouseEvent) => e.stopPropagation()} use:focusTrap>
      <h3 class="delete-title">{t('board.delete_category_confirm', { name: deletingCategory.name })}</h3>

      {#if deletingCategory.cardCount === 0}
        <p class="delete-msg">{t('board.delete_category_empty')}</p>
        <div class="delete-actions">
          <button class="btn-ghost" onclick={cancelDeleteCategory}>{t('common.cancel')}</button>
          <button class="btn-danger" onclick={confirmDeleteCategory}>{t('board.delete_only')}</button>
        </div>
      {:else}
        <p class="delete-msg">{t('board.delete_category_has_cards', { count: deletingCategory.cardCount })}</p>
        <label class="move-check">
          <input type="checkbox" bind:checked={moveCards} />
          <span>{t('board.move_cards_to')}</span>
        </label>
        {#if moveCards}
          <select class="move-select" bind:value={moveTargetId}>
            <option value="__inbox__">{t('board.move_to_inbox')}</option>
            {#each board.categories.filter(c => c.id !== deletingCategory?.id) as cat}
              <option value={cat.id}>{cat.name}</option>
            {/each}
          </select>
        {/if}
        {#if !moveCards}
          <p class="delete-warning">{t('board.delete_cards_warning', { count: deletingCategory.cardCount })}</p>
        {/if}
        <div class="delete-actions">
          <button class="btn-ghost" onclick={cancelDeleteCategory}>{t('common.cancel')}</button>
          <button class="btn-danger" onclick={confirmDeleteCategory} disabled={moveCards && !moveTargetId}>
            {moveCards ? t('board.delete_and_move') : t('board.delete_all')}
          </button>
        </div>
      {/if}
    </div>
  </div>
{/if}

{#if selectedCardId}
  <CardDetail
    cardId={selectedCardId}
    currentCategoryId={selectedCategoryId}
    currentCategoryName={selectedCategoryName}
    categoryAcceptedTypes={selectedCategoryId ? board.categories.find(c => c.id === selectedCategoryId)?.accepted_types : undefined}
    onClose={closeCardDetail}
    onUpdated={handleCardUpdated}
    onPin={handleCardPinned}
    {autoEditTitle}
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

  .move-check {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin-bottom: 0.5rem;
    font-size: 0.85rem;
    color: var(--text-secondary);
    cursor: pointer;
  }
  .move-check input[type="checkbox"] {
    accent-color: var(--accent);
  }

  .move-select {
    width: 100%;
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    outline: none;
    margin-bottom: 1rem;
  }
  .move-select:focus { border-color: var(--accent); }

  .delete-warning {
    margin: 0 0 1rem;
    font-size: 0.82rem;
    color: var(--danger);
    font-weight: 500;
  }

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
    min-height: 0;
    overflow-x: auto;
    overflow-y: hidden;
    padding: 1rem;
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
  .col-drop-indicator.copy {
    background: var(--success, #22c55e);
  }

  .loading {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    color: var(--text-muted);
    font-size: 0.9rem;
    animation: fade-in var(--duration-moderate) var(--ease-out);
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


</style>
