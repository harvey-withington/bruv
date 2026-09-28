<script lang="ts">
  import { X, FolderSearch } from 'lucide-svelte'
  import type { WorkspaceFileEntry, WorkspaceLocation } from '@shared/types'
  import { newWorkspaceFileEntry } from '@shared/workspaceFiles'
  import type { WorkspaceTreeNode } from '../../lib/workspaceTree.svelte'
  import { workspaceDirCache, refreshWorkspaceDirs } from '../../lib/workspaceLocations.svelte'
  import { t } from '../../lib/i18n.svelte'
  import { focusTrap } from '../../lib/actions'
  import WorkspaceFileTree from './WorkspaceFileTree.svelte'
  import WorkspaceStructureActions from './WorkspaceStructureActions.svelte'

  // The Workspace Files block's picker IS the workspace tree in selection
  // mode: browse, tick files and folders, and create new ones right here
  // (the same structure actions as the panel) so a chapter can be made and
  // linked in one go. Returns entries; the block merges them.
  let { location, cardTitle = '', onPick, onClose }: {
    location: WorkspaceLocation
    cardTitle?: string
    onPick: (entries: WorkspaceFileEntry[]) => void
    onClose: () => void
  } = $props()

  const wsId = $derived(location.workspace.id)
  const cache = $derived(workspaceDirCache(wsId))
  let collapsed = $state<Record<string, boolean>>({})
  let selected = $state<Record<string, boolean>>({})
  let picked = $state<Record<string, WorkspaceTreeNode>>({})
  let structure = $state<WorkspaceStructureActions | null>(null)

  const count = $derived(Object.keys(picked).length)

  function toggle(node: WorkspaceTreeNode) {
    if (selected[node.path]) {
      delete selected[node.path]
      delete picked[node.path]
    } else {
      selected[node.path] = true
      picked[node.path] = node
    }
  }

  // A freshly created entry is selected straight away — the reason to
  // create it from a card is to link it.
  function onCreated(rel: string, kind: 'file' | 'dir') {
    refreshWorkspaceDirs(wsId)
    const parent = rel.includes('/') ? rel.slice(0, rel.lastIndexOf('/')) : ''
    let p = ''
    for (const seg of parent.split('/').filter(Boolean)) {
      p = p ? `${p}/${seg}` : seg
      collapsed[p] = false
    }
    selected[rel] = true
    picked[rel] = { path: rel, name: rel.slice(rel.lastIndexOf('/') + 1), isDir: kind === 'dir', symlink: false }
  }

  function confirm() {
    onPick(Object.values(picked).map(n => newWorkspaceFileEntry(wsId, n.path, n.isDir)))
  }

  // Layered over CardDetail: shield both window keys (UI-CONVENTIONS §8.1).
  function onKeydown(e: KeyboardEvent) {
    e.stopPropagation()
    if (e.key === 'Escape') { e.preventDefault(); onClose() }
    else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); if (count > 0) confirm() }
  }
</script>

<div class="dialog-overlay" role="presentation" onclick={(e) => { if (e.target === e.currentTarget) onClose() }}>
  <div class="dialog" role="dialog" aria-label={t('workspace_files.pick_title')} tabindex="-1" use:focusTrap onkeydown={onKeydown}>
    <header>
      <h3><FolderSearch size={15} /> {t('workspace_files.pick_title')}</h3>
      <span class="project">{location.project_slug}</span>
      <button class="icon-btn" onclick={onClose} title={t('common.close')} aria-label={t('common.close')}><X size={16} /></button>
    </header>
    <div class="toolbar">
      <WorkspaceStructureActions bind:this={structure} brandSlug={location.brand_slug} streamSlug={location.stream_slug} projectSlug={location.project_slug} {cardTitle} {onCreated} />
    </div>
    <div class="tree-scroll">
      <WorkspaceFileTree
        {cache}
        {collapsed}
        workspaceId={wsId}
        {selected}
        onToggleSelect={toggle}
        onCreateIn={(dir, kind) => structure?.open(dir, kind)}
      />
    </div>
    <footer>
      <span class="count">{count > 0 ? t('workspace_files.pick_count', { count: String(count) }) : t('workspace_files.pick_hint')}</span>
      <button class="btn subtle" onclick={onClose}>{t('common.cancel')}</button>
      <button class="btn primary" disabled={count === 0} onclick={confirm}>{t('common.add')}</button>
    </footer>
  </div>
</div>

<style>
  .dialog-overlay {
    position: fixed;
    inset: 0;
    background: color-mix(in srgb, var(--bg-base) 60%, transparent);
    backdrop-filter: blur(2px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 97;
  }
  .dialog {
    width: min(520px, 92vw);
    height: min(70vh, 640px);
    display: flex;
    flex-direction: column;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 10px;
    box-shadow: var(--shadow-lg, 0 12px 40px rgba(0, 0, 0, 0.4));
    overflow: hidden;
  }
  header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.65rem 0.85rem;
    border-bottom: 1px solid var(--border-muted);
  }
  h3 {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin: 0;
    font-size: 0.9rem;
    font-weight: 600;
  }
  .project { font-size: 0.72rem; color: var(--text-faint); }
  .toolbar {
    display: flex;
    padding: 0.35rem 0.6rem;
    border-bottom: 1px solid var(--border-muted);
  }
  .tree-scroll {
    flex: 1;
    overflow: auto;
    padding: 0.4rem 0.6rem;
    background: var(--bg-surface);
  }
  footer {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.6rem 0.85rem;
    border-top: 1px solid var(--border-muted);
  }
  .count { flex: 1; font-size: 0.74rem; color: var(--text-muted); }
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
</style>
