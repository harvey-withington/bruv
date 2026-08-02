<script lang="ts">
  import { ChevronRight, ChevronDown, Folder, FileText, Link2, LayoutTemplate } from 'lucide-svelte'
  import type { WorkspaceTree } from '../../lib/workspaceTree'
  import { t } from '../../lib/i18n.svelte'
  import WorkspaceFileTree from './WorkspaceFileTree.svelte'

  // Recursive collapsible tree over a prebuilt index (lib/workspaceTree.ts).
  // `dir` scopes this level; children come back from one Map lookup instead
  // of each level re-filtering the whole flat entry list (see the helper for
  // why that mattered at the 20k-entry cap).
  //
  // `collapsed` is one shared $state record owned by the ROOT consumer
  // (WorkspacePanel) and passed down every level — that's what lets
  // Expand All / Collapse All and the accordion mode operate across the
  // whole tree instead of per-level islands.
  let { tree, dir = '', onOpenFile, depth = 0, collapsed, mode = 'multi' }: {
    tree: WorkspaceTree
    /** Directory this instance renders the children of ('' = root). */
    dir?: string
    onOpenFile?: (path: string) => void
    depth?: number
    collapsed: Record<string, boolean>
    /** 'single': expanding a folder collapses its siblings (accordion),
     *  matching the Sidebar project tree's mode toggle. */
    mode?: 'single' | 'multi'
  } = $props()

  const level = $derived(tree.childrenOf(dir))

  function toggleDir(path: string) {
    const expanding = collapsed[path]
    if (expanding && mode === 'single') {
      for (const sib of level) {
        if (sib.isDir && sib.path !== path) collapsed[sib.path] = true
      }
    }
    collapsed[path] = !collapsed[path]
  }
</script>

<ul class="tree" style:padding-left={depth > 0 ? '0.9rem' : '0'}>
  {#each level as n (n.path)}
    <li>
      {#if n.isDir && n.notIndexed}
        <!-- Recorded but never walked (node_modules & friends). No expander,
             and an explicit hint — rendering it as an empty folder would be
             a lie about what's on disk. -->
        <div class="node dir unindexed">
          <span class="chevron-slot" aria-hidden="true"></span>
          <Folder size={13} />
          <span class="name">{n.name}</span>
          <span class="tag">{t('workspace.not_indexed')}</span>
        </div>
      {:else if n.isDir}
        <button class="node dir" class:tpl={n.isTemplateRoot} onclick={() => toggleDir(n.path)}>
          {#if collapsed[n.path]}<ChevronRight size={12} />{:else}<ChevronDown size={12} />{/if}
          {#if n.isTemplateRoot}<LayoutTemplate size={13} />{:else}<Folder size={13} />{/if}
          <span class="name">{n.name}</span>
        </button>
        <!-- Children mount only while expanded: collapsed subtrees cost
             nothing to render and nothing to keep in the DOM. -->
        {#if !collapsed[n.path]}
          <WorkspaceFileTree {tree} dir={n.path} {onOpenFile} depth={depth + 1} {collapsed} {mode} />
        {/if}
      {:else}
        <button class="node file" onclick={() => onOpenFile?.(n.path)}>
          {#if n.symlink}<Link2 size={13} />{:else}<FileText size={13} />{/if}
          <span class="name">{n.name}</span>
        </button>
      {/if}
    </li>
  {/each}
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
  /* Inert row: nothing to open, nothing to expand. */
  .node.dir.unindexed {
    color: var(--text-muted);
    font-weight: 400;
    cursor: default;
  }
  .node.dir.unindexed:hover {
    background: none;
    color: var(--text-muted);
  }
  /* Keeps the label aligned with expandable siblings. */
  .chevron-slot {
    width: 12px;
    flex-shrink: 0;
  }
  .tag {
    flex-shrink: 0;
    font-size: 0.66rem;
    color: var(--text-faint);
    white-space: nowrap;
  }
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
