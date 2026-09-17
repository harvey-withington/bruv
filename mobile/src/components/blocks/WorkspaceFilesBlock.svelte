<script lang="ts">
  import { Plus, AlertTriangle, FileText, X } from 'lucide-svelte'
  import type { Block, CardPin, WorkspaceFileEntry, WorkspaceLocation, WorkspaceState } from '@shared/types'
  import { asWorkspaceFiles, asWorkspaceFilesDisplay, workspaceParentPath, workspacePathName } from '@shared/blockValues'
  import { buildWorkspaceFilesTree, mergeWorkspaceFiles, sortedWorkspaceFiles } from '@shared/workspaceFiles'
  import { repoRPC } from '../../lib/auth'
  import { t } from '../../lib/i18n.svelte'
  import { ensureWorkspaceLocation, workspaceLocation } from '../../lib/workspaceLocations.svelte'
  import { workspaceDocumentSource, type DocumentSource } from '../../lib/documentSource'
  import { withValue } from './narrow'
  import WorkspaceFilesNodes from './WorkspaceFilesNodes.svelte'
  import WorkspaceFilePickerSheet from '../WorkspaceFilePickerSheet.svelte'
  import DocumentSheet from '../DocumentSheet.svelte'

  // The Workspace Files block on the phone: the files a card is about,
  // opened right here in the document sheet over the connection — the
  // host holds the bytes, so nothing needs to be on the phone. The tree /
  // flat setting is the block's (meta.display), shared with desktop.
  let { block, cardId, onChange }: {
    block: Block
    cardId: string
    onChange: (next: Block) => void
  } = $props()

  const entries = $derived(asWorkspaceFiles(block.value))
  const display = $derived(asWorkspaceFilesDisplay(block.meta?.display))
  const tree = $derived(buildWorkspaceFilesTree(entries, labelFor))
  const flat = $derived(sortedWorkspaceFiles(entries))
  const workspaceIds = $derived([...new Set(entries.map(e => e.workspace_id))])
  const unresolved = $derived(workspaceIds.filter(id => workspaceLocation(id).status === 'error'))
  // Resolution is a write, so it lives in an effect — never in a derived.
  $effect(() => {
    for (const id of workspaceIds) ensureWorkspaceLocation(id)
  })

  let expanded = $state<Record<string, boolean>>({})
  let picker = $state<WorkspaceLocation | null>(null)
  let viewing = $state<DocumentSource | null>(null)
  let errorMsg = $state<string | null>(null)
  let resolving = $state(false)

  function labelFor(id: string): string {
    const st = workspaceLocation(id)
    return st.status === 'ready' ? st.location.project_slug : id
  }

  function locationOf(id: string): WorkspaceLocation | null {
    const st = workspaceLocation(id)
    return st.status === 'ready' ? st.location : null
  }

  // Which workspace to add from: the entries' own, or — for an empty
  // block — the first of the card's projects that has one attached.
  async function targetLocation(): Promise<WorkspaceLocation | null> {
    for (const id of workspaceIds) {
      const loc = locationOf(id)
      if (loc) return loc
    }
    resolving = true
    errorMsg = null
    try {
      const pins = (await repoRPC<CardPin[]>('GetCardPinBreadcrumbs', [cardId])) ?? []
      const seen = new Set<string>()
      for (const pin of pins) {
        const key = `${pin.brandSlug}/${pin.streamSlug}/${pin.projectSlug}`
        if (seen.has(key)) continue
        seen.add(key)
        const st = await repoRPC<WorkspaceState>('GetWorkspaceState', [pin.brandSlug, pin.streamSlug, pin.projectSlug])
        if (st?.attached && st.workspace) {
          return { brand_slug: pin.brandSlug, stream_slug: pin.streamSlug, project_slug: pin.projectSlug, workspace: st.workspace }
        }
      }
      errorMsg = t('workspace_files.no_workspace')
      return null
    } catch (e) {
      errorMsg = e instanceof Error ? e.message : String(e)
      return null
    } finally {
      resolving = false
    }
  }

  async function add() {
    const loc = await targetLocation()
    if (loc) picker = loc
  }

  function picked(added: WorkspaceFileEntry[]) {
    picker = null
    onChange(withValue(block, mergeWorkspaceFiles(entries, added)))
  }

  function remove(entry: WorkspaceFileEntry) {
    onChange(withValue(block, entries.filter(e => e.id !== entry.id)))
  }

  function open(entry: WorkspaceFileEntry) {
    openPath(entry.workspace_id, entry.path)
  }

  function openPath(workspaceId: string, path: string) {
    const loc = locationOf(workspaceId)
    if (!loc) {
      errorMsg = t('workspace_files.workspace_missing')
      return
    }
    errorMsg = null
    viewing = workspaceDocumentSource(loc.brand_slug, loc.stream_slug, loc.project_slug, path)
  }
</script>

<div class="wsfiles">
  {#if errorMsg}
    <p class="error"><AlertTriangle size={14} /> {errorMsg}</p>
  {/if}
  {#each unresolved as id (id)}
    <p class="error"><AlertTriangle size={14} /> {t('workspace_files.workspace_missing')}</p>
  {/each}

  {#if entries.length === 0}
    <p class="empty">{t('workspace_files.empty')}</p>
  {:else if display === 'flat'}
    <ul class="flat">
      {#each flat as entry (entry.id)}
        <li class="entry">
          <button type="button" class="row" onclick={() => open(entry)}>
            <FileText size={15} />
            <span class="text">
              <span class="name">{workspacePathName(entry.path)}</span>
              {#if workspaceParentPath(entry.path)}<span class="path">{workspaceParentPath(entry.path)}/</span>{/if}
            </span>
          </button>
          <button type="button" class="remove" onclick={() => remove(entry)} aria-label={t('workspace_files.remove')}><X size={14} /></button>
        </li>
      {/each}
    </ul>
  {:else}
    <WorkspaceFilesNodes nodes={tree} {expanded} onOpen={open} onOpenPath={openPath} onRemove={remove} />
  {/if}

  <button type="button" class="add" onclick={add} disabled={resolving}>
    <Plus size={14} />
    <span>{t('workspace_files.add')}</span>
  </button>
</div>

{#if picker}
  <WorkspaceFilePickerSheet location={picker} onPick={picked} onClose={() => (picker = null)} />
{/if}

{#if viewing}
  <DocumentSheet source={viewing} onClose={() => (viewing = null)} />
{/if}

<style>
  .wsfiles { display: flex; flex-direction: column; gap: 0.3rem; }
  .flat { list-style: none; margin: 0; padding: 0; }
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
  .text { display: flex; flex-direction: column; min-width: 0; line-height: 1.25; }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .path { font-size: 0.72rem; color: var(--text-faint); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .remove {
    background: transparent;
    border: none;
    color: var(--text-faint);
    padding: 0.5rem;
    display: inline-flex;
  }
  .empty { margin: 0; font-size: 0.85rem; color: var(--text-faint); }
  .error {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    margin: 0;
    font-size: 0.82rem;
    color: var(--danger-text);
  }
  .add {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    align-self: flex-start;
    min-height: 40px;
    padding: 0.35rem 0.75rem;
    border: 1px dashed var(--border);
    border-radius: 999px;
    background: transparent;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.85rem;
  }
  .add:active { color: var(--text); border-color: var(--accent); }
</style>
