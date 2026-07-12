<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte'
  import { ChevronRight, ChevronsUpDown, ChevronsDownUp, ListCollapse, ListTree, Search, Plus, MoreVertical, Pencil, Trash2, Upload, Layers } from 'lucide-svelte'
  import { repoRPC } from '../lib/auth'
  import { onReconnect } from '../lib/connectivity.svelte'
  import { navigate, cardURL } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
  import { renderInline } from '@shared/markdown'
  import { inlineEdit } from '@shared/inlineEdit'
  import { getCardTypeColor, getCardTypeLabel } from '@shared/cardTypes'
  import { repoMeta, loadProjectTags, projectKey as makeProjectKey } from '../lib/repoMeta.svelte'
  import {
    browse,
    setAccordionMode,
    applySingleModeToCategories,
    toggleCategoryExpansion,
    expandCategory,
    collapseCategory,
    collapseAllExcept,
    expandAllCategories,
    collapseAllCategories,
    ensureInitialExpansion,
    createCategory,
    renameCategory,
    deleteCategory,
    uniqueName,
  } from '../lib/browse.svelte'
  import type { Category, CardSummary } from '../lib/model'
  import { importCardFromJson, ImportError, type TypeConflictResolution } from '../lib/cardExport'
  import { showToast } from '../lib/toast.svelte'
  import ImportConfirmSheet from '../components/ImportConfirmSheet.svelte'
  import CategoryTypesSheet from '../components/CategoryTypesSheet.svelte'
  import DynamicIcon from '../components/DynamicIcon.svelte'
  import CardRow from '../components/CardRow.svelte'
  import SearchSheet from '../components/SearchSheet.svelte'
  import CaptureButton from '../components/CaptureButton.svelte'
  import ChatButton from '../components/chat/ChatButton.svelte'
  import ConfirmDialog from '../components/ConfirmDialog.svelte'
  import ErrorState from '../components/ErrorState.svelte'
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

  async function loadProject() {
    // Capture the route slugs into closure-stable locals before any
    // await — Svelte 5's $props are reactive and reading them mid-
    // async would otherwise warn (state_referenced_locally).
    const brandSlug = brand
    const streamSlug = stream
    const projectSlug = project
    // Reset load state so a retry shows the spinner and clears a prior
    // error (e.g. after reconnecting to Tailscale).
    loading = true
    errorMsg = null
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
            const ids = (await repoRPC<string[]>('ListCardIDsInCategory', [cat.id])) ?? []
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
      // Hash-driven focus: a #cat=<slug> in the URL (set when arriving
      // via a pin breadcrumb on a card view) overrides the default
      // first-non-empty expansion. We force the matching category open
      // and scroll it into view so the card's "home" is immediately
      // visible — without this, the user lands at the top of the
      // project and has to hunt for the category their card lives in.
      const hashMatch = window.location.hash.match(/^#cat=(.+)$/)
      const focusSlug = hashMatch ? decodeURIComponent(hashMatch[1]) : null
      const focusCat = focusSlug ? categories.find((c) => c.slug === focusSlug) : null

      if (focusCat) {
        // toggleCategoryExpansion inverts state; only call it when the
        // target is currently closed so we never accidentally collapse
        // the very category we're trying to focus. Read the store
        // directly here rather than via the `expanded` $derived above —
        // the derived chains through `projectID = categories[0]?...`,
        // which may not have settled in the same tick that we just
        // assigned `categories = populated`.
        const currentExpansion = browse.categoryExpansionFor(focusCat.project_id)
        if (currentExpansion[focusCat.id] !== true) {
          toggleCategoryExpansion(
            focusCat.project_id,
            focusCat.id,
            categories.map((c) => c.id),
          )
        }
        await tick()
        const el = document.querySelector(`[data-category-id="${focusCat.id}"]`)
        if (el) (el as HTMLElement).scrollIntoView({ behavior: 'smooth', block: 'start' })
      } else if (categories.length > 0) {
        // First-visit default: expand the first non-empty category if
        // the user hasn't touched the accordion yet for this project.
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
  }

  onMount(() => {
    void loadProject()
    // On reconnect, refetch the whole project if the initial load failed;
    // otherwise just refresh the board silently (no edit state to lose).
    return onReconnect(() => {
      mutationError = null
      if (errorMsg) void loadProject()
      else void reloadProject()
    })
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
    const next = browse.accordionMode === 'single' ? 'multi' : 'single'
    setAccordionMode(next)
    // Entering single mode collapses to one open category right away —
    // desktop parity with the sidebar/block accordions.
    if (next === 'single' && projectID) {
      applySingleModeToCategories(projectID, categories.map((c) => c.id))
    }
  }

  function handleHoverExpand(target: HTMLElement) {
    if (!projectID) return
    const catID = target.getAttribute('data-category-id')
    // Expand ONLY the hovered target — never route through
    // toggleCategory, whose single-mode path collapses every other
    // category (including the drag source). Collapsing the source
    // mid-drag unmounts the dragged row and the browser cancels the
    // drag, so the drop can never land. See expandCategory's doc.
    // Collapsing stale targets is handled by handleReconcileExpanded as
    // the finger moves, once the row has left them.
    if (catID && !isExpanded(catID)) expandCategory(projectID, catID)
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
      // Close the input — the inlineEdit action treats a commit as the
      // end of the edit session, so keep-open-and-retry isn't reachable
      // from the keyboard. The inline error explains; the user reopens
      // rename from the kebab menu to retry.
      renaming = null
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

  // Rename keyboard behaviour comes from the shared inlineEdit action
  // (Enter/blur commit, Escape cancels — deleting a fresh-create row).
  // ProjectPage isn't a closable container, so no EditScope is passed.

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

  // --- Accepted card types per category ------------------------------
  //
  // Mobile counterpart of desktop Column.svelte's Layers popover.
  // Persists via the same per-repo RPC (UpdateCategoryAcceptedTypes,
  // slug-addressed). An empty list = unrestricted. Restricting a
  // category with existing cards of other types is allowed — the
  // backend only gates NEW pins/moves (CategoryAcceptsType), existing
  // pins stay put, matching desktop.

  let typesSheetCat = $state<{ slug: string; name: string; accepted_types?: string[] } | null>(null)

  function openTypesSheet(cat: CategoryWithCards) {
    closeMenu()
    typesSheetCat = { slug: cat.slug, name: cat.name, accepted_types: cat.accepted_types }
  }

  async function saveAcceptedTypes(types: string[]) {
    const target = typesSheetCat
    if (!target) return
    try {
      await repoRPC('UpdateCategoryAcceptedTypes', [brand, stream, project, target.slug, types])
      // Optimistic local update — the backend doesn't emit a
      // category event for this change, so there's no SSE reload.
      const cat = categories.find((c) => c.slug === target.slug)
      if (cat) cat.accepted_types = types.length ? types : undefined
    } catch (err) {
      showToast(`${t('project.err_save_types')} ${err instanceof Error ? err.message : ''}`.trim(), 'error')
      throw err // keep the sheet open for a retry
    }
  }

  // --- Add card directly into a category -----------------------------
  //
  // Mirrors desktop Board.svelte handleAddCard: a category restricted
  // to exactly one accepted type creates the card AS that type; any
  // other case creates untyped ('' — user can pick later). Then PinCard
  // to this category and navigate to the new card so the user can fill
  // in the title and body without an extra hop.

  async function handleAddCard(cat: CategoryWithCards) {
    if (renaming) await commitRename()
    closeMenu()
    try {
      const cardType = cat.accepted_types?.length === 1 ? cat.accepted_types[0] : ''
      const card = await repoRPC<{ id: string }>('CreateCard', [cardType, t('project.default_card_name')])
      await repoRPC('PinCard', [card.id, cat.id])
      mutationError = null
      navigate(cardURL(card.id))
    } catch (err) {
      mutationError = `${t('project.err_create_card')} ${err instanceof Error ? err.message : ''}`.trim()
    }
  }

  // --- Import card from BRUV JSON file ----------------------------
  //
  // The hidden file input lives at the page level; we remember which
  // category triggered the picker so the import targets the right one.

  let importInputEl = $state<HTMLInputElement | null>(null)
  let importTarget = $state<{ id: string; name: string } | null>(null)
  // Import replay in flight — shows a blocking overlay spinner (the
  // triggering menu has already closed, so nothing else can host the
  // busy state) while attachments are decoded and RPCs replay.
  let importing = $state(false)
  // Type-conflict sheet state: set when the export's card type isn't
  // accepted by the target category; the shared replay awaits `resolve`.
  let importConflict = $state<{
    cardType: string
    categoryName: string
    acceptedTypes: string[]
    resolve: (resolution: TypeConflictResolution | null) => void
  } | null>(null)

  function triggerImportCard(cat: CategoryWithCards) {
    closeMenu()
    importTarget = { id: cat.id, name: cat.name }
    importInputEl?.click()
  }

  async function handleImportFileSelected(e: Event) {
    const input = e.currentTarget as HTMLInputElement
    const file = input.files?.[0]
    const target = importTarget
    input.value = ''
    importTarget = null
    if (!file || !target) return
    importing = true
    try {
      const text = await file.text()
      const result = await importCardFromJson(text, target.id, {
        categoryName: target.name,
        resolveTypeConflict: (cardType, catName, acceptedTypes) =>
          new Promise((resolve) => {
            importConflict = { cardType, categoryName: catName, acceptedTypes, resolve }
          }),
      })
      if (result === null) return // user cancelled the type-conflict sheet — nothing created
      // Toasts survive navigation, so we can open the imported card
      // immediately and still show the partial-restore warning.
      if (result.failedAttachments.length || result.failedComments.length) {
        showToast(t('card.import_partial'), 'warning')
      }
      navigate(cardURL(result.cardId))
    } catch (err) {
      if (err instanceof ImportError) {
        showToast(t(`card.import_err_${err.code}`), 'error')
      } else {
        showToast(`${t('card.import_err_generic')} ${err instanceof Error ? err.message : ''}`.trim(), 'error')
      }
    } finally {
      importing = false
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
            const ids = (await repoRPC<string[]>('ListCardIDsInCategory', [cat.id])) ?? []
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
  // Accepted-types gate for an in-flight drag, evaluated per target as
  // the finger moves (desktop parity: Column.svelte cardTypeAllowed).
  // Untyped cards and unrestricted categories always pass — same
  // semantics as the backend's CategoryAcceptsType.
  // Accordion bookkeeping for an in-flight drag. While dragging we keep
  // the source category and the current hover-target open and collapse
  // anything we auto-expanded on the way; when the drag settles we fall
  // back to single-expand (only the category the card landed in).
  let dragKeepOpenCatID: string | null = null

  function handleDragStart(detail: { fromCategoryID: string }) {
    dragKeepOpenCatID = detail.fromCategoryID
    // Start from a clean single-expand baseline: only the source open,
    // so the drag stays compact from the off. Collapsing here is safe —
    // the dragged row lives in the source, which we keep open.
    if (projectID && browse.accordionMode === 'single') {
      collapseAllExcept(projectID, categories.map((c) => c.id), detail.fromCategoryID)
    }
  }

  // The action reports which categories must stay open (source + where
  // the row is + the target under the finger); collapse every other
  // expanded one so only the source and current target remain open
  // mid-drag. Single mode only — multi-expand keeps the user's open set.
  // The dnd-source guard is belt-and-braces: never collapse a category
  // the live row is inside (that would unmount it and cancel the drag).
  function handleReconcileExpanded(keepCatIDs: string[]) {
    if (!projectID || browse.accordionMode !== 'single') return
    const keep = new Set(keepCatIDs)
    const liveRow = document.querySelector('.dnd-source')
    for (const cat of categories) {
      if (keep.has(cat.id) || !isExpanded(cat.id)) continue
      const el = document.querySelector(`[data-category-id="${cat.id}"]`)
      if (el && liveRow && el.contains(liveRow)) continue
      collapseCategory(projectID, cat.id)
    }
  }

  function handleDragEnd() {
    // Single-expand parity: once the drag settles, keep only the
    // category the card ended up in open (the source, if nothing moved).
    // Multi-expand leaves the user's open set alone.
    if (projectID && browse.accordionMode === 'single' && dragKeepOpenCatID) {
      collapseAllExcept(projectID, categories.map((c) => c.id), dragKeepOpenCatID)
    }
    dragKeepOpenCatID = null
  }

  // Dropped onto the "add category" affordance: create a fresh category
  // and move the card into it. The card was snapped back to its source
  // by the action, so this is a normal cross-category move into a
  // brand-new destination — then drop straight into rename, matching the
  // "+" button so the user can name it without an extra hop.
  async function handleDropCreate(detail: { cardID: string; fromCategoryID: string }) {
    try {
      const name = uniqueName(t('project.default_category_name'), categories.map((c) => c.name))
      const created = await createCategory(brand, stream, project, name, categories.length)
      await repoRPC('MoveCardToCategory', [detail.cardID, detail.fromCategoryID, created.id, 0])
      mutationError = null
      // Optimistically add the new category so its header (and the rename
      // input) renders now; the SSE reload from the card move fills in
      // the real card list shortly after.
      categories = [...categories, { ...created, cards: [] }]
      // Keep the new category open; in single mode collapse the rest so
      // the board doesn't sprawl. Overrides handleDragEnd's earlier
      // collapse (which ran before this async work resolved).
      if (browse.accordionMode === 'single') {
        collapseAllExcept(created.project_id, categories.map((c) => c.id), created.id)
      } else {
        expandCategory(created.project_id, created.id)
      }
      startRename(created.slug, created.name, true)
    } catch (err) {
      showToast(`${t('project.err_create_category')} ${err instanceof Error ? err.message : ''}`.trim(), 'error')
      void reloadProject()
    }
  }

  function findDraggedCard(row: HTMLElement): CardSummary | undefined {
    const cardID = row.getAttribute('data-card-id')
    if (!cardID) return undefined
    return categories.flatMap((c) => c.cards).find((c) => c.id === cardID)
  }

  function canDropCard(row: HTMLElement, target: HTMLElement): boolean {
    const cat = categories.find((c) => c.id === target.getAttribute('data-category-id'))
    if (!cat?.accepted_types?.length) return true
    const card = findDraggedCard(row)
    if (!card?.type) return true // untyped cards are always allowed
    return cat.accepted_types.includes(card.type)
  }

  // Desktop silently refuses (console.warn) because the red column is
  // explanation enough under a hovering cursor; touch has no hover
  // affordance, so a brief toast says why the card snapped back.
  function handleDropRejected(row: HTMLElement, target: HTMLElement) {
    const cat = categories.find((c) => c.id === target.getAttribute('data-category-id'))
    const card = findDraggedCard(row)
    showToast(
      t('project.drop_type_rejected', {
        category: cat?.name ?? '',
        type: getCardTypeLabel(card?.type, repoMeta.cardTypes),
      }),
      'warning',
    )
  }

  async function handleDnDMove(detail: DragMoveDetail) {
    const fromCat = categories.find((c) => c.id === detail.fromCategoryID)
    const toCat = categories.find((c) => c.id === detail.toCategoryID)
    if (!fromCat || !toCat) return
    const card = fromCat.cards.find((c) => c.id === detail.cardID)
    if (!card) return

    // The card lands in the destination — keep that category open once
    // the drag settles (handleDragEnd reads this). Set before any await
    // so it's in place by the time the action fires onDragEnd.
    dragKeepOpenCatID = detail.toCategoryID

    // Copy mode: duplicate the card into the destination category.
    // Don't mutate the source — the original stays put. Refetch on
    // success so the new card's server-side ID + state lands locally.
    if (detail.isCopy) {
      try {
        await repoRPC('DuplicateCard', [detail.cardID, detail.toCategoryID])
      } catch (err) {
        showToast(`${t('project.err_copy_card')} ${err instanceof Error ? err.message : ''}`.trim(), 'error')
      }
      // SSE will refresh the destination's card list shortly. No
      // optimistic update — we don't have a real card ID for the dupe.
      void reloadProject()
      return
    }

    // Move (default): optimistic state update mirroring the DOM the
    // action just produced.
    fromCat.cards = fromCat.cards.filter((c) => c.id !== detail.cardID)
    const toIdx = Math.max(0, Math.min(detail.toPosition, toCat.cards.length))
    toCat.cards = [...toCat.cards.slice(0, toIdx), card, ...toCat.cards.slice(toIdx)]

    try {
      if (detail.fromCategoryID === detail.toCategoryID) {
        // Same category: re-persist EVERY card's position, not just the
        // moved one. The backend stores an absolute position on each
        // card's own pin and never shifts siblings (repo.MoveCardInCategory),
        // so writing only the moved card leaves stale, colliding positions
        // that the next reload sorts back into the wrong order. Desktop
        // Board.svelte does the same full re-persist.
        await persistCategoryOrder(toCat)
      } else {
        // Cross category: change the moved card's membership first, then
        // re-persist positions in BOTH source and destination so neither
        // is left with gaps or collisions.
        await repoRPC('MoveCardToCategory', [
          detail.cardID,
          detail.fromCategoryID,
          detail.toCategoryID,
          toIdx,
        ])
        await Promise.all([persistCategoryOrder(fromCat), persistCategoryOrder(toCat)])
      }
    } catch (err) {
      // Surface the failure and converge on server truth by reloading —
      // a transient reorder hiccup shouldn't blow the whole board away
      // with a full-page ErrorState.
      showToast(`${t('project.err_move_card')} ${err instanceof Error ? err.message : ''}`.trim(), 'error')
      void reloadProject()
    }
  }

  // Rewrite every card's stored position in a category to its new
  // contiguous index. Positions are absolute per-pin and the backend
  // never shifts siblings, so any reorder must re-persist the whole
  // list or stale positions collide. Each card owns a separate pin
  // file, so these writes are independent and safe to parallelize.
  async function persistCategoryOrder(cat: CategoryWithCards) {
    await Promise.all(cat.cards.map((c, i) => repoRPC('MoveCardInCategory', [c.id, cat.id, i])))
  }
</script>

<svelte:window onpointerdown={onWindowPointerDown} />

<header class="topbar">
  <button type="button" class="back" onclick={() => navigate('/')}>
    <span aria-hidden="true">‹</span> {t('common.back')}
  </button>
  <h1 title={projectName}>{@html renderInline(projectName)}</h1>
  <div class="topbar-actions">
    <ChatButton scope={{ kind: 'project', brand, stream, project }} />
    <CaptureButton />
    <button type="button" class="topbar-search" onclick={() => (searchOpen = true)} aria-label={t('browse.search')} title={t('browse.search')}>
      <Search size={18} />
    </button>
  </div>
</header>

{#if searchOpen}
  <SearchSheet onClose={() => (searchOpen = false)} />
{/if}

<main>
  {#if loading}
    <p class="status">{t('common.loading')}</p>
  {:else if errorMsg}
    <ErrorState message={errorMsg} />
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
    <div
      class="categories"
      use:dragSortable={{
        onMove: handleDnDMove,
        onHoverExpand: handleHoverExpand,
        canDrop: canDropCard,
        onRejected: handleDropRejected,
        onDragStart: handleDragStart,
        onDragEnd: handleDragEnd,
        onDropCreate: handleDropCreate,
        onReconcileExpanded: handleReconcileExpanded,
      }}
    >
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
                  use:inlineEdit={{ onCommit: () => void commitRename(), onCancel: cancelRename }}
                  class="rename-input"
                  aria-label={t('project.rename_category')}
                  placeholder={t('project.rename_category')}
                  enterkeyhint="done"
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
                <span class="cat-name">{@html renderInline(cat.name)}</span>
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
            {/if}
          </header>
          {#if cat.accepted_types?.length}
            <!-- Accepted-types indicator: one colored segment per type,
                 mirroring desktop Column.svelte's type-color-bar. Sits
                 under the header so it shows in BOTH collapsed and
                 expanded states; absent = category accepts everything. -->
            <div class="type-line" aria-hidden="true">
              {#each cat.accepted_types as typeId (typeId)}
                <span class="type-line-seg" style:background={getCardTypeColor(typeId, repoMeta.cardTypes)}></span>
              {/each}
            </div>
          {/if}
          {#if openMenuKey === targetKey(cat.slug)}
            <div class="row-menu" role="menu">
              <button type="button" role="menuitem" class="row-menu-item" onclick={() => startRename(cat.slug, cat.name)}>
                <Pencil size={14} /> {t('project.action_rename')}
              </button>
              <button type="button" role="menuitem" class="row-menu-item" onclick={() => openTypesSheet(cat)}>
                <Layers size={14} /> {t('project.action_accepted_types')}
              </button>
              <button type="button" role="menuitem" class="row-menu-item" onclick={() => triggerImportCard(cat)}>
                <Upload size={14} /> {t('project.action_import_card')}
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
                {#each cat.cards as card (card.id)}
                  <li data-card-id={card.id}>
                    <CardRow {card} projectKey={pkey} onClick={() => navigate(cardURL(card.id))} />
                  </li>
                {/each}
                <!-- Always rendered; CSS hides it whenever the list holds
                     a card row. This keeps the "empty" placeholder visible
                     the instant a category's only card is physically
                     dragged out into another category (the dnd action
                     moves the <li>, so Svelte's card count is still 1). -->
                <li class="empty-hint cat-empty">{t('project.category_empty')}</li>
              </ul>
            </div>
          {/if}
        </section>
      {/each}
      <!-- Doubles as a drop target: releasing a dragged card here spins
           up a new category and moves the card into it (data-new-category
           tells the dnd action to highlight but never park the row). -->
      <button
        type="button"
        class="add-category-btn"
        onclick={handleAddCategory}
        aria-label={t('project.add_category')}
        data-drop-target="category"
        data-new-category="true"
      >
        <Plus size={14} />
        <span>{t('project.add_category')}</span>
      </button>
    </div>
  {/if}
</main>

<input
  type="file"
  accept=".json,application/json"
  bind:this={importInputEl}
  onchange={handleImportFileSelected}
  hidden
/>

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

{#if typesSheetCat}
  <CategoryTypesSheet
    categoryName={typesSheetCat.name}
    current={typesSheetCat.accepted_types}
    onSave={saveAcceptedTypes}
    onClose={() => (typesSheetCat = null)}
  />
{/if}

{#if importConflict}
  <ImportConfirmSheet
    cardType={importConflict.cardType}
    categoryName={importConflict.categoryName}
    acceptedTypes={importConflict.acceptedTypes}
    onResolve={(resolution) => {
      importConflict?.resolve(resolution)
      importConflict = null
    }}
  />
{/if}

{#if importing && !importConflict}
  <div class="import-busy" role="status" aria-live="polite">
    <span class="import-spinner" aria-hidden="true"></span>
    <span>{t('card.importing')}</span>
  </div>
{/if}

<style>
  /* Blocking busy pill while an import replay is in flight. */
  .import-busy {
    position: fixed;
    inset: 0;
    z-index: 70;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.55rem;
    background: rgba(0, 0, 0, 0.35);
    color: #fff;
    font-size: 0.9rem;
    font-weight: 500;
  }
  .import-spinner {
    width: 18px;
    height: 18px;
    border-radius: 50%;
    border: 2px solid rgba(255, 255, 255, 0.35);
    border-top-color: #fff;
    animation: import-spin 0.8s linear infinite;
  }
  @keyframes import-spin {
    to { transform: rotate(360deg); }
  }

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
    /* Three icon buttons on the right + back on the left: cap the
       title lower than the old 60vw so the row can't overflow on
       narrow phones. */
    max-width: 45vw;
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
  .topbar-actions {
    justify-self: end;
    display: inline-flex;
    align-items: center;
    gap: 0.15rem;
  }
  .topbar-search {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 8px;
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
    padding: 0.75rem 0.85rem 2rem;
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
    border-radius: 9px;
    transition: background 120ms ease;
  }
  /* When the category is expanded, the header sits flush against the
     panel body below — square off the bottom corners so the hover/focus
     fill follows the panel's rectangle rather than rounding off mid-row. */
  .category.expanded .cat-header {
    border-radius: 9px 9px 0 0;
  }
  /* Hover (or keyboard-focus on any header button) tints the whole row,
     including the right-side action buttons. Without this, the
     individual button hovers only paint themselves, so the highlight
     looks like it stops two-thirds across the row. */
  .cat-header:hover,
  .cat-header:focus-within {
    background: var(--bg-elev-1);
  }
  .category.expanded .cat-header:hover,
  .category.expanded .cat-header:focus-within {
    background: color-mix(in srgb, var(--accent) 8%, transparent);
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
    color: inherit;
    font: inherit;
    font-size: 0.95rem;
    font-weight: 600;
    cursor: pointer;
    text-align: left;
    min-height: 44px;
  }
  .cat-toggle:focus-visible {
    outline: none;
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

  /* Accepted-types indicator line (desktop Column.svelte type-color-bar
     equivalent): equal-width segments, one per accepted type. Rendered
     only when the category restricts types — no line = accepts all. */
  .type-line {
    display: flex;
    height: 3px;
    margin: 0 0.6rem 0.35rem;
    border-radius: 2px;
    overflow: hidden;
  }
  .type-line-seg {
    flex: 1;
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
  /* The hint is always in the DOM; hide it whenever the list actually
     holds a card row. Because the dnd action physically moves the <li>
     out during a drag, a source category whose only card is dragged
     away has no card rows left, so the placeholder reappears — the
     category reads as emptied, matching where the card now is. */
  .cards:has([data-card-id]) .empty-hint {
    display: none;
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

  /* Invalid drop target during a card drag: the dragged card's type
     isn't in this category's accepted_types. Mirrors desktop
     Column.svelte's .drop-rejected (danger outline + dimming). */
  .categories :global([data-drop-target='category'].dnd-target-invalid) {
    outline: 2px solid var(--danger-border);
    outline-offset: 4px;
    border-radius: 8px;
    opacity: 0.6;
    transition: outline-color 120ms ease, opacity 120ms ease;
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
