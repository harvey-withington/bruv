<script lang="ts">
  import { Folder, FolderPlus, ExternalLink, FolderSearch, Unlink } from 'lucide-svelte'
  import { ClearCardFolder, GetWorkspaceState, OpenWorkspacePath, RevealWorkspacePath } from '@shared/api'
  import type { Card, WorkspaceState } from '@shared/types'
  import { nav } from '../../lib/store.svelte'
  import { t } from '../../lib/i18n.svelte'
  import { showToast } from '../../lib/toast.svelte'
  import { showConfirm } from '../../lib/confirm.svelte'
  import CreateCardFolderDialog from './CreateCardFolderDialog.svelte'

  // Card Folder — the card's intrinsic workspace-subfolder binding
  // (plan/2026-07-05 card folders design.md). Availability rule: no
  // workspace → no UI at all; the CREATE affordance needs the project
  // context this card was opened from; a BOUND chip renders wherever the
  // card renders, with actions live when its workspace is reachable here.
  let { card, onCardUpdated }: {
    card: Card
    onCardUpdated: (c: Card) => void
  } = $props()

  let wsState = $state<WorkspaceState | null>(null)
  let showCreate = $state(false)

  $effect(() => {
    wsState = null
    if (nav.brandSlug && nav.streamSlug && nav.projectSlug) {
      GetWorkspaceState(nav.brandSlug, nav.streamSlug, nav.projectSlug)
        .then(s => { wsState = s })
        .catch(() => { wsState = null })
    }
  })

  // The binding's workspace must be the one in view for device actions —
  // the root path comes from it.
  const wsMatches = $derived(
    !!card.folder && wsState?.attached && wsState.workspace?.id === card.folder.workspace_id
  )
  const wsRoot = $derived(wsState?.workspace?.origin.url ?? '')
  const folderName = $derived(card.folder ? card.folder.path.split('/').pop() ?? card.folder.path : '')

  async function openFolder() {
    if (!card.folder || !wsMatches) return
    try {
      await OpenWorkspacePath(wsRoot, card.folder.path)
    } catch (e) {
      showToast(t('workspace.open_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    }
  }

  async function reveal() {
    if (!card.folder || !wsMatches) return
    try {
      await RevealWorkspacePath(wsRoot, card.folder.path)
    } catch (e) {
      showToast(t('workspace.open_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    }
  }

  async function unbind() {
    const ok = await showConfirm(t('workspace.folder_unbind_confirm'))
    if (!ok) return
    try {
      const updated = await ClearCardFolder(card.id)
      onCardUpdated(updated)
      showToast(t('workspace.folder_unbound'), 'success')
    } catch (e) {
      showToast(t('workspace.folder_unbind_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    }
  }
</script>

{#if card.folder || (wsState?.attached && nav.projectSlug)}
  <div class="field-cell">
    <span class="cell-label">{t('workspace.folder')}</span>
    {#if card.folder}
      <div class="folder-chip action-reveal-parent" title={card.folder.path}>
        <button class="chip-main" onclick={openFolder} disabled={!wsMatches}>
          <Folder size={13} />
          <span class="name">{folderName}</span>
        </button>
        {#if wsMatches}
          <button class="action-reveal" onclick={reveal} title={t('workspace.reveal')} aria-label={t('workspace.reveal')}><FolderSearch size={12} /></button>
          <button class="action-reveal" onclick={openFolder} title={t('workspace.open_external')} aria-label={t('workspace.open_external')}><ExternalLink size={12} /></button>
        {/if}
        <button class="action-reveal action-reveal--danger" onclick={unbind} title={t('workspace.folder_unbind')} aria-label={t('workspace.folder_unbind')}><Unlink size={12} /></button>
      </div>
    {:else}
      <button class="create-btn" onclick={() => showCreate = true}>
        <FolderPlus size={13} />
        <span>{t('workspace.folder_create')}</span>
      </button>
    {/if}
  </div>
{/if}

{#if showCreate && nav.brandSlug && nav.streamSlug && nav.projectSlug}
  <CreateCardFolderDialog
    brandSlug={nav.brandSlug}
    streamSlug={nav.streamSlug}
    projectSlug={nav.projectSlug}
    {card}
    onCreated={(c) => { showCreate = false; onCardUpdated(c) }}
    onClose={() => showCreate = false}
  />
{/if}

<style>
  .field-cell {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    min-width: 0;
  }
  /* Mirrors CardDetail's .field-label metrics. */
  .cell-label {
    font-size: 0.66rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-faint);
  }
  .folder-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.15rem;
    max-width: 100%;
  }
  .chip-main {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.25rem 0.55rem;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-elevated);
    color: var(--text-secondary);
    font-size: 0.78rem;
    cursor: pointer;
    min-width: 0;
    transition: color 0.12s, border-color 0.12s;
  }
  .chip-main:hover:not(:disabled),
  .chip-main:focus-visible:not(:disabled) {
    color: var(--text-primary);
    border-color: var(--accent);
  }
  .chip-main:disabled {
    cursor: default;
    opacity: 0.7;
  }
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .create-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.25rem 0.55rem;
    border: 1px dashed var(--border);
    border-radius: 6px;
    background: none;
    color: var(--text-muted);
    font-size: 0.78rem;
    cursor: pointer;
    transition: color 0.12s, border-color 0.12s;
  }
  .create-btn:hover,
  .create-btn:focus-visible {
    color: var(--text-primary);
    border-color: var(--accent);
    border-style: solid;
  }
</style>
