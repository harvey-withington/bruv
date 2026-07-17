<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { fade } from 'svelte/transition'
  import { showToast } from '../lib/toast.svelte'
  import { showConfirm } from '../lib/confirm.svelte'
  import { Plus, Pencil, Trash2, X, Upload, Download, FolderOpen } from 'lucide-svelte'
  import CardTypeEditor from './CardTypeEditor.svelte'
  import TemplateEditor from './TemplateEditor.svelte'
  import {
    CreateUserCardType, UpdateUserCardType, UpdateUserCardTypeIcon, DeleteUserCardType, UpdateBuiltinCardType,
    ListCardTemplates, CreateCardTemplate, UpdateCardTemplate, DeleteCardTemplate,
    ExportCardTypesToFile, ImportCardTypesFromFile, ImportCardTypesFromRepo,
    PickSaveFile, PickFile, PickFolder,
  } from '@shared/api'
  import type { CardTypesImportMode, CardTypesImportResult } from '@shared/types'
  import { cardTypes, loadCardTypes } from '../lib/store.svelte'
  import { draggable } from '../lib/draggable'
  import { focusTrap, portal } from '../lib/actions'
  import type { CardTypeInfo, UserCardType, CardTemplate } from '@shared/types'

  let { onClose }: { onClose: () => void } = $props()

  // Built-in types come from the store; user types are the non-builtin ones
  let builtinTypes = $derived(cardTypes.list.filter(t => t.builtin))
  let userTypes = $derived(cardTypes.list.filter(t => !t.builtin))

  let templates = $state<CardTemplate[]>([])
  let loadingTemplates = $state(false)

  async function loadTemplates() {
    loadingTemplates = true
    try {
      templates = await ListCardTemplates() || []
    } catch {
      templates = []
    }
    loadingTemplates = false
  }

  $effect(() => { loadTemplates() })

  // Editor modal state
  let editingType = $state<CardTypeInfo | undefined>(undefined)
  let showEditor = $state(false)

  // Import/export flow state
  let showImportDialog = $state(false)
  let importMode = $state<CardTypesImportMode>('merge')
  let importSource = $state<'file' | 'repo'>('file')

  async function handleExport() {
    try {
      const path = await PickSaveFile(
        t('card_types.export_title'),
        'bruv-card-types.json',
        t('card_types.file_filter_name'),
        '*.json',
      )
      if (!path) return
      await ExportCardTypesToFile(path)
      showToast(t('card_types.export_success'), 'success')
    } catch (e) {
      showToast(t('card_types.export_failed') + ': ' + String(e), 'error')
    }
  }

  function formatImportResult(r: CardTypesImportResult): string {
    const parts: string[] = []
    if (r.types_added) parts.push(t('card_types.import_result_types_added', { count: r.types_added }))
    if (r.types_overwritten) parts.push(t('card_types.import_result_types_overwritten', { count: r.types_overwritten }))
    if (r.types_skipped) parts.push(t('card_types.import_result_types_skipped', { count: r.types_skipped }))
    if (r.templates_added) parts.push(t('card_types.import_result_templates_added', { count: r.templates_added }))
    if (r.templates_overwritten) parts.push(t('card_types.import_result_templates_overwritten', { count: r.templates_overwritten }))
    if (r.templates_skipped) parts.push(t('card_types.import_result_templates_skipped', { count: r.templates_skipped }))
    return parts.length ? parts.join(', ') : t('card_types.import_result_nothing')
  }

  async function doImport() {
    showImportDialog = false
    try {
      let result: CardTypesImportResult
      if (importSource === 'file') {
        const path = await PickFile(t('card_types.import_file_title'), t('card_types.file_filter_name'), '*.json')
        if (!path) return
        result = await ImportCardTypesFromFile(path, importMode)
      } else {
        const path = await PickFolder(t('card_types.import_repo_title'))
        if (!path) return
        result = await ImportCardTypesFromRepo(path, importMode)
      }
      showToast(formatImportResult(result), 'success')
      await loadTemplates()
      await loadCardTypes()
    } catch (e) {
      showToast(t('card_types.import_failed') + ': ' + String(e), 'error')
    }
  }

  async function confirmImport() {
    if (importMode === 'replace') {
      const ok = await showConfirm(t('card_types.import_replace_confirm_body'))
      if (!ok) return
    }
    doImport()
  }

  function openCreate() {
    editingType = undefined
    showEditor = true
  }

  function openEdit(type: CardTypeInfo) {
    editingType = type
    showEditor = true
  }

  async function handleSave(saved: UserCardType, updatedTemplates: CardTemplate[]) {
    try {
      // Sync any new or updated templates
      const templateIdMap: Record<string, string> = {}
      for (const tmpl of updatedTemplates) {
        if (tmpl.id.startsWith('__new__')) {
          const created = await CreateCardTemplate(tmpl.name, tmpl.blocks)
          templateIdMap[tmpl.id] = created.id
        } else if (templates.some(t => t.id === tmpl.id)) {
          await UpdateCardTemplate(tmpl.id, tmpl.name, tmpl.blocks)
          templateIdMap[tmpl.id] = tmpl.id
        }
      }

      // Resolve the template_id if it was a temp id
      const resolvedTemplateId = saved.template_id
        ? (templateIdMap[saved.template_id] ?? saved.template_id)
        : ''

      let savedTypeId = editingType?.id ?? ''

      if (editingType?.builtin) {
        await UpdateBuiltinCardType(
          editingType.id,
          saved.color,
          resolvedTemplateId,
        )
      } else if (editingType) {
        await UpdateUserCardType(
          editingType.id,
          saved.label,
          saved.color,
          saved.description,
          saved.ai_hint ?? '',
          resolvedTemplateId,
        )
      } else {
        const created = await CreateUserCardType(
          saved.label,
          saved.color,
          saved.description,
          saved.ai_hint ?? '',
          resolvedTemplateId,
        )
        savedTypeId = created.id
      }

      // Icon isn't part of Create/UpdateUserCardType — persist it separately via
      // the dedicated RPC (mirrors CreateCardTypeFromCard's backend flow). Builtin
      // types don't support icon overrides, so skip those.
      if (!editingType?.builtin) {
        const previousIcon = editingType?.icon ?? ''
        const nextIcon = saved.icon ?? ''
        if (nextIcon !== previousIcon) {
          try {
            await UpdateUserCardTypeIcon(savedTypeId, nextIcon)
          } catch {
            showToast(t('error.card_type_icon_save_failed'), 'error')
          }
        }
      }

      await loadCardTypes()
      await loadTemplates()
      showEditor = false
    } catch (e) {
      showToast(t('error.card_type_save_failed'), 'error')
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key !== 'Escape') return
    if (showEditor || showTemplateEditor) return
    onClose()
  }

  function handleOverlayClick(e: MouseEvent) {
    if (showEditor || showTemplateEditor) return
    if (e.target === e.currentTarget) onClose()
  }

  async function handleDelete(type: CardTypeInfo) {
    const ok = await showConfirm(
      t('card_types.confirm_delete', { label: type.label })
    )
    if (!ok) return
    try {
      await DeleteUserCardType(type.id)
      await loadCardTypes()
    } catch {
      showToast(t('error.card_type_delete_failed'), 'error')
    }
  }

  // Standalone template editor — lets a template be edited (or created) directly
  // from its row in the Templates section, independent of a card type's picker.
  let editingTemplate = $state<CardTemplate | undefined>(undefined)
  let showTemplateEditor = $state(false)

  function openEditTemplate(tmpl: CardTemplate) {
    editingTemplate = tmpl
    showTemplateEditor = true
  }

  async function handleTemplateSave(saved: CardTemplate) {
    try {
      if (saved.id) {
        await UpdateCardTemplate(saved.id, saved.name, saved.blocks)
      } else {
        await CreateCardTemplate(saved.name, saved.blocks)
      }
      await loadTemplates()
      showTemplateEditor = false
    } catch (e) {
      showToast(t('error.template_save_failed'), 'error')
    }
  }

  async function handleDeleteTemplate(tmpl: CardTemplate) {
    const ok = await showConfirm(
      t('card_types.confirm_delete_template', { name: tmpl.name })
    )
    if (!ok) return
    try {
      await DeleteCardTemplate(tmpl.id)
      await loadTemplates()
    } catch {
      showToast(t('error.template_delete_failed'), 'error')
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" role="presentation" use:portal onclick={handleOverlayClick} out:fade={{ duration: 150 }}>
  <div class="dialog" role="dialog" use:draggable={{ handle: '.dialog-header' }} use:focusTrap>
    <div class="dialog-header">
      <h2>{t('card_types.title')}</h2>
      <div class="header-actions">
        <button class="icon-btn" onclick={handleExport} title={t('card_types.export_tooltip')} aria-label={t('card_types.export_tooltip')}>
          <Download size={14} />
        </button>
        <button class="icon-btn" onclick={() => { showImportDialog = true }} title={t('card_types.import_tooltip')} aria-label={t('card_types.import_tooltip')}>
          <Upload size={14} />
        </button>
        <button class="close-btn" onclick={onClose} title={t('common.close')}><X size={18} /></button>
      </div>
    </div>

    <div class="dialog-body">
      <section class="section">
        <div class="section-header">
          <h3 class="section-title">{t('card_types.section_user')}</h3>
          <button class="add-btn" onclick={openCreate}>
            <Plus size={13} /> {t('card_types.add')}
          </button>
        </div>

        {#if userTypes.length === 0}
          <p class="empty-hint">{t('card_types.empty')}</p>
        {:else}
          <ul class="type-list">
            {#each userTypes as type (type.id)}
              <li class="type-row">
                <span class="type-swatch" style:background={type.color}></span>
                <div class="type-info">
                  <span class="type-label">{type.label}</span>
                  {#if type.description}
                    <span class="type-desc">{type.description}</span>
                  {/if}
                </div>
                <div class="type-actions">
                  <button class="icon-btn" onclick={() => openEdit(type)} title={t('card_types.edit')}>
                    <Pencil size={13} />
                  </button>
                  <button class="icon-btn danger" onclick={() => handleDelete(type)} title={t('common.delete')}>
                    <Trash2 size={13} />
                  </button>
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section class="section">
        <div class="section-header">
          <h3 class="section-title">{t('card_types.section_builtin')}</h3>
        </div>
        <ul class="type-list readonly">
          {#each builtinTypes as type (type.id)}
            <li class="type-row">
              <span class="type-swatch" style:background={type.color}></span>
              <div class="type-info">
                <span class="type-label">{type.label}</span>
                {#if type.description}
                  <span class="type-desc">{type.description}</span>
                {/if}
              </div>
              <div class="type-actions">
                <button class="icon-btn" onclick={() => openEdit(type)} title={t('card_types.edit')}>
                  <Pencil size={13} />
                </button>
              </div>
              <span class="builtin-badge">{t('card_types.builtin_badge')}</span>
            </li>
          {/each}
        </ul>
      </section>

      <section class="section">
        <div class="section-header">
          <h3 class="section-title">{t('card_types.section_templates')}</h3>
        </div>

        {#if loadingTemplates}
          <p class="empty-hint">{t('common.loading')}</p>
        {:else if templates.length === 0}
          <p class="empty-hint">{t('card_types.templates_empty')}</p>
        {:else}
          <ul class="type-list">
            {#each templates as tmpl (tmpl.id)}
              <li class="type-row">
                <div class="type-info">
                  <span class="type-label">{tmpl.name}</span>
                </div>
                <div class="type-actions">
                  <button class="icon-btn" onclick={() => openEditTemplate(tmpl)} title={t('card_types.edit')}>
                    <Pencil size={13} />
                  </button>
                  <button class="icon-btn danger" onclick={() => handleDeleteTemplate(tmpl)} title={t('common.delete')}>
                    <Trash2 size={13} />
                  </button>
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    </div>
  </div>

  {#if showEditor}
    <CardTypeEditor
      type={editingType}
      {templates}
      allTypes={cardTypes.list}
      onSave={handleSave}
      onClose={() => showEditor = false}
    />
  {/if}

  {#if showTemplateEditor}
    <TemplateEditor
      template={editingTemplate}
      allTypes={cardTypes.list}
      onSave={handleTemplateSave}
      onClose={() => showTemplateEditor = false}
    />
  {/if}

  {#if showImportDialog}
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="overlay" role="presentation" use:portal onclick={(e) => { if (e.target === e.currentTarget) showImportDialog = false }}>
      <div class="dialog import-dialog" role="dialog" tabindex="-1" aria-label={t('card_types.import_title')} use:focusTrap>
        <div class="dialog-header">
          <h2>{t('card_types.import_title')}</h2>
          <button class="close-btn" onclick={() => { showImportDialog = false }} title={t('common.close')}><X size={18} /></button>
        </div>
        <div class="dialog-body">
          <p class="import-hint">{t('card_types.import_hint')}</p>

          <div class="import-section">
            <span class="import-section-label">{t('card_types.import_source_label')}</span>
            <label class="radio-row">
              <input type="radio" bind:group={importSource} value="file" />
              <span><FolderOpen size={12} /> {t('card_types.import_source_file')}</span>
            </label>
            <label class="radio-row">
              <input type="radio" bind:group={importSource} value="repo" />
              <span><FolderOpen size={12} /> {t('card_types.import_source_repo')}</span>
            </label>
          </div>

          <div class="import-section">
            <span class="import-section-label">{t('card_types.import_mode_label')}</span>
            <label class="radio-row">
              <input type="radio" bind:group={importMode} value="merge" />
              <span>
                <strong>{t('card_types.import_mode_merge')}</strong>
                <em>{t('card_types.import_mode_merge_desc')}</em>
              </span>
            </label>
            <label class="radio-row">
              <input type="radio" bind:group={importMode} value="merge_overwrite" />
              <span>
                <strong>{t('card_types.import_mode_merge_overwrite')}</strong>
                <em>{t('card_types.import_mode_merge_overwrite_desc')}</em>
              </span>
            </label>
            <label class="radio-row">
              <input type="radio" bind:group={importMode} value="replace" />
              <span>
                <strong class="danger-text">{t('card_types.import_mode_replace')}</strong>
                <em>{t('card_types.import_mode_replace_desc')}</em>
              </span>
            </label>
          </div>
        </div>
        <div class="dialog-footer">
          <button class="btn btn-ghost" onclick={() => { showImportDialog = false }}>{t('common.cancel')}</button>
          <button class="btn btn-primary" onclick={confirmImport}>{t('card_types.import_confirm')}</button>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 200;
    animation: fade-in var(--duration-normal) var(--ease-out);
  }
  .dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    width: 480px;
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px var(--shadow-lg);
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }
  .dialog-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
  }
  .dialog-header h2 { font-size: 1.1rem; font-weight: 600; margin: 0; }
  .close-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 0.25rem;
    line-height: 1;
    border-radius: 4px;
  }
  .close-btn:hover { color: var(--text-primary); }
  .dialog-body {
    padding: 1.25rem;
    overflow-y: auto;
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 24px;
    min-height: 0;
  }

  .section { display: flex; flex-direction: column; gap: 10px; }
  .section-header { display: flex; align-items: center; justify-content: space-between; }
  .section-title { font-size: 13px; font-weight: 600; margin: 0; color: var(--text); }

  .add-btn {
    display: flex;
    align-items: center;
    gap: 5px;
    background: none;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 5px 10px;
    font-size: 12px;
    color: var(--text-muted);
    cursor: pointer;
  }
  .add-btn:hover { color: var(--text); border-color: var(--text-muted); }

  .empty-hint { font-size: 12px; color: var(--text-muted); margin: 0; padding: 8px 0; }

  .type-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }

  .type-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 10px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }

  .type-swatch { width: 12px; height: 12px; border-radius: 50%; flex-shrink: 0; }

  .type-info { flex: 1; display: flex; flex-direction: column; gap: 2px; min-width: 0; }
  .type-label { font-size: 13px; font-weight: 500; }
  .type-desc { font-size: 11px; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

  .type-actions { display: flex; gap: 4px; }
  .icon-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 3px;
    border-radius: 4px;
    line-height: 1;
  }
  .icon-btn:hover { color: var(--text); background: var(--bg-hover); }
  .icon-btn.danger:hover { color: var(--danger); }

  .header-actions { display: flex; align-items: center; gap: 4px; }

  .import-dialog { width: 440px; }
  .import-dialog .dialog-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 0.75rem 1.25rem;
    border-top: 1px solid var(--border-muted);
  }
  .import-hint {
    font-size: 12px;
    color: var(--text-muted);
    margin: 0 0 14px;
    line-height: 1.5;
  }
  .import-section {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-bottom: 16px;
  }
  .import-section-label {
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted);
    margin-bottom: 2px;
  }
  .radio-row {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 6px 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    cursor: pointer;
  }
  .radio-row:hover { background: var(--bg-hover); }
  .radio-row input { margin-top: 3px; }
  .radio-row span {
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: 12px;
  }
  .radio-row strong { font-weight: 600; color: var(--text-primary); }
  .radio-row em { font-style: normal; color: var(--text-muted); font-size: 11px; }
  .danger-text { color: var(--danger); }
  .btn {
    padding: 6px 14px;
    font-size: 12px;
    border-radius: var(--radius);
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text-primary);
    cursor: pointer;
  }
  .btn:hover { background: var(--bg-hover); }
  .btn-primary {
    background: var(--accent);
    color: white;
    border-color: var(--accent);
  }
  .btn-primary:hover { filter: brightness(1.1); }
  .btn-ghost { background: transparent; }

  .builtin-badge {
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted);
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    padding: 1px 6px;
    border-radius: 4px;
    flex-shrink: 0;
  }
</style>
