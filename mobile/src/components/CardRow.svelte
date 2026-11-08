<script lang="ts">
  // CardRow — the visual representation of a single card in a list
  // (project view, category view, inbox). Extracted so the DnD action
  // and row-affordance code don't have to be re-written per page.
  //
  // Props are deliberately minimal: the card summary plus a click
  // handler. DnD and long-press are layered on by the parent via Svelte
  // actions on the <li> ancestor, not by this component, so this stays
  // presentational. The Inbox adds a breadcrumb line and its multi-select
  // checkbox through the optional props.

  import { getCardTypeColor, getCardTypeTextColor, getCardTypeLabel } from '@shared/cardTypes'
  import { repoMeta } from '../lib/repoMeta.svelte'
  import { t } from '../lib/i18n.svelte'
  import { renderInline } from '@shared/markdown'
  import { Check } from 'lucide-svelte'
  import OfflineMark from './OfflineMark.svelte'

  type CardSummaryLite = {
    id: string
    title: string
    type: string
    tags: string[]
  }

  let {
    card,
    projectKey,
    breadcrumb,
    selecting = false,
    selected = false,
    onClick,
  }: {
    card: CardSummaryLite
    /** Used for per-project tag colour lookups. Undefined = global colours. */
    projectKey?: string
    /** Where the card lives, under the title (Inbox › Recent). */
    breadcrumb?: string
    /** Multi-select mode: show the selection checkbox. */
    selecting?: boolean
    selected?: boolean
    onClick: () => void
  } = $props()
</script>

<button
  type="button"
  class="card-row"
  class:selected
  style:view-transition-name={`card-${card.id}`}
  onclick={onClick}
>
  {#if selecting}
    <span class="checkbox" class:checked={selected} aria-hidden="true">
      {#if selected}<Check size={14} />{/if}
    </span>
  {/if}
  <div class="card-main">
    <span class="card-title">{@html renderInline(card.title || t('inbox.untitled'))}</span>
    {#if breadcrumb}
      <span class="card-breadcrumb">{breadcrumb}</span>
    {/if}
    {#if card.tags?.length}
      <div class="tags">
        {#each card.tags as tag}
          <span class="tag" style:background={repoMeta.tagColor(tag, projectKey)}>{tag}</span>
        {/each}
      </div>
    {/if}
  </div>
  <div class="card-side">
    {#if card.type}
      <span
        class="card-type"
        style:background={getCardTypeColor(card.type, repoMeta.cardTypes)}
        style:color={getCardTypeTextColor(card.type)}
      >
        {getCardTypeLabel(card.type, repoMeta.cardTypes)}
      </span>
    {/if}
    <OfflineMark cardId={card.id} />
  </div>
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
    transition: border-color 120ms ease, background 120ms ease, transform 120ms ease, box-shadow 120ms ease;
    /* Allow vertical scroll on cards (browser handles pan-y). Long-
       press DnD coexists with this: if the user moves their finger
       more than MOVE_CANCEL_PX (~4px) before the press timer fires,
       the action cancels arming and the browser's already-running
       scroll continues smoothly. If the user holds still, the press
       timer fires and the action sets touch-action: none on the row
       to lock further scroll for the duration of the drag. Net
       behaviour: drag from a card to scroll, hold a card to drag. */
    touch-action: pan-y;
    -webkit-user-select: none;
    user-select: none;
    -webkit-touch-callout: none;
  }

  .card-row:hover,
  .card-row:focus-visible {
    border-color: var(--accent);
    outline: none;
  }

  .card-row.selected {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, var(--bg-elev-1));
  }

  .checkbox {
    width: 22px;
    height: 22px;
    border-radius: 50%;
    border: 2px solid var(--text-muted);
    background: var(--bg);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    margin-top: 2px;
    color: #fff;
    transition: background 120ms ease, border-color 120ms ease;
  }
  .checkbox.checked {
    background: var(--accent);
    border-color: var(--accent);
  }

  .card-breadcrumb {
    font-size: 0.72rem;
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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

  .card-side {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex-shrink: 0;
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
