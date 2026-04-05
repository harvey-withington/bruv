<script lang="ts">
  import CardItem from './CardItem.svelte'
  import type { RecentCard } from '../lib/types'
  import { t } from '../lib/i18n.svelte'

  let {
    cards,
    onCardClick,
  }: {
    cards: RecentCard[]
    onCardClick: (id: string) => void
  } = $props()

  function navigateToProject(card: RecentCard) {
    if (!card.brand_slug || !card.stream_slug || !card.project_slug) return
    document.dispatchEvent(new CustomEvent('bruv:select-project', {
      detail: { brandSlug: card.brand_slug, streamSlug: card.stream_slug, projectSlug: card.project_slug },
    }))
  }

  function asCardData(card: RecentCard) {
    return {
      id: card.id,
      type: card.type,
      title: card.title,
      tags: card.tags || [],
      due_date: card.due_date || null,
      checklist_total: 0,
      checklist_done: 0,
    }
  }
</script>

{#if cards.length === 0}
  <p class="empty">{t('board.inbox_recent_empty')}</p>
{:else}
  <div class="cards-grid" role="list">
    {#each cards as card (card.id)}
      <div class="card-wrapper" role="listitem">
        {#if card.breadcrumb}
          <button
            class="card-path"
            title={card.breadcrumb}
            onclick={() => navigateToProject(card)}
          >{card.breadcrumb}</button>
        {/if}
        <CardItem card={asCardData(card)} categoryId="__recent__" onclick={() => onCardClick(card.id)} />
      </div>
    {/each}
  </div>
{/if}

<style>
  .empty {
    color: var(--text-faint);
    font-size: 0.88rem;
    padding: 0.5rem 0;
    margin: 0;
  }

  .cards-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(230px, 1fr));
    gap: 0.5rem;
    align-content: start;
  }

  .card-wrapper {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .card-path {
    display: block;
    width: 100%;
    background: none;
    border: none;
    padding: 0 0.15rem;
    font-size: 0.67rem;
    font-family: inherit;
    color: var(--text-secondary);
    text-align: left;
    cursor: pointer;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    transition: color 0.12s;
  }
  .card-path:hover {
    color: var(--accent);
  }
</style>
