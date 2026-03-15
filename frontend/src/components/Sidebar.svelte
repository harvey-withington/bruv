<script lang="ts">
  import { nav, board, tagColors } from '../lib/store.svelte'
  import { CloseRepository, CreateBrand, CreateStream, CreateProject, DeleteBrand, DeleteStream, DeleteProject, ListBrands, ListStreams, ListProjects, ListCategories, GetCard, GetCardPins, ListCardIDsInCategory, GetTagColors } from '../lib/api'
  import { LogOut, Trash2, ChevronRight, ChevronDown, ChevronLeft, Plus, X, PanelLeftClose, PanelLeftOpen } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'

  async function handleCloseRepo() {
    await CloseRepository()
    nav.repoOpen = false
    nav.repoPath = ''
    nav.brandSlug = null
    nav.streamSlug = null
    nav.projectSlug = null
    localStorage.removeItem('bruv-last-nav')
    board.categories = []
    brands = []
    streamsByBrand = {}
    projectsByStream = {}
    expandedBrands = new Set()
    expandedStreams = new Set()
  }

  type Brand = { id: string; name: string; slug: string }
  type Stream = { id: string; name: string; slug: string }
  type Project = { id: string; name: string; slug: string }

  let brands = $state<Brand[]>([])
  let expandedBrands = $state<Set<string>>(new Set())
  let streamsByBrand = $state<Record<string, Stream[]>>({})
  let expandedStreams = $state<Set<string>>(new Set())
  let projectsByStream = $state<Record<string, Project[]>>({})

  // Inline creation state
  let addingBrand = $state(false)
  let newBrandName = $state('')
  let addingStreamFor = $state<string | null>(null)
  let newStreamName = $state('')
  let addingProjectFor = $state<string | null>(null)
  let newProjectName = $state('')

  $effect(() => {
    if (nav.repoOpen) {
      loadBrandsAndRestore()
    }
  })

  async function loadBrandsAndRestore() {
    await loadBrands()
    // Restore last nav state if available
    try {
      const raw = localStorage.getItem('bruv-last-nav')
      if (!raw) return
      const last = JSON.parse(raw) as { brandSlug: string; streamSlug: string; projectSlug: string }
      if (!last.brandSlug || !last.streamSlug || !last.projectSlug) return
      // Expand brand
      expandedBrands.add(last.brandSlug)
      expandedBrands = new Set(expandedBrands)
      streamsByBrand[last.brandSlug] = await ListStreams(last.brandSlug) || []
      // Expand stream
      const streamKey = `${last.brandSlug}/${last.streamSlug}`
      expandedStreams.add(streamKey)
      expandedStreams = new Set(expandedStreams)
      projectsByStream[streamKey] = await ListProjects(last.brandSlug, last.streamSlug) || []
      // Select the project and load board
      await selectProject(last.brandSlug, last.streamSlug, last.projectSlug)
    } catch { /* ignore — just show sidebar without selection */ }
  }

  async function loadBrands() {
    try {
      brands = await ListBrands() || []
    } catch { brands = [] }
  }

  async function toggleBrand(slug: string) {
    if (expandedBrands.has(slug)) {
      expandedBrands.delete(slug)
      expandedBrands = new Set(expandedBrands)
    } else {
      expandedBrands.add(slug)
      expandedBrands = new Set(expandedBrands)
      if (!streamsByBrand[slug]) {
        try {
          streamsByBrand[slug] = await ListStreams(slug) || []
        } catch { streamsByBrand[slug] = [] }
      }
    }
  }

  async function toggleStream(brandSlug: string, streamSlug: string) {
    const key = `${brandSlug}/${streamSlug}`
    if (expandedStreams.has(key)) {
      expandedStreams.delete(key)
      expandedStreams = new Set(expandedStreams)
    } else {
      expandedStreams.add(key)
      expandedStreams = new Set(expandedStreams)
      if (!projectsByStream[key]) {
        try {
          projectsByStream[key] = await ListProjects(brandSlug, streamSlug) || []
        } catch { projectsByStream[key] = [] }
      }
    }
  }

  async function selectProject(brandSlug: string, streamSlug: string, projectSlug: string) {
    nav.brandSlug = brandSlug
    nav.streamSlug = streamSlug
    nav.projectSlug = projectSlug
    localStorage.setItem('bruv-last-nav', JSON.stringify({ brandSlug, streamSlug, projectSlug }))
    await loadBoard(brandSlug, streamSlug, projectSlug)
  }

  async function loadBoard(brandSlug: string, streamSlug: string, projectSlug: string) {
    board.loading = true
    try {
      // Load tag colors so labels render correctly
      try { tagColors.map = await GetTagColors() || {} } catch { /* ignore */ }

      const cats = await ListCategories(brandSlug, streamSlug, projectSlug) || []
      const populated = await Promise.all(cats.map(async (cat: any) => {
        let cardIds: string[] = []
        try {
          cardIds = await ListCardIDsInCategory(cat.id, cat.id) || []
        } catch { /* no cards pinned yet */ }

        const cards = await Promise.all(cardIds.map(async (id: string) => {
          try {
            const card = await GetCard(id)
            return {
              id: card.id,
              type: card.type,
              title: card.title,
              tags: card.tags || [],
              due_date: card.due_date,
              checklist_total: card.checklist?.length || 0,
              checklist_done: card.checklist?.filter((c: any) => c.done).length || 0,
            }
          } catch { return null }
        }))

        return {
          id: cat.id,
          name: cat.name,
          slug: cat.slug,
          position: cat.position,
          cards: cards.filter((c): c is NonNullable<typeof c> => c !== null),
        }
      }))
      board.categories = populated
    } catch {
      board.categories = []
    }
    board.loading = false
  }

  export function refreshBoard() {
    if (nav.brandSlug && nav.streamSlug && nav.projectSlug) {
      loadBoard(nav.brandSlug, nav.streamSlug, nav.projectSlug)
    }
  }

  export function refreshSidebar() {
    loadBrands()
    streamsByBrand = {}
    projectsByStream = {}
    expandedBrands = new Set()
    expandedStreams = new Set()
  }

  function isSelected(brandSlug: string, streamSlug: string, projectSlug: string) {
    return nav.brandSlug === brandSlug && nav.streamSlug === streamSlug && nav.projectSlug === projectSlug
  }

  // --- Create handlers ---

  async function handleCreateBrand() {
    if (!newBrandName.trim()) return
    try {
      await CreateBrand(newBrandName.trim())
      newBrandName = ''
      addingBrand = false
      await loadBrands()
    } catch (e) { console.error('CreateBrand:', e) }
  }

  async function handleCreateStream(brandSlug: string) {
    if (!newStreamName.trim()) return
    try {
      await CreateStream(brandSlug, newStreamName.trim())
      newStreamName = ''
      addingStreamFor = null
      streamsByBrand[brandSlug] = await ListStreams(brandSlug) || []
    } catch (e) { console.error('CreateStream:', e) }
  }

  async function handleCreateProject(brandSlug: string, streamSlug: string) {
    if (!newProjectName.trim()) return
    try {
      await CreateProject(brandSlug, streamSlug, newProjectName.trim())
      newProjectName = ''
      addingProjectFor = null
      const key = `${brandSlug}/${streamSlug}`
      projectsByStream[key] = await ListProjects(brandSlug, streamSlug) || []
    } catch (e) { console.error('CreateProject:', e) }
  }

  // --- Delete handlers ---

  async function handleDeleteBrand(e: MouseEvent, slug: string) {
    e.stopPropagation()
    if (!confirm(`Delete brand "${slug}" and all its streams/projects?`)) return
    try {
      await DeleteBrand(slug)
      // Clear nav if we were viewing something inside this brand
      if (nav.brandSlug === slug) {
        nav.brandSlug = null
        nav.streamSlug = null
        nav.projectSlug = null
        board.categories = []
      }
      await loadBrands()
    } catch (e) { console.error('DeleteBrand:', e) }
  }

  async function handleDeleteStream(e: MouseEvent, brandSlug: string, streamSlug: string) {
    e.stopPropagation()
    if (!confirm(`Delete stream "${streamSlug}" and all its projects?`)) return
    try {
      await DeleteStream(brandSlug, streamSlug)
      if (nav.brandSlug === brandSlug && nav.streamSlug === streamSlug) {
        nav.streamSlug = null
        nav.projectSlug = null
        board.categories = []
      }
      streamsByBrand[brandSlug] = await ListStreams(brandSlug) || []
    } catch (e) { console.error('DeleteStream:', e) }
  }

  async function handleDeleteProject(e: MouseEvent, brandSlug: string, streamSlug: string, projectSlug: string) {
    e.stopPropagation()
    if (!confirm(`Delete project "${projectSlug}"?`)) return
    try {
      await DeleteProject(brandSlug, streamSlug, projectSlug)
      if (nav.brandSlug === brandSlug && nav.streamSlug === streamSlug && nav.projectSlug === projectSlug) {
        nav.projectSlug = null
        board.categories = []
      }
      const key = `${brandSlug}/${streamSlug}`
      projectsByStream[key] = await ListProjects(brandSlug, streamSlug) || []
    } catch (e) { console.error('DeleteProject:', e) }
  }
