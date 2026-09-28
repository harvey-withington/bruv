<script lang="ts">
  import { ChevronRight, Folder, FileText, Link2, LayoutTemplate, AlertTriangle, Square, SquareCheck, FilePlus, FolderPlus } from 'lucide-svelte'
  import type { WorkspaceDirCache, WorkspaceTreeNode } from '../../lib/workspaceTree.svelte'
  import { t } from '../../lib/i18n.svelte'
  import { setWorkspaceDrag } from '../../lib/workspaceDrag'
  import WorkspaceFileTree from './WorkspaceFileTree.svelte'

  // Recursive collapsible tree over a LAZY per-directory cache
  // (lib/workspaceTree.svelte.ts). One instance renders exactly one
  // directory level, and mounting it is what triggers that directory's
  // fetch — so a folder nobody opens is never read, however heavy it is
  // (node_modules, .git, a photo dump; see the cache for the ruling).
  //
  // `collapsed` is one shared $state record owned by the ROOT consumer
  // (WorkspacePanel) and passed down every level — that's what lets
  // Expand All / Collapse All and the accordion mode operate across the
  // whole tree instead of per-level islands. A directory is expanded ONLY
  // when `collapsed[path] === false`: absent means collapsed, so the tree
  // opens closed and stays cheap.
  //
  // Three optional behaviours ride the same rows (plan/2026-09-17):
  // structure actions on folders (`onCreateIn`), multi-select as a picker
  // (`selected` + `onToggleSelect`), and drag-out as a Workspace Files
  // entry (`workspaceId`). Absent props = plain browse tree.
  let { cache, dir = '', onOpenFile, depth = 0, collapsed, mode = 'multi', workspaceId, onCreateIn, selected, onToggleSelect }: {
    cache: WorkspaceDirCache
    /** Directory this instance renders the children of ('' = root). */
    dir?: string
    onOpenFile?: (path: string) => void
    depth?: number
    collapsed: Record<string, boolean>
    /** 'single': expanding a folder collapses its siblings (accordion),
     *  matching the Sidebar project tree's mode toggle. */
    mode?: 'single' | 'multi'
    /** When set, rows drag out as Workspace Files entries of this workspace. */
    workspaceId?: string
    /** New file / New folder / From template on a folder row (hover-revealed). */
    onCreateIn?: (dir: string, kind: 'file' | 'dir' | 'template') => void
    /** Picker mode: shared selection record (path → selected), same
     *  ownership rule as `collapsed`. Rows show a checkbox; a file click
     *  toggles instead of opening. */
    selected?: Record<string, boolean>
    onToggleSelect?: (node: WorkspaceTreeNode) => void
  } = $props()

  const state = $derived(cache.get(dir))
  const children = $derived(state?.status === 'ready' ? state.children : [])
  const selectable = $derived(selected !== undefined && onToggleSelect !== undefined)

  $effect(() => {
    // Reading the cache entry (not just calling ensure) is deliberate: after a
    // Refresh clears the cache this effect re-runs and the level reloads
    // itself instead of silently going blank. Only mounted — i.e. visible —
    // levels refetch, so a refresh costs what is on screen, not the tree.
    if (cache.get(dir) === undefined) void cache.ensure(dir)
  })

  function isExpanded(path: string): boolean {
    return collapsed[path] === false
  }

  function toggleDir(path: string) {
    const expanding = !isExpanded(path)
    if (expanding && mode === 'single') {
      for (const sib of children) {
        if (sib.isDir && sib.path !== path) collapsed[sib.path] = true
      }
    }
    collapsed[path] = !expanding
  }

  function onFileClick(n: WorkspaceTreeNode) {
    if (selectable) onToggleSelect?.(n)
    else onOpenFile?.(n.path)
  }

  function dragStart(e: DragEvent, n: WorkspaceTreeNode) {
    if (!workspaceId) return
    setWorkspaceDrag(e.dataTransfer, { workspaceId, path: n.path, isDir: n.isDir })
  }
</script>

