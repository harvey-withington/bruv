<script lang="ts">
  import { ChevronRight, Folder, Briefcase } from 'lucide-svelte'
  import type { WorkspaceFileEntry } from '@shared/types'
  import { isFolderNode, type WorkspaceFilesNode } from '@shared/workspaceFiles'
  import { workspaceDirCache } from '../../lib/workspaceLocations.svelte'
  import WorkspaceFileEntryRow from './WorkspaceFileEntryRow.svelte'
  import WorkspaceFileTree from './WorkspaceFileTree.svelte'
  import WorkspaceFilesTreeView from './WorkspaceFilesTreeView.svelte'

  // Tree display of a Workspace Files block: implied folders (from shared
  // path prefixes) open by default and hold entries beneath; a folder
  // ENTRY expands lazily into its live subtree through the same
  // WorkspaceFileTree the panel uses, so what the card names is exactly
  // what the workspace holds.
  let { nodes, expanded, depth = 0, onOpen, onOpenPath, onRemove, onRelink }: {
    nodes: WorkspaceFilesNode[]
    /** Shared expand state keyed by node key; implied folders default open. */
    expanded: Record<string, boolean>
    depth?: number
    onOpen: (entry: WorkspaceFileEntry) => void
    /** A file inside an expanded folder entry (workspace id + path). */
    onOpenPath: (workspaceId: string, path: string) => void
    onRemove: (entry: WorkspaceFileEntry) => void
    onRelink: (entry: WorkspaceFileEntry) => void
  } = $props()

  // The live subtree under a folder entry is a WorkspaceFileTree, whose
  // record has the opposite polarity (collapsed[path] === false = open)
  // and raw paths for keys — it gets its own record rather than sharing.
  let subtreeCollapsed = $state<Record<string, boolean>>({})

  function isOpen(n: WorkspaceFilesNode): boolean {
    const v = expanded[n.key]
    // Implied folders open by default; folder entries closed (their
    // subtree is a fetch).
    return v === undefined ? n.entry === undefined : v
  }
  function toggle(n: WorkspaceFilesNode) {
    expanded[n.key] = !isOpen(n)
  }
</script>

<ul class="view" style:padding-left={depth > 0 ? '0.9rem' : '0'}>
  {#each nodes as n (n.key)}
    <li>
      {#if n.entry}
        <WorkspaceFileEntryRow entry={n.entry} expanded={isOpen(n)} {onOpen} onToggle={() => toggle(n)} {onRemove} {onRelink} />
        {#if n.entry.is_dir && isOpen(n)}
          <div class="subtree">
            <WorkspaceFileTree cache={workspaceDirCache(n.workspaceId)} dir={n.path} collapsed={subtreeCollapsed} depth={1} onOpenFile={(p) => onOpenPath(n.workspaceId, p)} />
          </div>
        {/if}
      {:else}
        <button class="implied press-still" class:root={n.path === ''} onclick={() => toggle(n)}>
          <span class="chev" class:open={isOpen(n)}><ChevronRight size={12} /></span>
          {#if n.path === ''}<Briefcase size={13} />{:else}<Folder size={13} />{/if}
          <span class="name">{n.name}</span>
        </button>
        {#if isOpen(n) && (n.children.length > 0 || isFolderNode(n))}
          <WorkspaceFilesTreeView nodes={n.children} {expanded} depth={depth + 1} {onOpen} {onOpenPath} {onRemove} {onRelink} />
        {/if}
      {/if}
    </li>
  {/each}
</ul>

<style>
  .view { list-style: none; margin: 0; padding: 0; }
  .implied {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    width: 100%;
    padding: 0.22rem 0.35rem;
    border: none;
    background: none;
    color: var(--text-muted);
    font-size: 0.82rem;
    font-weight: 500;
    text-align: left;
    border-radius: 4px;
    cursor: pointer;
  }
  .implied.root { color: var(--text-primary); }
  .implied:hover,
  .implied:focus-visible {
    background: var(--accent-glow-2);
    color: var(--text-primary);
  }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .subtree { padding-left: 0.9rem; }
  .chev {
    display: inline-flex;
    flex-shrink: 0;
    transition: transform 120ms ease;
  }
  .chev.open { transform: rotate(90deg); }
</style>
