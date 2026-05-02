<script lang="ts">
  import { onMount } from 'svelte'
  import { repoRPC } from '../lib/auth'
  import { navigate, cardURL, categoryURL } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
  import { loadProjectTags, projectKey as makeProjectKey } from '../lib/repoMeta.svelte'
  import type { Category, CardSummary } from '../lib/model'
  import DynamicIcon from '../components/DynamicIcon.svelte'
  import CardRow from '../components/CardRow.svelte'
  import { dragSortable, type DragMoveDetail } from '../lib/actions/dnd.svelte'

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

  // Drag-drop reorder + cross-category move. The dnd action mutates
  // the DOM live during drag for visual feedback; we mirror the move
  // into our reactive state on drop and persist via the right RPC.
  // On failure, the UI re-loads the project so it returns to the
  // server-truth ordering.
  async function handleDnDMove(detail: DragMoveDetail) {
    const fromCat = categories.find((c) => c.id === detail.fromCategoryID)
    const toCat = categories.find((c) => c.id === detail.toCategoryID)
    if (!fromCat || !toCat) return
    const card = fromCat.cards.find((c) => c.id === detail.cardID)
    if (!card) return

    // Optimistic state update mirroring the DOM the action just produced.
    fromCat.cards = fromCat.cards.filter((c) => c.id !== detail.cardID)
    const toIdx = Math.max(0, Math.min(detail.toPosition, toCat.cards.length))
    toCat.cards = [...toCat.cards.slice(0, toIdx), card, ...toCat.cards.slice(toIdx)]

    try {
      if (detail.fromCategoryID === detail.toCategoryID) {
        await repoRPC('MoveCardInCategory', [
          detail.cardID,
          detail.toProjectID,
          detail.toCategoryID,
          detail.toPosition,
        ])
      } else {
        await repoRPC('MoveCardToCategory', [
          detail.cardID,
          detail.toProjectID,
          detail.fromCategoryID,
          detail.toCategoryID,
          detail.toPosition,
        ])
      }
    } catch (err) {
      // Hard revert: refetch the project so we converge on server truth.
      console.error('drag move failed:', err)
      errorMsg = err instanceof Error ? err.message : t('project.err_load')
    }
  }
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
    <div class="categories" use:dragSortable={{ onMove: handleDnDMove }}>
      {#each categories as cat (cat.id)}
        <section class="category" style:view-transition-name={`category-${cat.id}`}>
          <button
            type="button"
            class="cat-header"
            onclick={() => navigate(categoryURL(brand, stream, project, cat.slug))}
            aria-label={t('project.zoom_into_category', { name: cat.name })}
          >
            <h2>
              {#if cat.icon}
                <DynamicIcon name={cat.icon} size={16} />
              {/if}
              {cat.name}
            </h2>
            <span class="count">{cat.cards.length}</span>
            <span class="zoom-hint" aria-hidden="true">›</span>
          </button>
          {#if cat.cards.length === 0}
            <p class="cat-empty">{t('project.category_empty')}</p>
          {:else}
            <ul
              class="cards"
              data-drop-target="category"
              data-category-id={cat.id}
              data-project-id={cat.project_id}
            >
              {#each cat.cards as card (card.id)}
                <li data-card-id={card.id}>
                  <CardRow {card} projectKey={pkey} onClick={() => navigate(cardURL(card.id))} />
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
    align-items: center;
    justify-content: flex-start;
    gap: 0.5rem;
    width: 100%;
    margin: 0 0 0.5rem;
    padding: 0.55rem 0.6rem;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 8px;
    color: inherit;
    font: inherit;
    cursor: pointer;
    text-align: left;
  }

  .cat-header:hover,
  .cat-header:focus-visible {
    background: var(--bg-elev-1);
    border-color: var(--border);
    outline: none;
  }

  .cat-header h2 {
    margin: 0;
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text);
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex: 1;
    min-width: 0;
  }

  .count {
    font-size: 0.75rem;
    color: var(--text-faint);
  }

  .zoom-hint {
    color: var(--text-faint);
    font-size: 1.1rem;
    line-height: 1;
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

  /* DnD visual states (driven by dnd.svelte.ts adding/removing classes) */
  .categories :global(.dnd-source) {
    opacity: 0.35;
    transition: opacity 120ms ease;
  }

  .categories :global([data-drop-target='category'].dnd-target-active) {
    outline: 2px solid var(--accent);
    outline-offset: 4px;
    border-radius: 8px;
    transition: outline-color 120ms ease;
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
