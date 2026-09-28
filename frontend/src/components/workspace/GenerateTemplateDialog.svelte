<script lang="ts">
  import { X, ArrowLeft, LayoutTemplate } from 'lucide-svelte'
  import { GenerateWorkspaceTemplate, ListProjectTemplates } from '@shared/api'
  import type { WorkspaceTemplateEntry } from '@shared/types'
  import { t } from '../../lib/i18n.svelte'
  import { showToast } from '../../lib/toast.svelte'
  import { focusTrap } from '../../lib/actions'

  // Generate a Folder Template into the workspace: pick a template
  // (workspace-resident ones first, auto-selected when there's exactly
  // one), fill params, confirm the workspace-relative target. Reached
  // from a folder in the tree (target prefilled) or from a card's
  // Workspace Files picker (cardTitle prefills title-ish params and feeds
  // {{$bruvCard}}).
  let { brandSlug, streamSlug, projectSlug, targetDir = '', cardTitle = '', onGenerated, onClose }: {
    brandSlug: string
    streamSlug: string
    projectSlug: string
    /** Folder to generate under; '' lets the template's default target decide. */
    targetDir?: string
    cardTitle?: string
    onGenerated: (rel: string) => void
    onClose: () => void
  } = $props()

  let templates = $state<WorkspaceTemplateEntry[] | null>(null)
  let selected = $state<WorkspaceTemplateEntry | null>(null)
  let values = $state<Record<string, string>>({})
  // Prefilled from the folder the dialog was opened on; typing overrides.
  let targetOverride = $state<string | null>(null)
  const targetRel = $derived(targetOverride ?? targetDir)
  let busy = $state(false)

  const visibleParams = $derived(selected?.parameters?.filter(p => p.name && p.prompt) ?? [])

  $effect(() => {
    void loadTemplates()
  })

  async function loadTemplates() {
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

  function chooseTemplate(tpl: WorkspaceTemplateEntry) {
    selected = tpl
    values = {}
    for (const p of tpl.parameters ?? []) {
      if (!p.name || !p.prompt) continue
      values[p.name] = cardTitle && /title|name/i.test(p.name) ? cardTitle : (p.defaultValue ?? '')
    }
  }

  async function generate() {
    if (!selected || busy) return
    busy = true
    try {
      const rel = await GenerateWorkspaceTemplate(brandSlug, streamSlug, projectSlug, selected.id, targetRel, cardTitle, values)
      showToast(t('workspace.generated_at', { path: rel }), 'success')
      onGenerated(rel)
    } catch (e) {
      showToast(t('workspace.generate_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    } finally {
      busy = false
    }
  }

  function back() {
    if (selected && (templates?.length ?? 0) > 1) selected = null
    else onClose()
  }

  // Layered dialog: shield both window keys (UI-CONVENTIONS §8.1).
  function onKeydown(e: KeyboardEvent) {
    e.stopPropagation()
    if (e.key === 'Escape') { e.preventDefault(); onClose() }
    else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); void generate() }
  }
</script>

<div class="dialog-overlay" role="presentation" onclick={(e) => { if (e.target === e.currentTarget) onClose() }}>
  <div class="dialog" role="dialog" aria-label={t('workspace.from_template')} tabindex="-1" use:focusTrap onkeydown={onKeydown}>
    <header>
      {#if selected}
        <button class="icon-btn" onclick={back} title={t('common.back')} aria-label={t('common.back')}><ArrowLeft size={16} /></button>
      {/if}
      <h3><LayoutTemplate size={15} /> {t('workspace.from_template')}</h3>
      <button class="icon-btn" onclick={onClose} title={t('common.close')} aria-label={t('common.close')}><X size={16} /></button>
    </header>

    {#if templates === null}
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
          <span>{t('workspace.generate_target')}</span>
          <!-- Blank = the template's own defaultTargetPath (shown as the
               placeholder); typing overrides with a workspace-root-relative
               path. -->
          <input
            type="text"
            value={targetRel}
            oninput={(e) => targetOverride = e.currentTarget.value}
            placeholder={selected.default_target_path
              ? t('workspace.generate_target_tpl_default', { path: selected.default_target_path })
              : t('workspace.generate_target_placeholder')}
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
    z-index: 98;
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
  .tpl-name { margin: 0; font-size: 0.82rem; font-weight: 600; color: var(--text-primary); }
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
  footer { display: flex; justify-content: flex-end; gap: 0.5rem; padding-top: 0.25rem; }
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
