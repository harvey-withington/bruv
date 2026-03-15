<script lang="ts">
  import { GetCard, UpdateCardTitle, UpdateCardFields, UpdateCardTags, UpdateCardDueDate,
    AddChecklistItem, ToggleChecklistItem, RemoveChecklistItem, DeleteCard } from '../lib/api'

  let { cardId, onClose, onUpdated }: {
    cardId: string
    onClose: () => void
    onUpdated?: () => void
  } = $props()

  let card = $state<any>(null)
  let loading = $state(true)
  let editingTitle = $state(false)
  let titleDraft = $state('')
  let newTag = $state('')
  let newChecklistText = $state('')
  let descriptionDraft = $state('')
  let editingDescription = $state(false)

  const typeColors: Record<string, string> = {
    feature: '#6366f1',
    task: '#22c55e',
    brainstorm: '#f59e0b',
    episode: '#ec4899',
    reference: '#06b6d4',
  }

  $effect(() => {
    loadCard()
  })

  async function loadCard() {
    loading = true
    try {
      card = await GetCard(cardId)
      titleDraft = card.title
      descriptionDraft = card.fields?.description || ''
    } catch (e) {
      console.error('Failed to load card:', e)
    }
    loading = false
  }

  async function saveTitle() {
    if (!titleDraft.trim() || titleDraft === card.title) {
      editingTitle = false
      return
    }
    try {
      card = await UpdateCardTitle(cardId, titleDraft.trim())
      editingTitle = false
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  function handleTitleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') saveTitle()
    else if (e.key === 'Escape') { editingTitle = false; titleDraft = card.title }
  }

  async function saveDescription() {
    const fields = { ...(card.fields || {}), description: descriptionDraft }
    try {
      card = await UpdateCardFields(cardId, fields)
      editingDescription = false
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  function handleDescKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') { editingDescription = false; descriptionDraft = card.fields?.description || '' }
  }

  async function addTag() {
    const tag = newTag.trim().toLowerCase()
    if (!tag || card.tags?.includes(tag)) { newTag = ''; return }
    const tags = [...(card.tags || []), tag]
    try {
      card = await UpdateCardTags(cardId, tags)
      newTag = ''
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function removeTag(tag: string) {
    const tags = (card.tags || []).filter((t: string) => t !== tag)
    try {
      card = await UpdateCardTags(cardId, tags)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  function handleTagKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') addTag()
  }

  async function handleDueDateChange(e: Event) {
    const value = (e.target as HTMLInputElement).value
    try {
      card = await UpdateCardDueDate(cardId, value)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function addChecklist() {
    if (!newChecklistText.trim()) return
    try {
      card = await AddChecklistItem(cardId, newChecklistText.trim())
      newChecklistText = ''
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function toggleChecklist(itemId: string) {
    try {
      card = await ToggleChecklistItem(cardId, itemId)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function removeChecklist(itemId: string) {
    try {
      card = await RemoveChecklistItem(cardId, itemId)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  function handleChecklistKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') addChecklist()
  }

  async function handleDelete() {
    if (!confirm('Delete this card? This cannot be undone.')) return
    try {
      await DeleteCard(cardId)
      onUpdated?.()
      onClose()
    } catch (e) { console.error(e) }
  }

  function handleBackdropClick(e: MouseEvent) {
    if ((e.target as HTMLElement).classList.contains('modal-backdrop')) {
      onClose()
    }
  }

  function handleBackdropKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<svelte:window onkeydown={handleBackdropKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
<div class="modal-backdrop" onclick={handleBackdropClick}>
  <div class="modal">
    {#if loading}
      <div class="modal-loading">Loading card…</div>
    {:else if card}
      <div class="modal-header">
        <span class="type-badge" style="background: {typeColors[card.type] || '#71717a'}">{card.type}</span>

        {#if editingTitle}
          <input
            class="title-input"
            bind:value={titleDraft}
            onkeydown={handleTitleKeydown}
            onblur={saveTitle}
          />
        {:else}
          <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <h2 class="modal-title" onclick={() => { editingTitle = true; setTimeout(() => (document.querySelector('.title-input') as HTMLElement)?.focus(), 0) }}>
            {card.title}
          </h2>
        {/if}

        <button class="close-btn" onclick={onClose}>✕</button>
      </div>

      <div class="modal-body">
        <!-- Description -->
        <section class="section">
          <h3 class="section-title">Description</h3>
          {#if editingDescription}
            <textarea
              class="desc-textarea"
              bind:value={descriptionDraft}
              onkeydown={handleDescKeydown}
              rows="4"
            ></textarea>
            <div class="section-actions">
              <button class="btn-save" onclick={saveDescription}>Save</button>
              <button class="btn-cancel-sm" onclick={() => { editingDescription = false; descriptionDraft = card.fields?.description || '' }}>Cancel</button>
            </div>
          {:else}
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div class="desc-display" onclick={() => { editingDescription = true; setTimeout(() => (document.querySelector('.desc-textarea') as HTMLElement)?.focus(), 0) }}>
              {#if card.fields?.description}
                <p>{card.fields.description}</p>
              {:else}
                <p class="placeholder">Add a more detailed description…</p>
              {/if}
            </div>
          {/if}
        </section>

        <!-- Due Date -->
        <section class="section">
          <h3 class="section-title">Due Date</h3>
          <input
            type="date"
            class="date-input"
            value={card.due_date ? card.due_date.slice(0, 10) : ''}
            onchange={handleDueDateChange}
          />
        </section>

        <!-- Tags -->
        <section class="section">
          <h3 class="section-title">Tags</h3>
          <div class="tags-list">
            {#each (card.tags || []) as tag}
              <span class="tag">
                {tag}
                <button class="tag-remove" onclick={() => removeTag(tag)}>✕</button>
              </span>
            {/each}
          </div>
          <div class="tag-add">
            <input
              type="text"
              bind:value={newTag}
              onkeydown={handleTagKeydown}
              placeholder="Add a tag…"
              class="tag-input"
            />
          </div>
        </section>

        <!-- Checklist -->
        <section class="section">
          <h3 class="section-title">
            Checklist
            {#if card.checklist?.length > 0}
              <span class="checklist-progress">
                {card.checklist.filter((c: any) => c.done).length}/{card.checklist.length}
              </span>
            {/if}
          </h3>

          {#if card.checklist?.length > 0}
            <div class="checklist-bar">
              <div
                class="checklist-bar-fill"
                style="width: {(card.checklist.filter((c: any) => c.done).length / card.checklist.length) * 100}%"
              ></div>
            </div>
          {/if}

          <div class="checklist-items">
            {#each (card.checklist || []) as item}
              <div class="checklist-item" class:done={item.done}>
                <button class="checkbox" onclick={() => toggleChecklist(item.id)}>
                  {item.done ? '☑' : '☐'}
                </button>
                <span class="checklist-text">{item.text}</span>
                <button class="checklist-remove" onclick={() => removeChecklist(item.id)}>✕</button>
              </div>
            {/each}
          </div>

          <div class="checklist-add">
            <input
              type="text"
              bind:value={newChecklistText}
              onkeydown={handleChecklistKeydown}
              placeholder="Add an item…"
              class="checklist-input"
            />
            <button class="btn-add-sm" onclick={addChecklist}>Add</button>
          </div>
        </section>
      </div>

      <div class="modal-footer">
        <span class="meta">Created {card.created_at?.slice(0, 10) || '—'}</span>
        <button class="btn-delete" onclick={handleDelete}>Delete Card</button>
      </div>
    {/if}
  </div>
</div>

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: flex-start;
    justify-content: center;
    padding-top: 3rem;
    z-index: 100;
    overflow-y: auto;
  }

  .modal {
    background: #1c1c1f;
    border-radius: 10px;
    width: 600px;
    max-width: 95vw;
    max-height: 85vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
  }

  .modal-loading {
    padding: 2rem;
    text-align: center;
    color: #71717a;
  }

  .modal-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid #2e2e32;
  }

  .type-badge {
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.15rem 0.5rem;
    border-radius: 3px;
    color: #fff;
    flex-shrink: 0;
  }

  .modal-title {
    margin: 0;
    font-size: 1.1rem;
    font-weight: 600;
    color: #f5f5f5;
    flex: 1;
    cursor: pointer;
    line-height: 1.3;
  }
  .modal-title:hover {
    color: #a5b4fc;
  }

  .title-input {
    flex: 1;
    font-size: 1.1rem;
    font-weight: 600;
    background: #27272a;
    border: 1px solid #6366f1;
    border-radius: 4px;
    color: #f5f5f5;
    padding: 0.3rem 0.5rem;
    outline: none;
  }

  .close-btn {
    background: none;
    border: none;
    color: #71717a;
    font-size: 1.2rem;
    cursor: pointer;
    padding: 0.25rem;
    flex-shrink: 0;
  }
  .close-btn:hover { color: #f5f5f5; }

  .modal-body {
    flex: 1;
    overflow-y: auto;
    padding: 1rem 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
  }

  .section-title {
    font-size: 0.8rem;
    font-weight: 600;
    color: #a1a1aa;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin: 0 0 0.5rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .desc-display {
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 6px;
    transition: background 0.1s;
  }
  .desc-display:hover { background: #27272a; }
  .desc-display p { margin: 0; color: #d4d4d8; font-size: 0.9rem; line-height: 1.5; white-space: pre-wrap; }
  .desc-display .placeholder { color: #52525b; font-style: italic; }

  .desc-textarea {
    width: 100%;
    padding: 0.5rem;
    border-radius: 6px;
    border: 1px solid #3f3f46;
    background: #27272a;
    color: #f5f5f5;
    font-size: 0.9rem;
    font-family: inherit;
    resize: vertical;
    outline: none;
    box-sizing: border-box;
  }
  .desc-textarea:focus { border-color: #6366f1; }

  .section-actions {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.4rem;
  }

  .date-input {
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    border: 1px solid #3f3f46;
    background: #27272a;
    color: #f5f5f5;
    font-size: 0.85rem;
    outline: none;
  }
  .date-input:focus { border-color: #6366f1; }

  .tags-list {
    display: flex;
    gap: 0.3rem;
    flex-wrap: wrap;
    margin-bottom: 0.4rem;
  }

  .tag {
    font-size: 0.75rem;
    padding: 0.15rem 0.5rem;
    border-radius: 4px;
    background: #3f3f46;
    color: #d4d4d8;
    display: flex;
    align-items: center;
    gap: 0.3rem;
  }

  .tag-remove {
    background: none;
    border: none;
    color: #71717a;
    cursor: pointer;
    font-size: 0.7rem;
    padding: 0;
    line-height: 1;
  }
  .tag-remove:hover { color: #f87171; }

  .tag-input {
    padding: 0.35rem 0.5rem;
    border-radius: 4px;
    border: 1px solid #3f3f46;
    background: #27272a;
    color: #f5f5f5;
    font-size: 0.8rem;
    outline: none;
    width: 150px;
  }
  .tag-input:focus { border-color: #6366f1; }

  .checklist-progress {
    font-size: 0.7rem;
    color: #71717a;
    font-weight: 400;
  }

  .checklist-bar {
    height: 4px;
    background: #3f3f46;
    border-radius: 2px;
    overflow: hidden;
    margin-bottom: 0.5rem;
  }
  .checklist-bar-fill {
    height: 100%;
    background: #22c55e;
    border-radius: 2px;
    transition: width 0.2s;
  }

  .checklist-items {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .checklist-item {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.25rem 0;
  }

  .checkbox {
    background: none;
    border: none;
    color: #71717a;
    cursor: pointer;
    font-size: 1rem;
    padding: 0;
  }
  .checklist-item.done .checkbox { color: #22c55e; }
  .checklist-item.done .checklist-text { text-decoration: line-through; color: #52525b; }

  .checklist-text {
    flex: 1;
    font-size: 0.85rem;
    color: #d4d4d8;
  }

  .checklist-remove {
    background: none;
    border: none;
    color: transparent;
    cursor: pointer;
    font-size: 0.75rem;
    padding: 0.15rem 0.3rem;
  }
  .checklist-item:hover .checklist-remove { color: #71717a; }
  .checklist-remove:hover { color: #f87171 !important; }

  .checklist-add {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.4rem;
  }

  .checklist-input {
    flex: 1;
    padding: 0.35rem 0.5rem;
    border-radius: 4px;
    border: 1px solid #3f3f46;
    background: #27272a;
    color: #f5f5f5;
    font-size: 0.8rem;
    outline: none;
  }
  .checklist-input:focus { border-color: #6366f1; }

  .btn-save {
    padding: 0.3rem 0.7rem;
    border: none;
    border-radius: 4px;
    background: #6366f1;
    color: #fff;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .btn-save:hover { background: #4f46e5; }

  .btn-cancel-sm {
    padding: 0.3rem 0.7rem;
    border: none;
    border-radius: 4px;
    background: transparent;
    color: #71717a;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .btn-cancel-sm:hover { color: #f5f5f5; }

  .btn-add-sm {
    padding: 0.3rem 0.6rem;
    border: none;
    border-radius: 4px;
    background: #3f3f46;
    color: #d4d4d8;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .btn-add-sm:hover { background: #52525b; }

  .modal-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.75rem 1.25rem;
    border-top: 1px solid #2e2e32;
  }

  .meta {
    font-size: 0.75rem;
    color: #52525b;
  }

  .btn-delete {
    padding: 0.3rem 0.7rem;
    border: none;
    border-radius: 4px;
    background: transparent;
    color: #71717a;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .btn-delete:hover { color: #f87171; background: rgba(248, 113, 113, 0.1); }
</style>
