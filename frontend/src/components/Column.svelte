<script lang="ts">
  import CardItem from './CardItem.svelte'

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

  let { category, onCardClick, onAddCard }: {
    category: CategoryData
    onCardClick?: (cardId: string) => void
    onAddCard?: (categoryId: string) => void
  } = $props()

  let adding = $state(false)
  let newTitle = $state('')

  function startAdding() {
    adding = true
    newTitle = ''
    // Focus the input after DOM update
    setTimeout(() => {
      const input = document.querySelector(`#add-input-${category.id}`) as HTMLInputElement
      input?.focus()
    }, 0)
  }

  function cancelAdding() {
    adding = false
    newTitle = ''
  }

  function submitCard() {
    if (newTitle.trim() && onAddCard) {
      onAddCard(category.id)
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      submitCard()
    } else if (e.key === 'Escape') {
      cancelAdding()
    }
  }

  export function getNewTitle() {
    const title = newTitle.trim()
    newTitle = ''
    adding = false
    return title
  }

  export { newTitle, adding }
</script>

<div class="column">
  <div class="column-header">
    <h3 class="column-title">{category.name}</h3>
    <span class="card-count">{category.cards.length}</span>
  </div>

  <div class="card-list">
    {#each category.cards as card (card.id)}
      <CardItem {card} onclick={() => onCardClick?.(card.id)} />
    {/each}
  </div>

  <div class="column-footer">
    {#if adding}
      <div class="add-form">
        <input
          id="add-input-{category.id}"
          type="text"
          bind:value={newTitle}
          onkeydown={handleKeydown}
          placeholder="Enter a title for this card..."
          class="add-input"
        />
        <div class="add-actions">
          <button class="btn-add" onclick={submitCard}>Add card</button>
          <button class="btn-cancel" onclick={cancelAdding}>✕</button>
        </div>
      </div>
    {:else}
      <button class="add-card-btn" onclick={startAdding}>
        + Add a card
      </button>
    {/if}
  </div>
</div>

<style>
  .column {
    width: 272px;
    min-width: 272px;
    max-height: calc(100vh - 4rem);
    background: #1c1c1f;
    border-radius: 10px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .column-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.6rem 0.75rem;
  }

  .column-title {
    margin: 0;
    font-size: 0.85rem;
    font-weight: 600;
    color: #e4e4e7;
  }

  .card-count {
    font-size: 0.7rem;
    color: #71717a;
    background: #27272a;
    padding: 0.1rem 0.4rem;
    border-radius: 10px;
  }

  .card-list {
    flex: 1;
    overflow-y: auto;
    padding: 0 0.5rem;
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
    color: #71717a;
    font-size: 0.85rem;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s, color 0.1s;
  }

  .add-card-btn:hover {
    background: #27272a;
    color: #d4d4d8;
  }

  .add-form {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .add-input {
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

  .add-input:focus {
    box-shadow: 0 0 0 2px #6366f1;
  }

  .add-actions {
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
