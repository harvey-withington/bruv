<script lang="ts">
  // CardRow — the visual representation of a single card in a list
  // (project view, category view, inbox). Extracted so the DnD action
  // and chat-FAB-affordance code don't have to be re-written per page.
  //
  // Props are deliberately minimal: the card summary plus a click
  // handler. DnD is layered on by the parent via a Svelte action wrapping
  // the <li> ancestor, not by this component, so this stays presentational.

  import { getCardTypeColor, getCardTypeTextColor, getCardTypeLabel } from '@shared/cardTypes'
  import { repoMeta } from '../lib/repoMeta.svelte'
  import { t } from '../lib/i18n.svelte'

  type CardSummaryLite = {
    id: string
    title: string
    type: string
    tags: string[]
  }

  let {
    card,
    projectKey,
    onClick,
  }: {
    card: CardSummaryLite
    /** Used for per-project tag colour lookups. Undefined = global colours. */
    projectKey?: string
    onClick: () => void
  } = $props()
</script>

<button
  type="button"
  class="card-row"
  style:view-transition-name={`card-${card.id}`}
  onclick={onClick}
>
  <div class="card-main">
    <span class="card-title">{card.title || t('inbox.untitled')}</span>
    {#if card.tags?.length}
      <div class="tags">
        {#each card.tags as tag}
          <span class="tag" style:background={repoMeta.tagColor(tag, projectKey)}>{tag}</span>
        {/each}
      </div>
    {/if}
  </div>
  {#if card.type}
    <span
      class="card-type"
      style:background={getCardTypeColor(card.type, repoMeta.cardTypes)}
      style:color={getCardTypeTextColor(card.type)}
    >
      {getCardTypeLabel(card.type, repoMeta.cardTypes)}
    </span>
  {/if}
</button>

<style>
  .card-row {
    width: 100%;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0.85rem 1rem;
    color: var(--text);
    font: inherit;
    cursor: pointer;
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    text-align: left;
    transition: border-color 120ms ease, transform 120ms ease, box-shadow 120ms ease;
    /* Drag-pickup needs to win against the browser's scroll handler.
       touch-action: none on the card row tells the browser "don't
       interpret a touch starting here as a scroll" — without it,
       Android Chrome commits to scroll on the first few px of finger
       movement after long-press, cancelling our pointer events.
       Page scroll still works by touching anywhere outside cards
       (collapsed category headers, toolbar, topbar, gaps). */
    touch-action: none;
    -webkit-user-select: none;
    user-select: none;
    -webkit-touch-callout: none;
  }

  .card-row:hover,
  .card-row:focus-visible {
    border-color: var(--accent);
    outline: none;
  }

  .card-main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .card-title {
    font-size: 0.95rem;
    font-weight: 500;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }

  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
  }

  .tag {
    font-size: 0.7rem;
    color: #fff;
    padding: 0.1rem 0.45rem;
    border-radius: 4px;
  }

  .card-type {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    padding: 0.2rem 0.5rem;
    border-radius: 4px;
    font-weight: 500;
    flex-shrink: 0;
  }
</style>
