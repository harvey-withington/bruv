<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { fade } from 'svelte/transition'
  import { showToast } from '../lib/toast.svelte'
  import { focusTrap, portal, floatingDropdown, inlineEdit } from '../lib/actions'
  import { draggable } from '../lib/draggable'
  import { setContext } from 'svelte'
  import { EditScope, EDIT_SCOPE_KEY } from '@shared/editScope'
  import { X, Pencil, ChevronDown, Smile } from 'lucide-svelte'
  import TemplateEditor from './TemplateEditor.svelte'
  import DynamicIcon from './DynamicIcon.svelte'
  import IconPicker from './IconPicker.svelte'
  import type { CardTypeInfo, UserCardType, CardTemplate } from '@shared/types'

  let { type, templates, allTypes, onSave, onClose }: {
    type?: CardTypeInfo
    templates: CardTemplate[]
    allTypes: CardTypeInfo[]
    onSave: (saved: UserCardType, updatedTemplates: CardTemplate[]) => void
    onClose: () => void
  } = $props()

  // Keyboard entry contract: this dialog's own closable-container scope.
  // Nested TemplateEditor gets its own scope when open (setContext
  // shadows this one for its children), so the existing
  // showTemplateEditor guard below is what keeps the two dialogs'
  // Escape/Ctrl+Enter presses from fighting each other.
  const editScope = new EditScope()
  editScope.requestClose = () => { void save() }
  setContext(EDIT_SCOPE_KEY, editScope)

  const TYPE_PALETTE = [
    '#6366f1', '#22c55e', '#f59e0b', '#ec4899', '#06b6d4',
    '#ef4444', '#8b5cf6', '#f97316', '#10b981', '#3b82f6',
    '#e11d48', '#84cc16',
  ]

  // Form state seeded once from the incoming `type` prop; saved via onSave.
  // The editor is remounted per-open, so one-time capture is intentional.
  /* svelte-ignore state_referenced_locally */
  let label = $state(type?.label ?? '')
  /* svelte-ignore state_referenced_locally */
  let color = $state(type?.color ?? TYPE_PALETTE[0])
  /* svelte-ignore state_referenced_locally */
  let icon = $state(type?.icon ?? '')
  /* svelte-ignore state_referenced_locally */
  let description = $state(type?.description ?? '')
  /* svelte-ignore state_referenced_locally */
  let aiHint = $state(type?.ai_hint ?? '')
  /* svelte-ignore state_referenced_locally */
  let selectedTemplateId = $state(type?.template_id ?? '')
  let saving = $state(false)
  let showIconPicker = $state(false)
  /* svelte-ignore state_referenced_locally */
  const isBuiltin = type?.builtin ?? false

  // Plain always-visible fields (no edit-in-place toggle): Enter commits
  // by blurring — the type itself is saved via the dialog's Save button
  // — and Escape reverts to the value loaded when the dialog opened.
  let labelInputEl = $state<HTMLInputElement | null>(null)
  let descriptionInputEl = $state<HTMLTextAreaElement | null>(null)
  let aiHintInputEl = $state<HTMLTextAreaElement | null>(null)

  function cancelLabelEdit() { label = type?.label ?? ''; labelInputEl?.blur() }
  function cancelDescriptionEdit() { description = type?.description ?? ''; descriptionInputEl?.blur() }
  function cancelAiHintEdit() { aiHint = type?.ai_hint ?? ''; aiHintInputEl?.blur() }

  let slugPreview = $derived(
    label.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
  )

  // Template editor sub-modal
  let showTemplateEditor = $state(false)
  let editingTemplate = $state<CardTemplate | undefined>(undefined)
  /* svelte-ignore state_referenced_locally */
  let localTemplates = $state<CardTemplate[]>([...templates])

  function openNewTemplate() {
    showTemplatePicker = false
    editingTemplate = undefined
    showTemplateEditor = true
  }

  function openEditTemplate() {
    showTemplatePicker = false
    editingTemplate = localTemplates.find(tmpl => tmpl.id === selectedTemplateId)
    showTemplateEditor = true
  }

  function handleTemplateSave(saved: CardTemplate) {
    if (!saved.id) {
      const tempId = `__new__${Date.now()}`
      const withId = { ...saved, id: tempId }
      localTemplates = [...localTemplates, withId]
      selectedTemplateId = tempId
    } else {
      localTemplates = localTemplates.map(tmpl => tmpl.id === saved.id ? saved : tmpl)
    }
    showTemplateEditor = false
  }

  async function save() {
    if (!label.trim()) { showToast(t('card_type_editor.label_required'), 'error'); return }
    saving = true
    try {
      onSave(
        {
          id: type?.id ?? '',
          label: label.trim(),
          color,
          icon: icon || undefined,
          description: description.trim(),
          ai_hint: aiHint.trim() || undefined,
          template_id: selectedTemplateId || undefined,
        },
        localTemplates,
      )
    } finally {
      saving = false
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (showTemplateEditor) return
      if (showIconPicker) return
      if (showTemplatePicker) { showTemplatePicker = false; return }
      if (editScope.hasActive()) return
      onClose()
    } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      if (showTemplateEditor) return
      e.preventDefault()
      editScope.commitAll()
      void save()
    }
  }

  function handleOverlayClick(e: MouseEvent) {
    if (showTemplateEditor) return
    if (e.target === e.currentTarget) onClose()
  }

  let selectedTemplate = $derived(localTemplates.find(tmpl => tmpl.id === selectedTemplateId))

  // Custom template picker dropdown
  let templatePickerBtnEl = $state<HTMLButtonElement | null>(null)
  let showTemplatePicker = $state(false)

  function toggleTemplatePicker() {
    showTemplatePicker = !showTemplatePicker
  }

  function selectTemplate(id: string, e: MouseEvent) {
    e.stopPropagation()
    selectedTemplateId = id
    showTemplatePicker = false
  }

  function handleDialogClick(e: MouseEvent) {
    // Close dropdown when clicking anywhere in the dialog that isn't the trigger or dropdown
    if (showTemplatePicker) {
      const target = e.target as HTMLElement
      if (!target.closest('.template-picker-wrap') && !target.closest('.template-dropdown')) {
        showTemplatePicker = false
      }
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" role="presentation" use:portal onclick={handleOverlayClick} out:fade={{ duration: 150 }}>
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="dialog" role="dialog" tabindex="-1" aria-label={type ? t('card_type_editor.title_edit') : t('card_type_editor.title_create')} use:draggable={{ handle: '.dialog-header' }} use:focusTrap onclick={handleDialogClick}>
    <div class="dialog-header">
      <h2>{type ? t('card_type_editor.title_edit') : t('card_type_editor.title_create')}</h2>
      <button class="close-btn" onclick={onClose} title={t('common.close')}><X size={18} /></button>
    </div>

    <div class="dialog-body">
      {#if !isBuiltin}
        <div class="field-row">
          <span class="field-label">{t('card_type_editor.label')}</span>
          <input
            class="field-input"
            bind:this={labelInputEl}
            bind:value={label}
            placeholder={t('card_type_editor.label_placeholder')}
            use:inlineEdit={{ onCommit: () => labelInputEl?.blur(), onCancel: cancelLabelEdit }}
          />
          {#if !type && slugPreview}
            <span class="slug-preview">{t('card_type_editor.id_preview')}: <code>{slugPreview}</code></span>
          {/if}
        </div>
      {/if}

      <div class="field-row">
        <span class="field-label">{t('card_type_editor.color')}</span>
        <div class="color-palette">
          {#each TYPE_PALETTE as c}
            <button
              class="color-swatch"
              class:active={color === c}
              style:background={c}
              onclick={() => color = c}
              aria-label={c}
            ></button>
          {/each}
        </div>
        <div class="color-preview">
          <span class="type-badge-preview" style:background={color}>
            {#if icon}<DynamicIcon name={icon} size={12} className="badge-icon" />{/if}
            {label || t('card_type_editor.label_placeholder')}
          </span>
        </div>
      </div>

      <div class="field-row">
        <span class="field-label">{t('icon.pick')}</span>
        <div class="icon-picker-row">
          <button class="icon-picker-btn" onclick={() => showIconPicker = true}>
            {#if icon}
              <DynamicIcon name={icon} size={16} />
              <span class="icon-name">{icon}</span>
            {:else}
              <Smile size={16} />
              <span class="icon-name muted">{t('icon.none')}</span>
            {/if}
          </button>
        </div>
      </div>

      {#if !isBuiltin}
        <div class="field-row">
          <span class="field-label">{t('card_type_editor.description')}</span>
          <textarea
            class="field-input textarea"
            bind:this={descriptionInputEl}
            bind:value={description}
            placeholder={t('card_type_editor.description_placeholder')}
            rows={2}
            use:inlineEdit={{ multiline: true, onCommit: () => descriptionInputEl?.blur(), onCancel: cancelDescriptionEdit }}
          ></textarea>
        </div>

        <div class="field-row">
          <span class="field-label">{t('card_type_editor.ai_hint')}</span>
          <textarea
            class="field-input textarea"
            bind:this={aiHintInputEl}
            bind:value={aiHint}
            placeholder={t('card_type_editor.ai_hint_placeholder')}
            rows={2}
            use:inlineEdit={{ multiline: true, onCommit: () => aiHintInputEl?.blur(), onCancel: cancelAiHintEdit }}
          ></textarea>
        </div>
      {/if}

      <div class="field-row">
        <span class="field-label">{t('card_type_editor.template')}</span>
        <div class="template-row">
          <div class="template-picker-wrap">
            <button class="template-picker-btn" bind:this={templatePickerBtnEl} onclick={toggleTemplatePicker}>
              <span class="template-picker-label">
                {selectedTemplate?.name || t('card_type_editor.template_none')}
              </span>
              <ChevronDown size={14} />
            </button>
          </div>
          {#if selectedTemplateId && selectedTemplate}
            <button class="template-action-btn" onclick={openEditTemplate} title={t('card_type_editor.template_edit')}>
              <Pencil size={14} />
            </button>
          {/if}
        </div>
        <button class="link-btn" onclick={openNewTemplate}>{t('card_type_editor.template_new')}</button>
      </div>
    </div>

    <div class="dialog-footer">
      <button class="btn-secondary" onclick={onClose}>{t('common.cancel')}</button>
      <button class="btn-primary" onclick={save} disabled={saving}>
        {saving ? t('common.saving') : t('common.save')}
      </button>
    </div>
  </div>

  {#if showTemplatePicker && templatePickerBtnEl}
    <div class="template-dropdown" use:floatingDropdown={{ trigger: templatePickerBtnEl, matchWidth: true }}>
      <button
        class="template-dropdown-option"
        class:active={!selectedTemplateId}
        onclick={(e) => selectTemplate('', e)}
      >{t('card_type_editor.template_none')}</button>
      {#each localTemplates as tmpl}
        <button
          class="template-dropdown-option"
          class:active={selectedTemplateId === tmpl.id}
          onclick={(e) => selectTemplate(tmpl.id, e)}
        >{tmpl.name}</button>
      {/each}
    </div>
  {/if}

  {#if showTemplateEditor}
    <TemplateEditor
      template={editingTemplate}
      {allTypes}
      onSave={handleTemplateSave}
      onClose={() => showTemplateEditor = false}
    />
  {/if}

  {#if showIconPicker}
    <IconPicker
      value={icon}
      onSelect={(i) => { icon = i; showIconPicker = false }}
      onClose={() => showIconPicker = false}
    />
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
    width: 440px;
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    overflow: visible;
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
  .close-btn { background: none; border: none; cursor: pointer; color: var(--text-muted); padding: 0.25rem; line-height: 1; border-radius: 4px; }
  .close-btn:hover { color: var(--text-primary); }
  .dialog-body { padding: 1.25rem; overflow-y: auto; flex: 1; display: flex; flex-direction: column; gap: 0.85rem; min-height: 0; }
  .dialog-footer { padding: 0.75rem 1.25rem; border-top: 1px solid var(--border-muted); display: flex; justify-content: flex-end; gap: 0.5rem; }

  .field-row { display: flex; flex-direction: column; gap: 0.35rem; }
  .field-label { font-size: 0.85rem; font-weight: 500; color: var(--text-muted); }
  .field-input {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.45rem 0.6rem;
    color: var(--text-primary);
    font-size: 0.85rem;
    width: 100%;
    box-sizing: border-box;
    outline: none;
    font-family: inherit;
  }
  .field-input:focus { border-color: var(--accent); }
  .field-input.textarea { resize: vertical; min-height: 56px; }

  .slug-preview { font-size: 0.7rem; color: var(--text-muted); }
  .slug-preview code { color: var(--text-primary); background: var(--bg); padding: 1px 4px; border-radius: 3px; }

  .color-palette { display: flex; flex-wrap: wrap; gap: 6px; }
  .color-swatch {
    width: 22px; height: 22px;
    border-radius: 50%;
    border: 2px solid transparent;
    cursor: pointer;
    transition: transform 0.1s;
  }
  .color-swatch:hover { transform: scale(1.15); }
  .color-swatch.active { border-color: var(--text-primary); }

  .color-preview { display: flex; align-items: center; gap: 8px; margin-top: 4px; }
  .type-badge-preview {
    font-size: 0.7rem;
    font-weight: 600;
    color: #fff;
    padding: 2px 10px;
    border-radius: 999px;
    display: inline-block;
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .icon-picker-row { display: flex; align-items: center; }
  .icon-picker-btn {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.6rem;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-primary);
    font-size: 0.85rem;
    cursor: pointer;
  }
  .icon-picker-btn:hover { border-color: var(--accent); }
  .icon-name { font-size: 0.8rem; }
  .icon-name.muted { color: var(--text-muted); }
  :global(.badge-icon) { margin-right: 2px; }

  .template-row { display: flex; gap: 6px; align-items: center; }

  .template-picker-wrap {
    flex: 1;
    min-width: 0;
  }

  .template-picker-btn {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.45rem 0.6rem;
    color: var(--text-primary);
    font-size: 0.85rem;
    cursor: pointer;
    text-align: left;
    gap: 0.5rem;
  }
  .template-picker-btn:hover { border-color: var(--accent); }
  .template-picker-label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .template-dropdown {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 4px 16px var(--shadow-lg);
    padding: 0.25rem;
  }

  .template-dropdown-option {
    display: block;
    width: 100%;
    padding: 0.4rem 0.6rem;
    background: none;
    border: none;
    border-radius: 5px;
    cursor: pointer;
    color: var(--text-primary);
    font-size: 0.85rem;
    text-align: left;
  }
  .template-dropdown-option:hover,
  .template-dropdown-option.active {
    background: var(--bg-elevated);
  }

  .template-action-btn {
    background: none;
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 8px;
    cursor: pointer;
    color: var(--text-muted);
    line-height: 1;
    flex-shrink: 0;
  }
  .template-action-btn:hover { color: var(--text-primary); background: var(--bg-hover); }

  .link-btn {
    background: none;
    border: none;
    padding: 0;
    font-size: 0.8rem;
    color: var(--accent);
    cursor: pointer;
    text-align: left;
  }
  .link-btn:hover { text-decoration: underline; }

  .btn-primary {
    background: var(--accent);
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 0.5rem 1rem;
    font-size: 0.85rem;
    font-weight: 500;
    cursor: pointer;
  }
  .btn-primary:hover { opacity: 0.9; }
  .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn-secondary {
    background: none;
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.5rem 1rem;
    font-size: 0.85rem;
    color: var(--text-primary);
    cursor: pointer;
  }
  .btn-secondary:hover { background: var(--bg-hover); }
</style>
