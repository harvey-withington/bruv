<script lang="ts">
  import { Plus, ListTree, List, AlertTriangle } from 'lucide-svelte'
  import { GetCardPinBreadcrumbs, GetWorkspaceState } from '@shared/api'
  import type { WorkspaceFileEntry, WorkspaceFilesDisplay, WorkspaceLocation } from '@shared/types'
  import { buildWorkspaceFilesTree, mergeWorkspaceFiles, sortedWorkspaceFiles } from '@shared/workspaceFiles'
  import { t } from '../../lib/i18n.svelte'
  import { showToast } from '../../lib/toast.svelte'
  import { hasWorkspaceDrag, payloadToEntry, readWorkspaceDrag } from '../../lib/workspaceDrag'
  import { ensureWorkspaceLocation, workspaceLocation } from '../../lib/workspaceLocations.svelte'
  import WorkspaceFileEntryRow from './WorkspaceFileEntryRow.svelte'
  import WorkspaceFilesTreeView from './WorkspaceFilesTreeView.svelte'
  import WorkspaceFilePickerDialog from './WorkspaceFilePickerDialog.svelte'
  import WorkspaceFileViewer from './WorkspaceFileViewer.svelte'

  // The Workspace Files block: the files and folders a card is about,
  // opened in BRUV's document editor from right here. Structure (creating
  // files, folders, templates) belongs to the workspace tree; the picker
  // IS that tree, so both are one gesture away. Design:
  // plan/2026-09-17 workspace files block.md.
  let { entries, display, cardId, cardTitle, onUpdate, onDisplayChange }: {
    entries: WorkspaceFileEntry[]
    display: WorkspaceFilesDisplay
    cardId: string
    cardTitle: string
    onUpdate: (entries: WorkspaceFileEntry[]) => void
    onDisplayChange: (display: WorkspaceFilesDisplay) => void
  } = $props()

  let expanded = $state<Record<string, boolean>>({})
  let picker = $state<{ location: WorkspaceLocation; replacing?: WorkspaceFileEntry } | null>(null)
  let viewing = $state<{ location: WorkspaceLocation; path: string } | null>(null)
  let dragOver = $state(false)
  let resolvingTarget = $state(false)

  const workspaceIds = $derived([...new Set(entries.map(e => e.workspace_id))])
  // Resolution is a write, so it lives in an effect — never in a derived.
  $effect(() => {
    for (const id of workspaceIds) ensureWorkspaceLocation(id)
  })
  const tree = $derived(buildWorkspaceFilesTree(entries, labelFor))
  const flat = $derived(sortedWorkspaceFiles(entries))

  function labelFor(workspaceId: string): string {
    const st = workspaceLocation(workspaceId)
    return st.status === 'ready' ? st.location.project_slug : workspaceId
  }

  function locationOf(workspaceId: string): WorkspaceLocation | null {
    const st = workspaceLocation(workspaceId)
    return st.status === 'ready' ? st.location : null
  }

  /**
   * Which workspace to add from. Entries already name one; an empty block
   * asks the card's own pins — a card lives on projects, and a project has
   * at most one workspace. One match is used silently; none is a clear
   * message rather than an empty picker.
   */
  async function targetLocation(): Promise<WorkspaceLocation | null> {
    for (const id of workspaceIds) {
      const loc = locationOf(id)
      if (loc) return loc
    }
    resolvingTarget = true
    try {
      const pins = (await GetCardPinBreadcrumbs(cardId)) ?? []
      const seen = new Set<string>()
      for (const pin of pins) {
        const key = `${pin.brandSlug}/${pin.streamSlug}/${pin.projectSlug}`
        if (seen.has(key)) continue
        seen.add(key)
        const st = await GetWorkspaceState(pin.brandSlug, pin.streamSlug, pin.projectSlug)
        if (st.attached && st.workspace) {
          return { brand_slug: pin.brandSlug, stream_slug: pin.streamSlug, project_slug: pin.projectSlug, workspace: st.workspace }
        }
      }
      showToast(t('workspace_files.no_workspace'), 'error')
      return null
    } catch (e) {
      showToast(t('workspace.load_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
      return null
    } finally {
      resolvingTarget = false
    }
  }

  async function add() {
    const location = await targetLocation()
    if (location) picker = { location }
  }

  async function relink(entry: WorkspaceFileEntry) {
    const location = locationOf(entry.workspace_id) ?? await targetLocation()
    if (location) picker = { location, replacing: entry }
  }

  function picked(added: WorkspaceFileEntry[]) {
    const replacing = picker?.replacing
    picker = null
    const base = replacing ? entries.filter(e => e.id !== replacing.id) : entries
    onUpdate(mergeWorkspaceFiles(base, added))
  }

  function remove(entry: WorkspaceFileEntry) {
    onUpdate(entries.filter(e => e.id !== entry.id))
  }

  function open(entry: WorkspaceFileEntry) {
    openPath(entry.workspace_id, entry.path)
  }

  function openPath(workspaceId: string, path: string) {
    const location = locationOf(workspaceId)
    if (!location) {
      showToast(t('workspace_files.workspace_missing'), 'error')
      return
    }
    viewing = { location, path }
  }

  function onDragOver(e: DragEvent) {
    if (!hasWorkspaceDrag(e.dataTransfer)) return
    e.preventDefault()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'
    dragOver = true
  }

  function onDrop(e: DragEvent) {
    dragOver = false
    const payload = readWorkspaceDrag(e.dataTransfer)
    if (!payload) return
    e.preventDefault()
    onUpdate(mergeWorkspaceFiles(entries, [payloadToEntry(payload, `wsf-${crypto.randomUUID().slice(0, 8)}`)]))
  }

  // A workspace that no longer resolves (detached) is reported once at the
  // top, not per row — the rows can't know, only the resolver can.
  const unresolved = $derived(workspaceIds.filter(id => workspaceLocation(id).status === 'error'))
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="wsfiles"
  class:drag-over={dragOver}
  ondragover={onDragOver}
  ondragleave={() => dragOver = false}
  ondrop={onDrop}
>
  <div class="bar">
    <button class="add" onclick={add} disabled={resolvingTarget}><Plus size={12} /> {t('workspace_files.add')}</button>
    <span class="grow"></span>
    {#if entries.length > 0}
      <div class="segmented" role="group" aria-label={t('workspace_files.display')}>
        <button class="seg" class:on={display === 'tree'} onclick={() => onDisplayChange('tree')} title={t('workspace_files.display_tree')} aria-label={t('workspace_files.display_tree')} aria-pressed={display === 'tree'}><ListTree size={12} /></button>
        <button class="seg" class:on={display === 'flat'} onclick={() => onDisplayChange('flat')} title={t('workspace_files.display_flat')} aria-label={t('workspace_files.display_flat')} aria-pressed={display === 'flat'}><List size={12} /></button>
      </div>
    {/if}
  </div>

  {#each unresolved as id (id)}
    <p class="warning"><AlertTriangle size={12} /> {t('workspace_files.workspace_missing')}</p>
  {/each}

  {#if entries.length === 0}
    <p class="empty">{t('workspace_files.empty')}</p>
  {:else if display === 'flat'}
    <ul class="flat">
      {#each flat as entry (entry.id)}
        <li><WorkspaceFileEntryRow {entry} showPath onOpen={open} onRemove={remove} onRelink={relink} /></li>
      {/each}
    </ul>
  {:else}
    <WorkspaceFilesTreeView nodes={tree} {expanded} onOpen={open} onOpenPath={openPath} onRemove={remove} onRelink={relink} />
  {/if}
</div>

{#if picker}
  <WorkspaceFilePickerDialog location={picker.location} {cardTitle} onPick={picked} onClose={() => picker = null} />
{/if}

{#if viewing}
  <WorkspaceFileViewer
    brandSlug={viewing.location.brand_slug}
    streamSlug={viewing.location.stream_slug}
    projectSlug={viewing.location.project_slug}
    root={viewing.location.workspace.origin.url ?? ''}
    path={viewing.path}
    onClose={() => viewing = null}
  />
{/if}

<style>
  .wsfiles {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    border: 1px dashed transparent;
    border-radius: 6px;
    transition: border-color var(--duration-fast, 0.12s), background var(--duration-fast, 0.12s);
  }
  .wsfiles.drag-over {
    border-color: var(--accent);
    background: var(--accent-glow-2);
  }
  .bar { display: flex; align-items: center; gap: 0.3rem; }
  .grow { flex: 1; }
  .add {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.2rem 0.5rem;
    border: 1px dashed var(--border);
    border-radius: 999px;
    background: none;
    color: var(--text-muted);
    font-size: 0.74rem;
    cursor: pointer;
  }
  .add:hover,
  .add:focus-visible { color: var(--text-primary); border-color: var(--accent); }
  .segmented {
    display: flex;
    border: 1px solid var(--border-muted);
    border-radius: 4px;
    overflow: hidden;
  }
  .seg {
    display: flex;
    align-items: center;
    padding: 0.15rem 0.3rem;
    border: none;
    background: none;
    color: var(--text-faint);
    cursor: pointer;
  }
  .seg.on { color: var(--accent); background: var(--bg-elevated); }
  .seg:hover { color: var(--text-primary); }
  .flat { list-style: none; margin: 0; padding: 0; }
  .empty { margin: 0; padding: 0.3rem 0.35rem; font-size: 0.78rem; color: var(--text-faint); }
  .warning {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    margin: 0;
    font-size: 0.72rem;
    color: var(--warning, #f59e0b);
  }
</style>
