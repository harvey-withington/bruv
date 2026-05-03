<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { Inbox, Search, Bell, Settings, ChevronsUpDown, ChevronsDownUp, ListCollapse, ListTree } from 'lucide-svelte'
  import {
    browse,
    loadBrands,
    loadStreams,
    loadProjects,
    setAccordionMode,
    toggleBrandExpansion,
    toggleStreamExpansion,
    expandAllBrandsTree,
    collapseAllBrandsTree,
  } from '../lib/browse.svelte'
  import { navigate, projectURL } from '../lib/router.svelte'
  import { readActiveRepoID, apiFetch, repoRPC, machineRPC } from '../lib/auth'
  import { onEvent } from '../lib/events.svelte'
  import { t } from '../lib/i18n.svelte'
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
    setAccordionMode(browse.accordionMode === 'single' ? 'multi' : 'single')
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
    {#if browse.brands.items.length > 0}
      <div class="acc-toolbar" role="toolbar" aria-label={t('browse.accordion_toolbar')}>
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
      </div>
    {/if}
  </div>

  {#if browse.brands.state === 'loading'}
    <p class="status">{t('common.loading')}</p>
  {:else if browse.brands.state === 'error'}
    <p class="error">{browse.brands.error}</p>
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
          <button type="button" class="row brand-row" onclick={() => toggleBrand(brand)}>
            <span class="caret" class:open={expandedBrands[brand.slug]} aria-hidden="true">▸</span>
            {#if brand.icon}
              <DynamicIcon name={brand.icon} size={18} />
            {/if}
            <span class="row-name">{brand.name}</span>
          </button>

          {#if expandedBrands[brand.slug]}
            {@const streams = browse.streamsFor(brand.slug)}
            {#if !streams || (streams.state === 'loading' && streams.items.length === 0)}
              <p class="indent status">{t('common.loading')}</p>
            {:else if streams.state === 'error' && streams.items.length === 0}
              <p class="indent error">{streams.error}</p>
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
                    <button type="button" class="row stream-row" onclick={() => toggleStream(brand, stream)}>
                      <span class="caret" class:open={expandedStreams[streamKey]} aria-hidden="true">▸</span>
                      {#if stream.icon}
                        <DynamicIcon name={stream.icon} size={16} />
                      {/if}
                      <span class="row-name">{stream.name}</span>
                    </button>

                    {#if expandedStreams[streamKey]}
                      {@const projects = browse.projectsFor(brand.slug, stream.slug)}
                      {#if !projects || (projects.state === 'loading' && projects.items.length === 0)}
                        <p class="indent status">{t('common.loading')}</p>
                      {:else if projects.state === 'error' && projects.items.length === 0}
                        <p class="indent error">{projects.error}</p>
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
                            <li data-project-slug={project.slug}>
                              <button
                                type="button"
                                class="row project-row"
                                onclick={() => navigate(projectURL(brand.slug, stream.slug, project.slug))}
                              >
                                {#if project.icon}
                                  <DynamicIcon name={project.icon} size={16} />
                                {/if}
                                <span class="row-name">{project.name}</span>
                                <span class="row-arrow" aria-hidden="true">›</span>
                              </button>
                            </li>
                          {/each}
                        </ul>
                      {/if}
                    {/if}
                  </li>
                {/each}
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
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 8px;
    padding: 0.65rem 0.75rem;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    cursor: pointer;
    text-align: left;
    /* Allow vertical scroll on rows; long-press still wins for drag.
       See CardRow.svelte for the rationale and the move-cancel
       threshold tuning that makes the two gestures coexist. */
    touch-action: pan-y;
    -webkit-user-select: none;
    user-select: none;
    -webkit-touch-callout: none;
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

  .row:hover,
  .row:focus-visible {
    background: var(--bg-elev-1);
    border-color: var(--border);
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
