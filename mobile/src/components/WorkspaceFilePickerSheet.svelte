<script lang="ts">
  import { ChevronRight, Folder, FileText, Square, SquareCheck, FilePlus, FolderPlus } from 'lucide-svelte'
  import type { WorkspaceEntry, WorkspaceFileEntry, WorkspaceLocation } from '@shared/types'
  import { workspacePathName } from '@shared/blockValues'
  import { newWorkspaceFileEntry } from '@shared/workspaceFiles'
  import { repoRPC } from '../lib/auth'
  import { t } from '../lib/i18n.svelte'
  import BottomSheet from './BottomSheet.svelte'

  // Pick workspace files/folders for a Workspace Files block: browse one
  // folder at a time (breadcrumb up top), tick what the card is about,
  // and create a file or folder in the folder you're looking at — the
  // same structure actions the desktop tree has, sized for a thumb.
  // Template generation stays desktop-only (structure authoring).
  let { location, onPick, onClose }: {
    location: WorkspaceLocation
    onPick: (entries: WorkspaceFileEntry[]) => void
    onClose: () => void
  } = $props()

  const slugs = $derived([location.brand_slug, location.stream_slug, location.project_slug] as const)
  let dir = $state('')
  let entries = $state<WorkspaceEntry[] | null>(null)
  let error = $state<string | null>(null)
  let picked = $state<Record<string, WorkspaceEntry>>({})
  let creating = $state<'file' | 'dir' | null>(null)
  let newName = $state('')
  let busy = $state(false)

  const crumbs = $derived(dir ? dir.split('/') : [])
  const count = $derived(Object.keys(picked).length)

  $effect(() => {
    void load(dir)
  })

  async function load(d: string) {
    entries = null
    error = null
    try {
      entries = (await repoRPC<WorkspaceEntry[]>('ListWorkspaceDir', [...slugs, d])) ?? []
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      entries = []
    }
  }

  function toggle(e: WorkspaceEntry) {
    if (picked[e.path]) delete picked[e.path]
    else picked[e.path] = e
  }

  function goTo(i: number) {
    dir = crumbs.slice(0, i).join('/')
  }

  async function create() {
    const name = newName.trim()
    if (!name || !creating || busy) return
    busy = true
    error = null
    const rel = dir ? `${dir}/${name}` : name
    try {
      const made = await repoRPC<string>(creating === 'file' ? 'CreateWorkspaceFile' : 'CreateWorkspaceDir', [...slugs, rel])
      picked[made] = { path: made, is_dir: creating === 'dir' }
      creating = null
      newName = ''
      await load(dir)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }

  function confirm() {
    onPick(Object.values(picked).map(e => newWorkspaceFileEntry(location.workspace.id, e.path, e.is_dir === true)))
  }
</script>

<BottomSheet title={t('workspace_files.pick_title')} subtitle={location.project_slug} historyKey="workspaceFilePicker" {onClose}>
  <nav class="crumbs" aria-label={t('workspace_files.location')}>
    <button type="button" class="crumb" onclick={() => goTo(0)} disabled={crumbs.length === 0}>{t('workspace_files.root')}</button>
    {#each crumbs as c, i (i)}
      <ChevronRight size={12} />
      <button type="button" class="crumb" onclick={() => goTo(i + 1)} disabled={i === crumbs.length - 1}>{c}</button>
    {/each}
  </nav>

  <div class="tools">
    <button type="button" class="tool" onclick={() => { creating = 'file'; newName = '' }}><FilePlus size={14} /> {t('workspace.new_file')}</button>
    <button type="button" class="tool" onclick={() => { creating = 'dir'; newName = '' }}><FolderPlus size={14} /> {t('workspace.new_folder')}</button>
  </div>
  {#if creating}
    <form class="create" onsubmit={(e) => { e.preventDefault(); void create() }}>
      <input type="text" bind:value={newName} placeholder={creating === 'file' ? t('workspace.new_file_placeholder') : t('workspace.new_folder_placeholder')} />
      <button type="submit" class="btn" disabled={busy || !newName.trim()}>{t('common.create')}</button>
      <button type="button" class="btn subtle" onclick={() => (creating = null)}>{t('common.cancel')}</button>
    </form>
  {/if}

  {#if error}
    <p class="error">{error}</p>
  {/if}

  <ul class="list">
    {#if entries === null}
      <li class="status">{t('common.loading')}</li>
    {:else if entries.length === 0}
      <li class="status">{t('workspace_files.empty_folder')}</li>
    {:else}
      {#each entries as e (e.path)}
        <li class="row">
          <button type="button" class="check" class:on={!!picked[e.path]} onclick={() => toggle(e)} aria-pressed={!!picked[e.path]} aria-label={t('workspace.select_entry', { name: workspacePathName(e.path) })}>
            {#if picked[e.path]}<SquareCheck size={18} />{:else}<Square size={18} />{/if}
          </button>
          <button type="button" class="label" onclick={() => e.is_dir ? (dir = e.path) : toggle(e)}>
            {#if e.is_dir}<Folder size={15} />{:else}<FileText size={15} />{/if}
            <span class="name">{workspacePathName(e.path)}</span>
            {#if e.is_dir}<ChevronRight size={14} class="into" />{/if}
          </button>
        </li>
      {/each}
    {/if}
  </ul>

  <footer>
    <span class="count">{count > 0 ? t('workspace_files.pick_count', { count: String(count) }) : t('workspace_files.pick_hint')}</span>
    <button type="button" class="btn primary" disabled={count === 0} onclick={confirm}>{t('common.add')}</button>
  </footer>
</BottomSheet>

<style>
  .crumbs {
    display: flex;
    align-items: center;
    gap: 0.15rem;
    flex-wrap: wrap;
    color: var(--text-faint);
    font-size: 0.82rem;
  }
  .crumb {
    background: transparent;
    border: none;
    color: var(--accent);
    font: inherit;
    font-size: 0.82rem;
    padding: 0.2rem 0.1rem;
  }
  .crumb:disabled { color: var(--text); }
  .tools { display: flex; gap: 0.4rem; }
  .tool {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    min-height: 36px;
    padding: 0.3rem 0.7rem;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: transparent;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.82rem;
  }
  .create { display: flex; gap: 0.4rem; }
  .create input {
    flex: 1;
    min-width: 0;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text);
    font: inherit;
    padding: 0.45rem 0.6rem;
  }
  .btn {
    min-height: 40px;
    padding: 0.4rem 0.9rem;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
    font: inherit;
    font-size: 0.9rem;
  }
  .btn.primary { background: var(--accent); border-color: var(--accent); color: #fff; }
  .btn.subtle { border-color: transparent; color: var(--text-muted); }
  .btn:disabled { opacity: 0.5; }
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    max-height: 45vh;
    overflow: auto;
    border: 1px solid var(--border);
    border-radius: 10px;
  }
  .row { display: flex; align-items: center; border-bottom: 1px solid var(--border); }
  .row:last-child { border-bottom: none; }
  .check {
    background: transparent;
    border: none;
    color: var(--text-faint);
    padding: 0.6rem;
    display: inline-flex;
  }
  .check.on { color: var(--accent); }
  .label {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    flex: 1;
    min-width: 0;
    min-height: 44px;
    padding: 0.4rem 0.5rem 0.4rem 0;
    border: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.92rem;
    text-align: left;
  }
  .name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .label :global(.into) { color: var(--text-faint); }
  .status { padding: 0.75rem; font-size: 0.85rem; color: var(--text-faint); }
  .error { margin: 0; font-size: 0.82rem; color: var(--danger-text); }
  footer { display: flex; align-items: center; gap: 0.6rem; }
  .count { flex: 1; font-size: 0.82rem; color: var(--text-muted); }
</style>
