<script lang="ts">
  import { X, Plus, Trash2, GripVertical } from 'lucide-svelte'
  import { InspectWorkspaceTemplateFolder, SaveWorkspaceTemplate } from '@shared/api'
  import type { WorkspaceTemplateParameter } from '@shared/types'
  import { t } from '../../lib/i18n.svelte'
  import { showToast } from '../../lib/toast.svelte'
  import { focusTrap } from '../../lib/actions'
  import { computeReorder, wouldReorder, DROP_END } from '../../lib/reorder'

  // Feature-parity reproduction of the original Folder Templates app's
  // create-template form: name / description / default target path plus the
  // parameter grid. Writes .ft/template.json via SaveWorkspaceTemplate.
  // ref is a vault-relative template id or an absolute folder path (the
  // original's edit-in-place workflow); saving a folder without .ft/
  // templatizes it.
  let { templateRef, onSaved, onClose }: {
    templateRef: string
    onSaved?: () => void
    onClose: () => void
  } = $props()

  let loaded = $state(false)
  let name = $state('')
  let description = $state('')
  let defaultTargetPath = $state('')
  let saving = $state(false)

  // Parameters are plain objects with no natural stable id, but param
  // order = prompt order in the Create Card Folder dialog, so rows need
  // to be drag-reorderable and edit state needs to survive reordering
  // (CLAUDE.md: never key mutable state by array index). Each row gets
  // an ephemeral id on load; stripped back off on save.
  type ParamRow = WorkspaceTemplateParameter & { id: string }
  let paramRows = $state<ParamRow[]>([])

  // Guards against a second InspectWorkspaceTemplateFolder call (from
  // switching templateRef while the first is still in flight) clobbering
  // the newer load's state once the stale request finally resolves.
  let loadToken = 0

  $effect(() => {
    const ref = templateRef
    const token = ++loadToken
    loaded = false
    InspectWorkspaceTemplateFolder(ref)
      .then(insp => {
        if (token !== loadToken) return // superseded by a newer load
        name = insp.name
        description = insp.description
        defaultTargetPath = insp.default_target_path
        paramRows = (insp.parameters ?? []).map(p => ({ ...p, id: crypto.randomUUID() }))
        loaded = true
      })
      .catch((e: unknown) => {
        if (token !== loadToken) return
        showToast(t('workspace.editor_load_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
        onClose()
      })
  })

  function addParam() {
    paramRows = [...paramRows, {
      id: crypto.randomUUID(),
      name: '', type: 'text', prompt: '', placeholder: '',
      defaultValue: null, match: null,
      replaceInFileNames: true, replaceInFiles: true,
    }]
  }

  function removeParam(id: string) {
    paramRows = paramRows.filter(p => p.id !== id)
  }

  // --- Params drag-to-reorder ---
  let draggingId = $state<string | null>(null)
  let dropBeforeId = $state<string | typeof DROP_END | null>(null)

  function handleDragStart(e: DragEvent, id: string) {
    draggingId = id
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move'
      e.dataTransfer.setData('text/plain', id)
    }
  }

  function handleDragOver(e: DragEvent, overId: string, idx: number) {
    if (draggingId === null) return
    e.preventDefault()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const midY = rect.top + rect.height / 2
    let candidate: string | typeof DROP_END
    if (e.clientY < midY) {
      candidate = overId
    } else {
      const next = paramRows[idx + 1]
      candidate = next ? next.id : DROP_END
    }
    dropBeforeId = wouldReorder(paramRows, draggingId, candidate, 'move') ? candidate : null
  }

  function handleDragEnd() {
    draggingId = null
    dropBeforeId = null
  }

  function handleDrop(e: DragEvent) {
    e.preventDefault()
    if (draggingId === null || dropBeforeId === null) {
      handleDragEnd()
      return
    }
    const reordered = computeReorder(paramRows, draggingId, dropBeforeId, { mode: 'move' })
    handleDragEnd()
    if (reordered !== paramRows) paramRows = reordered
  }

  async function save() {
    saving = true
    try {
      await SaveWorkspaceTemplate(templateRef, {
        name, description, defaultTargetPath,
        // Empty optional fields serialize as null, matching the C# app.
        parameters: paramRows.map(({ id: _id, ...p }) => ({
          ...p,
          prompt: p.prompt || null,
          placeholder: p.placeholder || null,
          defaultValue: p.defaultValue || null,
          match: p.match || null,
        })),
      })
      showToast(t('workspace.editor_saved'), 'success')
      onSaved?.()
      onClose()
    } catch (e) {
      showToast(t('workspace.editor_save_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    } finally {
      saving = false
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<div class="dialog-overlay" role="presentation" onclick={(e) => { if (e.target === e.currentTarget) onClose() }}>
  <div class="dialog" role="dialog" aria-label={t('workspace.editor_title')} tabindex="-1" use:focusTrap onkeydown={onKeydown}>
    <header>
      <h3>{t('workspace.editor_title')}</h3>
      <button class="icon-btn" onclick={onClose} title={t('common.close')} aria-label={t('common.close')}><X size={16} /></button>
    </header>

    {#if !loaded}
      <p class="muted">{t('common.loading')}</p>
    {:else}
      <div class="form">
        <label>
          <span>{t('workspace.editor_name')}</span>
          <input type="text" bind:value={name} />
        </label>
        <label>
          <span>{t('workspace.editor_description')}</span>
          <input type="text" bind:value={description} />
        </label>
        <label>
          <span>{t('workspace.editor_target')}</span>
          <input type="text" bind:value={defaultTargetPath} />
        </label>

        <div class="params-head">
          <span class="label">{t('workspace.editor_params')}</span>
          <button class="btn subtle" onclick={addParam}><Plus size={13} /> {t('workspace.editor_add_param')}</button>
        </div>
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="param-list"
          role="list"
          ondrop={handleDrop}
          ondragover={(e) => { if (draggingId !== null) e.preventDefault() }}
        >
          {#each paramRows as p, i (p.id)}
            {#if draggingId !== null && dropBeforeId === p.id}
              <div class="param-drop-indicator"></div>
            {/if}
            <div
              class="param"
              class:param-dragging={draggingId === p.id}
              role="listitem"
              ondragover={(e) => handleDragOver(e, p.id, i)}
            >
              <div class="param-body">
                <div class="param-grid">
                  <label><span>{t('workspace.editor_param_name')}</span><input type="text" bind:value={p.name} /></label>
                  <label><span>{t('workspace.editor_param_prompt')}</span><input type="text" bind:value={p.prompt} /></label>
                  <label><span>{t('workspace.editor_param_placeholder')}</span><input type="text" bind:value={p.placeholder} /></label>
                  <label><span>{t('workspace.editor_param_default')}</span><input type="text" bind:value={p.defaultValue} /></label>
                  <label class="wide"><span>{t('workspace.editor_param_match')}</span><input type="text" bind:value={p.match} placeholder={p.name ? `\\{${p.name}\\}` : ''} /></label>
                </div>
                <div class="param-flags">
                  <label class="check"><input type="checkbox" bind:checked={p.replaceInFileNames} /> {t('workspace.editor_param_in_names')}</label>
                  <label class="check"><input type="checkbox" bind:checked={p.replaceInFiles} /> {t('workspace.editor_param_in_files')}</label>
                  <button class="icon-btn danger" onclick={() => removeParam(p.id)} title={t('workspace.editor_remove_param')} aria-label={t('workspace.editor_remove_param')}><Trash2 size={14} /></button>
                </div>
              </div>
              <!-- svelte-ignore a11y_no_static_element_interactions -->
              <span
                class="param-drag-handle"
                draggable={true}
                ondragstart={(e) => handleDragStart(e, p.id)}
                ondragend={handleDragEnd}
                role="button"
                tabindex="-1"
                aria-label={t('tooltip.drag_template_param')}
                title={t('tooltip.drag_template_param')}
              ><GripVertical size={14} /></span>
            </div>
          {/each}
          {#if draggingId !== null && dropBeforeId === DROP_END}
            <div class="param-drop-indicator"></div>
          {/if}
        </div>

        <footer>
          <button class="btn subtle" onclick={onClose}>{t('common.cancel')}</button>
          <button class="btn primary" disabled={saving} onclick={save}>{saving ? t('common.saving') : t('common.save')}</button>
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
    z-index: 96;
  }
  .dialog {
    width: min(620px, 94vw);
    max-height: 84vh;
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
    justify-content: space-between;
    padding: 0.65rem 0.85rem;
    border-bottom: 1px solid var(--border-muted);
  }
  h3 { margin: 0; font-size: 0.9rem; font-weight: 600; }
  .form {
    padding: 0.9rem;
    display: flex;
    flex-direction: column;
    gap: 0.65rem;
    overflow: auto;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    font-size: 0.74rem;
    color: var(--text-muted);
  }
  input[type="text"] {
    background: var(--bg-base);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-primary);
    padding: 0.35rem 0.5rem;
    font-size: 0.8rem;
  }
  input[type="text"]:focus { outline: none; border-color: var(--accent); }
  .params-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: 0.3rem;
  }
  .label {
    font-size: 0.66rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-faint);
  }
  .param-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .param-drop-indicator {
    height: 2px;
    background: var(--accent);
    border-radius: 1px;
    margin: 1px 0;
  }
  .param {
    border: 1px solid var(--border-muted);
    border-radius: 8px;
    padding: 0.6rem;
    display: flex;
    align-items: flex-start;
    gap: 0.4rem;
  }
  .param-dragging { opacity: 0.4; }
  .param-body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  /* Drag handle: revealed on row hover, mirroring EditableChecklist's
     .cl-drag-handle. */
  .param-drag-handle {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--text-faint);
    cursor: grab;
    flex-shrink: 0;
    margin-top: 0.2rem;
    opacity: 0;
    transition: opacity var(--duration-fast) var(--ease-out);
  }
  .param:hover .param-drag-handle,
  .param-drag-handle:focus-visible {
    opacity: 1;
  }
  .param-drag-handle:active { cursor: grabbing; }
  .param-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.5rem;
  }
  .param-grid .wide { grid-column: 1 / -1; }
  .param-flags {
    display: flex;
    align-items: center;
    gap: 1rem;
  }
  .param-flags .check {
    flex-direction: row;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.76rem;
  }
  .param-flags .icon-btn { margin-left: auto; }
  footer {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
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
  .icon-btn.danger:hover { color: var(--danger, #ef4444); }
  .muted { color: var(--text-faint); font-size: 0.8rem; padding: 0.8rem; }
</style>
