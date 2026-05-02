<script lang="ts">
  import { onMount } from 'svelte'
  import { repoRPC } from '../lib/auth'
  import { navigate, cardURL } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
  import { repoMeta, loadProjectTags, projectKey as makeProjectKey } from '../lib/repoMeta.svelte'
  import { getCardTypeColor, getCardTypeTextColor, getCardTypeLabel } from '@shared/cardTypes'
  import type { Category, CardSummary } from '../lib/model'
  import DynamicIcon from '../components/DynamicIcon.svelte'

  // Project view: list of categories with their cards. Phase 1 ships
  // this as a vertical scroll — categories stacked, each with its
  // pinned cards underneath. The full Project ↔ Category ↔ Card zoom
  // UI lands in step 10; this gets data on screen.

  let { brand, stream, project }: { brand: string; stream: string; project: string } = $props()

  type CategoryWithCards = Category & { cards: CardSummary[] }

  let categories = $state<CategoryWithCards[]>([])
  let loading = $state(true)
  let errorMsg = $state<string | null>(null)
  // svelte-ignore state_referenced_locally
  let projectName = $state<string>(project) // fallback to slug until the real name loads

  // Project key for tag colour lookups in this view.
  // svelte-ignore state_referenced_locally
  const pkey = $state(makeProjectKey(brand, stream, project))

  onMount(async () => {
    // Capture the route slugs into closure-stable locals before any
    // await — Svelte 5's $props are reactive and reading them mid-
    // async would otherwise warn (state_referenced_locally).
    const brandSlug = brand
    const streamSlug = stream
    const projectSlug = project
    // Pre-warm this project's tag definitions so chip colours light
    // up as soon as cards render. Fire-and-forget; cards still render
    // (in grey) if this lags.
    loadProjectTags(brandSlug, streamSlug, projectSlug)
    try {
      // Fetch categories first so we can show them ASAP, then fan out
      // to the per-category card lookups in parallel.
      const cats = (await repoRPC<Category[]>('ListCategories', [
        brandSlug,
        streamSlug,
        projectSlug,
      ])) ?? []

      const populated = await Promise.all(
        cats.map(async (cat) => {
          let cards: CardSummary[] = []
          try {
            // Server-side method takes (projectID, categoryID); the
            // desktop store passes cat.id for both, which is what
            // works in practice — see frontend/src/lib/store.svelte.ts.
            const ids = (await repoRPC<string[]>('ListCardIDsInCategory', [cat.id, cat.id])) ?? []
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
          } catch {
            cards = []
          }
          return { ...cat, cards }
        }),
      )
      categories = populated
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('project.err_load')
    } finally {
      loading = false
    }
  })
</script>

<header class="topbar">
  <button type="button" class="back" onclick={() => navigate('/')}>
    <span aria-hidden="true">‹</span> {t('common.back')}
  </button>
  <h1 title={projectName}>{projectName}</h1>
  <span class="spacer"></span>
</header>

<main>
  {#if loading}
    <p class="status">{t('common.loading')}</p>
  {:else if errorMsg}
    <p class="error">{errorMsg}</p>
  {:else if categories.length === 0}
    <div class="empty">
      <h2>{t('project.empty_title')}</h2>
      <p>{t('project.empty_body')}</p>
    </div>
  {:else}
    <div class="categories">
      {#each categories as cat (cat.id)}
        <section class="category">
          <header class="cat-header">
            <h2>
              {#if cat.icon}
                <DynamicIcon name={cat.icon} size={16} />
              {/if}
              {cat.name}
            </h2>
            <span class="count">{cat.cards.length}</span>
          </header>
          {#if cat.cards.length === 0}
            <p class="cat-empty">{t('project.category_empty')}</p>
          {:else}
            <ul class="cards">
              {#each cat.cards as card (card.id)}
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
                            <span class="tag" style:background={repoMeta.tagColor(tag, pkey)}>{tag}</span>
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
        </section>
      {/each}
    </div>
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
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 60vw;
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

  main {
    padding: 0.75rem 0.85rem 4rem;
    max-width: 600px;
    margin: 0 auto;
  }

  .categories {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
  }

  .cat-header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin: 0 0.25rem 0.5rem;
  }

  .cat-header h2 {
    margin: 0;
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text);
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }

  .count {
    font-size: 0.75rem;
    color: var(--text-faint);
  }

  .cards {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .cat-empty {
    color: var(--text-faint);
    font-size: 0.85rem;
    margin: 0.25rem 0.5rem;
    font-style: italic;
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
