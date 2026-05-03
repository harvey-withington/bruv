<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { repoRPC } from '../lib/auth'
  import { navigate, cardURL, projectURL } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
  import { repoMeta, loadProjectTags, projectKey as makeProjectKey } from '../lib/repoMeta.svelte'
  import type { Category, CardSummary } from '../lib/model'
  import DynamicIcon from '../components/DynamicIcon.svelte'
  import CardRow from '../components/CardRow.svelte'
  import { dragSortable, type DragMoveDetail } from '../lib/actions/dnd.svelte'
  import { onEvent } from '../lib/events.svelte'

  // Focused single-category view. Reachable via the three-level zoom
  // (ProjectPage → CategoryPage → CardPage) and as a deep link
  // (notification handlers in Phase 3 will land here for category-
  // scoped events). A category fills the screen with its pinned cards;
  // the card list supports drag-to-reorder and drag-to-other-category
  // from the project page only — within a single category, only
  // reorder is meaningful, so DnD here is constrained to one bucket.

  let {
    brand,
    stream,
    project,
    category,
  }: {
    brand: string
    stream: string
    project: string
    category: string
  } = $props()

  let loading = $state(true)
  let errorMsg = $state<string | null>(null)
  let cat = $state<Category | null>(null)
  let cards = $state<CardSummary[]>([])

  // svelte-ignore state_referenced_locally
  const pkey = $state(makeProjectKey(brand, stream, project))

  onMount(async () => {
    const brandSlug = brand
    const streamSlug = stream
    const projectSlug = project
    const categorySlug = category
    loadProjectTags(brandSlug, streamSlug, projectSlug)
    try {
      const cats = (await repoRPC<Category[]>('ListCategories', [
        brandSlug,
        streamSlug,
        projectSlug,
      ])) ?? []
      const found = cats.find((c) => c.slug === categorySlug) ?? null
      cat = found
      if (!found) {
        errorMsg = t('category.err_not_found')
        return
      }
      const ids = (await repoRPC<string[]>('ListCardIDsInCategory', [found.id, found.id])) ?? []
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
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('category.err_load')
    } finally {
      loading = false
    }
  })

  // Live updates: refetch cards in this category when any card or
  // category event fires. Filter on cat.id where the payload provides
  // it; otherwise reload conservatively.
  let liveReloadTimer: ReturnType<typeof setTimeout> | null = null
  async function reloadCards() {
    if (!cat) return
    try {
      const ids = (await repoRPC<string[]>('ListCardIDsInCategory', [cat.id, cat.id])) ?? []
      const fetched = await Promise.all(
        ids.map(async (id) => {
          try { return await repoRPC<CardSummary>('GetCard', [id]) }
          catch { return null }
        }),
      )
      cards = fetched.filter((c): c is CardSummary => c !== null)
    } catch {
      /* transient — keep what we have */
    }
  }
  function scheduleReload() {
    if (liveReloadTimer) clearTimeout(liveReloadTimer)
    liveReloadTimer = setTimeout(() => {
      liveReloadTimer = null
      void reloadCards()
    }, 150)
  }
  const unsubscribeEvents = onEvent((ev) => {
    if (
      ev.topic === 'card:created' ||
      ev.topic === 'card:updated' ||
      ev.topic === 'card:deleted' ||
      ev.topic === 'category:updated'
    ) {
      scheduleReload()
    }
  })
  onDestroy(() => {
    if (liveReloadTimer) clearTimeout(liveReloadTimer)
    unsubscribeEvents()
  })

  async function handleDnDMove(detail: DragMoveDetail) {
    if (!cat) return
    const card = cards.find((c) => c.id === detail.cardID)
    if (!card) return
    const idx = cards.findIndex((c) => c.id === detail.cardID)
    const updated = [...cards]
    updated.splice(idx, 1)
    const toIdx = Math.max(0, Math.min(detail.toPosition, updated.length))
    updated.splice(toIdx, 0, card)
    cards = updated
    try {
      await repoRPC('MoveCardInCategory', [
        detail.cardID,
        detail.toProjectID,
        detail.toCategoryID,
        detail.toPosition,
      ])
    } catch (err) {
      console.error('drag move failed:', err)
      errorMsg = err instanceof Error ? err.message : t('category.err_load')
    }
  }
</script>

<header class="topbar">
  <button type="button" class="back" onclick={() => navigate(projectURL(brand, stream, project))}>
    <span aria-hidden="true">‹</span> {t('common.back')}
  </button>
  <h1 title={cat?.name ?? category}>
    {cat?.name ?? category}
  </h1>
  <span class="spacer"></span>
</header>

<main style:view-transition-name={cat ? `category-${cat.id}` : undefined}>
  {#if loading}
    <p class="status">{t('common.loading')}</p>
  {:else if errorMsg}
    <p class="error">{errorMsg}</p>
  {:else if cat}
    <header class="cat-header">
      <h2>
        {#if cat.icon}
          <DynamicIcon name={cat.icon} size={18} />
        {/if}
        {cat.name}
      </h2>
      <span class="count">{cards.length}</span>
    </header>

    {#if cards.length === 0}
      <p class="cat-empty">{t('project.category_empty')}</p>
    {:else}
      <ul
        class="cards"
        data-drop-target="category"
        data-category-id={cat.id}
        data-project-id={cat.project_id}
        use:dragSortable={{ onMove: handleDnDMove }}
      >
        {#each cards as card (card.id)}
          <li data-card-id={card.id}>
            <CardRow {card} projectKey={pkey} onClick={() => navigate(cardURL(card.id))} />
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</main>

<style>
  .topbar {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: center;
    padding: 0.75rem;
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

  .cat-header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin: 0 0.25rem 0.75rem;
  }

  .cat-header h2 {
    margin: 0;
    font-size: 1.05rem;
    font-weight: 600;
    color: var(--text);
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }

  .count {
    font-size: 0.8rem;
    color: var(--text-faint);
  }

  .cat-empty {
    color: var(--text-faint);
    font-size: 0.9rem;
    margin: 1.5rem 0.5rem;
    font-style: italic;
    text-align: center;
  }

  .cards {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .cards :global(li) {
    list-style: none;
  }

  /* DnD visual states (driven by dnd.svelte.ts adding/removing classes) */
  .cards :global(.dnd-source) {
    opacity: 0.35;
    transition: opacity 120ms ease;
  }

  /* Action toggles `.dnd-target-active` on the same .cards <ul>. Use
     :global so the scoped-class JS selector still matches. */
  :global(.cards.dnd-target-active) {
    outline: 2px solid var(--accent);
    outline-offset: 4px;
    border-radius: 8px;
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
