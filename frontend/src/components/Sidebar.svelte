<script lang="ts">
  import { onMount } from 'svelte'
  import { nav, board, loadBoard } from '../lib/store.svelte'
  import { CloseRepository, CreateBrand, RenameBrand, CreateStream, RenameStream, CreateProject, RenameProject, DeleteBrand, DeleteStream, DeleteProject, ListBrands, ListStreams, ListProjects, GetCard, GetCardPins, ListOrphanedCardIDs, ReorderBrands, ReorderStreams, ReorderProjects, MoveStream, MoveProject, CopyBrand, CopyStream, CopyProject } from '../lib/api'
  import { LogOut, Trash2, Pencil, ChevronRight, ChevronDown, PanelLeftClose, PanelLeftOpen, Settings, UserCircle, Bot, Inbox } from 'lucide-svelte'
  import ThemeToggle from './ThemeToggle.svelte'
  import BruvIcon from './BruvIcon.svelte'
  import { t } from '../lib/i18n.svelte'
  import { renderInline } from '../lib/markdown'

  let { onOpenPrefs, onOpenProfile, onOpenLLMSettings }: {
    onOpenPrefs?: () => void
    onOpenProfile?: () => void
    onOpenLLMSettings?: () => void
  } = $props()

  async function handleCloseRepo() {
    await CloseRepository()
    nav.repoOpen = false
    nav.repoId = ''
    nav.brandSlug = null
    nav.streamSlug = null
    nav.projectSlug = null
    nav.brandName = ''
    nav.streamName = ''
    nav.projectName = ''
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
  let inboxCount = $state(0)

  // Inline rename state — unified for brand/stream/project
  let renaming = $state<{
    type: 'brand' | 'stream' | 'project'
    key: string
    name: string
    original: string
    isCreate: boolean
    brandSlug: string
    streamSlug: string
    projectSlug: string
  } | null>(null)

  $effect(() => {
    if (nav.repoOpen) {
      loadBrandsAndRestore()
    }
  })

  onMount(() => {
    async function handleInboxChanged() {
      await refreshInboxCount()
      if (nav.inboxMode) await selectInbox()
    }
    async function handleSidebarChanged() {
      await loadBrands()
    }
    document.addEventListener('bruv:inbox-changed', handleInboxChanged)
    document.addEventListener('bruv:sidebar-changed', handleSidebarChanged)
    return () => {
      document.removeEventListener('bruv:inbox-changed', handleInboxChanged)
      document.removeEventListener('bruv:sidebar-changed', handleSidebarChanged)
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
    await refreshInboxCount()
  }

  async function refreshInboxCount() {
    try {
      const ids = await ListOrphanedCardIDs() || []
      inboxCount = ids.length
    } catch { inboxCount = 0 }
  }

  async function selectInbox() {
    nav.inboxMode = true
    nav.brandSlug = null
    nav.streamSlug = null
    nav.projectSlug = null
    nav.brandName = ''
    nav.streamName = ''
    nav.projectName = ''
    localStorage.removeItem('bruv-last-nav')
    board.loading = true
    try {
      const ids = await ListOrphanedCardIDs() || []
      const cards = await Promise.all(ids.map(async (id: string) => {
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
      board.categories = [{
        id: '__inbox__',
        name: 'Inbox',
        slug: '__inbox__',
        position: 0,
        cards: cards.filter((c): c is NonNullable<typeof c> => c !== null),
      }]
    } catch {
      board.categories = []
    }
    board.loading = false
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
    nav.inboxMode = false
    nav.brandSlug = brandSlug
    nav.streamSlug = streamSlug
    nav.projectSlug = projectSlug
    localStorage.setItem('bruv-last-nav', JSON.stringify({ brandSlug, streamSlug, projectSlug }))
    // Expand the tree so the project is visible
    expandedBrands.add(brandSlug)
    expandedBrands = new Set(expandedBrands)
    if (!streamsByBrand[brandSlug]) streamsByBrand[brandSlug] = await ListStreams(brandSlug) || []
    const streamKey = `${brandSlug}/${streamSlug}`
    expandedStreams.add(streamKey)
    expandedStreams = new Set(expandedStreams)
    if (!projectsByStream[streamKey]) projectsByStream[streamKey] = await ListProjects(brandSlug, streamSlug) || []
    // Resolve display names from cached sidebar data
    nav.brandName = brands.find(b => b.slug === brandSlug)?.name || brandSlug
    nav.streamName = (streamsByBrand[brandSlug] || []).find(s => s.slug === streamSlug)?.name || streamSlug
    nav.projectName = (projectsByStream[streamKey] || []).find(p => p.slug === projectSlug)?.name || projectSlug
    await loadBoard(brandSlug, streamSlug, projectSlug)
  }

  // Allow programmatic project selection from internal link navigation
  onMount(() => {
    async function handleSelectProject(e: Event) {
      const { brandSlug, streamSlug, projectSlug, resolve } = (e as CustomEvent).detail
      await selectProject(brandSlug, streamSlug, projectSlug)
      resolve?.()
    }
    document.addEventListener('bruv:select-project', handleSelectProject)
    return () => document.removeEventListener('bruv:select-project', handleSelectProject)
  })

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

  // --- Create-then-rename handlers ---

  async function findUniqueName(baseName: string, existingNames: string[]): Promise<string> {
    const lower = existingNames.map(n => n.toLowerCase())
    if (!lower.includes(baseName.toLowerCase())) return baseName
    for (let i = 2; ; i++) {
      const candidate = `${baseName} ${i}`
      if (!lower.includes(candidate.toLowerCase())) return candidate
    }
  }

  async function handleCreateBrand() {
    try {
      const name = await findUniqueName(t('default.brand_name'), brands.map(b => b.name))
      const created = await CreateBrand(name)
      await loadBrands()
      expandedBrands.add(created.slug)
      expandedBrands = new Set(expandedBrands)
      streamsByBrand[created.slug] = []
      renaming = { type: 'brand', key: created.slug, name: created.name, original: created.name, isCreate: true, brandSlug: created.slug, streamSlug: '', projectSlug: '' }
      setTimeout(() => { const el = document.querySelector('.rename-input') as HTMLInputElement; el?.scrollIntoView({ block: 'nearest' }); el?.select() }, 0)
    } catch (e) { console.error('CreateBrand:', e) }
  }

  async function handleCreateStream(brandSlug: string) {
    try {
      const existing = streamsByBrand[brandSlug] || []
      const name = await findUniqueName(t('default.stream_name'), existing.map(s => s.name))
      const created = await CreateStream(brandSlug, name)
      streamsByBrand[brandSlug] = await ListStreams(brandSlug) || []
      const streamKey = `${brandSlug}/${created.slug}`
      expandedStreams.add(streamKey)
      expandedStreams = new Set(expandedStreams)
      projectsByStream[streamKey] = []
      renaming = { type: 'stream', key: streamKey, name: created.name, original: created.name, isCreate: true, brandSlug, streamSlug: created.slug, projectSlug: '' }
      setTimeout(() => { const el = document.querySelector('.rename-input') as HTMLInputElement; el?.scrollIntoView({ block: 'nearest' }); el?.select() }, 0)
    } catch (e) { console.error('CreateStream:', e) }
  }

  async function handleCreateProject(brandSlug: string, streamSlug: string) {
    try {
      const streamKey = `${brandSlug}/${streamSlug}`
      const existing = projectsByStream[streamKey] || []
      const name = await findUniqueName(t('default.project_name'), existing.map(p => p.name))
      const created = await CreateProject(brandSlug, streamSlug, name)
      projectsByStream[streamKey] = await ListProjects(brandSlug, streamSlug) || []
      renaming = { type: 'project', key: `${brandSlug}/${streamSlug}/${created.slug}`, name: created.name, original: created.name, isCreate: true, brandSlug, streamSlug, projectSlug: created.slug }
      setTimeout(() => { const el = document.querySelector('.rename-input') as HTMLInputElement; el?.scrollIntoView({ block: 'nearest' }); el?.select() }, 0)
    } catch (e) { console.error('CreateProject:', e) }
  }

  // --- Unified commit / cancel / edit ---

  async function commitRename() {
    if (!renaming) return
    const { type, name: rawName, brandSlug, streamSlug, projectSlug } = renaming
    const name = rawName.trim()
    renaming = null
    if (!name) return
    try {
      if (type === 'brand') {
        const updated = await RenameBrand(brandSlug, name)
        const newSlug = updated.slug
        if (newSlug !== brandSlug) {
          // Migrate expanded/cached state to new slug
          if (expandedBrands.has(brandSlug)) {
            expandedBrands.delete(brandSlug)
            expandedBrands.add(newSlug)
            expandedBrands = new Set(expandedBrands)
          }
          if (streamsByBrand[brandSlug]) {
            streamsByBrand[newSlug] = streamsByBrand[brandSlug]
            delete streamsByBrand[brandSlug]
          }
          const oldPfx = `${brandSlug}/`, newPfx = `${newSlug}/`
          const newExp = new Set<string>()
          for (const k of expandedStreams) newExp.add(k.startsWith(oldPfx) ? newPfx + k.slice(oldPfx.length) : k)
          expandedStreams = newExp
          const newProj: Record<string, Project[]> = {}
          for (const [k, v] of Object.entries(projectsByStream)) newProj[k.startsWith(oldPfx) ? newPfx + k.slice(oldPfx.length) : k] = v
          projectsByStream = newProj
          if (nav.brandSlug === brandSlug) {
            nav.brandSlug = newSlug
            localStorage.setItem('bruv-last-nav', JSON.stringify({ brandSlug: newSlug, streamSlug: nav.streamSlug, projectSlug: nav.projectSlug }))
          }
        }
        if (nav.brandSlug === (newSlug !== brandSlug ? newSlug : brandSlug)) nav.brandName = name
        await loadBrands()
      } else if (type === 'stream') {
        const updated = await RenameStream(brandSlug, streamSlug, name)
        const newSlug = updated.slug
        if (newSlug !== streamSlug) {
          const oldKey = `${brandSlug}/${streamSlug}`, newKey = `${brandSlug}/${newSlug}`
          if (expandedStreams.has(oldKey)) {
            expandedStreams.delete(oldKey)
            expandedStreams.add(newKey)
            expandedStreams = new Set(expandedStreams)
          }
          if (projectsByStream[oldKey]) {
            projectsByStream[newKey] = projectsByStream[oldKey]
            delete projectsByStream[oldKey]
          }
          if (nav.brandSlug === brandSlug && nav.streamSlug === streamSlug) {
            nav.streamSlug = newSlug
            localStorage.setItem('bruv-last-nav', JSON.stringify({ brandSlug: nav.brandSlug, streamSlug: newSlug, projectSlug: nav.projectSlug }))
          }
        }
        if (nav.brandSlug === brandSlug && nav.streamSlug === (newSlug !== streamSlug ? newSlug : streamSlug)) nav.streamName = name
        streamsByBrand[brandSlug] = await ListStreams(brandSlug) || []
      } else {
        const updated = await RenameProject(brandSlug, streamSlug, projectSlug, name)
        const newSlug = updated.slug
        if (newSlug !== projectSlug && nav.brandSlug === brandSlug && nav.streamSlug === streamSlug && nav.projectSlug === projectSlug) {
          nav.projectSlug = newSlug
          localStorage.setItem('bruv-last-nav', JSON.stringify({ brandSlug: nav.brandSlug, streamSlug: nav.streamSlug, projectSlug: newSlug }))
        }
        if (nav.brandSlug === brandSlug && nav.streamSlug === streamSlug && nav.projectSlug === (newSlug !== projectSlug ? newSlug : projectSlug)) nav.projectName = name
        const key = `${brandSlug}/${streamSlug}`
        projectsByStream[key] = await ListProjects(brandSlug, streamSlug) || []
      }
    } catch (e) { console.error('Rename:', e) }
  }

  async function cancelRename() {
    if (!renaming) return
    const { type, name: rawName, original, isCreate, brandSlug, streamSlug, projectSlug } = renaming
    const unchanged = rawName.trim() === original
    renaming = null
    if (isCreate && unchanged) {
      try {
        if (type === 'brand') {
          await DeleteBrand(brandSlug)
          await loadBrands()
        } else if (type === 'stream') {
          await DeleteStream(brandSlug, streamSlug)
          streamsByBrand[brandSlug] = await ListStreams(brandSlug) || []
        } else {
          await DeleteProject(brandSlug, streamSlug, projectSlug)
          const key = `${brandSlug}/${streamSlug}`
          projectsByStream[key] = await ListProjects(brandSlug, streamSlug) || []
        }
      } catch (e) { console.error('CancelRename:', e) }
    }
  }

  function startEdit(e: MouseEvent, type: 'brand' | 'stream' | 'project', key: string, name: string, brandSlug: string, streamSlug = '', projectSlug = '') {
    e.stopPropagation()
    renaming = { type, key, name, original: name, isCreate: false, brandSlug, streamSlug, projectSlug }
    setTimeout(() => { const el = document.querySelector('.rename-input') as HTMLInputElement; el?.focus(); el?.select() }, 0)
  }

  // --- Delete handlers ---

  async function handleDeleteBrand(e: MouseEvent, slug: string) {
    e.stopPropagation()
    const streams = streamsByBrand[slug] || []
    if (streams.length > 0 && !confirm(`Delete brand "${slug}" and all its streams/projects?`)) return
    try {
      await DeleteBrand(slug)
      if (nav.brandSlug === slug) {
        nav.brandSlug = null
        nav.streamSlug = null
        nav.projectSlug = null
        board.categories = []
      }
      await loadBrands()
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
    } catch (e) { console.error('DeleteBrand:', e) }
  }

  async function handleDeleteStream(e: MouseEvent, brandSlug: string, streamSlug: string) {
    e.stopPropagation()
    const key = `${brandSlug}/${streamSlug}`
    const projects = projectsByStream[key] || []
    if (projects.length > 0 && !confirm(`Delete stream "${streamSlug}" and all its projects?`)) return
    try {
      await DeleteStream(brandSlug, streamSlug)
      if (nav.brandSlug === brandSlug && nav.streamSlug === streamSlug) {
        nav.streamSlug = null
        nav.projectSlug = null
        board.categories = []
      }
      streamsByBrand[brandSlug] = await ListStreams(brandSlug) || []
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
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
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
    } catch (e) { console.error('DeleteProject:', e) }
  }

  // --- Drag & drop reorder / move / copy ---

  type DragItem =
    | { type: 'brand'; slug: string }
    | { type: 'stream'; brandSlug: string; slug: string }
    | { type: 'project'; brandSlug: string; streamSlug: string; slug: string }

  let dragging = $state<DragItem | null>(null)
  let dropTarget = $state<{ type: string; parentKey: string; index: number } | null>(null)
  let isCopyMode = $state(false)

  function handleDragStart(e: DragEvent, item: DragItem) {
    e.stopPropagation()
    dragging = item
    isCopyMode = e.ctrlKey
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'copyMove'
      e.dataTransfer.setData('text/plain', '')
    }
  }

  function handleDragEnd() {
    dragging = null
    dropTarget = null
    isCopyMode = false
  }

  function handleDragOverItem(e: DragEvent, type: string, parentKey: string, itemIndex: number) {
    if (!dragging || dragging.type !== type) return
    e.preventDefault()
    e.stopPropagation()
    isCopyMode = e.ctrlKey
    if (e.dataTransfer) e.dataTransfer.dropEffect = e.ctrlKey ? 'copy' : 'move'

    // Use mouse position to decide before/after
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const midY = rect.top + rect.height / 2
    const insertIndex = e.clientY < midY ? itemIndex : itemIndex + 1

    dropTarget = { type, parentKey, index: insertIndex }
  }

  function handleDragOverGap(e: DragEvent, type: string, parentKey: string, gapIndex: number) {
    if (!dragging || dragging.type !== type) return
    e.preventDefault()
    e.stopPropagation()
    isCopyMode = e.ctrlKey
    if (e.dataTransfer) e.dataTransfer.dropEffect = e.ctrlKey ? 'copy' : 'move'
    dropTarget = { type, parentKey, index: gapIndex }
  }

  async function handleDrop(e: DragEvent) {
    e.preventDefault()
    e.stopPropagation()
    if (!dragging || !dropTarget || dragging.type !== dropTarget.type) {
      dragging = null; dropTarget = null; isCopyMode = false
      return
    }

    const copy = e.ctrlKey
    const toIndex = dropTarget.index
    const toParent = dropTarget.parentKey
    const d = dragging
    dragging = null; dropTarget = null; isCopyMode = false

    try {
      if (d.type === 'brand') {
        if (copy) {
          await CopyBrand(d.slug)
          await loadBrands()
        } else {
          const fromIndex = brands.findIndex(b => b.slug === d.slug)
          if (fromIndex === -1) return
          const adjustedTo = toIndex > fromIndex ? toIndex - 1 : toIndex
          if (fromIndex === adjustedTo) return
          const [item] = brands.splice(fromIndex, 1)
          brands.splice(adjustedTo, 0, item)
          brands = [...brands]
          await ReorderBrands(brands.map(b => b.slug))
        }
      } else if (d.type === 'stream') {
        const fromBrand = d.brandSlug
        const toBrand = toParent // parentKey is brandSlug
        if (copy) {
          await CopyStream(fromBrand, d.slug, toBrand)
          streamsByBrand[toBrand] = await ListStreams(toBrand) || []
          if (fromBrand !== toBrand) streamsByBrand[fromBrand] = await ListStreams(fromBrand) || []
        } else if (fromBrand === toBrand) {
          // Reorder within same brand
          const streams = streamsByBrand[fromBrand] || []
          const fromIndex = streams.findIndex(s => s.slug === d.slug)
          if (fromIndex === -1) return
          const adjustedTo = toIndex > fromIndex ? toIndex - 1 : toIndex
          if (fromIndex === adjustedTo) return
          const [item] = streams.splice(fromIndex, 1)
          streams.splice(adjustedTo, 0, item)
          streamsByBrand[fromBrand] = [...streams]
          await ReorderStreams(fromBrand, streams.map(s => s.slug))
        } else {
          // Move to different brand
          await MoveStream(fromBrand, d.slug, toBrand)
          streamsByBrand[fromBrand] = await ListStreams(fromBrand) || []
          streamsByBrand[toBrand] = await ListStreams(toBrand) || []
        }
      } else if (d.type === 'project') {
        const fromBrand = d.brandSlug
        const fromStream = d.streamSlug
        const fromKey = `${fromBrand}/${fromStream}`
        // toParent is "brandSlug/streamSlug"
        const [toBrand, toStream] = toParent.split('/')
        const toKey = toParent
        if (copy) {
          await CopyProject(fromBrand, fromStream, d.slug, toBrand, toStream, toIndex)
          projectsByStream[toKey] = await ListProjects(toBrand, toStream) || []
          if (fromKey !== toKey) projectsByStream[fromKey] = await ListProjects(fromBrand, fromStream) || []
        } else if (fromKey === toKey) {
          // Reorder within same stream
          const projects = projectsByStream[fromKey] || []
          const fromIndex = projects.findIndex(p => p.slug === d.slug)
          if (fromIndex === -1) return
          const adjustedTo = toIndex > fromIndex ? toIndex - 1 : toIndex
          if (fromIndex === adjustedTo) return
          const [item] = projects.splice(fromIndex, 1)
          projects.splice(adjustedTo, 0, item)
          projectsByStream[fromKey] = [...projects]
          await ReorderProjects(fromBrand, fromStream, projects.map(p => p.slug))
        } else {
          // Move to different stream
          await MoveProject(fromBrand, fromStream, d.slug, toBrand, toStream)
          projectsByStream[fromKey] = await ListProjects(fromBrand, fromStream) || []
          projectsByStream[toKey] = await ListProjects(toBrand, toStream) || []
        }
      }
    } catch (err) { console.error('Drag/drop:', err) }
  }

  function isDropIndicator(type: string, parentKey: string, index: number): boolean {
    return dropTarget?.type === type && dropTarget?.parentKey === parentKey && dropTarget?.index === index
  }
</script>

<aside class="sidebar" class:collapsed={nav.sidebarCollapsed} style:width="{nav.sidebarCollapsed ? 48 : nav.sidebarWidth}px" style:min-width="{nav.sidebarCollapsed ? 48 : nav.sidebarWidth}px">
  <div class="sidebar-header">
    {#if !nav.sidebarCollapsed}<BruvIcon size={28} />{/if}
    {#if !nav.sidebarCollapsed}<span class="sidebar-title">BRUV</span>{/if}
    <button class="header-btn" onclick={() => nav.sidebarCollapsed = !nav.sidebarCollapsed} title={nav.sidebarCollapsed ? t('tooltip.expand_sidebar') : t('tooltip.collapse_sidebar')}>
      {#if nav.sidebarCollapsed}<PanelLeftOpen size={20} />{:else}<PanelLeftClose size={20} />{/if}
    </button>
  </div>

  {#if !nav.sidebarCollapsed}
    <button class="close-repo-btn" onclick={handleCloseRepo} title={t('tooltip.close_repo')}>
      <LogOut size={14} />
      {t('sidebar.close_repo')}
    </button>

    <div class="nav-tree" role="tree" tabindex="0"
      ondragover={(e) => { if (dragging) { e.preventDefault(); isCopyMode = e.ctrlKey; if (e.dataTransfer) e.dataTransfer.dropEffect = e.ctrlKey ? 'copy' : 'move' } }}
      ondrop={handleDrop}
    >
      <div class="tree-node">
        <div class="tree-row" role="treeitem" tabindex="-1" aria-selected={nav.inboxMode}>
          <button class="tree-item inbox-item" class:selected={nav.inboxMode} onclick={selectInbox}>
            <Inbox size={14} />
            <span class="label">Inbox</span>
            {#if inboxCount > 0}<span class="inbox-badge">{inboxCount}</span>{/if}
          </button>
        </div>
      </div>

      {#each brands as brand, brandIdx}
        {#if isDropIndicator('brand', '', brandIdx)}
          <div class="drop-indicator" class:copy-mode={isCopyMode}></div>
        {/if}
        <div class="tree-node">
          <div class="tree-row" role="treeitem" tabindex="-1" aria-selected={isSelected(brand.slug, '', '')}
            draggable={renaming?.key !== brand.slug}
            ondragstart={(e) => handleDragStart(e, { type: 'brand', slug: brand.slug })}
            ondragend={handleDragEnd}
            ondragover={(e) => handleDragOverItem(e, 'brand', '', brandIdx)}
            ondrop={handleDrop}
            class:dragging-item={dragging?.type === 'brand' && dragging?.slug === brand.slug}
          >
            {#if renaming && renaming.key === brand.slug}
              <input
                class="rename-input brand-level"
                bind:value={renaming.name}
                onkeydown={(e) => { if (e.key === 'Enter') commitRename(); if (e.key === 'Escape') cancelRename() }}
                onblur={() => commitRename()}
              />
            {:else}
              <button class="tree-item brand-item" onclick={() => toggleBrand(brand.slug)}>
                <span class="chevron">{#if expandedBrands.has(brand.slug)}<ChevronDown size={12} />{:else}<ChevronRight size={12} />{/if}</span>
                <span class="label">{@html renderInline(brand.name)}</span>
              </button>
              <button class="row-action edit-action" onclick={(e) => startEdit(e, 'brand', brand.slug, brand.name, brand.slug)} title={t('tooltip.rename_brand')}><Pencil size={12} /></button>
              <button class="row-action delete-action" onclick={(e) => handleDeleteBrand(e, brand.slug)} title={t('tooltip.delete_brand')}><Trash2 size={12} /></button>
            {/if}
          </div>

          {#if expandedBrands.has(brand.slug) && streamsByBrand[brand.slug]}
            <div class="tree-children">
              {#each streamsByBrand[brand.slug] as stream, streamIdx}
                {#if isDropIndicator('stream', brand.slug, streamIdx)}
                  <div class="drop-indicator" class:copy-mode={isCopyMode}></div>
                {/if}
                <div class="tree-node">
                  <div class="tree-row" role="treeitem" tabindex="-1" aria-selected={isSelected(brand.slug, stream.slug, '')}
                    draggable={renaming?.key !== `${brand.slug}/${stream.slug}`}
                    ondragstart={(e) => handleDragStart(e, { type: 'stream', brandSlug: brand.slug, slug: stream.slug })}
                    ondragend={handleDragEnd}
                    ondragover={(e) => handleDragOverItem(e, 'stream', brand.slug, streamIdx)}
                    ondrop={handleDrop}
                    class:dragging-item={dragging?.type === 'stream' && dragging?.slug === stream.slug}
                  >
                    {#if renaming && renaming.key === `${brand.slug}/${stream.slug}`}
                      <input
                        class="rename-input stream-level"
                        bind:value={renaming.name}
                        onkeydown={(e) => { if (e.key === 'Enter') commitRename(); if (e.key === 'Escape') cancelRename() }}
                        onblur={() => commitRename()}
                      />
                    {:else}
                      <button class="tree-item stream-item" onclick={() => toggleStream(brand.slug, stream.slug)}>
                        <span class="chevron">{#if expandedStreams.has(`${brand.slug}/${stream.slug}`)}<ChevronDown size={12} />{:else}<ChevronRight size={12} />{/if}</span>
                        <span class="label">{@html renderInline(stream.name)}</span>
                      </button>
                      <button class="row-action edit-action" onclick={(e) => startEdit(e, 'stream', `${brand.slug}/${stream.slug}`, stream.name, brand.slug, stream.slug)} title={t('tooltip.rename_stream')}><Pencil size={12} /></button>
                      <button class="row-action delete-action" onclick={(e) => handleDeleteStream(e, brand.slug, stream.slug)} title={t('tooltip.delete_stream')}><Trash2 size={12} /></button>
                    {/if}
                  </div>

                  {#if expandedStreams.has(`${brand.slug}/${stream.slug}`) && projectsByStream[`${brand.slug}/${stream.slug}`]}
                    <div class="tree-children">
                      {#each projectsByStream[`${brand.slug}/${stream.slug}`] as project, projectIdx}
                        {#if isDropIndicator('project', `${brand.slug}/${stream.slug}`, projectIdx)}
                          <div class="drop-indicator" class:copy-mode={isCopyMode}></div>
                        {/if}
                        <div class="tree-row" role="treeitem" tabindex="-1" aria-selected={isSelected(brand.slug, stream.slug, project.slug)}
                          draggable={renaming?.key !== `${brand.slug}/${stream.slug}/${project.slug}`}
                          ondragstart={(e) => handleDragStart(e, { type: 'project', brandSlug: brand.slug, streamSlug: stream.slug, slug: project.slug })}
                          ondragend={handleDragEnd}
                          ondragover={(e) => handleDragOverItem(e, 'project', `${brand.slug}/${stream.slug}`, projectIdx)}
                          ondrop={handleDrop}
                          class:dragging-item={dragging?.type === 'project' && dragging?.slug === project.slug}
                        >
                          {#if renaming && renaming.key === `${brand.slug}/${stream.slug}/${project.slug}`}
                            <input
                              class="rename-input project-level"
                              bind:value={renaming.name}
                              onkeydown={(e) => { if (e.key === 'Enter') commitRename(); if (e.key === 'Escape') cancelRename() }}
                              onblur={() => commitRename()}
                            />
                          {:else}
                            <button
                              class="tree-item project-item"
                              class:selected={isSelected(brand.slug, stream.slug, project.slug)}
                              onclick={() => selectProject(brand.slug, stream.slug, project.slug)}
                            >
                              <span class="label">{@html renderInline(project.name)}</span>
                            </button>
                            <button class="row-action edit-action" onclick={(e) => startEdit(e, 'project', `${brand.slug}/${stream.slug}/${project.slug}`, project.name, brand.slug, stream.slug, project.slug)} title={t('tooltip.rename_project')}><Pencil size={12} /></button>
                            <button class="row-action delete-action" onclick={(e) => handleDeleteProject(e, brand.slug, stream.slug, project.slug)} title={t('tooltip.delete_project')}><Trash2 size={12} /></button>
                          {/if}
                        </div>
                      {/each}
                      {#if isDropIndicator('project', `${brand.slug}/${stream.slug}`, projectsByStream[`${brand.slug}/${stream.slug}`]?.length ?? 0)}
                        <div class="drop-indicator" class:copy-mode={isCopyMode}></div>
                      {/if}

                      <button class="add-btn nested" onclick={() => handleCreateProject(brand.slug, stream.slug)} title={t('tooltip.add_project')}
                        ondragover={(e) => handleDragOverGap(e, 'project', `${brand.slug}/${stream.slug}`, projectsByStream[`${brand.slug}/${stream.slug}`]?.length ?? 0)}
                        ondrop={handleDrop}
                      >
                        + Add project
                      </button>
                    </div>
                  {/if}
                </div>
              {/each}
              {#if isDropIndicator('stream', brand.slug, streamsByBrand[brand.slug]?.length ?? 0)}
                <div class="drop-indicator" class:copy-mode={isCopyMode}></div>
              {/if}

              <button class="add-btn" onclick={() => handleCreateStream(brand.slug)} title={t('tooltip.add_stream')}
                ondragover={(e) => handleDragOverGap(e, 'stream', brand.slug, streamsByBrand[brand.slug]?.length ?? 0)}
                ondrop={handleDrop}
              >
                + Add stream
              </button>
            </div>
          {/if}
        </div>
      {/each}
      {#if isDropIndicator('brand', '', brands.length)}
        <div class="drop-indicator" class:copy-mode={isCopyMode}></div>
      {/if}

      {#if brands.length === 0}
        <p class="empty-hint">No brands yet.</p>
      {/if}

      <button class="add-btn" onclick={handleCreateBrand} title={t('tooltip.add_brand')}
        ondragover={(e) => handleDragOverGap(e, 'brand', '', brands.length)}
        ondrop={handleDrop}
      >
        + Add brand
      </button>
    </div>

    <div class="sidebar-footer">
      <button class="footer-btn" onclick={onOpenProfile} title={t('profile.title')}><UserCircle size={16} /></button>
      <button class="footer-btn" onclick={onOpenLLMSettings} title={t('tooltip.llm_settings')}><Bot size={16} /></button>
      <span class="footer-spacer"></span>
      <ThemeToggle />
      <button class="footer-btn" onclick={onOpenPrefs} title={t('prefs.title')}><Settings size={16} /></button>
    </div>
  {/if}

  {#if nav.sidebarCollapsed}
    <div class="sidebar-footer">
      <button class="footer-btn" onclick={onOpenProfile} title={t('profile.title')}><UserCircle size={16} /></button>
      <button class="footer-btn" onclick={onOpenLLMSettings} title={t('tooltip.llm_settings')}><Bot size={16} /></button>
      <span class="footer-spacer"></span>
      <ThemeToggle />
      <button class="footer-btn" onclick={onOpenPrefs} title={t('prefs.title')}><Settings size={16} /></button>
    </div>
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

  .inbox-item {
    gap: 0.5rem;
    margin-bottom: 0.25rem;
    border-bottom: 1px solid var(--border-muted);
    border-radius: 0;
    padding-bottom: 0.5rem;
  }

  .inbox-badge {
    margin-left: auto;
    font-size: 0.65rem;
    font-weight: 600;
    background: var(--accent);
    color: #fff;
    padding: 0.05rem 0.4rem;
    border-radius: 8px;
    flex-shrink: 0;
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
    cursor: grab;
  }

  .tree-row:active {
    cursor: grabbing;
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

  .row-action.edit-action:hover {
    color: var(--accent-light, var(--accent));
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

  .rename-input {
    flex: 1;
    padding: 0.25rem 0.5rem;
    border-radius: 4px;
    border: 1px solid var(--accent);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.8rem;
    outline: none;
    box-sizing: border-box;
  }
  .rename-input.brand-level {
    margin-left: 0.75rem;
    margin-right: 0.75rem;
  }
  .rename-input.stream-level {
    margin-left: 0.5rem;
    margin-right: 0.5rem;
  }
  .rename-input.project-level {
    margin-left: 0.5rem;
    margin-right: 0.5rem;
  }

  .sidebar-footer {
    display: flex;
    align-items: center;
    justify-content: flex-start;
    gap: 0.25rem;
    padding: 0.5rem;
    border-top: 1px solid var(--border-muted);
    margin-top: auto;
  }

  .footer-spacer {
    flex: 1;
  }

  .footer-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.35rem;
    border-radius: 6px;
    display: flex;
    align-items: center;
    transition: color 0.15s, background 0.15s;
  }
  .footer-btn:hover {
    color: var(--text-primary);
    background: var(--bg-subtle-hover);
  }

  /* Drag & drop */
  .dragging-item {
    opacity: 0.4;
  }

  .drop-indicator {
    height: 2px;
    background: var(--accent);
    border-radius: 1px;
    margin: 0 0.5rem;
  }

  .drop-indicator.copy-mode {
    background: var(--success, #22c55e);
    height: 3px;
  }
</style>
