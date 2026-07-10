<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte'
  import { Inbox, Search, Bell, Settings, ChevronsUpDown, ChevronsDownUp, ListCollapse, ListTree, Plus, MoreVertical, Pencil, Trash2 } from 'lucide-svelte'
  import {
    browse,
    loadBrands,
    loadStreams,
    loadProjects,
    setAccordionMode,
    applySingleModeToTree,
    toggleBrandExpansion,
    toggleStreamExpansion,
    expandAllBrandsTree,
    collapseAllBrandsTree,
    createBrand,
    createStream,
    createProject,
    renameBrand,
    renameStream,
    renameProject,
    deleteBrand,
    deleteStream,
    deleteProject,
    uniqueName,
  } from '../lib/browse.svelte'
  import ConfirmDialog from '../components/ConfirmDialog.svelte'
  import ErrorState from '../components/ErrorState.svelte'
  import { navigate, projectURL } from '../lib/router.svelte'
  import { readActiveRepoID, apiFetch, repoRPC, machineRPC } from '../lib/auth'
  import { showToast } from '../lib/toast.svelte'
  import { onEvent } from '../lib/events.svelte'
  import { onReconnect } from '../lib/connectivity.svelte'
  import { t } from '../lib/i18n.svelte'
  import { renderInline } from '@shared/markdown'
  import { inlineEdit } from '@shared/inlineEdit'
  import type { Brand, Stream } from '../lib/model'
  import type { AppNotification } from '@shared/types'
  import DynamicIcon from '../components/DynamicIcon.svelte'
  import NotificationsPanel from '../components/NotificationsPanel.svelte'
  import SearchSheet from '../components/SearchSheet.svelte'
  import { dragSortable, type DragMoveDetail } from '../lib/actions/dnd.svelte'

  // Top-level mobile entry: vault switcher + global search + notification
  // bell + settings gear in the topbar. Body has the Inbox tile, a
  // recently-updated cards shelf, and the Brand → Stream → Project tree
  // (with full DnD reordering / cross-parent moves / two-finger copy).

  let activeRepoName = $state<string | null>(null)
  // Expansion state lives in the browse store so it survives the
  // BrowsePage being unmounted while the user is inside a project /
  // category / card. Local component state would reset on every
  // remount and force the user to re-expand.
  const expandedBrands = browse.expandedBrands
  const expandedStreams = browse.expandedStreams

  // Header surfaces.
  let searchOpen = $state(false)
  let notificationsOpen = $state(false)
  let unreadCount = $state(0)

  // Inline rename state. The newly-created entity (after a "+" tap) or
  // a tapped Rename action enters this state and shows an autofocused
  // input in place of its name button. One row at a time — tapping "+"
  // again or another rename target commits the previous one first.
  type RowTarget =
    | { kind: 'brand'; brandSlug: string }
    | { kind: 'stream'; brandSlug: string; streamSlug: string }
    | { kind: 'project'; brandSlug: string; streamSlug: string; projectSlug: string }
  // `isCreate` distinguishes "renaming a fresh row" (where blurring
  // without changes should delete the row, mirroring desktop) from
  // "renaming an existing row" (where blur leaves the row alone).
  type RenameState = { target: RowTarget; isCreate: boolean }
  let renaming = $state<RenameState | null>(null)
  let renameDraft = $state('')
  let renameInputEl = $state<HTMLInputElement | null>(null)
  let renameBusy = $state(false)
  // Latest error from a create / rename / delete op. Surfaces inline
  // near the section heading so the user sees why their tap didn't
  // take. Cleared on the next successful op.
  let mutationError = $state<string | null>(null)
  // Per-row action menu (kebab popover). Only one open at a time.
  let openMenuKey = $state<string | null>(null)
  // Pending destructive confirmation. When set, ConfirmDialog renders.
  type DeletePending = { target: RowTarget; name: string }
  let pendingDelete = $state<DeletePending | null>(null)

  function targetKey(target: RowTarget): string {
    if (target.kind === 'brand') return `brand:${target.brandSlug}`
    if (target.kind === 'stream') return `stream:${target.brandSlug}/${target.streamSlug}`
    return `project:${target.brandSlug}/${target.streamSlug}/${target.projectSlug}`
  }
  let renamingKeyValue = $derived(renaming ? targetKey(renaming.target) : null)

  async function focusRenameInput() {
    await tick()
    renameInputEl?.focus()
    renameInputEl?.select()
  }

  function startRename(target: RowTarget, currentName: string, isCreate = false) {
    closeMenu()
    renaming = { target, isCreate }
    renameDraft = currentName
    void focusRenameInput()
  }

  async function commitRename() {
    if (!renaming || renameBusy) return
    const { target, isCreate } = renaming
    const next = renameDraft.trim()
    const current = currentNameFor(target)
    // Empty input or unchanged: for a fresh row, treat blur-without-rename
    // as "cancel" and remove the just-created entity. For existing rows,
    // just close the input.
    if (!next || next === current) {
      renaming = null
      if (isCreate) await silentDelete(target)
      return
    }
    renameBusy = true
    try {
      if (target.kind === 'brand') {
        await renameBrand(target.brandSlug, next)
      } else if (target.kind === 'stream') {
        await renameStream(target.brandSlug, target.streamSlug, next)
      } else {
        await renameProject(target.brandSlug, target.streamSlug, target.projectSlug, next)
      }
      mutationError = null
      renaming = null
    } catch (err) {
      mutationError = `${t('browse.err_rename')} ${err instanceof Error ? err.message : ''}`.trim()
      // Close the input — the inlineEdit action treats a commit as the
      // end of the edit session, so "stay open and retry" isn't
      // reachable from the keyboard any more. The inline error above
      // explains; the user reopens rename from the row menu to retry.
      renaming = null
    } finally {
      renameBusy = false
    }
  }

  function cancelRename() {
    const wasCreate = renaming?.isCreate
    const target = renaming?.target
    renaming = null
    // Cancelling a fresh-create deletes the entity; cancelling an
    // existing-row rename leaves it alone.
    if (wasCreate && target) void silentDelete(target)
  }

  // Quietly delete the row that was just created when the user backs
  // out of the rename. Errors here are non-fatal for the UI flow — at
  // worst the user sees an orphaned default-named row, which they can
  // delete via the menu.
  async function silentDelete(target: RowTarget) {
    try {
      if (target.kind === 'brand') await deleteBrand(target.brandSlug)
      else if (target.kind === 'stream') await deleteStream(target.brandSlug, target.streamSlug)
      else await deleteProject(target.brandSlug, target.streamSlug, target.projectSlug)
    } catch (err) {
      mutationError = `${t('browse.err_delete')} ${err instanceof Error ? err.message : ''}`.trim()
    }
  }

  function currentNameFor(target: RowTarget): string {
    if (target.kind === 'brand') {
      return browse.brands.items.find((b) => b.slug === target.brandSlug)?.name ?? ''
    }
    if (target.kind === 'stream') {
      const cache = browse.streamsFor(target.brandSlug)
      return cache?.items.find((s) => s.slug === target.streamSlug)?.name ?? ''
    }
    const cache = browse.projectsFor(target.brandSlug, target.streamSlug)
    return cache?.items.find((p) => p.slug === target.projectSlug)?.name ?? ''
  }

  // Rename keyboard behaviour comes from the shared inlineEdit action
  // (Enter/blur commit, Escape cancels — deleting a fresh-create row).
  // BrowsePage isn't a closable container, so no EditScope is passed.

  // --- Add buttons ---------------------------------------------------

  async function handleAddBrand() {
    if (renaming) await commitRename()
    try {
      const name = uniqueName(
        t('browse.default_brand_name'),
        browse.brands.items.map((b) => b.name),
      )
      const created = await createBrand(name)
      mutationError = null
      startRename({ kind: 'brand', brandSlug: created.slug }, created.name, true)
    } catch (err) {
      mutationError = `${t('browse.err_create_brand')} ${err instanceof Error ? err.message : ''}`.trim()
    }
  }

  async function handleAddStream(brand: Brand) {
    if (renaming) await commitRename()
    try {
      // Make sure the brand is expanded so the new row is visible.
      if (!expandedBrands[brand.slug]) toggleBrand(brand)
      const cache = browse.streamsFor(brand.slug)
      // The brand may have been just-loaded synchronously from a fresh
      // toggle; await loadStreams to be safe so the unique-name search
      // sees the real existing list.
      if (!cache || cache.state !== 'loaded') await loadStreams(brand.slug)
      const fresh = browse.streamsFor(brand.slug)
      const name = uniqueName(
        t('browse.default_stream_name'),
        fresh?.items.map((s) => s.name) ?? [],
      )
      const created = await createStream(brand.slug, name)
      mutationError = null
      startRename(
        { kind: 'stream', brandSlug: brand.slug, streamSlug: created.slug },
        created.name,
        true,
      )
    } catch (err) {
      mutationError = `${t('browse.err_create_stream')} ${err instanceof Error ? err.message : ''}`.trim()
    }
  }

  async function handleAddProject(brand: Brand, stream: Stream) {
    if (renaming) await commitRename()
    try {
      const key = `${brand.slug}/${stream.slug}`
      if (!expandedStreams[key]) toggleStream(brand, stream)
      const cache = browse.projectsFor(brand.slug, stream.slug)
      if (!cache || cache.state !== 'loaded') await loadProjects(brand.slug, stream.slug)
      const fresh = browse.projectsFor(brand.slug, stream.slug)
      const name = uniqueName(
        t('browse.default_project_name'),
        fresh?.items.map((p) => p.name) ?? [],
      )
      const created = await createProject(brand.slug, stream.slug, name)
      mutationError = null
      startRename(
        {
          kind: 'project',
          brandSlug: brand.slug,
          streamSlug: stream.slug,
          projectSlug: created.slug,
        },
        created.name,
        true,
      )
    } catch (err) {
      mutationError = `${t('browse.err_create_project')} ${err instanceof Error ? err.message : ''}`.trim()
    }
  }

  // --- Per-row action menu (kebab) -----------------------------------

  function toggleMenu(target: RowTarget) {
    const key = targetKey(target)
    openMenuKey = openMenuKey === key ? null : key
  }
  function closeMenu() {
    openMenuKey = null
  }
  function onWindowPointerDown(e: PointerEvent) {
    if (!openMenuKey) return
    const target = e.target as HTMLElement | null
    if (target && (target.closest('.row-menu') || target.closest('.row-kebab'))) return
    closeMenu()
  }

  // --- Delete flow ---------------------------------------------------

  function requestDelete(target: RowTarget) {
    closeMenu()
    pendingDelete = { target, name: currentNameFor(target) || '' }
  }

  async function confirmDelete() {
    if (!pendingDelete) return
    const { target } = pendingDelete
    try {
      if (target.kind === 'brand') await deleteBrand(target.brandSlug)
      else if (target.kind === 'stream') await deleteStream(target.brandSlug, target.streamSlug)
      else await deleteProject(target.brandSlug, target.streamSlug, target.projectSlug)
      mutationError = null
    } catch (err) {
      mutationError = `${t('browse.err_delete')} ${err instanceof Error ? err.message : ''}`.trim()
    } finally {
      pendingDelete = null
    }
  }

  function deleteDialogTitle(d: DeletePending): string {
    const name = d.name || '—'
    if (d.target.kind === 'brand') return t('browse.delete_brand_title', { name })
    if (d.target.kind === 'stream') return t('browse.delete_stream_title', { name })
    return t('browse.delete_project_title', { name })
  }
  function deleteDialogBody(d: DeletePending): string {
    if (d.target.kind === 'brand') return t('browse.delete_brand_body')
    if (d.target.kind === 'stream') return t('browse.delete_stream_body')
    return t('browse.delete_project_body')
  }

  async function loadUnread() {
    try {
      const list = (await machineRPC<AppNotification[]>('GetNotifications')) ?? []
      unreadCount = list.filter((n) => !n.read).length
    } catch {
      /* non-fatal — badge stays at last known count */
    }
  }

  onMount(async () => {
    loadBrands()
    void loadUnread()
    // Cosmetic: show the active vault name in the header. Best-effort;
    // missing/failed lookup doesn't block browsing.
    try {
      const activeID = readActiveRepoID()
      if (!activeID) return
      const res = await apiFetch('/repos')
      if (!res.ok) return
      const repos = (await res.json()) as Array<{ id: string; name: string }>
      activeRepoName = repos.find((r) => r.id === activeID)?.name ?? null
    } catch {
      /* silent — header label is decorative */
    }
  })

  const unsubEvents = onEvent((ev) => {
    if (ev.topic === 'notification:new') void loadUnread()
  })
  onDestroy(unsubEvents)

  // On reconnect, force-refresh brands and any expanded sub-trees so the
  // tree reflects server truth without a full reload (preserves which
  // rows are open).
  async function refreshBrowse() {
    mutationError = null
    await loadBrands(true)
    for (const brand of browse.brands.items) {
      if (!expandedBrands[brand.slug]) continue
      await loadStreams(brand.slug, true)
      const streams = browse.streamsFor(brand.slug)
      for (const stream of streams?.items ?? []) {
        if (expandedStreams[`${brand.slug}/${stream.slug}`]) {
          void loadProjects(brand.slug, stream.slug, true)
        }
      }
    }
    void loadUnread()
  }
  onDestroy(onReconnect(refreshBrowse))

  function closeNotifications() {
    notificationsOpen = false
    void loadUnread() // refresh badge after potential mark-read actions
  }

  function toggleBrand(brand: Brand) {
    const allSlugs = browse.brands.items.map((b) => b.slug)
    toggleBrandExpansion(brand.slug, allSlugs)
    if (expandedBrands[brand.slug]) loadStreams(brand.slug)
  }

  function toggleStream(brand: Brand, stream: Stream) {
    const cache = browse.streamsFor(brand.slug)
    const all = cache?.items?.map((s) => s.slug) ?? []
    toggleStreamExpansion(brand.slug, stream.slug, all)
    const key = `${brand.slug}/${stream.slug}`
    if (expandedStreams[key]) loadProjects(brand.slug, stream.slug)
  }

  async function expandAllInTree() {
    const slugs = browse.brands.items.map((b) => b.slug)
    expandAllBrandsTree(slugs)
    // Recursively expand streams under every brand. Wait for each
    // brand's streams to load, then mark every stream open. Project
    // lists themselves stay lazy — they're leaves in the expansion
    // sense (tapping a project navigates rather than expanding inline).
    await Promise.all(slugs.map((s) => loadStreams(s)))
    for (const brandSlug of slugs) {
      const cache = browse.streamsFor(brandSlug)
      if (!cache) continue
      for (const stream of cache.items) {
        expandedStreams[`${brandSlug}/${stream.slug}`] = true
        // Eagerly fetch projects for each expanded stream too so the
        // tree fills out without the user drilling into each one.
        void loadProjects(brandSlug, stream.slug)
      }
    }
  }

  function collapseAllInTree() {
    const slugs = browse.brands.items.map((b) => b.slug)
    collapseAllBrandsTree(slugs)
  }

  function toggleAccordionMode() {
    const next = browse.accordionMode === 'single' ? 'multi' : 'single'
    setAccordionMode(next)
    // Entering single mode collapses to one open path right away —
    // matches desktop's sidebar instead of leaving the multi-expand
    // state on screen until the next tap.
    if (next === 'single') applySingleModeToTree(browse.brands.items.map((b) => b.slug))
  }

  // --- DnD handlers ---
  //
  // Each level of the tree gets its own dragSortable instance scoped
  // to that level's <ul>. Three behaviours per handler:
  //   - Same parent + isCopy=false  → Reorder* RPC
  //   - Same parent + isCopy=true   → Copy* (no-op for in-place; ignored)
  //   - Cross parent + isCopy=false → Move* RPC
  //   - Cross parent + isCopy=true  → Copy* RPC
  // Cross-parent direction comes from data-brand-slug / data-stream-slug
  // on the destination <ul>, captured at the moment the drag commits.
  // Brands have no parent, so cross-parent doesn't apply at that level.

  async function handleBrandDrop(detail: DragMoveDetail) {
    if (detail.isCopy) {
      // CopyBrand has no destination position — call and refresh.
      try {
        await repoRPC('CopyBrand', [detail.cardID])
      } catch (err) {
        console.error('copy brand failed:', err)
        showToast(err instanceof Error ? err.message : t('error.network'), 'error')
      }
      void loadBrands(true)
      return
    }
    const items = browse.brands.items
    const idx = items.findIndex((b) => b.slug === detail.cardID)
    if (idx === -1) return
    const updated = [...items]
    const [moved] = updated.splice(idx, 1)
    const toIdx = Math.max(0, Math.min(detail.toPosition, updated.length))
    updated.splice(toIdx, 0, moved)
    browse.brands.items = updated
    try {
      await repoRPC('ReorderBrands', [updated.map((b) => b.slug)])
    } catch (err) {
      console.error('reorder brands failed:', err)
      void loadBrands(true)
    }
  }

  async function handleStreamDrop(srcBrand: Brand, detail: DragMoveDetail) {
    const dstBrandSlug = detail.toTarget.getAttribute('data-brand-slug') ?? srcBrand.slug
    const sameBrand = dstBrandSlug === srcBrand.slug

    if (!sameBrand && detail.isCopy) {
      // Copy: can't optimistically insert (no client-known ID). Trust
      // the silent refresh — items stay rendered while the new copy
      // arrives in the destination cache.
      try {
        await repoRPC('CopyStream', [srcBrand.slug, detail.cardID, dstBrandSlug])
      } catch (err) {
        console.error('copy stream failed:', err)
        showToast(err instanceof Error ? err.message : t('error.network'), 'error')
      }
      void loadStreams(srcBrand.slug, true)
      void loadStreams(dstBrandSlug, true)
      return
    }
    if (!sameBrand) {
      // Move: optimistically remove from source, append to destination.
      // Both caches stay populated through the refresh thanks to the
      // silent-refresh change in browse.svelte.ts.
      const srcCache = browse.streamsFor(srcBrand.slug)
      const dstCache = browse.streamsFor(dstBrandSlug)
      const item = srcCache?.items.find((s) => s.slug === detail.cardID)
      if (srcCache && dstCache && item) {
        srcCache.items = srcCache.items.filter((s) => s.slug !== detail.cardID)
        dstCache.items = [...dstCache.items, item]
      }
      try {
        await repoRPC('MoveStream', [srcBrand.slug, detail.cardID, dstBrandSlug])
      } catch (err) {
        console.error('move stream failed:', err)
        showToast(err instanceof Error ? err.message : t('error.network'), 'error')
      }
      void loadStreams(srcBrand.slug, true)
      void loadStreams(dstBrandSlug, true)
      return
    }

    // Same-brand reorder.
    const cache = browse.streamsFor(srcBrand.slug)
    if (!cache || cache.state !== 'loaded') return
    const items = cache.items
    const idx = items.findIndex((s) => s.slug === detail.cardID)
    if (idx === -1) return
    const updated = [...items]
    const [moved] = updated.splice(idx, 1)
    const toIdx = Math.max(0, Math.min(detail.toPosition, updated.length))
    updated.splice(toIdx, 0, moved)
    cache.items = updated
    try {
      await repoRPC('ReorderStreams', [srcBrand.slug, updated.map((s) => s.slug)])
    } catch (err) {
      console.error('reorder streams failed:', err)
      void loadStreams(srcBrand.slug, true)
    }
  }

  // Hover-expand for the tree: when a drag stalls over a collapsed
  // parent row, expand it after 500ms so its child list becomes a
  // valid drop target. Lets the user drop a stream into a collapsed
  // brand, or a project into a collapsed stream, without manually
  // expanding first. Target reads its own data-expand-target kind to
  // dispatch — same callback handles brand and stream expansions.
  function handleTreeHoverExpand(target: HTMLElement) {
    const kind = target.getAttribute('data-expand-target')
    if (kind === 'brand') {
      const slug = target.getAttribute('data-brand-slug')
      const brand = slug ? browse.brands.items.find((b) => b.slug === slug) : null
      if (brand && !expandedBrands[brand.slug]) toggleBrand(brand)
      return
    }
    if (kind === 'stream') {
      const streamSlug = target.getAttribute('data-stream-slug')
      const parentList = target.closest('[data-drop-target="stream-list"]') as HTMLElement | null
      const brandSlug = parentList?.getAttribute('data-brand-slug') ?? null
      if (!streamSlug || !brandSlug) return
      const brand = browse.brands.items.find((b) => b.slug === brandSlug)
      const cache = browse.streamsFor(brandSlug)
      const stream = cache?.items.find((s) => s.slug === streamSlug)
      const key = `${brandSlug}/${streamSlug}`
      if (brand && stream && !expandedStreams[key]) toggleStream(brand, stream)
    }
  }

  async function handleProjectDrop(srcBrand: Brand, srcStream: Stream, detail: DragMoveDetail) {
    const dstBrandSlug = detail.toTarget.getAttribute('data-brand-slug') ?? srcBrand.slug
    const dstStreamSlug = detail.toTarget.getAttribute('data-stream-slug') ?? srcStream.slug
    const sameStream = dstBrandSlug === srcBrand.slug && dstStreamSlug === srcStream.slug

    if (!sameStream && detail.isCopy) {
      try {
        await repoRPC('CopyProject', [
          srcBrand.slug, srcStream.slug, detail.cardID,
          dstBrandSlug, dstStreamSlug, detail.toPosition,
        ])
      } catch (err) {
        console.error('copy project failed:', err)
        showToast(err instanceof Error ? err.message : t('error.network'), 'error')
      }
      void loadProjects(srcBrand.slug, srcStream.slug, true)
      void loadProjects(dstBrandSlug, dstStreamSlug, true)
      return
    }
    if (!sameStream) {
      // Optimistic move: pluck from source, insert into dest at the
      // user's drop position. Caches stay populated through the silent
      // refresh that follows.
      const srcCache = browse.projectsFor(srcBrand.slug, srcStream.slug)
      const dstCache = browse.projectsFor(dstBrandSlug, dstStreamSlug)
      const item = srcCache?.items.find((p) => p.slug === detail.cardID)
      if (srcCache && dstCache && item) {
        srcCache.items = srcCache.items.filter((p) => p.slug !== detail.cardID)
        const toIdx = Math.max(0, Math.min(detail.toPosition, dstCache.items.length))
        dstCache.items = [...dstCache.items.slice(0, toIdx), item, ...dstCache.items.slice(toIdx)]
      }
      try {
        await repoRPC('MoveProject', [
          srcBrand.slug, srcStream.slug, detail.cardID,
          dstBrandSlug, dstStreamSlug,
        ])
      } catch (err) {
        console.error('move project failed:', err)
        showToast(err instanceof Error ? err.message : t('error.network'), 'error')
      }
      void loadProjects(srcBrand.slug, srcStream.slug, true)
      void loadProjects(dstBrandSlug, dstStreamSlug, true)
      return
    }

    // Same-stream reorder.
    const cache = browse.projectsFor(srcBrand.slug, srcStream.slug)
    if (!cache || cache.state !== 'loaded') return
    const items = cache.items
    const idx = items.findIndex((p) => p.slug === detail.cardID)
    if (idx === -1) return
    const updated = [...items]
    const [moved] = updated.splice(idx, 1)
    const toIdx = Math.max(0, Math.min(detail.toPosition, updated.length))
    updated.splice(toIdx, 0, moved)
    cache.items = updated
    try {
      await repoRPC('ReorderProjects', [srcBrand.slug, srcStream.slug, updated.map((p) => p.slug)])
    } catch (err) {
      console.error('reorder projects failed:', err)
      void loadProjects(srcBrand.slug, srcStream.slug, true)
    }
  }
