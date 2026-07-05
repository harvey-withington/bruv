<script lang="ts">
  import { X, ArrowLeft, FolderPlus, LayoutTemplate, Link2, Folder } from 'lucide-svelte'
  import { GenerateCardFolder, GetWorkspaceState, LinkCardFolder, ListProjectTemplates } from '@shared/api'
  import type { Card, WorkspaceEntry, WorkspaceTemplateEntry } from '@shared/types'
  import { t } from '../../lib/i18n.svelte'
  import { showToast } from '../../lib/toast.svelte'
  import { focusTrap } from '../../lib/actions'

  // Create a card folder: pick a template (workspace-resident ones first,
  // auto-selected when there's exactly one), fill params (card title
  // pre-fills title-ish params), confirm the workspace-relative target.
  let { brandSlug, streamSlug, projectSlug, card, onCreated, onClose }: {
    brandSlug: string
    streamSlug: string
    projectSlug: string
    card: Card
    onCreated: (updated: Card) => void
    onClose: () => void
  } = $props()

  // menu → (template list → params) | link-existing dir list
  let view = $state<'menu' | 'template' | 'link'>('menu')
  let templates = $state<WorkspaceTemplateEntry[] | null>(null)
  let selected = $state<WorkspaceTemplateEntry | null>(null)
  let values = $state<Record<string, string>>({})
  let targetRel = $state('')
  let dirs = $state<WorkspaceEntry[] | null>(null)
  let busy = $state(false)

  const visibleParams = $derived(selected?.parameters?.filter(p => p.name && p.prompt) ?? [])

  async function openTemplates() {
    view = 'template'
    if (templates !== null) return
    try {
      templates = (await ListProjectTemplates(brandSlug, streamSlug, projectSlug)) ?? []
      // Preselect when the workspace scope has exactly one template —
      // the common case (the show's own episode template).
      const wsScoped = templates.filter(tpl => tpl.scope === 'workspace')
      if (wsScoped.length === 1) chooseTemplate(wsScoped[0])
    } catch (e) {
      templates = []
      showToast(t('workspace.templates_load_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    }
  }

  async function openLink() {
    view = 'link'
    if (dirs !== null) return
    try {
      const state = await GetWorkspaceState(brandSlug, streamSlug, projectSlug)
      dirs = (state.index?.tree ?? []).filter(e => e.is_dir)
    } catch (e) {
      dirs = []
      showToast(t('workspace.load_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    }
  }

  async function linkExisting(rel: string) {
    busy = true
    try {
      const updated = await LinkCardFolder(brandSlug, streamSlug, projectSlug, card.id, rel)
      showToast(t('workspace.folder_linked'), 'success')
      onCreated(updated)
    } catch (e) {
      showToast(t('workspace.folder_link_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    } finally {
      busy = false
    }
  }

  function chooseTemplate(tpl: WorkspaceTemplateEntry) {
    selected = tpl
    values = {}
    for (const p of tpl.parameters ?? []) {
      if (!p.name || !p.prompt) continue
      // Card-aware prefill: a title-ish param gets the card's title.
      values[p.name] = /title|name/i.test(p.name) ? card.title : (p.defaultValue ?? '')
    }
    targetRel = '' // workspace root by default; template may suggest better
  }

  async function generate() {
    if (!selected) return
    busy = true
    try {
      const updated = await GenerateCardFolder(brandSlug, streamSlug, projectSlug, card.id, selected.id, targetRel, values)
      showToast(t('workspace.folder_created'), 'success')
      onCreated(updated)
    } catch (e) {
      showToast(t('workspace.folder_create_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    } finally {
      busy = false
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<div class="dialog-overlay" role="presentation" onclick={(e) => { if (e.target === e.currentTarget) onClose() }}>
  <div class="dialog" role="dialog" aria-label={t('workspace.folder_create_title')} tabindex="-1" use:focusTrap onkeydown={onKeydown}>
    <header>
      {#if view !== 'menu'}
        <button class="icon-btn" onclick={() => { if (selected && (templates?.length ?? 0) > 1) { selected = null } else { selected = null; view = 'menu' } }} title={t('common.back')} aria-label={t('common.back')}><ArrowLeft size={16} /></button>
      {/if}
      <h3><FolderPlus size={15} /> {t('workspace.folder_create_title')}</h3>
      <button class="icon-btn" onclick={onClose} title={t('common.close')} aria-label={t('common.close')}><X size={16} /></button>
    </header>

    {#if view === 'menu'}
      <div class="choices">
        <button class="choice" onclick={openTemplates}>
          <LayoutTemplate size={20} />
          <strong>{t('workspace.from_template')}</strong>
          <span>{t('workspace.folder_from_template_hint')}</span>
        </button>
        <button class="choice" onclick={openLink}>
          <Link2 size={20} />
          <strong>{t('workspace.folder_link')}</strong>
          <span>{t('workspace.folder_link_hint')}</span>
        </button>
      </div>
    {:else if view === 'link'}
      <div class="list">
        {#if dirs === null}
          <p class="muted">{t('common.loading')}</p>
        {:else if dirs.length === 0}
          <p class="muted">{t('workspace.folder_link_empty')}</p>
        {:else}
          {#each dirs as d (d.path)}
            <button class="dir-row" disabled={busy} style:padding-left={`${0.7 + (d.path.split('/').length - 1) * 0.8}rem`} onclick={() => linkExisting(d.path)}>
              <Folder size={13} />
              <span class="name">{d.path.split('/').pop()}</span>
            </button>
          {/each}
        {/if}
      </div>
    {:else if templates === null}
      <p class="muted">{t('common.loading')}</p>
    {:else if !selected}
      <div class="list">
        {#if templates.length === 0}
          <p class="muted">{t('workspace.no_templates')}</p>
        {:else}
          {#each templates as tpl (tpl.id)}
            <button class="template-row" onclick={() => chooseTemplate(tpl)}>
              <strong>{tpl.name}</strong>
              {#if tpl.description}<span class="desc">{tpl.description}</span>{/if}
              <span class="scope">{tpl.scope === 'global' ? t('workspace.scope_global') : tpl.scope === 'workspace' ? t('workspace.scope_workspace') : tpl.scope}</span>
            </button>
          {/each}
        {/if}
      </div>
    {:else}
      <div class="form">
        <p class="tpl-name">{selected.name}</p>
        {#each visibleParams as p (p.name)}
          <label>
            <span>{p.prompt}</span>
            <input type="text" bind:value={values[p.name]} placeholder={p.placeholder ?? ''} />
          </label>
        {/each}
        <label>
          <span>{t('workspace.folder_target')}</span>
          <!-- Blank = the template's own defaultTargetPath (shown as the
               placeholder); typing overrides with a workspace-root-relative
               path. -->
          <input
            type="text"
            bind:value={targetRel}
            placeholder={selected.default_target_path
              ? t('workspace.folder_target_tpl_default', { path: selected.default_target_path })
              : t('workspace.folder_target_placeholder')}
          />
        </label>
        <footer>
          <button class="btn subtle" onclick={onClose}>{t('common.cancel')}</button>
          <button class="btn primary" disabled={busy} onclick={generate}>
            {busy ? t('workspace.generating') : t('workspace.generate')}
          </button>
        </footer>
      </div>
    {/if}
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
    z-index: 97;
  }
  .dialog {
    width: min(440px, 92vw);
    max-height: 78vh;
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
  .list {
    overflow: auto;
    padding: 0.6rem;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }
  .template-row {
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
  .template-row:hover { border-color: var(--accent); }
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
  .dir-row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.3rem 0.7rem;
    border: none;
    border-radius: 5px;
    background: none;
    color: var(--text-body);
    font-size: 0.82rem;
    text-align: left;
    cursor: pointer;
  }
  .dir-row:hover,
  .dir-row:focus-visible {
    background: var(--accent-glow-2);
    color: var(--text-primary);
  }
  .dir-row .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
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
  .tpl-name {
    margin: 0;
    font-size: 0.82rem;
    font-weight: 600;
    color: var(--text-primary);
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
  footer {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    padding-top: 0.25rem;
  }
  .btn {
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
  .muted { color: var(--text-faint); font-size: 0.8rem; padding: 0.8rem; }
</style>
