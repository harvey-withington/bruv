<script lang="ts">
  import { fly } from 'svelte/transition'
  import { X, RefreshCw, Unlink, Briefcase, Plus, AlertTriangle } from 'lucide-svelte'
  import { DetachWorkspace, GetWorkspaceState, RefreshWorkspaceIndex, SetWorkspaceLaunchCommand } from '@shared/api'
  import type { Workspace, WorkspaceState } from '@shared/types'
  import { t } from '../../lib/i18n.svelte'
  import { showToast } from '../../lib/toast.svelte'
  import { showConfirm } from '../../lib/confirm.svelte'
  import { onEvent } from '../../lib/events'
  import EditableText from '../EditableText.svelte'
  import WorkspaceFileTree from './WorkspaceFileTree.svelte'
  import WorkspaceFileViewer from './WorkspaceFileViewer.svelte'
  import AttachWorkspaceDialog from './AttachWorkspaceDialog.svelte'

  let { brandSlug, streamSlug, projectSlug, onClose }: {
    brandSlug: string
    streamSlug: string
    projectSlug: string
    onClose: () => void
  } = $props()

  let wsState = $state<WorkspaceState | null>(null)
  let refreshing = $state(false)
  let showAttach = $state(false)
  let openFilePath = $state<string | null>(null)

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

  const ws = $derived(wsState?.workspace)
  const idx = $derived(wsState?.index)
</script>

<aside class="workspace-panel" transition:fly={{ x: 320, duration: 200 }}>
  <header>
    <span class="title"><Briefcase size={15} /> {t('workspace.title')}</span>
    <div class="actions">
      {#if wsState?.attached}
        <button class="icon-btn" class:spin={refreshing} onclick={refresh} title={t('workspace.refresh')} aria-label={t('workspace.refresh')}><RefreshCw size={14} /></button>
        <button class="icon-btn danger" onclick={detach} title={t('workspace.detach')} aria-label={t('workspace.detach')}><Unlink size={14} /></button>
      {/if}
      <button class="icon-btn" onclick={onClose} title={t('common.close')} aria-label={t('common.close')}><X size={15} /></button>
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
        <div class="badge-row">
          <span class="badge adapter">{ws.adapter}</span>
          <span class="badge tier">{t('workspace.tier_local')}</span>
        </div>
        {#if ws.origin.url}
          <p class="origin" title={ws.origin.url}>{ws.origin.url}</p>
        {/if}
        {#if idx?.summary}
          <p class="summary">{idx.summary}</p>
        {/if}
        {#if idx?.warnings?.length}
          {#each idx.warnings as w (w)}
            <p class="warning"><AlertTriangle size={12} /> {w}</p>
          {/each}
        {/if}
        <div class="launch">
          <span class="label">{t('workspace.launch_command')}</span>
          <EditableText
            value={ws.launch_command ?? ''}
            placeholder={t('workspace.launch_placeholder')}
            onSave={saveLaunchCommand}
          />
        </div>
      </section>

      <section class="files">
        <span class="label">{t('workspace.files')}</span>
        {#if idx && idx.tree.length > 0}
          <div class="tree-scroll">
            <WorkspaceFileTree entries={idx.tree} onOpenFile={(p) => openFilePath = p} />
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

{#if openFilePath}
  <WorkspaceFileViewer {brandSlug} {streamSlug} {projectSlug} path={openFilePath} onClose={() => openFilePath = null} />
{/if}

<style>
  .workspace-panel {
    width: 320px;
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    border-left: 1px solid var(--border-muted);
    background: var(--bg-base);
    overflow: hidden;
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.55rem 0.75rem;
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
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
  .empty p { margin: 0; font-size: 0.8rem; }

  .body {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .meta {
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
  .origin {
    margin: 0;
    font-size: 0.7rem;
    font-family: var(--font-mono, monospace);
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .summary { margin: 0; font-size: 0.76rem; color: var(--text-secondary); line-height: 1.4; }
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
  .launch { display: flex; flex-direction: column; gap: 0.2rem; font-size: 0.78rem; }

  .files {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    padding: 0.6rem 0.5rem 0.6rem 0.75rem;
    overflow: hidden;
  }
  .tree-scroll { flex: 1; overflow: auto; }
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
