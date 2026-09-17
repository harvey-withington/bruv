<script lang="ts">
  import { ChevronRight, Folder, FileText, X, Briefcase } from 'lucide-svelte'
  import type { WorkspaceFileEntry } from '@shared/types'
  import type { WorkspaceFilesNode } from '@shared/workspaceFiles'
  import { t } from '../../lib/i18n.svelte'
  import WorkspaceDirList from './WorkspaceDirList.svelte'
  import WorkspaceFilesNodes from './WorkspaceFilesNodes.svelte'

  // Recursive rows of a Workspace Files block on mobile: implied folders
  // (open by default), entry rows (tap a file to open it, a folder to
  // list it), and a remove control per entry.
  let { nodes, expanded, depth = 0, onOpen, onOpenPath, onRemove }: {
    nodes: WorkspaceFilesNode[]
    expanded: Record<string, boolean>
    depth?: number
    onOpen: (entry: WorkspaceFileEntry) => void
    onOpenPath: (workspaceId: string, path: string) => void
    onRemove: (entry: WorkspaceFileEntry) => void
  } = $props()

  function isOpen(n: WorkspaceFilesNode): boolean {
    const v = expanded[n.key]
    return v === undefined ? n.entry === undefined : v
  }
</script>

<ul class="nodes" style:padding-left={depth > 0 ? '0.9rem' : '0'}>
  {#each nodes as n (n.key)}
    <li>
      {#if n.entry}
        {@const entry = n.entry}
        <div class="entry">
          <button type="button" class="row" onclick={() => entry.is_dir ? expanded[n.key] = !isOpen(n) : onOpen(entry)}>
            {#if entry.is_dir}
              <span class="chev" class:open={isOpen(n)}><ChevronRight size={14} /></span>
              <Folder size={15} />
            {:else}
              <FileText size={15} />
            {/if}
            <span class="name">{n.name}</span>
          </button>
          <button type="button" class="remove" onclick={() => onRemove(entry)} aria-label={t('workspace_files.remove')}><X size={14} /></button>
        </div>
        {#if entry.is_dir && isOpen(n)}
          <div class="sub"><WorkspaceDirList workspaceId={n.workspaceId} dir={n.path} onOpenFile={(p) => onOpenPath(n.workspaceId, p)} /></div>
        {/if}
      {:else}
        <button type="button" class="row implied" onclick={() => expanded[n.key] = !isOpen(n)}>
          <span class="chev" class:open={isOpen(n)}><ChevronRight size={14} /></span>
          {#if n.path === ''}<Briefcase size={15} />{:else}<Folder size={15} />{/if}
          <span class="name">{n.name}</span>
        </button>
        {#if isOpen(n)}
          <WorkspaceFilesNodes nodes={n.children} {expanded} depth={depth + 1} {onOpen} {onOpenPath} {onRemove} />
        {/if}
      {/if}
    </li>
  {/each}
</ul>

<style>
  .nodes { list-style: none; margin: 0; padding: 0; }
  .entry { display: flex; align-items: center; }
  .row {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    flex: 1;
    min-width: 0;
    min-height: 40px;
    padding: 0.35rem 0.4rem;
    border: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.9rem;
    text-align: left;
    border-radius: 6px;
  }
  .row:active { background: var(--bg-elev-1); }
  .row.implied { color: var(--text-muted); font-weight: 500; }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .remove {
    background: transparent;
    border: none;
    color: var(--text-faint);
    padding: 0.5rem;
    display: inline-flex;
  }
  .sub { padding-left: 0.9rem; }
  .chev {
    display: inline-flex;
    flex-shrink: 0;
    transition: transform 120ms ease;
  }
  .chev.open { transform: rotate(90deg); }
</style>