</script>

<svelte:window onpointerdown={onWindowPointerDown} />

<header class="topbar">
  <button type="button" class="vault-button" onclick={() => navigate('/repos')} title={t('browse.switch_vault')}>
    <span class="vault-name">{activeRepoName ?? t('common.loading')}</span>
    <span class="vault-arrow">›</span>
  </button>
  <div class="topbar-actions">
    <button type="button" class="icon-btn" onclick={() => (searchOpen = true)} aria-label={t('browse.search')} title={t('browse.search')}>
      <Search size={18} />
    </button>
    <button
      type="button"
      class="icon-btn bell"
      onclick={() => (notificationsOpen = true)}
      aria-label={t('browse.notifications')}
      title={t('browse.notifications')}
    >
      <Bell size={18} />
      {#if unreadCount > 0}
        <span class="badge" aria-label={String(unreadCount)}>{unreadCount > 99 ? '99+' : unreadCount}</span>
      {/if}
    </button>
    <button type="button" class="icon-btn" onclick={() => navigate('/settings')} aria-label={t('browse.settings')} title={t('browse.settings')}>
      <Settings size={18} />
    </button>
  </div>
</header>

<main>
  <button type="button" class="inbox-tile" onclick={() => navigate('/inbox')}>
    <span class="inbox-icon" aria-hidden="true"><Inbox size={22} /></span>
    <div class="tile-text">
      <span class="tile-title">{t('browse.inbox')}</span>
      <span class="tile-sub">{t('browse.inbox_sub')}</span>
    </div>
  </button>

  <div class="brands-header">
    <h2 class="section brands-section">{t('browse.brands')}</h2>
    <div class="acc-toolbar" role="toolbar" aria-label={t('browse.accordion_toolbar')}>
      {#if browse.brands.items.length > 0}
        <button type="button" class="acc-btn" onclick={expandAllInTree} aria-label={t('project.expand_all')} title={t('project.expand_all')}>
          <ChevronsUpDown size={14} />
        </button>
        <button type="button" class="acc-btn" onclick={collapseAllInTree} aria-label={t('project.collapse_all')} title={t('project.collapse_all')}>
          <ChevronsDownUp size={14} />
        </button>
        <button
          type="button"
          class="acc-btn"
          onclick={toggleAccordionMode}
          aria-label={browse.accordionMode === 'single' ? t('project.mode_single') : t('project.mode_multi')}
          title={browse.accordionMode === 'single' ? t('project.mode_single_hint') : t('project.mode_multi_hint')}
        >
          {#if browse.accordionMode === 'single'}
            <ListCollapse size={14} />
          {:else}
            <ListTree size={14} />
          {/if}
        </button>
      {/if}
      <button
        type="button"
        class="acc-btn add-brand-btn"
        onclick={handleAddBrand}
        aria-label={t('browse.add_brand')}
        title={t('browse.add_brand')}
      >
        <Plus size={14} />
      </button>
    </div>
  </div>

  {#if mutationError}
    <p class="error tree-error" role="alert">{mutationError}</p>
  {/if}

  {#if browse.brands.state === 'loading'}
    <p class="status">{t('common.loading')}</p>
  {:else if browse.brands.state === 'error'}
    <ErrorState message={browse.brands.error ?? t('common.error_generic')} />
  {:else if browse.brands.items.length === 0}
    <p class="status">{t('browse.empty')}</p>
  {:else}
    <ul
      class="tree"
      data-drop-target="brand-list"
      use:dragSortable={{
        onMove: handleBrandDrop,
        rowSelector: '[data-brand-slug]',
        dropTargetSelector: '[data-drop-target="brand-list"]',
        rowIdAttribute: 'data-brand-slug',
      }}
    >
      {#each browse.brands.items as brand (brand.id)}
        <li
          class="brand"
          data-brand-slug={brand.slug}
          data-expand-target="brand"
          data-collapsed={expandedBrands[brand.slug] ? null : 'true'}
        >
          {#if renamingKeyValue === `brand:${brand.slug}`}
            <div class="row brand-row renaming">
              <span class="caret row-caret" class:open={expandedBrands[brand.slug]} aria-hidden="true">▸</span>
              {#if brand.icon}
                <DynamicIcon name={brand.icon} size={18} />
              {/if}
              <input
                bind:this={renameInputEl}
                bind:value={renameDraft}
                use:inlineEdit={{ onCommit: () => void commitRename(), onCancel: cancelRename }}
                class="rename-input"
                aria-label={t('browse.rename_brand')}
                placeholder={t('browse.rename_brand')}
                enterkeyhint="done"
                disabled={renameBusy}
              />
            </div>
          {:else}
            <div class="row brand-row">
              <button type="button" class="row-main" onclick={() => toggleBrand(brand)}>
                <span class="caret row-caret" class:open={expandedBrands[brand.slug]} aria-hidden="true">▸</span>
                {#if brand.icon}
                  <DynamicIcon name={brand.icon} size={18} />
                {/if}
                <span class="row-name">{@html renderInline(brand.name)}</span>
              </button>
              <button
                type="button"
                class="row-kebab"
                onclick={(e) => { e.stopPropagation(); toggleMenu({ kind: 'brand', brandSlug: brand.slug }) }}
                aria-label={t('browse.row_actions', { name: brand.name })}
                aria-expanded={openMenuKey === `brand:${brand.slug}`}
                aria-haspopup="menu"
              >
                <MoreVertical size={16} />
              </button>
            </div>
            {#if openMenuKey === `brand:${brand.slug}`}
              <div class="row-menu" role="menu">
                <button type="button" role="menuitem" class="row-menu-item" onclick={() => startRename({ kind: 'brand', brandSlug: brand.slug }, brand.name)}>
                  <Pencil size={14} /> {t('browse.action_rename')}
                </button>
                <button type="button" role="menuitem" class="row-menu-item danger" onclick={() => requestDelete({ kind: 'brand', brandSlug: brand.slug })}>
                  <Trash2 size={14} /> {t('browse.action_delete')}
                </button>
              </div>
            {/if}
          {/if}

          {#if expandedBrands[brand.slug]}
            {@const streams = browse.streamsFor(brand.slug)}
            {#if !streams || (streams.state === 'loading' && streams.items.length === 0)}
              <p class="indent status">{t('common.loading')}</p>
            {:else if streams.state === 'error' && streams.items.length === 0}
              <div class="indent">
                <ErrorState compact message={streams.error ?? t('common.error_generic')} />
              </div>
            {:else}
              <ul
                class="streams"
                data-drop-target="stream-list"
                data-brand-slug={brand.slug}
                use:dragSortable={{
                  onMove: (d) => handleStreamDrop(brand, d),
                  onHoverExpand: handleTreeHoverExpand,
                  rowSelector: '[data-stream-slug]',
                  dropTargetSelector: '[data-drop-target="stream-list"]',
                  rowIdAttribute: 'data-stream-slug',
                  expandOnHoverSelector: '[data-expand-target="brand"]',
                }}
              >
                {#if streams.items.length === 0}
                  <li class="empty-hint indent status">{t('browse.empty_stream')}</li>
                {/if}
                {#each streams.items as stream (stream.id)}
                  {@const streamKey = `${brand.slug}/${stream.slug}`}
                  <li
                    class="stream"
                    data-stream-slug={stream.slug}
                    data-expand-target="stream"
                    data-collapsed={expandedStreams[streamKey] ? null : 'true'}
                  >
                    {#if renamingKeyValue === `stream:${streamKey}`}
                      <div class="row stream-row renaming">
                        <span class="caret row-caret" class:open={expandedStreams[streamKey]} aria-hidden="true">▸</span>
                        {#if stream.icon}
                          <DynamicIcon name={stream.icon} size={16} />
                        {/if}
                        <input
                          bind:this={renameInputEl}
                          bind:value={renameDraft}
                          use:inlineEdit={{ onCommit: () => void commitRename(), onCancel: cancelRename }}
                          class="rename-input"
                          aria-label={t('browse.rename_stream')}
                          placeholder={t('browse.rename_stream')}
                          enterkeyhint="done"
                          disabled={renameBusy}
                        />
                      </div>
                    {:else}
                      <div class="row stream-row">
                        <button type="button" class="row-main" onclick={() => toggleStream(brand, stream)}>
                          <span class="caret row-caret" class:open={expandedStreams[streamKey]} aria-hidden="true">▸</span>
                          {#if stream.icon}
                            <DynamicIcon name={stream.icon} size={16} />
                          {/if}
                          <span class="row-name">{@html renderInline(stream.name)}</span>
                        </button>
                        <button
                          type="button"
                          class="row-kebab"
                          onclick={(e) => { e.stopPropagation(); toggleMenu({ kind: 'stream', brandSlug: brand.slug, streamSlug: stream.slug }) }}
                          aria-label={t('browse.row_actions', { name: stream.name })}
                          aria-expanded={openMenuKey === `stream:${streamKey}`}
                          aria-haspopup="menu"
                        >
                          <MoreVertical size={16} />
                        </button>
                      </div>
                      {#if openMenuKey === `stream:${streamKey}`}
                        <div class="row-menu" role="menu">
                          <button type="button" role="menuitem" class="row-menu-item" onclick={() => startRename({ kind: 'stream', brandSlug: brand.slug, streamSlug: stream.slug }, stream.name)}>
                            <Pencil size={14} /> {t('browse.action_rename')}
                          </button>
                          <button type="button" role="menuitem" class="row-menu-item danger" onclick={() => requestDelete({ kind: 'stream', brandSlug: brand.slug, streamSlug: stream.slug })}>
                            <Trash2 size={14} /> {t('browse.action_delete')}
                          </button>
                        </div>
                      {/if}
                    {/if}

                    {#if expandedStreams[streamKey]}
                      {@const projects = browse.projectsFor(brand.slug, stream.slug)}
                      {#if !projects || (projects.state === 'loading' && projects.items.length === 0)}
                        <p class="indent status">{t('common.loading')}</p>
                      {:else if projects.state === 'error' && projects.items.length === 0}
                        <div class="indent">
                          <ErrorState compact message={projects.error ?? t('common.error_generic')} />
                        </div>
                      {:else}
                        <ul
                          class="projects"
                          data-drop-target="project-list"
                          data-brand-slug={brand.slug}
                          data-stream-slug={stream.slug}
                          use:dragSortable={{
                            onMove: (d) => handleProjectDrop(brand, stream, d),
                            onHoverExpand: handleTreeHoverExpand,
                            rowSelector: '[data-project-slug]',
                            dropTargetSelector: '[data-drop-target="project-list"]',
                            rowIdAttribute: 'data-project-slug',
                            expandOnHoverSelector: '[data-expand-target]',
                          }}
                        >
                          {#if projects.items.length === 0}
                            <li class="empty-hint indent status">{t('browse.empty_project')}</li>
                          {/if}
                          {#each projects.items as project (project.id)}
                            {@const projectKey = `${brand.slug}/${stream.slug}/${project.slug}`}
                            <li data-project-slug={project.slug}>
                              {#if renamingKeyValue === `project:${projectKey}`}
                                <div class="row project-row renaming">
                                  {#if project.icon}
                                    <DynamicIcon name={project.icon} size={16} />
                                  {/if}
                                  <input
                                    bind:this={renameInputEl}
                                    bind:value={renameDraft}
                                    use:inlineEdit={{ onCommit: () => void commitRename(), onCancel: cancelRename }}
                                    class="rename-input"
                                    aria-label={t('browse.rename_project')}
                                    placeholder={t('browse.rename_project')}
                                    enterkeyhint="done"
                                    disabled={renameBusy}
                                  />
                                </div>
                              {:else}
                                <div class="row project-row">
                                  <button
                                    type="button"
                                    class="row-main"
                                    onclick={() => navigate(projectURL(brand.slug, stream.slug, project.slug))}
                                  >
                                    {#if project.icon}
                                      <DynamicIcon name={project.icon} size={16} />
                                    {/if}
                                    <span class="row-name">{@html renderInline(project.name)}</span>
                                    <span class="row-arrow" aria-hidden="true">›</span>
                                  </button>
                                  <button
                                    type="button"
                                    class="row-kebab"
                                    onclick={(e) => { e.stopPropagation(); toggleMenu({ kind: 'project', brandSlug: brand.slug, streamSlug: stream.slug, projectSlug: project.slug }) }}
                                    aria-label={t('browse.row_actions', { name: project.name })}
                                    aria-expanded={openMenuKey === `project:${projectKey}`}
                                    aria-haspopup="menu"
                                  >
                                    <MoreVertical size={16} />
                                  </button>
                                </div>
                                {#if openMenuKey === `project:${projectKey}`}
                                  <div class="row-menu" role="menu">
                                    <button type="button" role="menuitem" class="row-menu-item" onclick={() => startRename({ kind: 'project', brandSlug: brand.slug, streamSlug: stream.slug, projectSlug: project.slug }, project.name)}>
                                      <Pencil size={14} /> {t('browse.action_rename')}
                                    </button>
                                    <button type="button" role="menuitem" class="row-menu-item danger" onclick={() => requestDelete({ kind: 'project', brandSlug: brand.slug, streamSlug: stream.slug, projectSlug: project.slug })}>
                                      <Trash2 size={14} /> {t('browse.action_delete')}
                                    </button>
                                  </div>
                                {/if}
                              {/if}
                            </li>
                          {/each}
                          <li class="add-row">
                            <button
                              type="button"
                              class="row add-row-btn"
                              onclick={() => handleAddProject(brand, stream)}
                              aria-label={t('browse.add_project')}
                            >
                              <span class="add-icon" aria-hidden="true"><Plus size={14} /></span>
                              <span class="row-name">{t('browse.add_project')}</span>
                            </button>
                          </li>
                        </ul>
                      {/if}
                    {/if}
                  </li>
                {/each}
                <li class="add-row">
                  <button
                    type="button"
                    class="row add-row-btn"
                    onclick={() => handleAddStream(brand)}
                    aria-label={t('browse.add_stream')}
                  >
                    <span class="add-icon" aria-hidden="true"><Plus size={14} /></span>
                    <span class="row-name">{t('browse.add_stream')}</span>
                  </button>
                </li>
              </ul>
            {/if}
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</main>

{#if searchOpen}
  <SearchSheet onClose={() => (searchOpen = false)} />
{/if}

{#if notificationsOpen}
  <NotificationsPanel onClose={closeNotifications} />
{/if}

{#if pendingDelete}
  <ConfirmDialog
    title={deleteDialogTitle(pendingDelete)}
    body={deleteDialogBody(pendingDelete)}
    confirmLabel={t('browse.action_delete')}
    destructive
    onConfirm={confirmDelete}
    onCancel={() => (pendingDelete = null)}
  />
{/if}

<style>
  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.65rem 0.65rem 0.65rem 1rem;
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    background: var(--bg);
    z-index: 10;
  }

  .topbar-actions {
    display: flex;
    align-items: center;
    gap: 0.15rem;
  }
  .icon-btn {
    position: relative;
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
  .icon-btn:hover,
  .icon-btn:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }
  .badge {
    position: absolute;
    top: 4px;
    right: 4px;
    min-width: 18px;
    height: 18px;
    padding: 0 5px;
    border-radius: 999px;
    background: var(--accent);
    color: #18181b;
    font-size: 0.65rem;
    font-weight: 700;
    line-height: 18px;
    text-align: center;
    box-sizing: border-box;
  }

  .vault-button {
    background: transparent;
    border: none;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.35rem 0.5rem;
    border-radius: 6px;
    flex: 1;
    min-width: 0;
  }

  .vault-button:hover {
    color: var(--text);
    background: var(--bg-elev-1);
  }

  .vault-name {
    max-width: 60vw;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .vault-arrow {
    color: var(--text-faint);
  }

  main {
    padding: 1rem 0.85rem 4rem;
    max-width: 600px;
    margin: 0 auto;
  }

  .inbox-tile {
    width: 100%;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 1rem 1.1rem;
    color: var(--text);
    font: inherit;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 0.85rem;
    text-align: left;
    transition: border-color 120ms ease;
  }

  .inbox-tile:hover,
  .inbox-tile:focus-visible {
    border-color: var(--accent);
    outline: none;
  }

  .inbox-icon {
    color: var(--accent);
    display: inline-flex;
    align-items: center;
  }

  .tile-text {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
  }

  .tile-title {
    font-weight: 600;
    font-size: 1rem;
  }

  .tile-sub {
    font-size: 0.8rem;
    color: var(--text-muted);
    margin-top: 0.15rem;
  }

  .section {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-faint);
    margin: 1.75rem 0.25rem 0.75rem;
  }

  .brands-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: 1.75rem;
    margin-bottom: 0.75rem;
    padding: 0 0.25rem;
  }
  .brands-section {
    margin: 0;
  }
  .acc-toolbar {
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
  }
  .acc-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: 1px solid var(--border);
    color: var(--text-muted);
    border-radius: 6px;
    width: 32px;
    height: 32px;
    cursor: pointer;
    padding: 0;
  }
  .acc-btn:hover,
  .acc-btn:focus-visible {
    color: var(--text);
    border-color: var(--text-muted);
    background: var(--bg-elev-1);
    outline: none;
  }


  .tree,
  .streams,
  .projects {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .streams,
  .projects {
    margin-left: 0.65rem;
    padding-left: 0.65rem;
    border-left: 1px solid var(--border);
    margin-top: 0.25rem;
  }

  .row {
    display: flex;
    align-items: stretch;
    width: 100%;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 8px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    /* Allow vertical scroll on rows; long-press still wins for drag.
       See CardRow.svelte for the rationale and the move-cancel
       threshold tuning that makes the two gestures coexist. */
    touch-action: pan-y;
    -webkit-user-select: none;
    user-select: none;
    -webkit-touch-callout: none;
  }
  .row.renaming {
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 0.75rem;
    border-color: var(--accent);
    background: var(--bg-elev-1);
  }
  .row-main {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    background: transparent;
    border: 0;
    color: inherit;
    font: inherit;
    font-size: inherit;
    cursor: pointer;
    text-align: left;
    padding: 0.65rem 0.5rem 0.65rem 0.75rem;
    border-top-left-radius: 7px;
    border-bottom-left-radius: 7px;
  }
  .row-kebab {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: 0;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0 0.65rem;
    min-width: 36px;
    border-top-right-radius: 7px;
    border-bottom-right-radius: 7px;
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
    margin: 0.15rem 0 0.5rem 1.85rem;
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
  .row-menu-item:hover,
  .row-menu-item:focus-visible {
    background: var(--bg);
    outline: none;
  }
  .row-menu-item.danger {
    color: #fca5a5;
  }
  .row-menu-item.danger:hover,
  .row-menu-item.danger:focus-visible {
    background: rgba(239, 68, 68, 0.12);
  }
  .add-row {
    list-style: none;
  }
  .add-row-btn {
    color: var(--text-muted);
    font: inherit;
    font-size: 0.85rem;
    padding: 0.5rem 0.75rem;
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
  .add-row-btn:hover,
  .add-row-btn:focus-visible {
    color: var(--text);
    border-color: var(--text-muted);
    background: var(--bg-elev-1);
    outline: none;
  }
  .add-icon {
    display: inline-flex;
    align-items: center;
    color: var(--accent);
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
  .add-brand-btn {
    color: var(--accent);
  }
  .add-brand-btn:hover,
  .add-brand-btn:focus-visible {
    color: var(--text);
    border-color: var(--accent);
    background: var(--bg-elev-1);
  }
  .tree-error {
    margin: 0.25rem 0.25rem 0.5rem;
    padding: 0.5rem 0.75rem;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 8px;
    color: #fca5a5;
    font-size: 0.85rem;
  }

  /* DnD visual states (driven by dnd.svelte.ts adding/removing classes) */
  :global(.tree .dnd-source),
  :global(.streams .dnd-source),
  :global(.projects .dnd-source) {
    opacity: 0.35;
    transition: opacity 120ms ease;
  }

  /* Empty-list hint inside an otherwise-empty drop-target <ul>. Gives
     the <ul> hit-testable height so the user can drop into a brand
     with no streams, or a stream with no projects, and have it land
     correctly. Non-interactive: no data-*-slug, so the action skips
     it for row purposes. */
  .empty-hint {
    list-style: none;
    pointer-events: none;
  }

  .row:has(.row-main:hover),
  .row:has(.row-main:focus-visible) {
    background: var(--bg-elev-1);
    border-color: var(--border);
  }
  .row-main:focus-visible {
    outline: none;
  }

  .row-name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .row-arrow {
    color: var(--text-faint);
    font-size: 1.1rem;
  }

  .caret {
    color: var(--text-muted);
    font-size: 0.7rem;
    width: 0.7rem;
    text-align: center;
    transition: transform 120ms ease;
  }

  .caret.open {
    transform: rotate(90deg);
  }

  .brand-row {
    font-weight: 600;
  }

  .stream-row {
    font-weight: 500;
    font-size: 0.9rem;
  }

  .project-row {
    font-size: 0.9rem;
  }

  .indent {
    margin: 0.25rem 0 0.5rem 1.4rem;
    font-size: 0.85rem;
  }

  .status {
    color: var(--text-muted);
    margin: 0.5rem 0.25rem;
    font-size: 0.85rem;
  }

  .error {
    color: #fca5a5;
    margin: 0.5rem 0.25rem;
    font-size: 0.85rem;
  }
</style>
