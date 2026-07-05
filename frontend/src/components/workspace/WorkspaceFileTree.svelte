<script lang="ts">
  import { ChevronRight, ChevronDown, Folder, FileText, Link2, LayoutTemplate } from 'lucide-svelte'
  import type { WorkspaceEntry } from '@shared/types'
  import WorkspaceFileTree from './WorkspaceFileTree.svelte'

  // Recursive collapsible tree over the flat, sorted index entries.
  // `prefix` scopes this level: entries directly under it render here,
  // deeper ones render in the recursive child instances.
  //
  // `collapsed` is one shared $state record owned by the ROOT consumer
  // (WorkspacePanel) and passed down every level — that's what lets
  // Expand All / Collapse All and the accordion mode operate across the
  // whole tree instead of per-level islands.
  let { entries, prefix = '', onOpenFile, depth = 0, collapsed, mode = 'multi', templateRoots }: {
    entries: WorkspaceEntry[]
    prefix?: string
    onOpenFile?: (path: string) => void
    depth?: number
    collapsed: Record<string, boolean>
    /** 'single': expanding a folder collapses its siblings (accordion),
     *  matching the Sidebar project tree's mode toggle. */
    mode?: 'single' | 'multi'
    /** Folder-Template roots (dirs containing .ft/template.json), computed
     *  once at the root instance and passed down. */
    templateRoots?: Set<string>
  } = $props()

  // Icons distinguish elements: template roots render distinctly.
  const tplRoots = $derived(
    templateRoots ?? new Set(
      entries
        .filter(e => !e.is_dir && e.path.endsWith('/.ft/template.json'))
        .map(e => e.path.slice(0, -'/.ft/template.json'.length))
    )
  )

  const level = $derived(entries.filter(e => {
    if (!e.path.startsWith(prefix)) return false
    const rest = e.path.slice(prefix.length)
    return rest.length > 0 && !rest.includes('/')
  }))

  function name(e: WorkspaceEntry): string {
    return e.path.slice(prefix.length)
  }

  function toggleDir(path: string) {
    const expanding = collapsed[path]
    if (expanding && mode === 'single') {
      for (const sib of level) {
        if (sib.is_dir && sib.path !== path) collapsed[sib.path] = true
      }
    }
    collapsed[path] = !collapsed[path]
  }
</script>

<ul class="tree" style:padding-left={depth > 0 ? '0.9rem' : '0'}>
  {#each level as e (e.path)}
    <li>
      {#if e.is_dir}
        <button class="node dir" class:tpl={tplRoots.has(e.path)} onclick={() => toggleDir(e.path)}>
          {#if collapsed[e.path]}<ChevronRight size={12} />{:else}<ChevronDown size={12} />{/if}
          {#if tplRoots.has(e.path)}<LayoutTemplate size={13} />{:else}<Folder size={13} />{/if}
          <span class="name">{name(e)}</span>
        </button>
        {#if !collapsed[e.path]}
          <WorkspaceFileTree {entries} prefix={e.path + '/'} {onOpenFile} depth={depth + 1} {collapsed} {mode} templateRoots={tplRoots} />
        {/if}
      {:else}
        <button class="node file" onclick={() => onOpenFile?.(e.path)}>
          {#if e.symlink}<Link2 size={13} />{:else}<FileText size={13} />{/if}
          <span class="name">{name(e)}</span>
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
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
