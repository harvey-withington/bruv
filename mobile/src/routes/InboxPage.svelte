<script lang="ts">
  import { onMount } from 'svelte'
  import { repoRPC } from '../lib/auth'
  import { navigate, cardURL } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
  import { repoMeta } from '../lib/repoMeta.svelte'
  import { getCardTypeColor, getCardTypeTextColor, getCardTypeLabel } from '@shared/cardTypes'
  import type { CardSummary } from '../lib/model'

  // Inbox = orphaned cards (created but not yet pinned to any
  // category). The server returns just the IDs; we fan out to GetCard
  // in parallel for the metadata. Acceptable for v1 — most inboxes
  // are small (triage habit). If they grow, swap for a richer batch
  // endpoint later.

  let cards = $state<CardSummary[]>([])
  let loading = $state(true)
  let errorMsg = $state<string | null>(null)

  onMount(async () => {
    try {
      const ids = (await repoRPC<string[]>('ListOrphanedCardIDs')) ?? []
      const fetched = await Promise.all(
        ids.map(async (id) => {
          try {
            return await repoRPC<CardSummary>('GetCard', [id])
          } catch {
            return null
          }
        }),
      )
      cards = fetched.filter((c): c is CardSummary => c !== null)
      // Newest first — orphans are typically captures, ordered by
      // recency of capture is the most useful triage view.
      cards.sort((a, b) => (a.updated_at < b.updated_at ? 1 : -1))
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('inbox.err_load')
    } finally {
      loading = false
    }
  })
</script>

<header class="topbar">
  <button type="button" class="back" onclick={() => navigate('/')}>
    <span aria-hidden="true">‹</span> {t('common.back')}
  </button>
  <h1>{t('inbox.title')}</h1>
  <span class="spacer"></span>
</header>

<main>
  {#if loading}
    <p class="status">{t('common.loading')}</p>
  {:else if errorMsg}
    <p class="error">{errorMsg}</p>
  {:else if cards.length === 0}
    <div class="empty">
      <h2>{t('inbox.empty_title')}</h2>
      <p>{t('inbox.empty_body')}</p>
    </div>
  {:else}
    <ul class="cards">
      {#each cards as card (card.id)}
        <li>
          <button
            type="button"
            class="card-row"
            style:view-transition-name={`card-${card.id}`}
            onclick={() => navigate(cardURL(card.id))}
          >
            <div class="card-main">
              <span class="card-title">{card.title || t('inbox.untitled')}</span>
              {#if card.tags?.length}
                <div class="tags">
                  {#each card.tags as tag}
                    <span class="tag" style:background={repoMeta.tagColor(tag)}>{tag}</span>
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
        </li>
      {/each}
    </ul>
  {/if}
</main>

<style>
  .topbar {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: center;
    padding: 0.75rem 0.75rem;
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    background: var(--bg);
    z-index: 10;
  }

  .topbar h1 {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
    color: var(--text);
    text-align: center;
  }

  .back {
    background: transparent;
    border: none;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.9rem;
    cursor: pointer;
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    justify-self: start;
  }

  .back:hover,
  .back:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }

  /* .spacer is a layout placeholder — keeps the title centred even
     when the back button has variable width. No styles needed. */

  main {
    padding: 0.75rem 0.85rem 4rem;
    max-width: 600px;
    margin: 0 auto;
  }

  .cards {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

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
    transition: border-color 120ms ease;
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
    /* background set inline; falls back to --border via tagColors.for() */
    padding: 0.1rem 0.45rem;
    border-radius: 4px;
  }

  .card-type {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    /* background + color set inline */
    padding: 0.2rem 0.5rem;
    border-radius: 4px;
    font-weight: 500;
    flex-shrink: 0;
  }

  .empty {
    text-align: center;
    margin-top: 4rem;
    color: var(--text-muted);
  }

  .empty h2 {
    margin: 0 0 0.5rem;
    font-size: 1.1rem;
    color: var(--text);
  }

  .empty p {
    margin: 0;
    font-size: 0.9rem;
    line-height: 1.5;
  }

  .status {
    color: var(--text-muted);
    text-align: center;
    margin: 2rem 0;
    font-size: 0.95rem;
  }

  .error {
    margin: 2rem 0;
    padding: 1rem;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 8px;
    color: #fca5a5;
    text-align: center;
    font-size: 0.9rem;
  }
</style>
