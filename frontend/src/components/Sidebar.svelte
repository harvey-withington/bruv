<script lang="ts">
  import { onMount } from 'svelte'
  import { nav, board, loadBoard, checklistProgress, type LegacyCard } from '../lib/store.svelte'
  import { CreateBrand, RenameBrand, UpdateBrandDescription, UpdateBrandIcon, CreateStream, RenameStream, UpdateStreamDescription, UpdateStreamIcon, CreateProject, RenameProject, UpdateProjectDescription, UpdateProjectIcon, DeleteBrand, DeleteStream, DeleteProject, ListBrands, ListStreams, ListProjects, GetCard, ListOrphanedCardIDs, ReorderBrands, ReorderStreams, ReorderProjects, MoveStream, MoveProject, CopyBrand, CopyStream, CopyProject, GetUIPreferences, GetRepoDescription, UpdateRepoDescription } from '@shared/api'
  import { ChevronLeft, Trash2, Pencil, ChevronRight, ChevronDown, PanelLeftClose, PanelLeftOpen, Settings, UserCircle, Inbox, Timer, ChevronsUpDown, ChevronsDownUp, Smile, Upload, Info, Server, Monitor, ListCollapse, ListTree } from 'lucide-svelte'
  import { connections, isLocalActive, activeConnectionLabel } from '../lib/connections.svelte'
  import ThemeToggle from './ThemeToggle.svelte'
  import BruvIcon from './BruvIcon.svelte'
  import DynamicIcon from './DynamicIcon.svelte'
  import IconPicker from './IconPicker.svelte'
  import ImportTrelloDialog from './ImportTrelloDialog.svelte'
  import { t } from '../lib/i18n.svelte'
  import { renderInline } from '@shared/markdown'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { inlineEdit } from '../lib/actions'

  let { onOpenPrefs, onOpenProfile, onOpenAbout, onOpenConnections }: {
    onOpenPrefs?: () => void
    onOpenProfile?: () => void
    onOpenAbout?: () => void
    onOpenConnections?: () => void
  } = $props()

  // closeRepoAndReturn is the "back to picker" action wired to the
  // chevron next to the connection chip. Uniform across every
  // connection (including Local, which is just another connection
  // since the local-as-remote pivot): clear THIS device's
  // last-active-repo pointer for the active connection so the
  // post-reload boot resolves with no repoID and lands on the picker.
  // The runtime stays loaded — supervisor lazy-unloads via
  // SetEnabled / Remove / process exit. Closing a repo is reversible
  // (the registry entry stays + a fresh open re-creates per-repo
  // state), so we treat it as navigation rather than a destructive
  // action — no confirm. Reload mirrors switchConnection / selectRepo:
  // tears down every in-memory cache cleanly without the sidebar
  // having to enumerate them.
  async function closeRepoAndReturn() {
    type ShellBack = {
      SetActiveRepoForConnection?: (connID: string, repoID: string) => Promise<void>
    }
    const s = (window as unknown as { go?: { main?: { ShellAPI?: ShellBack } } }).go?.main?.ShellAPI
    try {
      if (s?.SetActiveRepoForConnection) {
        await s.SetActiveRepoForConnection(connections.active, '')
      }
    } catch { /* still reload — try to recover regardless */ }
    setTimeout(() => window.location.reload(), 50)
  }

  type Brand = { id: string; name: string; slug: string; description?: string; icon?: string }
  type Stream = { id: string; name: string; slug: string; description?: string; icon?: string }
  type Project = { id: string; name: string; slug: string; description?: string; icon?: string }

  let brands = $state<Brand[]>([])
  let expandedBrands = $state<Set<string>>(new Set())
  let streamsByBrand = $state<Record<string, Stream[]>>({})
  let expandedStreams = $state<Set<string>>(new Set())
  const ACCORDION_MODE_KEY = 'bruv:accordionMode'
  type AccordionMode = 'single' | 'multi'
  function readAccordionMode(): AccordionMode {
    const v = localStorage.getItem(ACCORDION_MODE_KEY)
    return v === 'multi' ? 'multi' : 'single'
  }
  let accordionMode = $state<AccordionMode>(readAccordionMode())

  function applySingleExpandCollapses() {
    if (nav.brandSlug) {
      expandedBrands = new Set([nav.brandSlug])
      if (nav.streamSlug) {
        expandedStreams = new Set([`${nav.brandSlug}/${nav.streamSlug}`])
      } else {
        expandedStreams = new Set()
      }
    } else {
      if (expandedBrands.size > 1) {
        const first = Array.from(expandedBrands)[0]
        expandedBrands = new Set([first])
        const prefix = `${first}/`
        const nextStreams = new Set<string>()
        for (const s of expandedStreams) {
          if (s.startsWith(prefix)) {
            nextStreams.add(s)
            break
          }
        }
        expandedStreams = nextStreams
      }
    }
  }

  function toggleAccordionMode() {
    accordionMode = accordionMode === 'single' ? 'multi' : 'single'
    localStorage.setItem(ACCORDION_MODE_KEY, accordionMode)
    if (accordionMode === 'single') {
      applySingleExpandCollapses()
    }
  }
  let projectsByStream = $state<Record<string, Project[]>>({})
  let inboxCount = $state(0)
  let repoDescription = $state('')
  let editingRepoDesc = $state(false)
  let repoDescDraft = $state('')

  // Icon picker state
  let iconPickerTarget = $state<{
    type: 'brand' | 'stream' | 'project'
    brandSlug: string
    streamSlug: string
    projectSlug: string
    currentIcon: string
  } | null>(null)

  // Inline rename state — unified for brand/stream/project
  let renaming = $state<{
    type: 'brand' | 'stream' | 'project'
    key: string
    name: string
    original: string
    description: string
    originalDescription: string
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
      // Re-fetch children for all currently expanded brands and streams
      // so newly created hierarchy items appear immediately
      for (const brandSlug of expandedBrands) {
        try {
          streamsByBrand[brandSlug] = await ListStreams(brandSlug) || []
        } catch { streamsByBrand[brandSlug] = [] }
      }
      for (const streamKey of expandedStreams) {
        const [brandSlug, streamSlug] = streamKey.split('/')
        try {
          projectsByStream[streamKey] = await ListProjects(brandSlug, streamSlug) || []
        } catch { projectsByStream[streamKey] = [] }
      }
    }
    document.addEventListener('bruv:inbox-changed', handleInboxChanged)
    document.addEventListener('bruv:sidebar-changed', handleSidebarChanged)
    return () => {
      document.removeEventListener('bruv:inbox-changed', handleInboxChanged)
      document.removeEventListener('bruv:sidebar-changed', handleSidebarChanged)
    }
  })

  // lastNavKey returns the per-repo localStorage key for "last viewed
  // brand/stream/project". Pre-fix this was a single global key
  // (`bruv-last-nav`), which meant switching repo or connection would
  // restore a project from the OLD repo — usually nonexistent in the
  // new one, so the sidebar tried to expand a brand that wasn't
  // there and silently bailed. Scoping by repo ID (now unified with
  // manifest ID) gives each repo its own remembered selection.
  function lastNavKey(): string {
    return `bruv-last-nav:${nav.repoId || '__none__'}`
  }

  async function loadBrandsAndRestore() {
    await loadBrands()
    try { repoDescription = await GetRepoDescription() || '' } catch { repoDescription = '' }
    // Check if user prefers a collapsed tree on load
    let collapseOnLoad = false
    try {
      const p = await GetUIPreferences()
      collapseOnLoad = p?.sidebar_collapse_default ?? false
    } catch { /* use default */ }
    // Restore last nav state if available, otherwise default to Inbox
    try {
      const raw = localStorage.getItem(lastNavKey())
      if (!raw) {
        // No saved project — expand all brands if not collapsed
        if (!collapseOnLoad) await expandAll()
        await selectInbox()
        return
      }
      const last = JSON.parse(raw) as { brandSlug: string; streamSlug: string; projectSlug: string }
      if (!last.brandSlug || !last.streamSlug || !last.projectSlug) {
        if (!collapseOnLoad) await expandAll()
        return
      }
      // Always expand the path to the saved project so it's visible in the tree
      if (accordionMode === 'single') {
        expandedBrands = new Set([last.brandSlug])
      } else {
        expandedBrands.add(last.brandSlug)
        expandedBrands = new Set(expandedBrands)
      }
      streamsByBrand[last.brandSlug] = await ListStreams(last.brandSlug) || []
      const streamKey = `${last.brandSlug}/${last.streamSlug}`
      if (accordionMode === 'single') {
        expandedStreams = new Set([streamKey])
      } else {
        expandedStreams.add(streamKey)
        expandedStreams = new Set(expandedStreams)
      }
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
    nav.agentsMode = false
    nav.brandSlug = null
    nav.streamSlug = null
    nav.projectSlug = null
    nav.brandName = ''
    nav.streamName = ''
    nav.projectName = ''
    localStorage.removeItem(lastNavKey())
    board.loading = true
    try {
      const ids = await ListOrphanedCardIDs() || []
      const cards = await Promise.all(ids.map(async (id: string) => {
        try {
          const card: LegacyCard = await GetCard(id)
          const checklist = checklistProgress(card)
          return {
            id: card.id,
            type: card.type,
            title: card.title,
            tags: card.tags || [],
            due_date: card.due_date,
            checklist_total: checklist.total,
            checklist_done: checklist.done,
          }
        } catch { return null }
      }))
      board.categories = [{
        id: '__inbox__',
        name: t('sidebar.inbox'),
        slug: '__inbox__',
        position: 0,
        cards: cards.filter((c): c is NonNullable<typeof c> => c !== null),
      }]
    } catch {
      board.categories = []
    }
    board.loading = false
  }

  function selectAgents() {
    nav.agentsMode = true
    nav.inboxMode = false
    nav.brandSlug = null
    nav.streamSlug = null
    nav.projectSlug = null
    nav.brandName = ''
    nav.streamName = ''
    nav.projectName = ''
    localStorage.removeItem(lastNavKey())
  }

  async function expandAll() {
    for (const brand of brands) {
      if (!streamsByBrand[brand.slug]) {
        try { streamsByBrand[brand.slug] = await ListStreams(brand.slug) || [] } catch { streamsByBrand[brand.slug] = [] }
      }
      expandedBrands.add(brand.slug)
    }
    expandedBrands = new Set(expandedBrands)
    for (const brand of brands) {
      for (const stream of (streamsByBrand[brand.slug] || [])) {
        const key = `${brand.slug}/${stream.slug}`
        if (!projectsByStream[key]) {
          try { projectsByStream[key] = await ListProjects(brand.slug, stream.slug) || [] } catch { projectsByStream[key] = [] }
        }
        expandedStreams.add(key)
      }
    }
    expandedStreams = new Set(expandedStreams)
  }

  function collapseAll() {
    expandedBrands = new Set()
    expandedStreams = new Set()
  }

  async function toggleBrand(slug: string) {
    if (expandedBrands.has(slug)) {
      expandedBrands.delete(slug)
      expandedBrands = new Set(expandedBrands)
    } else {
      if (accordionMode === 'single') {
        expandedBrands = new Set([slug])
      } else {
        expandedBrands.add(slug)
        expandedBrands = new Set(expandedBrands)
      }
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
      if (accordionMode === 'single') {
        const prefix = `${brandSlug}/`
        const nextStreams = new Set<string>()
        for (const k of expandedStreams) {
          if (!k.startsWith(prefix)) {
            nextStreams.add(k)
          }
        }
        nextStreams.add(key)
        expandedStreams = nextStreams
      } else {
        expandedStreams.add(key)
        expandedStreams = new Set(expandedStreams)
      }
      if (!projectsByStream[key]) {
        try {
          projectsByStream[key] = await ListProjects(brandSlug, streamSlug) || []
        } catch { projectsByStream[key] = [] }
      }
    }
  }

  async function selectProject(brandSlug: string, streamSlug: string, projectSlug: string) {
    nav.inboxMode = false
    nav.agentsMode = false
    nav.brandSlug = brandSlug
    nav.streamSlug = streamSlug
    nav.projectSlug = projectSlug
    localStorage.setItem(lastNavKey(), JSON.stringify({ brandSlug, streamSlug, projectSlug }))
    // Expand the tree so the project is visible
    if (accordionMode === 'single') {
      expandedBrands = new Set([brandSlug])
      expandedStreams = new Set([`${brandSlug}/${streamSlug}`])
    } else {
      expandedBrands.add(brandSlug)
      expandedBrands = new Set(expandedBrands)
    }
    if (!streamsByBrand[brandSlug]) streamsByBrand[brandSlug] = await ListStreams(brandSlug) || []
    const streamKey = `${brandSlug}/${streamSlug}`
    if (accordionMode !== 'single') {
      expandedStreams.add(streamKey)
      expandedStreams = new Set(expandedStreams)
    }
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
      // Same-project refresh: silent so in-place edits don't flash loading.
      loadBoard(nav.brandSlug, nav.streamSlug, nav.projectSlug, { silent: true })
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
      if (accordionMode === 'single') {
        expandedBrands = new Set([created.slug])
      } else {
        expandedBrands.add(created.slug)
        expandedBrands = new Set(expandedBrands)
      }
      streamsByBrand[created.slug] = []
      renaming = { type: 'brand', key: created.slug, name: created.name, original: created.name, description: '', originalDescription: '', isCreate: true, brandSlug: created.slug, streamSlug: '', projectSlug: '' }
      setTimeout(() => { const el = document.querySelector('.rename-input') as HTMLInputElement; el?.scrollIntoView({ block: 'nearest' }); el?.select() }, 0)
    } catch (e) {
      console.error('CreateBrand:', e)
      showToast(t('error.create_failed'), 'error')
    }
  }

  async function handleCreateStream(brandSlug: string) {
    try {
      const existing = streamsByBrand[brandSlug] || []
      const name = await findUniqueName(t('default.stream_name'), existing.map(s => s.name))
      const created = await CreateStream(brandSlug, name)
      streamsByBrand[brandSlug] = await ListStreams(brandSlug) || []
      const streamKey = `${brandSlug}/${created.slug}`
      if (accordionMode === 'single') {
        expandedBrands = new Set([brandSlug])
        const prefix = `${brandSlug}/`
        const nextStreams = new Set<string>()
        for (const k of expandedStreams) {
          if (!k.startsWith(prefix)) {
            nextStreams.add(k)
          }
        }
        nextStreams.add(streamKey)
        expandedStreams = nextStreams
      } else {
        expandedStreams.add(streamKey)
        expandedStreams = new Set(expandedStreams)
      }
      projectsByStream[streamKey] = []
      renaming = { type: 'stream', key: streamKey, name: created.name, original: created.name, description: '', originalDescription: '', isCreate: true, brandSlug, streamSlug: created.slug, projectSlug: '' }
      setTimeout(() => { const el = document.querySelector('.rename-input') as HTMLInputElement; el?.scrollIntoView({ block: 'nearest' }); el?.select() }, 0)
    } catch (e) {
      console.error('CreateStream:', e)
      showToast(t('error.create_failed'), 'error')
    }
  }

  async function handleCreateProject(brandSlug: string, streamSlug: string) {
    try {
      const streamKey = `${brandSlug}/${streamSlug}`
      const existing = projectsByStream[streamKey] || []
      const name = await findUniqueName(t('default.project_name'), existing.map(p => p.name))
      const created = await CreateProject(brandSlug, streamSlug, name)
      projectsByStream[streamKey] = await ListProjects(brandSlug, streamSlug) || []
      renaming = { type: 'project', key: `${brandSlug}/${streamSlug}/${created.slug}`, name: created.name, original: created.name, description: '', originalDescription: '', isCreate: true, brandSlug, streamSlug, projectSlug: created.slug }
      setTimeout(() => { const el = document.querySelector('.rename-input') as HTMLInputElement; el?.scrollIntoView({ block: 'nearest' }); el?.select() }, 0)
    } catch (e) {
      console.error('CreateProject:', e)
      showToast(t('error.create_failed'), 'error')
    }
  }

  // --- Trello import dialog ---
  let trelloImportTarget = $state<{ brandSlug: string; streamSlug: string } | null>(null)

  function openTrelloImport(brandSlug: string, streamSlug: string) {
    trelloImportTarget = { brandSlug, streamSlug }
  }

  async function handleTrelloImportDone(brandSlug: string, streamSlug: string) {
    const streamKey = `${brandSlug}/${streamSlug}`
    projectsByStream[streamKey] = await ListProjects(brandSlug, streamSlug) || []
    // Expand the brand + stream so the new project is visible immediately.
    if (accordionMode === 'single') {
      expandedBrands = new Set([brandSlug])
      expandedStreams = new Set([streamKey])
    } else {
      if (!expandedBrands.has(brandSlug)) expandedBrands = new Set([...expandedBrands, brandSlug])
      if (!expandedStreams.has(streamKey)) expandedStreams = new Set([...expandedStreams, streamKey])
    }
    trelloImportTarget = null
  }

  // --- Unified commit / cancel / edit ---

  async function commitRename() {
    if (!renaming) return
    const { type, name: rawName, description, originalDescription, brandSlug, streamSlug, projectSlug } = renaming
    const name = rawName.trim()
    renaming = null
    if (!name) return
    try {
      if (type === 'brand') {
        const updated = await RenameBrand(brandSlug, name)
        if (description !== originalDescription) await UpdateBrandDescription(updated.slug, description)
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
            localStorage.setItem(lastNavKey(), JSON.stringify({ brandSlug: newSlug, streamSlug: nav.streamSlug, projectSlug: nav.projectSlug }))
          }
        }
        if (nav.brandSlug === (newSlug !== brandSlug ? newSlug : brandSlug)) nav.brandName = name
        await loadBrands()
      } else if (type === 'stream') {
        const updated = await RenameStream(brandSlug, streamSlug, name)
        if (description !== originalDescription) await UpdateStreamDescription(brandSlug, updated.slug, description)
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
            localStorage.setItem(lastNavKey(), JSON.stringify({ brandSlug: nav.brandSlug, streamSlug: newSlug, projectSlug: nav.projectSlug }))
          }
        }
        if (nav.brandSlug === brandSlug && nav.streamSlug === (newSlug !== streamSlug ? newSlug : streamSlug)) nav.streamName = name
        streamsByBrand[brandSlug] = await ListStreams(brandSlug) || []
      } else {
        const updated = await RenameProject(brandSlug, streamSlug, projectSlug, name)
        if (description !== originalDescription) await UpdateProjectDescription(brandSlug, streamSlug, updated.slug, description)
        const newSlug = updated.slug
        if (newSlug !== projectSlug && nav.brandSlug === brandSlug && nav.streamSlug === streamSlug && nav.projectSlug === projectSlug) {
          nav.projectSlug = newSlug
          localStorage.setItem(lastNavKey(), JSON.stringify({ brandSlug: nav.brandSlug, streamSlug: nav.streamSlug, projectSlug: newSlug }))
        }
        if (nav.brandSlug === brandSlug && nav.streamSlug === streamSlug && nav.projectSlug === (newSlug !== projectSlug ? newSlug : projectSlug)) nav.projectName = name
        const key = `${brandSlug}/${streamSlug}`
        projectsByStream[key] = await ListProjects(brandSlug, streamSlug) || []
      }
    } catch (e) {
      console.error('Rename:', e)
      showToast(t('error.rename_failed'), 'error')
    }
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
      } catch (e) {
        console.error('CancelRename:', e)
        showToast(t('error.delete_failed'), 'error')
      }
    }
  }

  function startEditRepoDesc() {
    repoDescDraft = repoDescription
    editingRepoDesc = true
    setTimeout(() => { const el = document.querySelector('.repo-desc-input') as HTMLTextAreaElement; el?.focus() }, 0)
  }

  async function commitRepoDesc() {
    editingRepoDesc = false
    const trimmed = repoDescDraft.trim()
    if (trimmed === repoDescription) return
    repoDescription = trimmed
    try {
      await UpdateRepoDescription(trimmed)
    } catch (e) {
      console.error('UpdateRepoDescription:', e)
      showToast(t('error.save_failed'), 'error')
    }
  }

  function cancelRepoDesc() {
    editingRepoDesc = false
    repoDescDraft = repoDescription
  }

  function startEdit(e: MouseEvent, type: 'brand' | 'stream' | 'project', key: string, name: string, brandSlug: string, streamSlug = '', projectSlug = '', description = '') {
    e.stopPropagation()
    renaming = { type, key, name, original: name, description, originalDescription: description, isCreate: false, brandSlug, streamSlug, projectSlug }
    setTimeout(() => { const el = document.querySelector('.rename-input') as HTMLInputElement; el?.focus(); el?.select() }, 0)
  }

  // --- Icon picker handlers ---

  function openIconPicker(e: MouseEvent, type: 'brand' | 'stream' | 'project', brandSlug: string, streamSlug: string, projectSlug: string, currentIcon: string) {
    e.stopPropagation()
    iconPickerTarget = { type, brandSlug, streamSlug, projectSlug, currentIcon }
  }

  async function handleIconSelect(icon: string) {
    if (!iconPickerTarget) return
    const { type, brandSlug, streamSlug, projectSlug } = iconPickerTarget
    try {
      if (type === 'brand') {
        await UpdateBrandIcon(brandSlug, icon)
        const brand = brands.find(b => b.slug === brandSlug)
        if (brand) brand.icon = icon
        brands = [...brands]
      } else if (type === 'stream') {
        await UpdateStreamIcon(brandSlug, streamSlug, icon)
        const streams = streamsByBrand[brandSlug] || []
        const stream = streams.find(s => s.slug === streamSlug)
        if (stream) stream.icon = icon
        streamsByBrand[brandSlug] = [...streams]
      } else {
        await UpdateProjectIcon(brandSlug, streamSlug, projectSlug, icon)
        const key = `${brandSlug}/${streamSlug}`
        const projects = projectsByStream[key] || []
        const project = projects.find(p => p.slug === projectSlug)
        if (project) project.icon = icon
        projectsByStream[key] = [...projects]
      }
    } catch (e) {
      showToast(t('error.save_failed'), 'error')
    }
  }

  // --- Delete handlers ---

  async function handleDeleteBrand(e: MouseEvent, slug: string) {
    e.stopPropagation()
    // Ruling 2026-07-10: delete buttons ALWAYS confirm — empty
    // containers included. (The silent cleanup of an untouched
    // just-created placeholder in cancelRename is an add-cancel and
    // stays promptless.)
    if (!await showConfirm(t('sidebar.confirm_delete_brand', { name: slug }))) return
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
    } catch (e) { showToast(t('error.delete_failed'), 'error') }
  }

  async function handleDeleteStream(e: MouseEvent, brandSlug: string, streamSlug: string) {
    e.stopPropagation()
    // Ruling 2026-07-10: delete buttons always confirm (see
    // handleDeleteBrand).
    if (!await showConfirm(t('sidebar.confirm_delete_stream', { name: streamSlug }))) return
    try {
      await DeleteStream(brandSlug, streamSlug)
      if (nav.brandSlug === brandSlug && nav.streamSlug === streamSlug) {
        nav.streamSlug = null
        nav.projectSlug = null
        board.categories = []
      }
      streamsByBrand[brandSlug] = await ListStreams(brandSlug) || []
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
    } catch (e) { showToast(t('error.delete_failed'), 'error') }
  }

  async function handleDeleteProject(e: MouseEvent, brandSlug: string, streamSlug: string, projectSlug: string) {
    e.stopPropagation()
    if (!await showConfirm(t('sidebar.confirm_delete_project', { name: projectSlug }))) return
    try {
      await DeleteProject(brandSlug, streamSlug, projectSlug)
      if (nav.brandSlug === brandSlug && nav.streamSlug === streamSlug && nav.projectSlug === projectSlug) {
        nav.projectSlug = null
        board.categories = []
      }
      const key = `${brandSlug}/${streamSlug}`
      projectsByStream[key] = await ListProjects(brandSlug, streamSlug) || []
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
    } catch (e) { showToast(t('error.delete_failed'), 'error') }
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
    } catch (err) {
      console.error('Drag/drop:', err)
      showToast(t('error.move_failed'), 'error')
      // Roll back the optimistic splice by reloading the affected
      // level from disk — otherwise the local order stays diverged
      // from persisted order until the next full reload.
      try {
        if (d.type === 'brand') {
          await loadBrands()
        } else if (d.type === 'stream') {
          streamsByBrand[d.brandSlug] = await ListStreams(d.brandSlug) || []
          if (toParent !== d.brandSlug) streamsByBrand[toParent] = await ListStreams(toParent) || []
        } else if (d.type === 'project') {
          const fromKey = `${d.brandSlug}/${d.streamSlug}`
          projectsByStream[fromKey] = await ListProjects(d.brandSlug, d.streamSlug) || []
          if (toParent !== fromKey) {
            const [toBrand, toStream] = toParent.split('/')
            projectsByStream[toParent] = await ListProjects(toBrand, toStream) || []
          }
        }
      } catch { /* reload is best-effort; the toast already fired */ }
    }
  }

  function isDropIndicator(type: string, parentKey: string, index: number): boolean {
    return dropTarget?.type === type && dropTarget?.parentKey === parentKey && dropTarget?.index === index
  }
