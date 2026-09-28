<script lang="ts">
  import { FilePlus, FolderPlus, LayoutTemplate } from 'lucide-svelte'
  import { t } from '../../lib/i18n.svelte'
  import WorkspaceNewEntryDialog from './WorkspaceNewEntryDialog.svelte'
  import GenerateTemplateDialog from './GenerateTemplateDialog.svelte'

  // The three structure actions (New file / New folder / From template)
  // and the dialogs they open, as one reusable cluster: the panel's Files
  // header shows it for the root, and the same component hosts the dialogs
  // the tree's per-folder hover actions open (`open(dir, kind)`). One
  // place decides what "create" means; the panel and the picker only ask.
  let { brandSlug, streamSlug, projectSlug, cardTitle = '', onCreated, compact = false }: {
    brandSlug: string
    streamSlug: string
    projectSlug: string
    cardTitle?: string
    /** The created/generated path, workspace-relative, plus what it is. */
    onCreated: (rel: string, kind: 'file' | 'dir') => void
    /** Icon-only buttons (panel header); labelled otherwise (picker). */
    compact?: boolean
  } = $props()

  let pending = $state<{ dir: string; kind: 'file' | 'dir' | 'template' } | null>(null)

  export function open(dir: string, kind: 'file' | 'dir' | 'template') {
    pending = { dir, kind }
  }

  function created(rel: string, kind: 'file' | 'dir') {
    pending = null
    onCreated(rel, kind)
  }
</script>

<span class="structure-actions" class:compact>
  <button class="act" onclick={() => open('', 'file')} title={t('workspace.new_file')} aria-label={t('workspace.new_file')}><FilePlus size={13} />{#if !compact}<span>{t('workspace.new_file')}</span>{/if}</button>
  <button class="act" onclick={() => open('', 'dir')} title={t('workspace.new_folder')} aria-label={t('workspace.new_folder')}><FolderPlus size={13} />{#if !compact}<span>{t('workspace.new_folder')}</span>{/if}</button>
  <button class="act" onclick={() => open('', 'template')} title={t('workspace.from_template')} aria-label={t('workspace.from_template')}><LayoutTemplate size={13} />{#if !compact}<span>{t('workspace.from_template')}</span>{/if}</button>
</span>

{#if pending && pending.kind !== 'template'}
  <WorkspaceNewEntryDialog {brandSlug} {streamSlug} {projectSlug} dir={pending.dir} kind={pending.kind} onCreated={(rel) => created(rel, pending!.kind === 'dir' ? 'dir' : 'file')} onClose={() => pending = null} />
{:else if pending}
  <GenerateTemplateDialog {brandSlug} {streamSlug} {projectSlug} targetDir={pending.dir} {cardTitle} onGenerated={(rel) => created(rel, 'dir')} onClose={() => pending = null} />
{/if}

<style>
  .structure-actions { display: flex; gap: 0.15rem; }
  .act {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.2rem 0.4rem;
    border: none;
    border-radius: 4px;
    background: none;
    color: var(--text-muted);
    font-size: 0.74rem;
    cursor: pointer;
  }
  .compact .act { padding: 0.15rem; }
  .act:hover,
  .act:focus-visible {
    color: var(--text-primary);
    background: var(--bg-elevated);
  }
</style>
