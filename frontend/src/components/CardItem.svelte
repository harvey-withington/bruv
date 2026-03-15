<script lang="ts">
  type CardData = {
    id: string
    type: string
    title: string
    tags: string[]
    due_date: string | null
    checklist_total: number
    checklist_done: number
  }

  let { card, onclick }: { card: CardData; onclick?: () => void } = $props()

  const typeColors: Record<string, string> = {
    feature: '#6366f1',
    task: '#22c55e',
    brainstorm: '#f59e0b',
    episode: '#ec4899',
    reference: '#06b6d4',
  }

  function badgeColor(type: string) {
    return typeColors[type] || '#71717a'
  }
</script>

<button class="card-item" onclick={onclick}>
  <div class="card-header">
    <span class="type-badge" style="background: {badgeColor(card.type)}">{card.type}</span>
    {#if card.due_date}
      <span class="due-date">📅 {card.due_date.slice(0, 10)}</span>
    {/if}
  </div>

  <p class="card-title">{card.title}</p>

  <div class="card-footer">
    {#if card.checklist_total > 0}
      <span class="checklist-count" class:all-done={card.checklist_done === card.checklist_total}>
        ✓ {card.checklist_done}/{card.checklist_total}
      </span>
    {/if}

    {#if card.tags.length > 0}
      <div class="tags">
        {#each card.tags.slice(0, 3) as tag}
          <span class="tag">{tag}</span>
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
    background: #27272a;
    border: 1px solid #3f3f46;
    border-radius: 8px;
    padding: 0.6rem 0.75rem;
    cursor: pointer;
    text-align: left;
    width: 100%;
    transition: border-color 0.15s, box-shadow 0.15s;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .card-item:hover {
    border-color: #52525b;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
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
    color: #a1a1aa;
  }

  .card-title {
    margin: 0;
    font-size: 0.85rem;
    color: #e4e4e7;
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
    color: #71717a;
  }

  .checklist-count.all-done {
    color: #22c55e;
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
    background: #3f3f46;
    color: #a1a1aa;
  }

  .tag-more {
    background: transparent;
    color: #71717a;
  }
</style>
