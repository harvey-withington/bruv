<script lang="ts">
  import { ChevronRight, Folder, FileText } from 'lucide-svelte'
  import type { WorkspaceEntry } from '@shared/types'
  import { workspacePathName } from '@shared/blockValues'
  import { listWorkspaceDir } from '../../lib/workspaceLocations.svelte'
  import { t } from '../../lib/i18n.svelte'
  import WorkspaceDirList from './WorkspaceDirList.svelte'

  // Lazy listing of one workspace folder, for a folder entry expanded in
  // a Workspace Files block. One RPC per folder on first expand; nothing
  // beneath is read until tapped (the same rule as the desktop tree).
  let { workspaceId, dir, depth = 0, onOpenFile }: {
    workspaceId: string
    dir: string
    depth?: number
    onOpenFile: (path: string) => void
  } = $props()

  let entries = $state<WorkspaceEntry[] | null>(null)
  let error = $state<string | null>(null)
  let open = $state<Record<string, boolean>>({})

  $effect(() => {
    void dir
    entries = null
    error = null
    listWorkspaceDir(workspaceId, dir)
      .then((list) => { entries = list })
      .catch((e: unknown) => { error = e instanceof Error ? e.message : String(e) })
  })
</script>

<ul class="dir" style:padding-left={depth > 0 ? '0.9rem' : '0'}>
  {#if error}
    <li class="status error">{t('workspace_files.dir_failed')}</li>
  {:else if entries === null}
    <li class="status">{t('common.loading')}</li>
  {:else}
    {#each entries as e (e.path)}
      <li>
        {#if e.is_dir}
          <button type="button" class="row" onclick={() => open[e.path] = !open[e.path]}>
            <span class="chev" class:open={open[e.path]}><ChevronRight size={14} /></span>
            <Folder size={15} />
            <span class="name">{workspacePathName(e.path)}</span>
          </button>
          {#if open[e.path]}
            <WorkspaceDirList {workspaceId} dir={e.path} depth={depth + 1} {onOpenFile} />
          {/if}
        {:else}
          <button type="button" class="row" onclick={() => onOpenFile(e.path)}>
            <FileText size={15} />
            <span class="name">{workspacePathName(e.path)}</span>
          </button>
        {/if}
      </li>
    {/each}
  {/if}
</ul>

<style>
  .dir { list-style: none; margin: 0; padding: 0; }
  .row {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    width: 100%;
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
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .status { padding: 0.35rem 0.4rem; font-size: 0.82rem; color: var(--text-faint); }
  .status.error { color: var(--danger-text); }
  .chev {
    display: inline-flex;
    flex-shrink: 0;
    transition: transform 120ms ease;
  }
  .chev.open { transform: rotate(90deg); }
</style>
