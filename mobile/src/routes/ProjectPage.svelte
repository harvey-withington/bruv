<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte'
  import { ChevronRight, ChevronsUpDown, ChevronsDownUp, ListCollapse, ListTree, Search, Plus, MoreVertical, Pencil, Trash2 } from 'lucide-svelte'
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
    createCategory,
    renameCategory,
    deleteCategory,
    uniqueName,
  } from '../lib/browse.svelte'
  import type { Category, CardSummary } from '../lib/model'
  import DynamicIcon from '../components/DynamicIcon.svelte'
  import CardRow from '../components/CardRow.svelte'
  import SearchSheet from '../components/SearchSheet.svelte'
  import ConfirmDialog from '../components/ConfirmDialog.svelte'
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
  let searchOpen = $state(false)
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

  // --- Category create / rename / delete -----------------------------
  //
  // Mirrors the BrowsePage Brand/Stream/Project flow: tap "+" creates
  // a default-named category and immediately enters inline-rename mode;
  // blur/Enter commits via RenameCategory; blur on an unchanged fresh
  // row deletes it (so "+ then back out" cancels). Per-row kebab opens
  // a Rename / Delete menu. Delete is gated by ConfirmDialog and
  // disabled when only one category remains (backend rejects).

  let renaming = $state<{ categorySlug: string; isCreate: boolean } | null>(null)
  let renameDraft = $state('')
  let renameInputEl = $state<HTMLInputElement | null>(null)
  let renameBusy = $state(false)
  let mutationError = $state<string | null>(null)
  let openMenuKey = $state<string | null>(null)
  let pendingDelete = $state<{ categorySlug: string; name: string } | null>(null)

  function targetKey(slug: string): string { return `cat:${slug}` }

  async function focusRenameInput() {
    await tick()
    renameInputEl?.focus()
    renameInputEl?.select()
  }

  function startRename(categorySlug: string, currentName: string, isCreate = false) {
    closeMenu()
    renaming = { categorySlug, isCreate }
    renameDraft = currentName
    void focusRenameInput()
  }

  async function commitRename() {
    if (!renaming || renameBusy) return
    const { categorySlug, isCreate } = renaming
    const next = renameDraft.trim()
    const current = categories.find((c) => c.slug === categorySlug)?.name ?? ''
    if (!next || next === current) {
      renaming = null
      // Cancel-on-empty for a fresh-create deletes the just-created row.
      if (isCreate) await silentDelete(categorySlug)
      return
    }
    renameBusy = true
    try {
      await renameCategory(brand, stream, project, categorySlug, next)
      mutationError = null
      renaming = null
      void reloadProject()
    } catch (err) {
      mutationError = `${t('project.err_rename_category')} ${err instanceof Error ? err.message : ''}`.trim()
    } finally {
      renameBusy = false
    }
  }

  function cancelRename() {
    const wasCreate = renaming?.isCreate
    const slug = renaming?.categorySlug
    renaming = null
    if (wasCreate && slug) void silentDelete(slug)
  }

  async function silentDelete(categorySlug: string) {
    try {
      await deleteCategory(brand, stream, project, categorySlug)
      void reloadProject()
    } catch (err) {
      mutationError = `${t('project.err_delete_category')} ${err instanceof Error ? err.message : ''}`.trim()
    }
  }

  function onRenameKey(e: KeyboardEvent) {
    if (e.key === 'Enter') { e.preventDefault(); void commitRename() }
    else if (e.key === 'Escape') { e.preventDefault(); cancelRename() }
  }

  async function handleAddCategory() {
    if (renaming) await commitRename()
    try {
      const name = uniqueName(t('project.default_category_name'), categories.map((c) => c.name))
      // Position = end of current list. Backend will append.
      const created = await createCategory(brand, stream, project, name, categories.length)
      mutationError = null
      // Optimistically push so the rename input renders before the
      // SSE-driven reloadProject() completes.
      categories = [...categories, { ...created, cards: [] }]
      startRename(created.slug, created.name, true)
    } catch (err) {
      mutationError = `${t('project.err_create_category')} ${err instanceof Error ? err.message : ''}`.trim()
    }
  }

  function toggleMenu(slug: string) {
    const key = targetKey(slug)
    openMenuKey = openMenuKey === key ? null : key
  }
  function closeMenu() { openMenuKey = null }
  function onWindowPointerDown(e: PointerEvent) {
    if (!openMenuKey) return
    const target = e.target as HTMLElement | null
    if (target && (target.closest('.row-menu') || target.closest('.row-kebab'))) return
    closeMenu()
  }

  function requestDelete(categorySlug: string, name: string) {
    closeMenu()
    pendingDelete = { categorySlug, name }
  }

  async function confirmDelete() {
    if (!pendingDelete) return
    const slug = pendingDelete.categorySlug
    try {
      await deleteCategory(brand, stream, project, slug)
      mutationError = null
      void reloadProject()
    } catch (err) {
      mutationError = `${t('project.err_delete_category')} ${err instanceof Error ? err.message : ''}`.trim()
    } finally {
      pendingDelete = null
    }
  }

  // --- Add card directly into a category -----------------------------
  //
  // Mirrors desktop Board.svelte: CreateCard with the empty-string type
  // (None — user can pick later), then PinCard to this category. The
  // PinCard quirk — projectID and categoryID are both the category ID —
  // is the same one ListCardIDsInCategory and MoveCardToCategory use.
  // Navigates to the new card so the user can fill in the title and
  // body without an extra hop.

  async function handleAddCard(cat: CategoryWithCards) {
    if (renaming) await commitRename()
    closeMenu()
    try {
      const card = await repoRPC<{ id: string }>('CreateCard', ['', t('project.default_card_name')])
      await repoRPC('PinCard', [card.id, cat.id, cat.id])
      mutationError = null
      navigate(cardURL(card.id))
    } catch (err) {
      mutationError = `${t('project.err_create_card')} ${err instanceof Error ? err.message : ''}`.trim()
    }
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

<svelte:window onpointerdown={onWindowPointerDown} />

<header class="topbar">
  <button type="button" class="back" onclick={() => navigate('/')}>
    <span aria-hidden="true">‹</span> {t('common.back')}
  </button>
  <h1 title={projectName}>{projectName}</h1>
  <button type="button" class="topbar-search" onclick={() => (searchOpen = true)} aria-label={t('browse.search')} title={t('browse.search')}>
    <Search size={18} />
  </button>
</header>

{#if searchOpen}
  <SearchSheet onClose={() => (searchOpen = false)} />
{/if}

<main>
  {#if loading}
    <p class="status">{t('common.loading')}</p>
  {:else if errorMsg}
    <p class="error">{errorMsg}</p>
  {:else if categories.length === 0}
    <div class="empty">
      <h2>{t('project.empty_title')}</h2>
      <p>{t('project.empty_body')}</p>
      <button type="button" class="empty-add-btn" onclick={handleAddCategory}>
        <Plus size={14} /> <span>{t('project.add_category')}</span>
      </button>
    </div>
  {:else}
    {#if mutationError}
      <p class="error tree-error" role="alert">{mutationError}</p>
    {/if}
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
            {#if renaming?.categorySlug === cat.slug}
              <div class="cat-toggle renaming">
                <span class="caret" class:open aria-hidden="true">
                  <ChevronRight size={14} />
                </span>
                {#if cat.icon}
                  <DynamicIcon name={cat.icon} size={16} />
                {/if}
                <input
                  bind:this={renameInputEl}
                  bind:value={renameDraft}
                  onblur={commitRename}
                  onkeydown={onRenameKey}
                  class="rename-input"
                  aria-label={t('project.rename_category')}
                  placeholder={t('project.rename_category')}
                  disabled={renameBusy}
                />
              </div>
            {:else}
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
                class="cat-add-card"
                onclick={(e) => { e.stopPropagation(); handleAddCard(cat) }}
                aria-label={t('project.add_card_to', { name: cat.name })}
                title={t('project.add_card_to', { name: cat.name })}
              >
                <Plus size={16} />
              </button>
              <button
                type="button"
                class="row-kebab cat-kebab"
                onclick={(e) => { e.stopPropagation(); toggleMenu(cat.slug) }}
                aria-label={t('project.row_actions', { name: cat.name })}
                aria-expanded={openMenuKey === targetKey(cat.slug)}
                aria-haspopup="menu"
              >
                <MoreVertical size={16} />
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
            {/if}
          </header>
          {#if openMenuKey === targetKey(cat.slug)}
            <div class="row-menu" role="menu">
              <button type="button" role="menuitem" class="row-menu-item" onclick={() => startRename(cat.slug, cat.name)}>
                <Pencil size={14} /> {t('project.action_rename')}
              </button>
              <button
                type="button"
                role="menuitem"
                class="row-menu-item danger"
                disabled={categories.length <= 1}
                title={categories.length <= 1 ? t('project.cant_delete_last') : ''}
                onclick={() => requestDelete(cat.slug, cat.name)}
              >
                <Trash2 size={14} /> {t('project.action_delete')}
              </button>
            </div>
          {/if}
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
      <button type="button" class="add-category-btn" onclick={handleAddCategory} aria-label={t('project.add_category')}>
        <Plus size={14} />
        <span>{t('project.add_category')}</span>
      </button>
    </div>
  {/if}
</main>

{#if pendingDelete}
  <ConfirmDialog
    title={t('project.delete_category_title', { name: pendingDelete.name })}
    body={t('project.delete_category_body')}
    confirmLabel={t('project.action_delete')}
    destructive
    onConfirm={confirmDelete}
    onCancel={() => (pendingDelete = null)}
  />
{/if}

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
  .topbar-search {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 8px;
    justify-self: end;
    min-width: 40px;
    min-height: 40px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .topbar-search:hover,
  .topbar-search:focus-visible {
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

  .add-category-btn {
    margin-top: 0.6rem;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.85rem;
    padding: 0.65rem 0.75rem;
    cursor: pointer;
    background: transparent;
    border: 1px dashed var(--border);
    border-radius: 8px;
    width: 100%;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    text-align: left;
  }
  .add-category-btn:hover,
  .add-category-btn:focus-visible {
    color: var(--text);
    border-color: var(--text-muted);
    background: var(--bg-elev-1);
    outline: none;
  }
  .add-category-btn :global(svg) {
    color: var(--accent);
  }
  .empty-add-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 1rem;
    padding: 0.6rem 1rem;
    background: transparent;
    border: 1px dashed var(--border);
    border-radius: 8px;
    color: var(--text);
    font: inherit;
    font-size: 0.9rem;
    cursor: pointer;
  }
  .empty-add-btn:hover,
  .empty-add-btn:focus-visible {
    border-color: var(--accent);
    background: var(--bg-elev-1);
    outline: none;
  }
  .empty-add-btn :global(svg) {
    color: var(--accent);
  }

  .cat-add-card {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: 0;
    color: var(--accent);
    cursor: pointer;
    padding: 0 0.55rem;
    min-width: 36px;
    min-height: 44px;
  }
  .cat-add-card:hover,
  .cat-add-card:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }

  /* Per-row kebab + popover menu — same shape as BrowsePage's. */
  .row-kebab,
  .cat-kebab {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: 0;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0 0.65rem;
    min-width: 36px;
    min-height: 44px;
  }
  .row-kebab:hover,
  .row-kebab:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }
  .row-menu {
    display: flex;
    flex-direction: column;
    margin: 0.15rem 0.6rem 0.5rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 8px;
    overflow: hidden;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.25);
  }
  .row-menu-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    background: transparent;
    border: 0;
    color: var(--text);
    font: inherit;
    font-size: 0.9rem;
    padding: 0.65rem 0.85rem;
    cursor: pointer;
    text-align: left;
  }
  .row-menu-item:hover:not(:disabled),
  .row-menu-item:focus-visible:not(:disabled) {
    background: var(--bg);
    outline: none;
  }
  .row-menu-item.danger {
    color: #fca5a5;
  }
  .row-menu-item.danger:hover:not(:disabled),
  .row-menu-item.danger:focus-visible:not(:disabled) {
    background: rgba(239, 68, 68, 0.12);
  }
  .row-menu-item:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .cat-toggle.renaming {
    background: var(--bg-elev-1);
    border-radius: 8px 0 0 8px;
  }
  .rename-input {
    flex: 1;
    min-width: 0;
    background: var(--bg);
    border: 1px solid var(--accent);
    border-radius: 6px;
    color: var(--text);
    font: inherit;
    font-size: inherit;
    padding: 0.35rem 0.5rem;
    outline: none;
  }
  .tree-error {
    margin: 0.25rem 0 0.6rem;
    padding: 0.5rem 0.75rem;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 8px;
    color: #fca5a5;
    font-size: 0.85rem;
    text-align: left;
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
