<script lang="ts">
  import { RefreshCw, Unlink, Briefcase, Plus, AlertTriangle, FolderOpen, Play, ChevronDown, ChevronRight, ChevronsUpDown, ChevronsDownUp, ListCollapse, ListTree } from 'lucide-svelte'
  import { DetachWorkspace, GetWorkspaceState, OpenWorkspacePath, RefreshWorkspaceIndex, RunWorkspaceLaunchCommand, SetWorkspaceLaunchCommand } from '@shared/api'
  import type { Workspace, WorkspaceState } from '@shared/types'
  import { t } from '../../lib/i18n.svelte'
  import { showToast } from '../../lib/toast.svelte'
  import { showConfirm } from '../../lib/confirm.svelte'
  import { onEvent } from '../../lib/events'
  import EditableText from '../EditableText.svelte'
  import WorkspaceFileTree from './WorkspaceFileTree.svelte'
  import WorkspaceFileViewer from './WorkspaceFileViewer.svelte'
  import AttachWorkspaceDialog from './AttachWorkspaceDialog.svelte'

  // Geometry-less: fills its host (SidePanel tab pane), which owns width,
  // resize, slide animations, and closing.
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

  // Details expando — the meta section is reference info, the tree is the
  // working surface; let the former fold away. Persisted per device.
  const META_COLLAPSED_KEY = 'bruv:wsDetailsCollapsed'
  let metaCollapsed = $state(localStorage.getItem(META_COLLAPSED_KEY) === '1')
  function toggleMeta() {
    metaCollapsed = !metaCollapsed
    localStorage.setItem(META_COLLAPSED_KEY, metaCollapsed ? '1' : '0')
  }

  // Tree expand state, hoisted here so Expand All / Collapse All / the
  // single-multi accordion mode work across the whole recursive tree.
  // Same semantics + persistence key style as the Sidebar's project tree.
  const TREE_MODE_KEY = 'bruv:wsTreeAccordion'
  let treeCollapsed = $state<Record<string, boolean>>({})
  let treeMode = $state<'single' | 'multi'>(localStorage.getItem(TREE_MODE_KEY) === 'single' ? 'single' : 'multi')
  function toggleTreeMode() {
    treeMode = treeMode === 'single' ? 'multi' : 'single'
    localStorage.setItem(TREE_MODE_KEY, treeMode)
  }
  function expandAllTree() {
    treeCollapsed = {}
  }
  function collapseAllTree() {
    const m: Record<string, boolean> = {}
    for (const e of idx?.tree ?? []) {
      if (e.is_dir) m[e.path] = true
    }
    treeCollapsed = m
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
    load()
    const off = onEvent<{ brand_slug?: string; project_slug?: string }>('workspace:updated', (ev) => {
      if (!ev.project_slug || ev.project_slug === projectSlug) load()
    })
    const offDel = onEvent<{ project_slug?: string }>('workspace:deleted', (ev) => {
      if (!ev.project_slug || ev.project_slug === projectSlug) load()
    })
    return () => { off(); offDel() }
  })

  async function refresh() {
    refreshing = true
    try {
      await RefreshWorkspaceIndex(brandSlug, streamSlug, projectSlug)
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

  async function saveLaunchCommand(cmd: string) {
    try {
      const ws = await SetWorkspaceLaunchCommand(brandSlug, streamSlug, projectSlug, cmd)
      if (wsState) wsState = { ...wsState, workspace: ws }
    } catch {
      showToast(t('error.save_failed'), 'error')
    }
  }

  function onAttached(_ws: Workspace) {
    showAttach = false
    load()
  }

  async function openFolder() {
    if (!ws?.origin.url) return
    try {
      await OpenWorkspacePath(ws.origin.url, '')
    } catch (e) {
      showToast(t('workspace.open_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    }
  }

  async function runLaunch() {
    if (!ws?.origin.url || !ws.launch_command) return
    try {
      await RunWorkspaceLaunchCommand(ws.origin.url, ws.launch_command)
    } catch (e) {
      showToast(t('workspace.launch_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    }
  }

  const ws = $derived(wsState?.workspace)
  const idx = $derived(wsState?.index)

  // Pick-to-fill launch suggestions, ordered by adapter (an Obsidian vault
  // most likely opens in Obsidian; a repo in an editor). Free text stays
  // the model — these just save you knowing that VS Code's binary is
  // `code`, not `vscode`.
  const launchSuggestions = $derived.by(() => {
    if (!ws) return []
    const obsidian = {
      label: t('workspace.launch_suggest_obsidian'),
      command: `obsidian://open?path=${encodeURIComponent(ws.origin.url ?? '')}`,
    }
    const vscode = { label: t('workspace.launch_suggest_vscode'), command: 'code .' }
    const terminal = { label: t('workspace.launch_suggest_terminal'), command: 'wt -d .' }
    return ws.adapter === 'obsidian-vault' ? [obsidian, vscode, terminal] : [vscode, terminal, obsidian]
  })

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
      <section class="meta">
        <button class="section-toggle" onclick={toggleMeta} aria-expanded={!metaCollapsed}>
          {#if metaCollapsed}<ChevronRight size={13} />{:else}<ChevronDown size={13} />{/if}
          <span class="label">{t('workspace.details')}</span>
          {#if metaCollapsed}
            <span class="badge adapter">{ws.adapter}</span>
          {/if}
        </button>
        {#if !metaCollapsed}
        <div class="badge-row">
          <span class="badge adapter">{ws.adapter}</span>
          <span class="badge tier">{t('workspace.tier_local')}</span>
        </div>
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
        <!-- Open/Launch stay reachable with Details collapsed — they're
             the two actions in constant rotation. -->
        <div class="action-row">
          <button class="btn" onclick={openFolder}><FolderOpen size={13} /> {t('workspace.open_folder')}</button>
          {#if ws.launch_command}
            <button class="btn" onclick={runLaunch} title={ws.launch_command}><Play size={13} /> {t('workspace.launch')}</button>
          {/if}
        </div>
        {#if !metaCollapsed}
        <div class="launch">
          <span class="label">{t('workspace.launch_command')}</span>
          <EditableText
            value={ws.launch_command ?? ''}
            placeholder={t('workspace.launch_placeholder')}
            onSave={saveLaunchCommand}
          />
          {#if !ws.launch_command}
            <p class="hint">{t('workspace.launch_hint')}</p>
            <div class="chip-row">
              {#each launchSuggestions as s (s.label)}
                <button class="chip" title={s.command} onclick={() => saveLaunchCommand(s.command)}>{s.label}</button>
              {/each}
            </div>
          {/if}
        </div>
        {/if}
        <!-- Straddles the meta/files divider, same as the Sidebar's
             cluster over the project-tree divider. -->
        <div class="tree-ctrl-group">
          <button class="tree-ctrl-btn" onclick={expandAllTree} title={t('sidebar.expandAll')}><ChevronsUpDown size={12} /></button>
          <button class="tree-ctrl-btn" onclick={collapseAllTree} title={t('sidebar.collapseAll')}><ChevronsDownUp size={12} /></button>
          <button
            class="tree-ctrl-btn"
            onclick={toggleTreeMode}
            aria-label={treeMode === 'single' ? t('project.mode_single') : t('project.mode_multi')}
            title={treeMode === 'single' ? t('project.mode_single_hint') : t('project.mode_multi_hint')}
          >
            {#if treeMode === 'single'}
              <ListCollapse size={12} />
            {:else}
              <ListTree size={12} />
            {/if}
          </button>
        </div>
      </section>

      <section class="files">
        <span class="label">{t('workspace.files')}</span>
        {#if idx && idx.tree?.length > 0}
          <div class="tree-scroll">
            <WorkspaceFileTree entries={idx.tree} collapsed={treeCollapsed} mode={treeMode} onOpenFile={(p) => openFilePath = p} />
          </div>
        {:else}
          <p class="muted">{t('workspace.no_index')}</p>
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
  .meta {
    position: relative; /* anchors .tree-ctrl-group on the divider */
    padding: 0.7rem 0.75rem;
    border-bottom: 1px solid var(--border-muted);
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
    flex-shrink: 0;
  }
  .badge-row { display: flex; gap: 0.35rem; }
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
  .label {
    font-size: 0.66rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-faint);
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
  .section-toggle:hover .label,
  .section-toggle:focus-visible .label {
    color: var(--text-primary);
  }
  /* Same control cluster as the Sidebar's project tree: centred over the
     section divider (absolute at the meta section's bottom edge). */
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
  .tree-ctrl-btn:hover,
  .tree-ctrl-btn:focus-visible {
    color: var(--text-strong);
    background: var(--bg-elevated);
  }
  .launch { display: flex; flex-direction: column; gap: 0.2rem; font-size: 0.78rem; }
  .hint {
    margin: 0.1rem 0 0;
    font-size: 0.72rem;
    color: var(--text-muted);
    line-height: 1.4;
  }
  .chip-row { display: flex; flex-wrap: wrap; gap: 0.3rem; margin-top: 0.25rem; }
  .chip {
    padding: 0.2rem 0.55rem;
    font-size: 0.72rem;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--bg-elevated);
    color: var(--text-secondary);
    cursor: pointer;
    transition: color 0.12s, border-color 0.12s;
  }
  .chip:hover,
  .chip:focus-visible {
    color: var(--text-primary);
    border-color: var(--accent);
  }
  .action-row { display: flex; gap: 0.4rem; flex-wrap: wrap; }
  .action-row .btn { padding: 0.28rem 0.6rem; font-size: 0.74rem; }

  .files {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    padding: 0.6rem 0.5rem 0.6rem 0.75rem;
    overflow: hidden;
  }
  .tree-scroll {
    flex: 1;
    overflow: auto;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.3rem;
  }
  .muted { color: var(--text-faint); font-size: 0.78rem; padding: 0.6rem 0.75rem; }
  .btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-secondary);
    border-radius: 6px;
    padding: 0.4rem 0.8rem;
    font-size: 0.78rem;
    cursor: pointer;
  }
  .btn.primary { background: var(--accent); border-color: var(--accent); color: white; }
  .btn.primary:hover { filter: brightness(1.08); }
</style>
