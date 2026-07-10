<script lang="ts">
  import { X, FolderOpen, LayoutTemplate, ArrowLeft, Pencil, FolderInput, Trash2 } from 'lucide-svelte'
  import { AttachWorkspace, DeleteWorkspaceTemplate, GenerateWorkspaceFromTemplate, ImportWorkspaceTemplate, InspectWorkspaceTemplateFolder, ListWorkspaceTemplates, PickFolder } from '@shared/api'
  import type { Workspace, WorkspaceTemplateEntry } from '@shared/types'
  import { t } from '../../lib/i18n.svelte'
  import { showToast } from '../../lib/toast.svelte'
  import { showConfirm } from '../../lib/confirm.svelte'
  import { focusTrap } from '../../lib/actions'
  import TemplateEditorDialog from './TemplateEditorDialog.svelte'

  let { brandSlug, streamSlug, projectSlug, onAttached, onClose }: {
    brandSlug: string
    streamSlug: string
    projectSlug: string
    onAttached: (ws: Workspace) => void
    onClose: () => void
  } = $props()

  type Step = 'choose' | 'template' | 'params'
  let step = $state<Step>('choose')
  let busy = $state(false)

  let templates = $state<WorkspaceTemplateEntry[] | null>(null)
  let selected = $state<WorkspaceTemplateEntry | null>(null)
  // Parameter values keyed by parameter name (names are stable IDs in the
  // .ft format; never key by index).
  let values = $state<Record<string, string>>({})
  let targetParent = $state('')

  const visibleParams = $derived(selected?.parameters?.filter(p => p.name && p.prompt) ?? [])

  async function attachExisting() {
    busy = true
    try {
      const path = await PickFolder(t('workspace.pick_folder_title'))
      if (!path) return
      const ws = await AttachWorkspace(brandSlug, streamSlug, projectSlug, path)
      showToast(t('workspace.attached'), 'success')
      onAttached(ws)
    } catch (e) {
      showToast(t('workspace.attach_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    } finally {
      busy = false
    }
  }

  let editorRef = $state<string | null>(null)

  async function loadTemplates() {
    try {
      // ?? [] belt-and-braces: null must never land in `templates` — it's
      // the "still loading" sentinel, and an old server's Go nil slice
      // marshals to JSON null.
      templates = (await ListWorkspaceTemplates()) ?? []
    } catch (e) {
      templates = []
      showToast(t('workspace.templates_load_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    }
  }

  async function openTemplates() {
    step = 'template'
    if (templates === null) await loadTemplates()
  }

  async function importTemplate() {
    try {
      const src = await PickFolder(t('workspace.import_template'))
      if (!src) return
      const insp = await InspectWorkspaceTemplateFolder(src)
      if (!insp.is_template) {
        showToast(t('workspace.import_not_template'), 'error')
        return
      }
      if (insp.large_warning) {
        const size = `${Math.round(insp.size_bytes / (1024 * 1024))} MB`
        if (!await showConfirm(t('workspace.import_large_confirm', { size }))) return
      }
      await ImportWorkspaceTemplate(src, '')
      showToast(t('workspace.import_done'), 'success')
      await loadTemplates()
    } catch (e) {
      showToast(t('workspace.import_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    }
  }

  async function deleteTemplate(tpl: WorkspaceTemplateEntry) {
    if (!await showConfirm(t('workspace.delete_template_confirm', { name: tpl.name }))) return
    try {
      await DeleteWorkspaceTemplate(tpl.id)
      showToast(t('workspace.template_deleted'), 'success')
      await loadTemplates()
    } catch (e) {
      showToast(t('workspace.template_delete_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    }
  }

  function chooseTemplate(tpl: WorkspaceTemplateEntry) {
    selected = tpl
    values = {}
    for (const p of tpl.parameters ?? []) {
      if (p.name && p.prompt) values[p.name] = p.defaultValue ?? ''
    }
    targetParent = ''
    step = 'params'
  }

  async function pickTarget() {
    try {
      const path = await PickFolder(t('workspace.pick_target_title'))
      if (path) targetParent = path
    } catch (e) {
      showToast(t('workspace.attach_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    }
  }

  async function generate() {
    if (!selected || !targetParent) return
    busy = true
    try {
      const ws = await GenerateWorkspaceFromTemplate(brandSlug, streamSlug, projectSlug, selected.id, targetParent, values)
      showToast(t('workspace.generated'), 'success')
      onAttached(ws)
    } catch (e) {
      showToast(t('workspace.generate_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    } finally {
      busy = false
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<div class="dialog-overlay" role="presentation" onclick={(e) => { if (e.target === e.currentTarget) onClose() }}>
  <div class="dialog" role="dialog" aria-label={t('workspace.attach_title')} tabindex="-1" use:focusTrap onkeydown={onKeydown}>
    <header>
      {#if step !== 'choose'}
        <button class="icon-btn" onclick={() => step = step === 'params' ? 'template' : 'choose'} title={t('common.back')} aria-label={t('common.back')}><ArrowLeft size={16} /></button>
      {/if}
      <h3>{t('workspace.attach_title')}</h3>
      <button class="icon-btn" onclick={onClose} title={t('common.close')} aria-label={t('common.close')}><X size={16} /></button>
    </header>

    {#if step === 'choose'}
      <div class="choices">
        <button class="choice" disabled={busy} onclick={attachExisting}>
          <FolderOpen size={22} />
          <strong>{t('workspace.attach_existing')}</strong>
          <span>{t('workspace.attach_existing_hint')}</span>
        </button>
        <button class="choice" disabled={busy} onclick={openTemplates}>
          <LayoutTemplate size={22} />
          <strong>{t('workspace.from_template')}</strong>
          <span>{t('workspace.from_template_hint')}</span>
        </button>
      </div>

    {:else if step === 'template'}
      <div class="list">
        {#if templates === null}
          <p class="muted">{t('common.loading')}</p>
        {:else if templates.length === 0}
          <p class="muted">{t('workspace.no_templates')}</p>
        {:else}
          {#each templates as tpl (tpl.id)}
            <div class="template-row-wrap">
              <button class="template-row" onclick={() => chooseTemplate(tpl)}>
                <strong>{tpl.name}</strong>
                {#if tpl.description}<span class="desc">{tpl.description}</span>{/if}
                <span class="scope">{tpl.scope === 'global' ? t('workspace.scope_global') : tpl.scope}</span>
              </button>
              <button class="icon-btn" onclick={() => editorRef = tpl.id} title={t('workspace.edit_template')} aria-label={t('workspace.edit_template')}><Pencil size={13} /></button>
              <button class="icon-btn danger" onclick={() => deleteTemplate(tpl)} title={t('workspace.delete_template')} aria-label={t('workspace.delete_template')}><Trash2 size={13} /></button>
            </div>
          {/each}
        {/if}
        <button class="btn subtle import-btn" onclick={importTemplate}><FolderInput size={14} /> {t('workspace.import_template')}</button>
      </div>

    {:else if step === 'params' && selected}
      <div class="form">
        {#each visibleParams as p (p.name)}
          <label>
            <span>{p.prompt}</span>
            <input type="text" bind:value={values[p.name]} placeholder={p.placeholder ?? ''} />
          </label>
        {/each}
        <label>
          <span>{t('workspace.target_folder')}</span>
          <div class="target-row">
            <input type="text" bind:value={targetParent} placeholder={t('workspace.target_placeholder')} />
            <button class="btn subtle" onclick={pickTarget}><FolderOpen size={14} /> {t('common.browse')}</button>
          </div>
        </label>
        <footer>
          <button class="btn subtle" onclick={onClose}>{t('common.cancel')}</button>
          <button class="btn primary" disabled={busy || !targetParent} onclick={generate}>
            {busy ? t('workspace.generating') : t('workspace.generate')}
          </button>
        </footer>
      </div>
    {/if}
  </div>
</div>

{#if editorRef}
  <TemplateEditorDialog templateRef={editorRef} onSaved={loadTemplates} onClose={() => editorRef = null} />
{/if}

<style>
  .dialog-overlay {
    position: fixed;
    inset: 0;
    background: color-mix(in srgb, var(--bg-base) 60%, transparent);
    backdrop-filter: blur(2px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 95;
  }
  .dialog {
    width: min(480px, 92vw);
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 10px;
    box-shadow: var(--shadow-lg, 0 12px 40px rgba(0, 0, 0, 0.4));
    overflow: hidden;
  }
  header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.65rem 0.85rem;
    border-bottom: 1px solid var(--border-muted);
  }
  h3 {
    flex: 1;
    margin: 0;
    font-size: 0.9rem;
    font-weight: 600;
  }
  .choices {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.75rem;
    padding: 1rem;
  }
  .choice {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.4rem;
    padding: 0.9rem;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--bg-base);
    color: var(--text-secondary);
    text-align: left;
    cursor: pointer;
  }
  .choice:hover { border-color: var(--accent); color: var(--text-primary); }
  .choice strong { font-size: 0.85rem; color: var(--text-primary); }
  .choice span { font-size: 0.72rem; color: var(--text-faint); }
  .list {
    overflow: auto;
    padding: 0.6rem;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }
  .template-row-wrap {
    display: flex;
    align-items: stretch;
    gap: 0.25rem;
  }
  .template-row-wrap .icon-btn { align-self: center; }
  .template-row {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.15rem;
    padding: 0.55rem 0.7rem;
    border: 1px solid var(--border-muted);
    border-radius: 7px;
    background: none;
    color: var(--text-secondary);
    text-align: left;
    cursor: pointer;
  }
  .import-btn { align-self: flex-start; margin-top: 0.2rem; }
  .template-row:hover { border-color: var(--accent); }
  .template-row strong { font-size: 0.82rem; color: var(--text-primary); }
  .template-row .desc { font-size: 0.74rem; color: var(--text-muted); }
  .template-row .scope { font-size: 0.66rem; color: var(--text-faint); text-transform: uppercase; letter-spacing: 0.04em; }
  .form {
    padding: 0.9rem;
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
    overflow: auto;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    font-size: 0.78rem;
    color: var(--text-muted);
  }
  input {
    background: var(--bg-base);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-primary);
    padding: 0.4rem 0.55rem;
    font-size: 0.82rem;
  }
  input:focus { outline: none; border-color: var(--accent); }
  .target-row { display: flex; gap: 0.4rem; }
  .target-row input { flex: 1; }
  footer {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    padding-top: 0.25rem;
  }
  .btn {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    border: 1px solid var(--border);
    background: var(--bg-base);
    color: var(--text-secondary);
    border-radius: 6px;
    padding: 0.35rem 0.75rem;
    font-size: 0.78rem;
    cursor: pointer;
  }
  .btn:hover { color: var(--text-primary); background: var(--bg-subtle-hover); }
  .btn.primary { background: var(--accent); border-color: var(--accent); color: white; }
  .btn.primary:disabled { opacity: 0.6; cursor: default; }
  .icon-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.3rem;
    border-radius: 6px;
    display: flex;
  }
  .icon-btn:hover { color: var(--text-primary); background: var(--bg-subtle-hover); }
  .icon-btn.danger:hover { color: var(--danger); }
  .muted { color: var(--text-faint); font-size: 0.8rem; padding: 0.5rem; }
</style>
