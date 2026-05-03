<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { ChevronRight, ChevronsUpDown, ChevronsDownUp, ListCollapse, ListTree } from 'lucide-svelte'
  import { repoRPC } from '../lib/auth'
  import { navigate, cardURL, categoryURL } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
  import { loadProjectTags, projectKey as makeProjectKey } from '../lib/repoMeta.svelte'
  import {
    browse,
    setAccordionMode,
    toggleCategoryExpansion,
    expandAllCategories,
    collapseAllCategories,
    ensureInitialExpansion,
  } from '../lib/browse.svelte'
  import type { Category, CardSummary } from '../lib/model'
  import DynamicIcon from '../components/DynamicIcon.svelte'
  import CardRow from '../components/CardRow.svelte'
  import { dragSortable, type DragMoveDetail } from '../lib/actions/dnd.svelte'
  import { onEvent } from '../lib/events.svelte'

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
      // First-visit default: expand the first non-empty category if
      // the user hasn't touched the accordion yet for this project.
      if (categories.length > 0) {
        ensureInitialExpansion(
          categories[0].project_id,
          categories.map((c) => ({ id: c.id, cardCount: c.cards.length })),
        )
      }
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('project.err_load')
    } finally {
      loading = false
    }
  })

  // Read the per-project expansion map reactively. project_id comes
  // from the first loaded category. If categories is empty, the map
  // is irrelevant — nothing to expand.
  const projectID = $derived(categories[0]?.project_id ?? '')
  const expanded = $derived(
    projectID ? browse.categoryExpansionFor(projectID) : ({} as Record<string, boolean>),
  )

  function isExpanded(catID: string): boolean {
    return expanded[catID] === true
  }

  function toggleCategory(catID: string) {
    if (!projectID) return
    toggleCategoryExpansion(
      projectID,
      catID,
      categories.map((c) => c.id),
    )
  }

  function expandAll() {
    if (!projectID) return
    expandAllCategories(projectID, categories.map((c) => c.id))
  }
  function collapseAll() {
    if (!projectID) return
    collapseAllCategories(projectID, categories.map((c) => c.id))
  }
  function toggleMode() {
    setAccordionMode(browse.accordionMode === 'single' ? 'multi' : 'single')
  }

  function handleHoverExpand(target: HTMLElement) {
    const catID = target.getAttribute('data-category-id')
    if (catID && !isExpanded(catID)) toggleCategory(catID)
  }

  // Live updates: refetch the project's categories+cards when any
  // card or category event fires. Coarse, but the fetch is cheap
  // and avoids tracking which event affects which view. Coalesced
  // by a tiny debounce so a burst of N events fires one reload.
  let liveReloadTimer: ReturnType<typeof setTimeout> | null = null
  async function reloadProject() {
    try {
      const cats = (await repoRPC<Category[]>('ListCategories', [brand, stream, project])) ?? []
      const populated = await Promise.all(
        cats.map(async (cat) => {
          let cards: CardSummary[] = []
          try {
            const ids = (await repoRPC<string[]>('ListCardIDsInCategory', [cat.id, cat.id])) ?? []
            const fetched = await Promise.all(
              ids.map(async (id) => {
                try { return await repoRPC<CardSummary>('GetCard', [id]) }
                catch { return null }
              }),
            )
            cards = fetched.filter((c): c is CardSummary => c !== null)
          } catch { cards = [] }
          return { ...cat, cards }
        }),
      )
      categories = populated
    } catch {
      /* transient — keep what we have */
    }
  }
  function scheduleReload() {
    if (liveReloadTimer) clearTimeout(liveReloadTimer)
    liveReloadTimer = setTimeout(() => {
      liveReloadTimer = null
      void reloadProject()
    }, 150)
  }
  const unsubscribeEvents = onEvent((ev) => {
    if (
      ev.topic === 'card:created' ||
      ev.topic === 'card:updated' ||
      ev.topic === 'card:deleted' ||
      ev.topic === 'category:updated' ||
      ev.topic === 'category:deleted'
    ) {
      scheduleReload()
    }
  })
  onDestroy(() => {
    if (liveReloadTimer) clearTimeout(liveReloadTimer)
    unsubscribeEvents()
  })

  // Drag-drop reorder + cross-category move + two-finger-copy. The dnd
  // action mutates the DOM live during drag for visual feedback; we
  // mirror the move into our reactive state on drop and persist via
  // the right RPC. Copy mode (engaged by a second pointer during drag)
  // routes to DuplicateCard. On failure, the UI re-loads the project
  // so it returns to the server-truth ordering.
  async function handleDnDMove(detail: DragMoveDetail) {
    const fromCat = categories.find((c) => c.id === detail.fromCategoryID)
    const toCat = categories.find((c) => c.id === detail.toCategoryID)
    if (!fromCat || !toCat) return
    const card = fromCat.cards.find((c) => c.id === detail.cardID)
    if (!card) return

    // Copy mode: duplicate the card into the destination category.
    // Don't mutate the source — the original stays put. Refetch on
    // success so the new card's server-side ID + state lands locally.
    if (detail.isCopy) {
      try {
        await repoRPC('DuplicateCard', [detail.cardID, detail.toCategoryID])
      } catch (err) {
        console.error('duplicate card failed:', err)
        errorMsg = err instanceof Error ? err.message : t('project.err_load')
      }
      // SSE will refresh the destination's card list shortly. No
      // optimistic update — we don't have a real card ID for the dupe.
      return
    }

    // Move (default): optimistic state update mirroring the DOM the
    // action just produced.
    fromCat.cards = fromCat.cards.filter((c) => c.id !== detail.cardID)
    const toIdx = Math.max(0, Math.min(detail.toPosition, toCat.cards.length))
    toCat.cards = [...toCat.cards.slice(0, toIdx), card, ...toCat.cards.slice(toIdx)]

    try {
      // Quirk: the server's RPC declares its second arg as `projectID`,
      // but the canonical caller (desktop Board.svelte) passes the
      // SOURCE category ID there. Pin records on cards use the
      // category ID where the API names suggest project ID — same
      // workaround as ListCardIDsInCategory(cat.id, cat.id). Mobile
      // mirrors what desktop does so the server's pin-lookup matches.
      if (detail.fromCategoryID === detail.toCategoryID) {
        await repoRPC('MoveCardInCategory', [
          detail.cardID,
          detail.toCategoryID,
          detail.toCategoryID,
          detail.toPosition,
        ])
      } else {
        await repoRPC('MoveCardToCategory', [
          detail.cardID,
          detail.fromCategoryID,
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
    <div class="acc-toolbar" role="toolbar" aria-label={t('project.accordion_toolbar')}>
      <button type="button" class="tool-btn" onclick={expandAll} aria-label={t('project.expand_all')} title={t('project.expand_all')}>
        <ChevronsUpDown size={15} />
      </button>
      <button type="button" class="tool-btn" onclick={collapseAll} aria-label={t('project.collapse_all')} title={t('project.collapse_all')}>
        <ChevronsDownUp size={15} />
      </button>
      <button
        type="button"
        class="tool-btn mode-btn"
        onclick={toggleMode}
        aria-label={browse.accordionMode === 'single' ? t('project.mode_single') : t('project.mode_multi')}
        title={browse.accordionMode === 'single' ? t('project.mode_single_hint') : t('project.mode_multi_hint')}
      >
        {#if browse.accordionMode === 'single'}
          <ListCollapse size={15} />
        {:else}
          <ListTree size={15} />
        {/if}
      </button>
    </div>
    <div class="categories" use:dragSortable={{ onMove: handleDnDMove, onHoverExpand: handleHoverExpand }}>
      {#each categories as cat (cat.id)}
        {@const open = isExpanded(cat.id)}
        <section
          class="category"
          class:expanded={open}
          data-drop-target="category"
          data-category-id={cat.id}
          data-project-id={cat.project_id}
          data-collapsed={open ? null : 'true'}
          style:view-transition-name={`category-${cat.id}`}
        >
          <header class="cat-header">
            <button
              type="button"
              class="cat-toggle"
              onclick={() => toggleCategory(cat.id)}
              aria-expanded={open}
              aria-label={open ? t('project.collapse_category', { name: cat.name }) : t('project.expand_category', { name: cat.name })}
            >
              <span class="caret" class:open aria-hidden="true">
                <ChevronRight size={14} />
              </span>
              {#if cat.icon}
                <DynamicIcon name={cat.icon} size={16} />
              {/if}
              <span class="cat-name">{cat.name}</span>
              <span class="count">{cat.cards.length}</span>
            </button>
            <button
              type="button"
              class="cat-zoom"
              onclick={() => navigate(categoryURL(brand, stream, project, cat.slug))}
              aria-label={t('project.zoom_into_category', { name: cat.name })}
              title={t('project.zoom_into_category', { name: cat.name })}
            >
              <ChevronRight size={16} />
            </button>
          </header>
          {#if open}
            <div class="cat-body">
              <ul class="cards" data-card-list>
                {#if cat.cards.length === 0}
                  <li class="empty-hint cat-empty">{t('project.category_empty')}</li>
                {/if}
                {#each cat.cards as card (card.id)}
                  <li data-card-id={card.id}>
                    <CardRow {card} projectKey={pkey} onClick={() => navigate(cardURL(card.id))} />
                  </li>
                {/each}
              </ul>
            </div>
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

  .acc-toolbar {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.4rem 0.25rem 0.65rem;
    border-bottom: 1px dashed var(--border);
    margin-bottom: 0.65rem;
  }

  .tool-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: 1px solid var(--border);
    color: var(--text-muted);
    border-radius: 6px;
    width: 36px;
    height: 36px;
    cursor: pointer;
    padding: 0;
  }
  .tool-btn:hover,
  .tool-btn:focus-visible {
    color: var(--text);
    border-color: var(--text-muted);
    background: var(--bg-elev-1);
    outline: none;
  }
  .tool-btn.mode-btn {
    margin-left: auto;
  }

  .categories {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .category {
    border: 1px solid transparent;
    border-radius: 10px;
    transition: border-color 120ms ease, background 120ms ease;
  }
  .category.expanded {
    border-color: var(--border);
    background: var(--bg-elev-1);
  }

  .cat-header {
    display: flex;
    align-items: center;
    width: 100%;
    margin: 0;
    padding: 0;
    gap: 0;
  }

  .cat-toggle {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex: 1;
    min-width: 0;
    padding: 0.65rem 0.6rem;
    background: transparent;
    border: none;
    border-radius: 8px 0 0 8px;
    color: inherit;
    font: inherit;
    font-size: 0.95rem;
    font-weight: 600;
    cursor: pointer;
    text-align: left;
    min-height: 44px;
  }
  .cat-toggle:hover,
  .cat-toggle:focus-visible {
    background: var(--bg-elev-1);
    outline: none;
  }
  .category.expanded .cat-toggle:hover,
  .category.expanded .cat-toggle:focus-visible {
    background: color-mix(in srgb, var(--accent) 8%, transparent);
  }

  .caret {
    display: inline-flex;
    color: var(--text-muted);
    transition: transform 160ms cubic-bezier(0.16, 1, 0.3, 1);
    flex-shrink: 0;
  }
  .caret.open {
    transform: rotate(90deg);
  }

  .cat-name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text);
  }

  .count {
    font-size: 0.75rem;
    color: var(--text-faint);
    margin-right: 0.25rem;
  }

  .cat-zoom {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: none;
    border-radius: 0 8px 8px 0;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0 0.55rem;
    min-width: 44px;
    min-height: 44px;
  }
  .cat-zoom:hover,
  .cat-zoom:focus-visible {
    color: var(--accent);
    background: var(--bg-elev-1);
    outline: none;
  }

  .cat-body {
    padding: 0.25rem 0.6rem 0.65rem;
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
    list-style: none;
  }

  /* Non-interactive empty-list hint inside an otherwise-empty drop-
     target <ul>. Gives the <ul> hit-testable height so dragging a
     card into an empty category lands correctly. */
  .empty-hint {
    pointer-events: none;
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
