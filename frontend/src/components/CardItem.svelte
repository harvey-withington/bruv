<script lang="ts">
  import { search, dnd, getTagColor, cardTypes } from '../lib/store.svelte'
  import { renderInline } from '../lib/markdown'
  import { t } from '../lib/i18n.svelte'
  import { getCardTypeColor, getCardTypeTextColor } from '../lib/cardTypes'

  type CardData = {
    id: string
    type: string
    title: string
    tags: string[]
    due_date: string | null
    checklist_total: number
    checklist_done: number
  }

  let { card, categoryId, onclick }: { card: CardData; categoryId: string; onclick?: () => void } = $props()

  function handleDragStart(e: DragEvent) {
    if (!e.dataTransfer) return
    e.dataTransfer.effectAllowed = 'copyMove'
    e.dataTransfer.setData('text/plain', card.id)
    dnd.dragging = { type: 'card', cardId: card.id, fromCategoryId: categoryId, cardType: card.type || '' }
  }

  function handleDragEnd() {
    dnd.dragging = null
    dnd.overCategoryId = null
    dnd.overCardIndex = null
  }

</script>

<button
  class="card-item"
  class:search-highlight={search.matchingIds.has(card.id)}
  class:search-dimmed={search.query.trim() && search.matchingIds.size > 0 && !search.matchingIds.has(card.id)}
  class:dragging={dnd.dragging?.type === 'card' && dnd.dragging.cardId === card.id}
  draggable="true"
  ondragstart={handleDragStart}
  ondragend={handleDragEnd}
  onclick={onclick}
>
  <div class="card-header">
    <span class="type-badge" style="background: {getCardTypeColor(card.type, cardTypes.list)}; color: {getCardTypeTextColor(card.type)}">{cardTypes.list.find(t => t.id === card.type)?.label || card.type || t('card.type_none')}</span>
    {#if card.due_date}
      <span class="due-date">📅 {card.due_date.slice(0, 10)}</span>
    {/if}
  </div>

  <p class="card-title">{@html renderInline(card.title)}</p>

  <div class="card-footer">
    {#if card.checklist_total > 0}
      <span class="checklist-count" class:all-done={card.checklist_done === card.checklist_total}>
        ✓ {card.checklist_done}/{card.checklist_total}
      </span>
    {/if}

    {#if card.tags.length > 0}
      <div class="tags">
        {#each card.tags.slice(0, 3) as tag}
          <span class="tag" style:background={getTagColor(tag)}>{tag}</span>
        {/each}
        {#if card.tags.length > 3}
          <span class="tag tag-more">+{card.tags.length - 3}</span>
        {/if}
      </div>
    {/if}
  </div>
</button>

<style>
  .card-item {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.6rem 0.75rem;
    cursor: grab;
    text-align: left;
    width: 100%;
    transition: border-color 0.15s, box-shadow 0.15s, opacity 0.15s;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .card-item.dragging {
    opacity: 0.4;
  }

  .card-item:hover {
    border-color: var(--border-hover);
    box-shadow: 0 2px 8px var(--shadow);
  }

  .card-item.search-dimmed {
    opacity: 0.5;
  }

  .card-item.search-highlight {
    border-color: var(--accent-light);
    box-shadow: 0 0 8px var(--accent-glow-1), 0 0 20px var(--accent-glow-2), 0 0 40px var(--accent-glow-3);
  }

  .card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .type-badge {
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.1rem 0.4rem;
    border-radius: 3px;
    color: #fff;
    line-height: 1.4;
  }

  .due-date {
    font-size: 0.7rem;
    color: var(--text-secondary);
  }

  .card-title {
    margin: 0;
    font-size: 0.85rem;
    color: var(--text-strong);
    line-height: 1.3;
    font-weight: 400;
  }

  .card-footer {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .checklist-count {
    font-size: 0.7rem;
    color: var(--text-muted);
  }

  .checklist-count.all-done {
    color: var(--success);
  }

  .tags {
    display: flex;
    gap: 0.25rem;
    flex-wrap: wrap;
  }

  .tag {
    font-size: 0.6rem;
    padding: 0.05rem 0.35rem;
    border-radius: 3px;
    background: var(--border);
    color: #fff;
  }

  .tag-more {
    background: transparent;
    color: var(--text-muted);
  }
</style>
