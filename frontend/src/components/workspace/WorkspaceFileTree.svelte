<script lang="ts">
  import { ChevronRight, ChevronDown, Folder, FileText, Link2 } from 'lucide-svelte'
  import type { WorkspaceEntry } from '@shared/types'
  import WorkspaceFileTree from './WorkspaceFileTree.svelte'

  // Recursive collapsible tree over the flat, sorted index entries.
  // `prefix` scopes this level: entries directly under it render here,
  // deeper ones render in the recursive child instances.
  let { entries, prefix = '', onOpenFile, depth = 0 }: {
    entries: WorkspaceEntry[]
    prefix?: string
    onOpenFile?: (path: string) => void
    depth?: number
  } = $props()

  let collapsed = $state<Record<string, boolean>>({})

  const level = $derived(entries.filter(e => {
    if (!e.path.startsWith(prefix)) return false
    const rest = e.path.slice(prefix.length)
    return rest.length > 0 && !rest.includes('/')
  }))

  function name(e: WorkspaceEntry): string {
    return e.path.slice(prefix.length)
  }
</script>

<ul class="tree" style:padding-left={depth > 0 ? '0.9rem' : '0'}>
  {#each level as e (e.path)}
    <li>
      {#if e.is_dir}
        <button class="node dir" onclick={() => collapsed[e.path] = !collapsed[e.path]}>
          {#if collapsed[e.path]}<ChevronRight size={12} />{:else}<ChevronDown size={12} />{/if}
          <Folder size={13} />
          <span class="name">{name(e)}</span>
        </button>
        {#if !collapsed[e.path]}
          <WorkspaceFileTree {entries} prefix={e.path + '/'} {onOpenFile} depth={depth + 1} />
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
  .node {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    width: 100%;
    padding: 0.15rem 0.3rem;
    border: none;
    background: none;
    color: var(--text-secondary);
    font-size: 0.78rem;
    text-align: left;
    border-radius: 4px;
    cursor: pointer;
  }
  .node:hover,
  .node:focus-visible {
    background: var(--bg-subtle-hover);
    color: var(--text-primary);
  }
  .node.dir {
    color: var(--text-muted);
    font-weight: 500;
  }
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