</script>

<aside class="sidebar" class:collapsed={nav.sidebarCollapsed} style:width="{nav.sidebarCollapsed ? 48 : nav.sidebarWidth}px" style:min-width="{nav.sidebarCollapsed ? 48 : nav.sidebarWidth}px">
  <div class="sidebar-header">
    {#if !nav.sidebarCollapsed}<BruvIcon size={28} />{/if}
    {#if !nav.sidebarCollapsed}<span class="sidebar-title">{t('app.name')}</span>{/if}
    <button class="header-btn" onclick={() => nav.sidebarCollapsed = !nav.sidebarCollapsed} title={nav.sidebarCollapsed ? t('tooltip.expand_sidebar') : t('tooltip.collapse_sidebar')}>
      {#if nav.sidebarCollapsed}<PanelLeftOpen size={20} />{:else}<PanelLeftClose size={20} />{/if}
    </button>
  </div>

  {#if nav.sidebarCollapsed}
    <!-- Collapsed-mode "where am I" stack: back button (when a repo
         is open) + connection icon. Mirrors the expanded chip just
         above; the footer no longer carries this since it's better
         placed at the top alongside the collapse toggle. -->
    <div class="connection-stack">
      {#if nav.repoOpen}
        <button
          class="header-btn"
          onclick={closeRepoAndReturn}
          title={t('tooltip.back_to_picker')}
          aria-label={t('tooltip.back_to_picker')}
        >
          <ChevronLeft size={16} />
        </button>
      {/if}
      {#if connections.available}
        <button
          class="header-btn"
          class:remote={!isLocalActive()}
          onclick={onOpenConnections}
          title={t('connection.indicator_title', { name: activeConnectionLabel() })}
        >
          {#if isLocalActive()}<Monitor size={16} />{:else}<Server size={16} />{/if}
        </button>
      {/if}
    </div>
  {/if}

  {#if !nav.sidebarCollapsed}
    <div class="connection-row">
      {#if nav.repoOpen}
        <button
          class="back-btn"
          onclick={closeRepoAndReturn}
          title={t('tooltip.back_to_picker')}
          aria-label={t('tooltip.back_to_picker')}
        >
          <ChevronLeft size={16} />
        </button>
      {/if}
      <button
        class="connection-chip"
        class:remote={!isLocalActive()}
        onclick={onOpenConnections}
        title={t('connection.indicator_title', { name: activeConnectionLabel() })}
      >
        {#if isLocalActive()}<Monitor size={14} />{:else}<Server size={14} />{/if}
        <span class="connection-chip-label">
          {activeConnectionLabel()}{#if nav.repoOpen && nav.repoName}<span class="chip-sep"> / </span><span class="chip-repo">{nav.repoName}</span>{/if}
        </span>
      </button>
    </div>

    <div class="repo-description-area">
      {#if editingRepoDesc}
        <textarea
          class="repo-desc-input"
          bind:value={repoDescDraft}
          placeholder={t('sidebar.repoDescriptionPlaceholder')}
          rows="2"
          use:inlineEdit={{ multiline: true, onCommit: commitRepoDesc, onCancel: cancelRepoDesc }}
        ></textarea>
      {:else}
        <button class="repo-desc-display" onclick={startEditRepoDesc} title={t('sidebar.repoDescriptionEdit')}>
          {#if repoDescription}
            <span class="repo-desc-text">{repoDescription}</span>
          {:else}
            <span class="repo-desc-placeholder">{t('sidebar.repoDescriptionPlaceholder')}</span>
          {/if}
          <Pencil size={10} />
        </button>
      {/if}
    </div>

    <div class="tree-node inbox-node">
      <div class="tree-row" role="treeitem" tabindex="-1" aria-selected={nav.inboxMode}>
        <button class="tree-item inbox-item" class:selected={nav.inboxMode} onclick={selectInbox}>
          <Inbox size={14} />
          <span class="label">{t('sidebar.inbox')}</span>
          {#if inboxCount > 0}<span class="inbox-badge">{inboxCount}</span>{/if}
        </button>
      </div>
      <div class="tree-row" role="treeitem" tabindex="-1" aria-selected={nav.agentsMode}>
        <button class="tree-item inbox-item" class:selected={nav.agentsMode} onclick={selectAgents}>
          <Timer size={14} />
          <span class="label">{t('sidebar.agents')}</span>
        </button>
      </div>
      <div class="tree-ctrl-group">
        <button class="tree-ctrl-btn" onclick={expandAll} title={t('sidebar.expandAll')}><ChevronsUpDown size={12} /></button>
        <button class="tree-ctrl-btn" onclick={collapseAll} title={t('sidebar.collapseAll')}><ChevronsDownUp size={12} /></button>
        <button
          class="tree-ctrl-btn"
          onclick={toggleAccordionMode}
          aria-label={accordionMode === 'single' ? t('project.mode_single') : t('project.mode_multi')}
          title={accordionMode === 'single' ? t('project.mode_single_hint') : t('project.mode_multi_hint')}
        >
          {#if accordionMode === 'single'}
            <ListCollapse size={12} />
          {:else}
            <ListTree size={12} />
          {/if}
        </button>
      </div>
    </div>

    <div class="nav-tree" role="tree" tabindex="0"
      ondragover={(e) => { if (dragging) { e.preventDefault(); isCopyMode = e.ctrlKey; if (e.dataTransfer) e.dataTransfer.dropEffect = e.ctrlKey ? 'copy' : 'move' } }}
      ondrop={handleDrop}
    >
      {#each brands as brand, brandIdx}
        {#if isDropIndicator('brand', '', brandIdx)}
          <div class="drop-indicator" class:copy-mode={isCopyMode}></div>
        {/if}
        <div class="tree-node">
          <div class="tree-row action-reveal-parent" role="treeitem" tabindex="-1" aria-selected={isSelected(brand.slug, '', '')}
            draggable={renaming?.key !== brand.slug}
            ondragstart={(e) => handleDragStart(e, { type: 'brand', slug: brand.slug })}
            ondragend={handleDragEnd}
            ondragover={(e) => handleDragOverItem(e, 'brand', '', brandIdx)}
            ondrop={handleDrop}
            class:dragging-item={dragging?.type === 'brand' && dragging?.slug === brand.slug}
          >
            {#if renaming && renaming.key === brand.slug}
              <div class="rename-group">
                <input
                  class="rename-input brand-level"
                  bind:value={renaming.name}
                  use:inlineEdit={{ onCommit: () => commitRename(), onCancel: () => cancelRename(), container: '.rename-group' }}
                />
                <textarea
                  class="description-input"
                  bind:value={renaming.description}
                  placeholder={t('sidebar.descriptionPlaceholder')}
                  rows="2"
                  use:inlineEdit={{ multiline: true, onCommit: () => commitRename(), onCancel: () => cancelRename(), container: '.rename-group' }}
                ></textarea>
              </div>
            {:else}
              <button class="tree-item brand-item" onclick={() => toggleBrand(brand.slug)}>
                <span class="chevron">{#if expandedBrands.has(brand.slug)}<ChevronDown size={12} />{:else}<ChevronRight size={12} />{/if}</span>
                {#if brand.icon}
                  <DynamicIcon name={brand.icon} size={14} className="tree-icon" />
                {/if}
                <span class="label-group">
                  <span class="label" title={brand.description ? `${brand.name} — ${brand.description}` : brand.name}>{@html renderInline(brand.name)}</span>
                </span>
              </button>
              <button class="row-action action-reveal action-reveal--danger" onclick={(e) => handleDeleteBrand(e, brand.slug)} title={t('tooltip.delete_brand')}><Trash2 size={12} /></button>
              <button class="row-action action-reveal action-reveal--edit" onclick={(e) => startEdit(e, 'brand', brand.slug, brand.name, brand.slug, '', '', brand.description)} title={t('tooltip.rename_brand')}><Pencil size={12} /></button>
              <button class="row-action action-reveal action-reveal--icon" onclick={(e) => openIconPicker(e, 'brand', brand.slug, '', '', brand.icon || '')} title={t('icon.pick')}><Smile size={12} /></button>
            {/if}
          </div>

          {#if expandedBrands.has(brand.slug) && streamsByBrand[brand.slug]}
            <div class="tree-children">
              {#each streamsByBrand[brand.slug] as stream, streamIdx}
                {#if isDropIndicator('stream', brand.slug, streamIdx)}
                  <div class="drop-indicator" class:copy-mode={isCopyMode}></div>
                {/if}
                <div class="tree-node">
                  <div class="tree-row action-reveal-parent" role="treeitem" tabindex="-1" aria-selected={isSelected(brand.slug, stream.slug, '')}
                    draggable={renaming?.key !== `${brand.slug}/${stream.slug}`}
                    ondragstart={(e) => handleDragStart(e, { type: 'stream', brandSlug: brand.slug, slug: stream.slug })}
                    ondragend={handleDragEnd}
                    ondragover={(e) => handleDragOverItem(e, 'stream', brand.slug, streamIdx)}
                    ondrop={handleDrop}
                    class:dragging-item={dragging?.type === 'stream' && dragging?.slug === stream.slug}
                  >
                    {#if renaming && renaming.key === `${brand.slug}/${stream.slug}`}
                      <div class="rename-group">
                        <input
                          class="rename-input stream-level"
                          bind:value={renaming.name}
                          use:inlineEdit={{ onCommit: () => commitRename(), onCancel: () => cancelRename(), container: '.rename-group' }}
                        />
                        <textarea
                          class="description-input"
                          bind:value={renaming.description}
                          placeholder={t('sidebar.descriptionPlaceholder')}
                          rows="2"
                          onkeydown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); commitRename() } if (e.key === 'Escape') cancelRename() }}
                          onblur={(e) => { const related = (e as FocusEvent).relatedTarget as HTMLElement | null; if (!related || !related.closest('.rename-group')) commitRename() }}
                        ></textarea>
                      </div>
                    {:else}
                      <button class="tree-item stream-item" onclick={() => toggleStream(brand.slug, stream.slug)}>
                        <span class="chevron">{#if expandedStreams.has(`${brand.slug}/${stream.slug}`)}<ChevronDown size={12} />{:else}<ChevronRight size={12} />{/if}</span>
                        {#if stream.icon}
                          <DynamicIcon name={stream.icon} size={14} className="tree-icon" />
                        {/if}
                        <span class="label-group">
                          <span class="label" title={stream.description ? `${stream.name} — ${stream.description}` : stream.name}>{@html renderInline(stream.name)}</span>
                        </span>
                      </button>
                      <button class="row-action action-reveal action-reveal--danger" onclick={(e) => handleDeleteStream(e, brand.slug, stream.slug)} title={t('tooltip.delete_stream')}><Trash2 size={12} /></button>
                      <button class="row-action action-reveal action-reveal--edit" onclick={(e) => startEdit(e, 'stream', `${brand.slug}/${stream.slug}`, stream.name, brand.slug, stream.slug, '', stream.description)} title={t('tooltip.rename_stream')}><Pencil size={12} /></button>
                      <button class="row-action action-reveal action-reveal--icon" onclick={(e) => openIconPicker(e, 'stream', brand.slug, stream.slug, '', stream.icon || '')} title={t('icon.pick')}><Smile size={12} /></button>
                    {/if}
                  </div>

                  {#if expandedStreams.has(`${brand.slug}/${stream.slug}`) && projectsByStream[`${brand.slug}/${stream.slug}`]}
                    <div class="tree-children">
                      {#each projectsByStream[`${brand.slug}/${stream.slug}`] as project, projectIdx}
                        {#if isDropIndicator('project', `${brand.slug}/${stream.slug}`, projectIdx)}
                          <div class="drop-indicator" class:copy-mode={isCopyMode}></div>
                        {/if}
                        <div class="tree-row action-reveal-parent" role="treeitem" tabindex="-1" aria-selected={isSelected(brand.slug, stream.slug, project.slug)}
                          draggable={renaming?.key !== `${brand.slug}/${stream.slug}/${project.slug}`}
                          ondragstart={(e) => handleDragStart(e, { type: 'project', brandSlug: brand.slug, streamSlug: stream.slug, slug: project.slug })}
                          ondragend={handleDragEnd}
                          ondragover={(e) => handleDragOverItem(e, 'project', `${brand.slug}/${stream.slug}`, projectIdx)}
                          ondrop={handleDrop}
                          class:dragging-item={dragging?.type === 'project' && dragging?.slug === project.slug}
                        >
                          {#if renaming && renaming.key === `${brand.slug}/${stream.slug}/${project.slug}`}
                            <div class="rename-group">
                              <input
                                class="rename-input project-level"
                                bind:value={renaming.name}
                                use:inlineEdit={{ onCommit: () => commitRename(), onCancel: () => cancelRename(), container: '.rename-group' }}
                              />
                              <textarea
                                class="description-input"
                                bind:value={renaming.description}
                                placeholder={t('sidebar.descriptionPlaceholder')}
                                rows="2"
                                onkeydown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); commitRename() } if (e.key === 'Escape') cancelRename() }}
                                onblur={(e) => { const related = (e as FocusEvent).relatedTarget as HTMLElement | null; if (!related || !related.closest('.rename-group')) commitRename() }}
                              ></textarea>
                            </div>
                          {:else}
                            <button
                              class="tree-item project-item"
                              class:selected={isSelected(brand.slug, stream.slug, project.slug)}
                              onclick={() => selectProject(brand.slug, stream.slug, project.slug)}
                            >
                              {#if project.icon}
                                <DynamicIcon name={project.icon} size={14} className="tree-icon" />
                              {/if}
                              <span class="label-group">
                                <span class="label" title={project.description ? `${project.name} — ${project.description}` : project.name}>{@html renderInline(project.name)}</span>
                              </span>
                            </button>
                            <button class="row-action action-reveal action-reveal--danger" onclick={(e) => handleDeleteProject(e, brand.slug, stream.slug, project.slug)} title={t('tooltip.delete_project')}><Trash2 size={12} /></button>
                            <button class="row-action action-reveal action-reveal--edit" onclick={(e) => startEdit(e, 'project', `${brand.slug}/${stream.slug}/${project.slug}`, project.name, brand.slug, stream.slug, project.slug, project.description)} title={t('tooltip.rename_project')}><Pencil size={12} /></button>
                            <button class="row-action action-reveal action-reveal--icon" onclick={(e) => openIconPicker(e, 'project', brand.slug, stream.slug, project.slug, project.icon || '')} title={t('icon.pick')}><Smile size={12} /></button>
                          {/if}
                        </div>
                      {/each}
                      {#if isDropIndicator('project', `${brand.slug}/${stream.slug}`, projectsByStream[`${brand.slug}/${stream.slug}`]?.length ?? 0)}
                        <div class="drop-indicator" class:copy-mode={isCopyMode}></div>
                      {/if}

                      <div class="add-btn-row action-reveal-parent">
                        <button class="add-btn nested" onclick={() => handleCreateProject(brand.slug, stream.slug)} title={t('tooltip.add_project')}
                          ondragover={(e) => handleDragOverGap(e, 'project', `${brand.slug}/${stream.slug}`, projectsByStream[`${brand.slug}/${stream.slug}`]?.length ?? 0)}
                          ondrop={handleDrop}
                        >
                          {t('sidebar.add_project')}
                        </button>
                        <button
                          class="add-btn-action action-reveal"
                          onclick={() => openTrelloImport(brand.slug, stream.slug)}
                          title={t('sidebar.import_trello')}
                        ><Upload size={12} /></button>
                      </div>
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
                {t('sidebar.add_stream')}
              </button>
            </div>
          {/if}
        </div>
      {/each}
      {#if isDropIndicator('brand', '', brands.length)}
        <div class="drop-indicator" class:copy-mode={isCopyMode}></div>
      {/if}

      {#if brands.length === 0}
        <p class="empty-hint">{t('sidebar.no_brands')}</p>
      {/if}

      <button class="add-btn" onclick={handleCreateBrand} title={t('tooltip.add_brand')}
        ondragover={(e) => handleDragOverGap(e, 'brand', '', brands.length)}
        ondrop={handleDrop}
      >
        {t('sidebar.add_brand')}
      </button>
    </div>

    <div class="sidebar-footer">
      <button class="footer-btn" onclick={onOpenProfile} title={t('profile.title')}><UserCircle size={16} /></button>
      <span class="footer-spacer"></span>
      <ThemeToggle />
      <button class="footer-btn" onclick={onOpenAbout} title={t('about.title')} aria-label={t('about.title')}><Info size={16} /></button>
      <button class="footer-btn" onclick={onOpenPrefs} title={t('prefs.title')}><Settings size={16} /></button>
    </div>
  {/if}

  {#if nav.sidebarCollapsed}
    <div class="sidebar-footer">
      <button class="footer-btn" onclick={onOpenProfile} title={t('profile.title')}><UserCircle size={16} /></button>
      <span class="footer-spacer"></span>
      <ThemeToggle />
      <button class="footer-btn" onclick={onOpenAbout} title={t('about.title')} aria-label={t('about.title')}><Info size={16} /></button>
      <button class="footer-btn" onclick={onOpenPrefs} title={t('prefs.title')}><Settings size={16} /></button>
    </div>
  {/if}
</aside>

{#if iconPickerTarget}
  <IconPicker
    value={iconPickerTarget.currentIcon}
    onSelect={handleIconSelect}
    onClose={() => iconPickerTarget = null}
  />
{/if}

{#if trelloImportTarget}
  <ImportTrelloDialog
    brandSlug={trelloImportTarget.brandSlug}
    streamSlug={trelloImportTarget.streamSlug}
    onClose={() => trelloImportTarget = null}
    onImported={handleTrelloImportDone}
  />
{/if}

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
  .collapsed .sidebar-header {
    justify-content: center;
    padding: 0.75rem 0.5rem;
  }
  .collapsed .sidebar-footer {
    flex-direction: column;
    align-items: center;
    gap: 0.15rem;
    padding: 0.5rem 0.25rem;
  }
  .collapsed .footer-spacer {
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

  /* Top-of-sidebar row: back-to-picker chevron (only when a repo is
     open) + connection chip. The chevron's "back" motif matches what
     CloseRepository actually does — return to the welcome picker —
     better than the old "Close Repository" framing implied. */
  .connection-row {
    display: flex;
    align-items: stretch;
    gap: 0.4rem;
    margin: 0.5rem;
  }
  .back-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0 0.35rem;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-elevated);
    color: var(--text-muted);
    cursor: pointer;
    transition: background 0.15s, color 0.15s, border-color 0.15s;
  }
  .back-btn:hover {
    background: var(--border);
    color: var(--text-strong);
    border-color: var(--border-hover);
  }
  .connection-chip {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    padding: 0.45rem 0.75rem;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-elevated);
    color: var(--text-secondary);
    font-size: 0.8rem;
    font-weight: 500;
    cursor: pointer;
    transition: background 0.15s, color 0.15s, border-color 0.15s;
    min-width: 0;
  }
  .connection-chip:hover {
    background: var(--border);
    color: var(--text-strong);
    border-color: var(--border-hover);
  }
  .connection-chip.remote {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 35%, var(--border));
  }
  .connection-chip.remote:hover {
    background: color-mix(in srgb, var(--accent) 10%, transparent);
    color: var(--accent);
  }
  .connection-chip-label {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .chip-sep { color: var(--text-muted); margin: 0 0.05rem; }
  .chip-repo { color: var(--text-strong); font-weight: 600; }
  .connection-chip.remote .chip-repo { color: var(--accent); }

  .repo-description-area {
    padding: 0 0.5rem;
  }
  .repo-desc-display {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    width: 100%;
    padding: 0.3rem 0.5rem;
    border: 1px dashed transparent;
    border-radius: 4px;
    background: none;
    color: var(--text-secondary);
    font-size: 0.75rem;
    line-height: 1.3;
    cursor: pointer;
    text-align: left;
    transition: border-color 0.15s;
  }
  .repo-desc-display:hover {
    border-color: var(--border);
  }
  .repo-desc-display :global(svg) {
    flex-shrink: 0;
    opacity: 0;
    transition: opacity 0.15s;
  }
  .repo-desc-display:hover :global(svg) {
    opacity: 0.6;
  }
  .repo-desc-text {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .repo-desc-placeholder {
    flex: 1;
    font-style: italic;
    opacity: 0.5;
  }
  .repo-desc-input {
    width: 100%;
    padding: 0.3rem 0.5rem;
    border: 1px solid var(--accent);
    border-radius: 4px;
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.75rem;
    font-family: inherit;
    line-height: 1.3;
    resize: vertical;
  }

  .inbox-node {
    position: relative;
    padding-top: 0.4rem;
    padding-bottom: 0.4rem;
    margin-bottom: 0.25rem;
    border-bottom: 1px solid var(--border-muted);
  }
  .tree-ctrl-group {
    position: absolute;
    bottom: 0;
    right: 0.5rem;
    transform: translateY(50%);
    display: flex;
    gap: 0.15rem;
    z-index: 1;
    background: var(--bg-surface);
    border: 1px solid var(--border-muted);
    border-radius: 4px;
    padding: 0 0.1rem;
  }
  .tree-ctrl-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0.1rem;
    border: none;
    border-radius: 3px;
    background: none;
    color: var(--text-muted);
    cursor: pointer;
    transition: color 0.12s, background 0.12s;
  }
  .tree-ctrl-btn:hover {
    color: var(--text-strong);
    background: var(--bg-elevated);
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
    transition: background var(--duration-fast) var(--ease-out), color var(--duration-fast);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    position: relative;
  }

  .tree-item:hover {
    background: var(--accent-glow-2);
  }

  .tree-item.selected {
    background: var(--accent-glow-1);
    color: var(--text-primary);
    font-weight: 500;
  }

  .inbox-item {
    gap: 0.5rem;
    border-radius: 0;
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
    display: inline-flex;
    transition: transform var(--duration-moderate) var(--ease-out);
  }

  .tree-children {
    padding-left: 1rem;
    animation: fade-in-up var(--duration-moderate) var(--ease-out);
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
    position: relative;
  }

  .tree-row:active {
    cursor: grabbing;
  }

  .tree-row .tree-item {
    flex: 1;
    min-width: 0;
  }

  .row-action {
    font-size: 0.7rem;
    flex-shrink: 0;
    position: absolute;
    right: 0.25rem;
    top: 50%;
    transform: translateY(-50%);
    background: transparent;
    padding: 0.15rem 0.2rem;
    transition: color var(--duration-fast), background var(--duration-fast);
  }
  /* Let hover events pass through the SVG so the button's title attribute
     (HTML tooltip) applies — otherwise hovering the icon interior hits the
     svg, which has no SVG <title> child, and no tooltip is shown. */
  .row-action :global(svg) {
    pointer-events: none;
  }
  .row-action + .row-action {
    right: 1.3rem;
  }
  .row-action + .row-action + .row-action {
    right: 2.35rem;
  }
  /* Keep backgrounds transparent on hover — icons overlay the row */
  .tree-row .row-action:hover {
    background: transparent;
  }
  /* Brighter idle icon color on the selected row so they read against the stronger purple */
  .tree-row:has(.tree-item.selected):hover .row-action {
    color: var(--text-primary);
  }

  /* Whole-row hover highlight (so action icons and label share the same background) */
  .tree-row.action-reveal-parent:hover .tree-item:not(.selected) {
    background: var(--accent-glow-2);
  }

  /* Fade the label text into the row background behind the revealed icons.
     Masking the label-group (not the whole tree-item) keeps the button's
     own background intact, so the fade ends in the exact row color without
     stacking an extra translucent layer on top. */
  .tree-row.action-reveal-parent:hover .label-group {
    -webkit-mask-image: linear-gradient(to right, #000 calc(100% - 3.75rem), transparent calc(100% - 2.75rem));
    mask-image: linear-gradient(to right, #000 calc(100% - 3.75rem), transparent calc(100% - 2.75rem));
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

  .add-btn-row {
    position: relative;
    display: flex;
    align-items: center;
  }
  .add-btn-row .add-btn {
    flex: 1;
  }
  /* Visibility is driven by the shared .action-reveal utility —
     transparent by default, revealed on .action-reveal-parent:hover. */
  .add-btn-action {
    padding: 0.25rem 0.5rem;
    margin-right: 0.25rem;
    display: flex;
    align-items: center;
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

  .rename-group {
    display: flex;
    flex-direction: column;
    flex: 1;
    gap: 0.25rem;
  }

  .description-input {
    flex: 1;
    padding: 0.25rem 0.5rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.75rem;
    outline: none;
    resize: none;
    box-sizing: border-box;
    margin-left: 0.5rem;
    margin-right: 0.5rem;
    font-family: inherit;
  }

  .label-group {
    min-width: 0;
    flex: 1;
    overflow: hidden;
  }
  .label-group .label {
    display: block;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
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

  /* Collapsed-mode "where am I" stack: lives just under the header
     in the same icon-only column as the collapse-toggle. The .remote
     modifier on a stacked icon tints when the active connection is
     non-local — same posture the rich expanded chip uses. */
  .connection-stack {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.25rem;
    padding: 0.25rem 0;
  }
  .header-btn.remote {
    color: var(--accent);
  }
  .header-btn.remote:hover {
    color: var(--accent);
    background: color-mix(in srgb, var(--accent) 12%, transparent);
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
