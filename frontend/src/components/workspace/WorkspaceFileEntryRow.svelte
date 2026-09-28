<script lang="ts">
  import { FileText, Folder, AlertTriangle, X, Link2, ChevronRight } from 'lucide-svelte'
  import type { WorkspaceFileEntry } from '@shared/types'
  import { workspaceParentPath, workspacePathName } from '@shared/blockValues'
  import { workspaceDirCache } from '../../lib/workspaceLocations.svelte'
  import { t } from '../../lib/i18n.svelte'

  // One entry of a Workspace Files block. Clicking a file opens the
  // editor; a folder entry expands to its live subtree (the parent decides
  // what expansion renders). The row checks its own existence against the
  // parent folder's listing — one RPC per distinct folder, shared through
  // the per-workspace cache — and shows a broken state with Relink when
  // the path no longer resolves. Paths are the identity; BRUV does not
  // track renames made outside it.
  let { entry, showPath = false, expanded = false, onOpen, onToggle, onRemove, onRelink }: {
    entry: WorkspaceFileEntry
    /** Flat display: show the full path under the name. */
    showPath?: boolean
    expanded?: boolean
    onOpen: (entry: WorkspaceFileEntry) => void
    onToggle?: (entry: WorkspaceFileEntry) => void
    onRemove: (entry: WorkspaceFileEntry) => void
    onRelink: (entry: WorkspaceFileEntry) => void
  } = $props()

  const cache = $derived(workspaceDirCache(entry.workspace_id))
  const parent = $derived(workspaceParentPath(entry.path))
  const name = $derived(workspacePathName(entry.path))
  const parentState = $derived(cache.get(parent))

  $effect(() => {
    if (cache.get(parent) === undefined) void cache.ensure(parent)
  })

  // Unknown while the listing loads or failed — never flagged broken on a
  // guess; a failed listing is the folder's problem, not this entry's.
  const missing = $derived(parentState?.status === 'ready' && !parentState.children.some(c => c.path === entry.path))
  const isDir = $derived(entry.is_dir === true)

  function activate() {
    if (missing) return
    if (isDir) onToggle?.(entry)
    else onOpen(entry)
  }
</script>

<div class="entry" class:missing class:dir={isDir}>
  <button class="main press-still" onclick={activate} disabled={missing} title={entry.path}>
    {#if isDir}
      <span class="chev" class:open={expanded}><ChevronRight size={12} /></span>
      <Folder size={13} />
    {:else}
      <FileText size={13} />
    {/if}
    <span class="text">
      <span class="name">{name}</span>
      {#if showPath && parent}<span class="path">{parent}/</span>{/if}
    </span>
    {#if missing}
      <span class="broken" title={t('workspace_files.missing_hint', { path: entry.path })}><AlertTriangle size={12} /> {t('workspace_files.missing')}</span>
    {/if}
  </button>
  <span class="actions">
    {#if missing}
      <button class="mini" onclick={() => onRelink(entry)} title={t('workspace_files.relink')} aria-label={t('workspace_files.relink')}><Link2 size={12} /></button>
    {/if}
    <button class="mini danger" onclick={() => onRemove(entry)} title={t('workspace_files.remove')} aria-label={t('workspace_files.remove')}><X size={12} /></button>
  </span>
</div>

<style>
  .entry {
    display: flex;
    align-items: center;
    gap: 0.1rem;
    border-radius: 4px;
  }
  .entry:hover,
  .entry:focus-within {
    background: var(--accent-glow-2);
  }
  .main {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    flex: 1;
    min-width: 0;
    padding: 0.22rem 0.35rem;
    border: none;
    background: none;
    color: var(--text-body);
    font-size: 0.82rem;
    text-align: left;
    cursor: pointer;
  }
  .main:disabled { cursor: default; }
  .entry:hover .main { color: var(--text-primary); }
  .dir .main { color: var(--text-primary); font-weight: 500; }
  .text { display: flex; flex-direction: column; min-width: 0; line-height: 1.2; }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .path {
    font-size: 0.68rem;
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .missing .main { color: var(--text-faint); }
  .broken {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    margin-left: auto;
    font-size: 0.68rem;
    color: var(--warning, #f59e0b);
    white-space: nowrap;
  }
  .actions { display: none; gap: 0; padding-right: 0.15rem; }
  .entry:hover .actions,
  .entry:focus-within .actions { display: flex; }
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
  .mini:hover,
  .mini:focus-visible { color: var(--text-primary); }
  .mini.danger:hover { color: var(--danger, #ef4444); }
  .chev {
    display: inline-flex;
    flex-shrink: 0;
    transition: transform 120ms ease;
  }
  .chev.open { transform: rotate(90deg); }
</style>
