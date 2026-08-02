<script lang="ts">
  import { ChevronRight, ChevronDown, Folder, FileText, Link2, LayoutTemplate, AlertTriangle } from 'lucide-svelte'
  import type { WorkspaceDirCache } from '../../lib/workspaceTree.svelte'
  import { t } from '../../lib/i18n.svelte'
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
  let { cache, dir = '', onOpenFile, depth = 0, collapsed, mode = 'multi' }: {
    cache: WorkspaceDirCache
    /** Directory this instance renders the children of ('' = root). */
    dir?: string
    onOpenFile?: (path: string) => void
    depth?: number
    collapsed: Record<string, boolean>
    /** 'single': expanding a folder collapses its siblings (accordion),
     *  matching the Sidebar project tree's mode toggle. */
    mode?: 'single' | 'multi'
  } = $props()

  const state = $derived(cache.get(dir))
  const children = $derived(state?.status === 'ready' ? state.children : [])

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
      <li>
        {#if n.isDir}
          <button class="node dir" class:tpl={cache.isTemplateRoot(n.path)} onclick={() => toggleDir(n.path)}>
            {#if isExpanded(n.path)}<ChevronDown size={12} />{:else}<ChevronRight size={12} />{/if}
            {#if cache.isTemplateRoot(n.path)}<LayoutTemplate size={13} />{:else}<Folder size={13} />{/if}
            <span class="name">{n.name}</span>
          </button>
          <!-- Children mount only while expanded, and mounting is what loads
               them: collapsed subtrees cost neither DOM nor an RPC. -->
          {#if isExpanded(n.path)}
            <WorkspaceFileTree {cache} dir={n.path} {onOpenFile} depth={depth + 1} {collapsed} {mode} />
          {/if}
        {:else}
          <button class="node file" onclick={() => onOpenFile?.(n.path)}>
            {#if n.symlink}<Link2 size={13} />{:else}<FileText size={13} />{/if}
            <span class="name">{n.name}</span>
          </button>
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
  /* Row treatment mirrors the Sidebar's project tree (.tree-item):
     body-contrast text, accent-glow hover, primary for emphasis. */
  .node {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    width: 100%;
    padding: 0.2rem 0.35rem;
    border: none;
    background: none;
    color: var(--text-body);
    font-size: 0.82rem;
    text-align: left;
    border-radius: 4px;
    cursor: pointer;
  }
  .node:hover,
  .node:focus-visible {
    background: var(--accent-glow-2);
    color: var(--text-primary);
  }
  .node.dir {
    color: var(--text-primary);
    font-weight: 500;
  }
  /* Folder-Template roots: cyan-tinted (--template-accent, theme-aware) so
     they read as generators, not ordinary content folders. */
  .node.dir.tpl {
    color: var(--template-accent);
  }
  .node.dir.tpl:hover,
  .node.dir.tpl:focus-visible {
    color: var(--template-accent);
    filter: brightness(1.15);
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
</style>
