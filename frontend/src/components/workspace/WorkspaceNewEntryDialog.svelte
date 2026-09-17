<script lang="ts">
  import { X, FilePlus, FolderPlus } from 'lucide-svelte'
  import { CreateWorkspaceDir, CreateWorkspaceFile } from '@shared/api'
  import { t } from '../../lib/i18n.svelte'
  import { showToast } from '../../lib/toast.svelte'
  import { focusOnMount, focusTrap } from '../../lib/actions'

  // "New file" / "New folder" inside one workspace directory. One text
  // field; the backend refuses an existing path, so a typo can never
  // truncate a file. Structure belongs to the workspace tree — this
  // dialog is reached from it (and from the picker, which IS the tree).
  let { brandSlug, streamSlug, projectSlug, dir, kind, onCreated, onClose }: {
    brandSlug: string
    streamSlug: string
    projectSlug: string
    /** Parent directory, workspace-relative ('' = root). */
    dir: string
    kind: 'file' | 'dir'
    onCreated: (rel: string) => void
    onClose: () => void
  } = $props()

  let name = $state('')
  let busy = $state(false)

  const title = $derived(kind === 'file' ? t('workspace.new_file') : t('workspace.new_folder'))
  const placeholder = $derived(kind === 'file' ? t('workspace.new_file_placeholder') : t('workspace.new_folder_placeholder'))

  async function create() {
    const trimmed = name.trim()
    if (!trimmed || busy) return
    busy = true
    const rel = dir ? `${dir}/${trimmed}` : trimmed
    try {
      const made = kind === 'file'
        ? await CreateWorkspaceFile(brandSlug, streamSlug, projectSlug, rel)
        : await CreateWorkspaceDir(brandSlug, streamSlug, projectSlug, rel)
      onCreated(made)
    } catch (e) {
      showToast(t('workspace.create_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    } finally {
      busy = false
    }
  }

  // Layered over CardDetail / the picker: both window keys stay here
  // (UI-CONVENTIONS §8.1).
  function onKeydown(e: KeyboardEvent) {
    e.stopPropagation()
    if (e.key === 'Escape') { e.preventDefault(); onClose() }
    else if (e.key === 'Enter') { e.preventDefault(); void create() }
  }
</script>

<div class="dialog-overlay" role="presentation" onclick={(e) => { if (e.target === e.currentTarget) onClose() }}>
  <div class="dialog" role="dialog" aria-label={title} tabindex="-1" use:focusTrap onkeydown={onKeydown}>
    <header>
      <h3>{#if kind === 'file'}<FilePlus size={15} />{:else}<FolderPlus size={15} />{/if} {title}</h3>
      <button class="icon-btn" onclick={onClose} title={t('common.close')} aria-label={t('common.close')}><X size={16} /></button>
    </header>
    <div class="form">
      <label>
        <span>{dir ? t('workspace.new_in', { dir }) : t('workspace.new_at_root')}</span>
        <input type="text" use:focusOnMount bind:value={name} {placeholder} />
      </label>
      <footer>
        <button class="btn subtle" onclick={onClose}>{t('common.cancel')}</button>
        <button class="btn primary" disabled={busy || !name.trim()} onclick={create}>{t('common.create')}</button>
      </footer>
    </div>
  </div>
</div>

<style>
  .dialog-overlay {
    position: fixed;
    inset: 0;
    background: color-mix(in srgb, var(--bg-base) 60%, transparent);
    backdrop-filter: blur(2px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 98;
  }
  .dialog {
    width: min(380px, 92vw);
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
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin: 0;
    font-size: 0.9rem;
    font-weight: 600;
  }
  .form { padding: 0.9rem; display: flex; flex-direction: column; gap: 0.7rem; }
  label { display: flex; flex-direction: column; gap: 0.25rem; font-size: 0.78rem; color: var(--text-muted); }
  input {
    background: var(--bg-base);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-primary);
    padding: 0.4rem 0.55rem;
    font-size: 0.82rem;
  }
  input:focus { outline: none; border-color: var(--accent); }
  footer { display: flex; justify-content: flex-end; gap: 0.5rem; }
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
</style>