</script>

<aside class="sidebar" class:collapsed={nav.sidebarCollapsed} style:width="{nav.sidebarCollapsed ? 48 : nav.sidebarWidth}px" style:min-width="{nav.sidebarCollapsed ? 48 : nav.sidebarWidth}px">
  <div class="sidebar-header">
    <span class="sidebar-title">BRUV</span>
    <button class="header-btn" onclick={() => nav.sidebarCollapsed = !nav.sidebarCollapsed}>
      {#if nav.sidebarCollapsed}<PanelLeftOpen size={16} />{:else}<PanelLeftClose size={16} />{/if}
    </button>
  </div>

  {#if !nav.sidebarCollapsed}
    <button class="close-repo-btn" onclick={handleCloseRepo}>
      <LogOut size={14} />
      {t('sidebar.close_repo')}
    </button>

    <nav class="nav-tree">
      {#each brands as brand}
        <div class="tree-node">
          <div class="tree-row">
            <button class="tree-item brand-item" onclick={() => toggleBrand(brand.slug)}>
              <span class="chevron">{#if expandedBrands.has(brand.slug)}<ChevronDown size={12} />{:else}<ChevronRight size={12} />{/if}</span>
              <span class="label">{brand.name}</span>
            </button>
            <button class="row-action delete-action" onclick={(e) => handleDeleteBrand(e, brand.slug)} title="Delete brand"><Trash2 size={12} /></button>
          </div>

          {#if expandedBrands.has(brand.slug) && streamsByBrand[brand.slug]}
            <div class="tree-children">
              {#each streamsByBrand[brand.slug] as stream}
                <div class="tree-node">
                  <div class="tree-row">
                    <button class="tree-item stream-item" onclick={() => toggleStream(brand.slug, stream.slug)}>
                      <span class="chevron">{#if expandedStreams.has(`${brand.slug}/${stream.slug}`)}<ChevronDown size={12} />{:else}<ChevronRight size={12} />{/if}</span>
                      <span class="label">{stream.name}</span>
                    </button>
                    <button class="row-action delete-action" onclick={(e) => handleDeleteStream(e, brand.slug, stream.slug)} title="Delete stream"><Trash2 size={12} /></button>
                  </div>

                  {#if expandedStreams.has(`${brand.slug}/${stream.slug}`) && projectsByStream[`${brand.slug}/${stream.slug}`]}
                    <div class="tree-children">
                      {#each projectsByStream[`${brand.slug}/${stream.slug}`] as project}
                        <div class="tree-row">
                          <button
                            class="tree-item project-item"
                            class:selected={isSelected(brand.slug, stream.slug, project.slug)}
                            onclick={() => selectProject(brand.slug, stream.slug, project.slug)}
                          >
                            <span class="label">{project.name}</span>
                          </button>
                          <button class="row-action delete-action" onclick={(e) => handleDeleteProject(e, brand.slug, stream.slug, project.slug)} title="Delete project"><Trash2 size={12} /></button>
                        </div>
                      {/each}

                      <!-- Add Project -->
                      {#if addingProjectFor === `${brand.slug}/${stream.slug}`}
                        <div class="add-inline nested">
                          <input
                            type="text"
                            bind:value={newProjectName}
                            onkeydown={(e) => { if (e.key === 'Enter') handleCreateProject(brand.slug, stream.slug); if (e.key === 'Escape') { addingProjectFor = null; newProjectName = '' } }}
                            placeholder="Project name…"
                            class="inline-input"
                          />
                          <div class="inline-actions">
                            <button class="inline-btn-ok" onclick={() => handleCreateProject(brand.slug, stream.slug)}>Add</button>
                            <button class="inline-btn-cancel" onclick={() => { addingProjectFor = null; newProjectName = '' }}><X size={14} /></button>
                          </div>
                        </div>
                      {:else}
                        <button class="add-btn nested" onclick={() => { addingProjectFor = `${brand.slug}/${stream.slug}`; newProjectName = ''; setTimeout(() => (document.querySelector('.add-inline.nested .inline-input') as HTMLElement)?.focus(), 0) }}>
                          + Add project
                        </button>
                      {/if}
                    </div>
                  {/if}
                </div>
              {/each}

              <!-- Add Stream -->
              {#if addingStreamFor === brand.slug}
                <div class="add-inline">
                  <input
                    type="text"
                    bind:value={newStreamName}
                    onkeydown={(e) => { if (e.key === 'Enter') handleCreateStream(brand.slug); if (e.key === 'Escape') { addingStreamFor = null; newStreamName = '' } }}
                    placeholder="Stream name…"
                    class="inline-input"
                  />
                  <div class="inline-actions">
                    <button class="inline-btn-ok" onclick={() => handleCreateStream(brand.slug)}>Add</button>
                    <button class="inline-btn-cancel" onclick={() => { addingStreamFor = null; newStreamName = '' }}><X size={14} /></button>
                  </div>
                </div>
              {:else}
                <button class="add-btn" onclick={() => { addingStreamFor = brand.slug; newStreamName = ''; setTimeout(() => (document.querySelector('.add-inline .inline-input') as HTMLElement)?.focus(), 0) }}>
                  + Add stream
                </button>
              {/if}
            </div>
          {/if}
        </div>
      {/each}

      {#if brands.length === 0 && !addingBrand}
        <p class="empty-hint">No brands yet.</p>
      {/if}

      <!-- Add Brand -->
      {#if addingBrand}
        <div class="add-inline">
          <input
            type="text"
            bind:value={newBrandName}
            onkeydown={(e) => { if (e.key === 'Enter') handleCreateBrand(); if (e.key === 'Escape') { addingBrand = false; newBrandName = '' } }}
            placeholder="Brand name…"
            class="inline-input"
          />
          <div class="inline-actions">
            <button class="inline-btn-ok" onclick={handleCreateBrand}>Add</button>
            <button class="inline-btn-cancel" onclick={() => { addingBrand = false; newBrandName = '' }}><X size={14} /></button>
          </div>
        </div>
      {:else}
        <button class="add-btn" onclick={() => { addingBrand = true; setTimeout(() => (document.querySelector('.inline-input') as HTMLElement)?.focus(), 0) }}>
          + Add brand
        </button>
      {/if}
    </nav>
  {/if}
</aside>

<style>
  .sidebar {
    background: var(--bg-surface);
    border-right: 1px solid var(--border-muted);
    display: flex;
    flex-direction: column;
    height: 100vh;
    overflow: hidden;
  }

  .sidebar-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--border-muted);
  }

  .sidebar-title {
    font-size: 1rem;
    font-weight: 700;
    letter-spacing: 0.1em;
    color: var(--text-primary);
  }

  .collapsed .sidebar-title {
    display: none;
  }

  .header-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 0.9rem;
    padding: 0.25rem;
  }
  .header-btn:hover {
    color: var(--text-primary);
  }

  .close-repo-btn {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: calc(100% - 1rem);
    margin: 0.5rem;
    padding: 0.45rem 0.75rem;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-elevated);
    color: var(--text-secondary);
    font-size: 0.8rem;
    font-weight: 500;
    cursor: pointer;
    transition: background 0.15s, color 0.15s, border-color 0.15s;
  }

  .close-repo-btn:hover {
    background: var(--border);
    color: var(--text-strong);
    border-color: var(--border-hover);
  }

  .nav-tree {
    flex: 1;
    overflow-y: auto;
    padding: 0.5rem 0;
  }

  .tree-node {
    display: flex;
    flex-direction: column;
  }

  .tree-item {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.35rem 0.75rem;
    background: none;
    border: none;
    color: var(--text-body);
    font-size: 0.85rem;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .tree-item:hover {
    background: var(--bg-elevated);
  }

  .tree-item.selected {
    background: var(--border);
    color: var(--text-primary);
    font-weight: 500;
  }

  .chevron {
    font-size: 0.7rem;
    width: 0.8rem;
    flex-shrink: 0;
    color: var(--text-muted);
  }

  .tree-children {
    padding-left: 1rem;
  }

  .brand-item {
    font-weight: 600;
    color: var(--text-strong);
  }

  .stream-item {
    color: var(--text-secondary);
  }

  .project-item {
    padding-left: 2rem;
    color: var(--text-body);
  }

  .tree-row {
    display: flex;
    align-items: center;
  }

  .tree-row .tree-item {
    flex: 1;
    min-width: 0;
  }

  .row-action {
    background: none;
    border: none;
    color: transparent;
    cursor: pointer;
    font-size: 0.7rem;
    padding: 0.2rem 0.4rem;
    flex-shrink: 0;
    transition: color 0.1s;
  }

  .tree-row:hover .row-action {
    color: var(--text-faint);
  }

  .row-action.delete-action:hover {
    color: var(--danger-light);
  }

  .empty-hint {
    padding: 1rem;
    color: var(--text-faint);
    font-size: 0.8rem;
    text-align: center;
  }

  .add-btn {
    display: block;
    width: 100%;
    padding: 0.3rem 0.75rem;
    background: none;
    border: none;
    color: var(--text-faint);
    font-size: 0.8rem;
    cursor: pointer;
    text-align: left;
    transition: color 0.1s;
  }
  .add-btn:hover {
    color: var(--text-secondary);
  }
  .add-btn.nested {
    padding-left: 2rem;
  }

  .add-inline {
    padding: 0.3rem 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }
  .add-inline.nested {
    padding-left: 2rem;
  }

  .inline-input {
    width: 100%;
    padding: 0.3rem 0.5rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.8rem;
    outline: none;
    box-sizing: border-box;
  }
  .inline-input:focus {
    border-color: var(--accent);
  }

  .inline-actions {
    display: flex;
    gap: 0.3rem;
  }

  .inline-btn-ok {
    padding: 0.2rem 0.5rem;
    border: none;
    border-radius: 3px;
    background: var(--accent);
    color: #fff;
    font-size: 0.75rem;
    cursor: pointer;
  }
  .inline-btn-ok:hover {
    background: var(--accent-hover);
  }

  .inline-btn-cancel {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 0.85rem;
    padding: 0.1rem 0.3rem;
  }
  .inline-btn-cancel:hover {
    color: var(--text-primary);
  }
</style>