<ul class="tree" style:padding-left={depth > 0 ? '0.9rem' : '0'}>
  {#if state === undefined || state.status === 'loading'}
    <li class="status muted">{t('common.loading')}</li>
  {:else if state.status === 'error'}
    <!-- Never fall back to "empty": a folder that failed to list must say so
         and offer the retry, or it reads as an empty folder on disk. -->
    <li class="status error">
      <AlertTriangle size={12} />
      <span class="msg" title={state.error}>{t('workspace.dir_failed')}</span>
      <button class="retry" onclick={() => cache.reload(dir)}>{t('common.retry')}</button>
    </li>
  {:else}
    {#each children as n (n.path)}
      <li class="row-host">
        <div class="row" draggable={!!workspaceId} ondragstart={(e) => dragStart(e, n)} role="presentation">
          {#if selectable}
            <button class="check" class:on={selected?.[n.path]} onclick={() => onToggleSelect?.(n)} aria-pressed={selected?.[n.path] === true} aria-label={t('workspace.select_entry', { name: n.name })}>
              {#if selected?.[n.path]}<SquareCheck size={13} />{:else}<Square size={13} />{/if}
            </button>
          {/if}
          {#if n.isDir}
            <button class="node dir press-still" class:tpl={cache.isTemplateRoot(n.path)} onclick={() => toggleDir(n.path)}>
              <span class="chev" class:open={isExpanded(n.path)}><ChevronRight size={12} /></span>
              {#if cache.isTemplateRoot(n.path)}<LayoutTemplate size={13} />{:else}<Folder size={13} />{/if}
              <span class="name">{n.name}</span>
            </button>
            {#if onCreateIn}
              <span class="create-actions">
                <button class="mini" onclick={() => onCreateIn?.(n.path, 'file')} title={t('workspace.new_file')} aria-label={t('workspace.new_file')}><FilePlus size={12} /></button>
                <button class="mini" onclick={() => onCreateIn?.(n.path, 'dir')} title={t('workspace.new_folder')} aria-label={t('workspace.new_folder')}><FolderPlus size={12} /></button>
                <button class="mini" onclick={() => onCreateIn?.(n.path, 'template')} title={t('workspace.from_template')} aria-label={t('workspace.from_template')}><LayoutTemplate size={12} /></button>
              </span>
            {/if}
          {:else}
            <button class="node file press-still" onclick={() => onFileClick(n)}>
              {#if n.symlink}<Link2 size={13} />{:else}<FileText size={13} />{/if}
              <span class="name">{n.name}</span>
            </button>
          {/if}
        </div>
        <!-- Children mount only while expanded, and mounting is what loads
             them: collapsed subtrees cost neither DOM nor an RPC. -->
        {#if n.isDir && isExpanded(n.path)}
          <WorkspaceFileTree {cache} dir={n.path} {onOpenFile} depth={depth + 1} {collapsed} {mode} {workspaceId} {onCreateIn} {selected} {onToggleSelect} />
        {/if}
      </li>
    {/each}
  {/if}
</ul>

<style>
  .tree {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 0.1rem;
    border-radius: 4px;
  }
  .row:hover,
  .row:focus-within {
    background: var(--accent-glow-2);
  }
  /* Row treatment mirrors the Sidebar's project tree (.tree-item):
     body-contrast text, accent-glow hover, primary for emphasis. */
  .node {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    flex: 1;
    min-width: 0;
    padding: 0.2rem 0.35rem;
    border: none;
    background: none;
    color: var(--text-body);
    font-size: 0.82rem;
    text-align: left;
    border-radius: 4px;
    cursor: pointer;
  }
  .row:hover .node,
  .node:focus-visible {
    color: var(--text-primary);
  }
  .node.dir {
    color: var(--text-primary);
    font-weight: 500;
  }
  /* Folder-Template roots: cyan-tinted (--template-accent, theme-aware) so
     they read as generators, not ordinary content folders. */
  .node.dir.tpl,
  .row:hover .node.dir.tpl {
    color: var(--template-accent);
  }
  .row:hover .node.dir.tpl {
    filter: brightness(1.15);
  }
  .check,
  .mini {
    display: flex;
    align-items: center;
    padding: 0.15rem;
    border: none;
    background: none;
    color: var(--text-faint);
    cursor: pointer;
    border-radius: 3px;
  }
  .check.on { color: var(--accent); }
  .check:hover,
  .mini:hover,
  .mini:focus-visible {
    color: var(--text-primary);
  }
  /* Structure actions reveal on the folder row's hover/focus only —
     three icons per row at rest would drown the tree. */
  .create-actions {
    display: none;
    gap: 0;
    padding-right: 0.2rem;
  }
  .row:hover .create-actions,
  .row:focus-within .create-actions {
    display: flex;
  }
  /* Per-level loading / failure rows, indented with the children they stand
     in for. */
  .status {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.2rem 0.35rem;
    font-size: 0.76rem;
  }
  .status.muted {
    color: var(--text-faint);
  }
  .status.error {
    color: var(--danger, #ef4444);
  }
  .status .msg {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .retry {
    flex-shrink: 0;
    border: none;
    background: none;
    padding: 0 0.15rem;
    color: var(--accent);
    font-size: 0.76rem;
    text-decoration: underline;
    cursor: pointer;
  }
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .chev {
    display: inline-flex;
    flex-shrink: 0;
    transition: transform 120ms ease;
  }
  .chev.open { transform: rotate(90deg); }
</style>
