<script lang="ts">
  import { RefreshCw, Unlink, Briefcase, Plus, AlertTriangle, ChevronDown, ChevronRight, ChevronsUpDown, ChevronsDownUp, ListCollapse, ListTree, MonitorSmartphone } from 'lucide-svelte'
  import { DetachWorkspace, GetWorkspaceState, ListWorkspaceDir, RefreshWorkspaceIndex } from '@shared/api'
  import type { Workspace, WorkspaceCheckoutInfo, WorkspaceState } from '@shared/types'
  import { t } from '../../lib/i18n.svelte'
  import { showToast } from '../../lib/toast.svelte'
  import { showConfirm } from '../../lib/confirm.svelte'
  import { onEvent } from '../../lib/events'
  import { createWorkspaceDirCache } from '../../lib/workspaceTree.svelte'
  import { refreshWorkspaceDirs } from '../../lib/workspaceLocations.svelte'
  import WorkspaceFileTree from './WorkspaceFileTree.svelte'
  import WorkspaceFileViewer from './WorkspaceFileViewer.svelte'
  import AttachWorkspaceDialog from './AttachWorkspaceDialog.svelte'
  import WorkspaceStructureActions from './WorkspaceStructureActions.svelte'
  import WorkspaceDeviceSection from './WorkspaceDeviceSection.svelte'
  import { activeConnectionLabel, isLocalActive } from '../../lib/connections.svelte'

  // Geometry-less: fills its host (SidePanel tab pane), which owns width,
  // resize, slide animations, and closing.
  //
  // Three sections, in this order (plan/2026-09-17 workspace files block.md
  // §5): FILES — the tree with the structure actions, the working surface;
  // ON THIS DEVICE — clone/open/launch, collapsed by default so editing
  // never reads as part of cloning; DETAILS — adapter, origin, warnings.
  let { brandSlug, streamSlug, projectSlug, openRequest = null, onRequestHandled }: {
    brandSlug: string
    streamSlug: string
    projectSlug: string
    // A workspace://<ws-id>/<path> card link asking to open a file.
    openRequest?: { wsId: string; path: string } | null
    onRequestHandled?: () => void
  } = $props()

  let wsState = $state<WorkspaceState | null>(null)
  let refreshing = $state(false)
  let showAttach = $state(false)
  let openFilePath = $state<string | null>(null)
  let structure = $state<WorkspaceStructureActions | null>(null)

  // On a remote connection the workspace's files sit on the server, so this
  // device's Tier 1 actions need its own working copy — which may or may not
  // exist yet. WorkspaceLocalCopy owns that lifecycle and reports it here.
  const serverIsThisMachine = isLocalActive()
  const serverName = activeConnectionLabel()
  let checkout = $state<WorkspaceCheckoutInfo | null>(null)

  // Section expandos, persisted per device. Files is always open.
  const DEVICE_KEY = 'bruv:wsDeviceCollapsed'
  const DETAILS_KEY = 'bruv:wsDetailsCollapsed'
  let deviceCollapsed = $state(localStorage.getItem(DEVICE_KEY) !== '0')
  let detailsCollapsed = $state(localStorage.getItem(DETAILS_KEY) !== '0')
  function toggleDevice() {
    deviceCollapsed = !deviceCollapsed
    localStorage.setItem(DEVICE_KEY, deviceCollapsed ? '1' : '0')
  }
  function toggleDetails() {
    detailsCollapsed = !detailsCollapsed
    localStorage.setItem(DETAILS_KEY, detailsCollapsed ? '1' : '0')
  }

  // The file tree browses lazily: one RPC per directory, on first expand.
  // The cache lives here (root consumer) so Refresh can drop it wholesale and
  // every level shares one store. The loader reads the slug props at call
  // time, so it always targets the project currently on screen.
  const dirCache = createWorkspaceDirCache((rel) => ListWorkspaceDir(brandSlug, streamSlug, projectSlug, rel))

  // Tree expand state, hoisted here so Expand All / Collapse All / the
  // single-multi accordion mode work across the whole recursive tree.
  // Same persistence key style as the Sidebar's project tree. NOTE the
  // polarity: a folder is expanded only when `treeCollapsed[path] === false`,
  // so the default (absent) is COLLAPSED and the tree opens closed.
  const TREE_MODE_KEY = 'bruv:wsTreeAccordion'
  let treeCollapsed = $state<Record<string, boolean>>({})
  let treeMode = $state<'single' | 'multi'>(localStorage.getItem(TREE_MODE_KEY) === 'single' ? 'single' : 'multi')
  function toggleTreeMode() {
    treeMode = treeMode === 'single' ? 'multi' : 'single'
    localStorage.setItem(TREE_MODE_KEY, treeMode)
  }
  // BOUNDED BY DESIGN: "expand everything" would mean walking the whole
  // workspace again — the exact cost lazy loading exists to avoid. So this
  // expands only the levels already in the cache and fetches nothing; the
  // button is labelled "Expand loaded folders" to say so.
  function expandLoadedTree() {
    const m: Record<string, boolean> = { ...treeCollapsed }
    for (const d of dirCache.loadedDirs()) {
      if (d !== '') m[d] = false
    }
    treeCollapsed = m
  }
  // Absent = collapsed, so collapsing everything is just dropping the record.
  function collapseAllTree() {
    treeCollapsed = {}
  }

  async function load() {
    try {
      wsState = await GetWorkspaceState(brandSlug, streamSlug, projectSlug)
    } catch (e) {
      showToast(t('workspace.load_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    }
  }

  $effect(() => {
    // Reload when the project changes and on live workspace events.
    void brandSlug; void streamSlug; void projectSlug
    // A different project is a different filesystem — drop the cached
    // directories (and the expand state that indexed them) before reloading.
    resetTree()
    load()
    const off = onEvent<{ brand_slug?: string; project_slug?: string }>('workspace:updated', (ev) => {
      if (!ev.project_slug || ev.project_slug === projectSlug) load()
    })
    const offDel = onEvent<{ project_slug?: string }>('workspace:deleted', (ev) => {
      if (!ev.project_slug || ev.project_slug === projectSlug) load()
    })
    return () => { off(); offDel() }
  })

  // Refresh re-reads from disk: the cached directory listings are exactly
  // what must not survive it. Mounted levels reload themselves on the next
  // render, so only what's visible is refetched.
  function resetTree() {
    dirCache.clear()
    treeCollapsed = {}
  }

  async function refresh() {
    refreshing = true
    try {
      await RefreshWorkspaceIndex(brandSlug, streamSlug, projectSlug)
      resetTree()
      await load()
    } catch (e) {
      showToast(t('workspace.refresh_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    } finally {
      refreshing = false
    }
  }

  async function detach() {
    const ok = await showConfirm(t('workspace.detach_confirm'))
    if (!ok) return
    try {
      await DetachWorkspace(brandSlug, streamSlug, projectSlug)
      showToast(t('workspace.detached'), 'success')
      await load()
    } catch (e) {
      showToast(t('workspace.detach_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    }
  }

  function onAttached(_ws: Workspace) {
    showAttach = false
    load()
  }

  // A structure change (new file/folder, generated template): re-read the
  // parent so the new entry appears, expand to it, and open a new file
  // straight into the editor — creating a chapter and then hunting for it
  // is the friction this exists to remove.
  function onCreated(rel: string, kind: 'file' | 'dir') {
    const parent = rel.includes('/') ? rel.slice(0, rel.lastIndexOf('/')) : ''
    void dirCache.reload(parent)
    if (ws) refreshWorkspaceDirs(ws.id)
    let p = ''
    for (const seg of parent.split('/').filter(Boolean)) {
      p = p ? `${p}/${seg}` : seg
      treeCollapsed[p] = false
    }
    if (kind === 'file') openFilePath = rel
    else treeCollapsed[rel] = false
  }

  const ws = $derived(wsState?.workspace)
  const idx = $derived(wsState?.index)

  // The folder THIS device can open: the origin itself when the vault is
  // served from this machine, otherwise this device's clone. Undefined
  // means there is nothing local to act on, and the actions that would need
  // it are hidden rather than offered and then failing.
  const deviceRoot = $derived(
    serverIsThisMachine ? ws?.origin.url : (checkout?.has_copy ? checkout.local_path : undefined),
  )

  // Card-link open requests resolve once the workspace state is loaded.
  // Links are scoped to their own project's workspace in v1 — a link whose
  // ws-id doesn't match gets a clear message instead of the wrong file.
  $effect(() => {
    if (!openRequest || wsState === null) return
    if (!wsState.attached || !wsState.workspace) {
      showToast(t('workspace.link_no_workspace'), 'error')
    } else if (openRequest.wsId && wsState.workspace.id !== openRequest.wsId) {
      showToast(t('workspace.link_other_workspace'), 'error')
    } else if (openRequest.path) {
      openFilePath = openRequest.path
    }
    onRequestHandled?.()
  })
</script>

<aside class="workspace-panel">
  <header>
    <span class="title"><Briefcase size={15} /> {t('workspace.title')}</span>
    <div class="actions">
      {#if wsState?.attached}
        <button class="icon-btn" class:spin={refreshing} onclick={refresh} title={t('workspace.refresh')} aria-label={t('workspace.refresh')}><RefreshCw size={14} /></button>
        <button class="icon-btn danger" onclick={detach} title={t('workspace.detach')} aria-label={t('workspace.detach')}><Unlink size={14} /></button>
      {/if}
    </div>
  </header>

  {#if wsState === null}
    <p class="muted">{t('common.loading')}</p>
  {:else if !wsState.attached}
    <div class="empty">
      <Briefcase size={28} />
      <p>{t('workspace.empty_hint')}</p>
      <button class="btn primary" onclick={() => showAttach = true}><Plus size={14} /> {t('workspace.attach_action')}</button>
    </div>
  {:else if ws}
    <div class="body">
      <!-- The tree owns its own loading/error rows per directory, so the
           panel paints immediately and the root listing streams in. -->
      <section class="files">
        <div class="section-head">
          <span class="label">{t('workspace.files')}</span>
          <WorkspaceStructureActions bind:this={structure} {brandSlug} {streamSlug} {projectSlug} {onCreated} compact />
          <div class="tree-ctrl-group">
            <!-- Not `sidebar.expandAll`: this one can only expand cached
                 levels, and the label must not promise the whole tree. -->
            <button class="tree-ctrl-btn" onclick={expandLoadedTree} title={t('workspace.expand_loaded')} aria-label={t('workspace.expand_loaded')}><ChevronsUpDown size={12} /></button>
            <button class="tree-ctrl-btn" onclick={collapseAllTree} title={t('sidebar.collapseAll')} aria-label={t('sidebar.collapseAll')}><ChevronsDownUp size={12} /></button>
            <button
              class="tree-ctrl-btn"
              onclick={toggleTreeMode}
              aria-label={treeMode === 'single' ? t('project.mode_single') : t('project.mode_multi')}
              title={treeMode === 'single' ? t('project.mode_single_hint') : t('project.mode_multi_hint')}
            >
              {#if treeMode === 'single'}<ListCollapse size={12} />{:else}<ListTree size={12} />{/if}
            </button>
          </div>
        </div>
        <div class="tree-scroll">
          <WorkspaceFileTree
            cache={dirCache}
            collapsed={treeCollapsed}
            mode={treeMode}
            workspaceId={ws.id}
            onOpenFile={(p) => openFilePath = p}
            onCreateIn={(dir, kind) => structure?.open(dir, kind)}
          />
        </div>
      </section>

      <section class="expando">
        <button class="section-toggle" onclick={toggleDevice} aria-expanded={!deviceCollapsed}>
          {#if deviceCollapsed}<ChevronRight size={13} />{:else}<ChevronDown size={13} />{/if}
          <MonitorSmartphone size={12} />
          <span class="label">{t('workspace.on_this_device')}</span>
          <span class="badge tier">{serverIsThisMachine || checkout?.has_copy ? t('workspace.tier_local') : t('workspace.tier_remote', { server: serverName })}</span>
        </button>
        {#if !deviceCollapsed}
          <WorkspaceDeviceSection
            {ws} {brandSlug} {streamSlug} {projectSlug} {serverIsThisMachine} {serverName} {deviceRoot}
            onWorkspaceChange={(next) => { if (wsState) wsState = { ...wsState, workspace: next } }}
            onCheckoutChange={(info) => checkout = info}
          />
        {/if}
      </section>

      <section class="expando">
        <button class="section-toggle" onclick={toggleDetails} aria-expanded={!detailsCollapsed}>
          {#if detailsCollapsed}<ChevronRight size={13} />{:else}<ChevronDown size={13} />{/if}
          <span class="label">{t('workspace.details')}</span>
          <span class="badge adapter">{ws.adapter}</span>
        </button>
        {#if !detailsCollapsed}
          <div class="meta-card">
            {#if idx?.summary}
              <p class="summary">{idx.summary}</p>
            {/if}
            {#if ws.origin.url}
              <p class="origin" title={ws.origin.url}>{ws.origin.url}</p>
            {/if}
          </div>
          {#if idx?.warnings?.length}
            {#each idx.warnings as w (w)}
              <p class="warning"><AlertTriangle size={12} /> {w}</p>
            {/each}
          {/if}
        {/if}
      </section>
    </div>
  {/if}
</aside>

{#if showAttach}
  <AttachWorkspaceDialog {brandSlug} {streamSlug} {projectSlug} {onAttached} onClose={() => showAttach = false} />
{/if}

{#if openFilePath && ws}
  <WorkspaceFileViewer {brandSlug} {streamSlug} {projectSlug} root={ws.origin.url ?? ''} path={openFilePath} onClose={() => openFilePath = null} />
{/if}

<style>
  /* Three-shade ladder, mirroring Project Chat (header bg-base →
     content well bg-surface → cards bg-elevated) so the two tabs share
     one visual language. */
  .workspace-panel {
    width: 100%;
    height: 100%;
    display: flex;
    flex-direction: column;
    background: var(--bg-surface);
    overflow: hidden;
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.55rem 0.75rem;
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
    background: var(--bg-base);
  }
  .title {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font-size: 0.82rem;
    font-weight: 600;
    color: var(--text-strong);
  }
  .actions { display: flex; gap: 0.15rem; }
  .icon-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.3rem;
    border-radius: 6px;
    display: flex;
  }
  .icon-btn:hover { color: var(--text-primary); background: var(--bg-subtle-hover); }
  .icon-btn.danger:hover { color: var(--danger, #ef4444); }
  .icon-btn.spin :global(svg) { animation: spin 0.9s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }

  .empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.7rem;
    padding: 2.2rem 1.2rem;
    color: var(--text-faint);
    text-align: center;
  }
  .empty p { margin: 0; font-size: 0.82rem; color: var(--text-muted); }

  .body {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .files {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    padding: 0.6rem 0.5rem 0.6rem 0.75rem;
  }
  .section-head {
    display: flex;
    align-items: center;
    gap: 0.35rem;
  }
  .section-head .label { flex: 1; }
  .tree-scroll {
    flex: 1;
    overflow: auto;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.3rem;
  }
  .tree-ctrl-group {
    display: flex;
    gap: 0.15rem;
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
  .tree-ctrl-btn:hover,
  .tree-ctrl-btn:focus-visible {
    color: var(--text-strong);
    background: var(--bg-elevated);
  }
  .expando {
    padding: 0.5rem 0.75rem;
    border-top: 1px solid var(--border-muted);
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
    flex-shrink: 0;
    max-height: 45%;
    overflow: auto;
  }
  .section-toggle {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.1rem 0;
    border: none;
    background: none;
    color: var(--text-muted);
    cursor: pointer;
    text-align: left;
  }
  .section-toggle .label { flex: 1; }
  .section-toggle:hover .label,
  .section-toggle:focus-visible .label {
    color: var(--text-primary);
  }
  .label {
    font-size: 0.66rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-faint);
  }
  .badge {
    font-size: 0.66rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    padding: 0.15rem 0.45rem;
    border-radius: 999px;
    border: 1px solid var(--border);
    color: var(--text-muted);
  }
  .badge.tier { color: var(--accent); border-color: var(--accent); }
  /* Elevated container + near-black/white body text — matches the
     Sidebar/Chat contrast hierarchy (see UI-CONVENTIONS §13). */
  .meta-card {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.55rem 0.65rem;
  }
  .origin {
    margin: 0;
    font-size: 0.7rem;
    font-family: var(--font-mono, monospace);
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .summary { margin: 0; font-size: 0.8rem; color: var(--text-body); line-height: 1.45; }
  .warning {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    margin: 0;
    font-size: 0.72rem;
    color: var(--warning, #f59e0b);
  }
  .muted { color: var(--text-faint); font-size: 0.78rem; padding: 0.6rem 0.75rem; }
</style>
