<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { fade } from 'svelte/transition'
  import { showToast } from '../lib/toast.svelte'
  import { showConfirm } from '../lib/confirm.svelte'
  import { focusTrap, portal, inlineEdit } from '../lib/actions'
  import { draggable } from '../lib/draggable'
  import { setContext } from 'svelte'
  import { EditScope, EDIT_SCOPE_KEY } from '@shared/editScope'
  import { GripVertical, Plus, Trash2, Type, ListChecks, List, Film, Link, Minus, X, ChevronDown, Hash, Calendar, Star, ToggleLeft, CircleDot, ImageIcon, ChartColumn, Bell } from 'lucide-svelte'
  import EditableChecklist from './EditableChecklist.svelte'
  import EditableList from './EditableList.svelte'
  import type { CardTemplate, Block, BlockMeta, ChecklistItem, ListItem } from '@shared/types'
  import type { CardTypeInfo } from '@shared/types'

  let { template, allTypes, onSave, onClose }: {
    template?: CardTemplate
    allTypes: CardTypeInfo[]
    onSave: (t: CardTemplate) => void
    onClose: () => void
  } = $props()

  // Keyboard entry contract: this dialog's own closable-container scope.
  // Its affirmative "commit + close" action is Save (there's no
  // per-field auto-save — everything is staged locally until Save is
  // clicked), so Ctrl+Enter from any registered field commits then
  // saves-and-closes via requestClose. Mirrors OptionsEditorDialog,
  // which this dialog can itself open on top of (the block-options
  // editor) — each gets its own scope so the two never fight over the
  // same Escape/Ctrl+Enter press.
  const editScope = new EditScope()
  editScope.requestClose = () => { void save() }
  setContext(EDIT_SCOPE_KEY, editScope)

  // Form state seeded once from the incoming `template` prop; saved via onSave.
  /* svelte-ignore state_referenced_locally */
  let name = $state(template?.name ?? '')
  let nameInputEl = $state<HTMLInputElement | null>(null)
  /* svelte-ignore state_referenced_locally */
  let blocks = $state<Block[]>(template?.blocks ? JSON.parse(JSON.stringify(template.blocks)) : [])
  let saving = $state(false)

  const BLOCK_OPTIONS = [
    { type: 'text',      label: t('block.text'),      icon: 'Type' },
    { type: 'checklist', label: t('block.checklist'), icon: 'ListChecks' },
    { type: 'list',      label: t('block.list'),      icon: 'List' },
    { type: 'media',     label: t('block.media'),     icon: 'Film' },
    { type: 'url',       label: t('block.url'),       icon: 'Link' },
    { type: 'divider',   label: t('block.divider'),   icon: 'Minus' },
    { type: 'select',    label: t('block.select'),    icon: 'ChevronDown' },
    { type: 'number',    label: t('block.number'),    icon: 'Hash' },
    { type: 'date',      label: t('block.date'),      icon: 'Calendar' },
    { type: 'rating',    label: t('block.rating'),    icon: 'Star' },
    { type: 'checkbox',       label: t('block.checkbox'),       icon: 'ToggleLeft' },
    { type: 'radio',          label: t('block.radio'),          icon: 'CircleDot' },
    { type: 'checkbox_group', label: t('block.checkbox_group'), icon: 'ListChecks' },
    { type: 'image',          label: t('block.image'),          icon: 'ImageIcon' },
    { type: 'progress',       label: t('block.progress'),       icon: 'ChartColumn' },
    { type: 'alarm',          label: t('block.alarm'),          icon: 'Bell' },
  ] as const

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const BLOCK_ICON_MAP: Record<string, any> = {
    Type, ListChecks, List, Film, Link, Minus, ChevronDown, Hash, Calendar, Star, ToggleLeft, CircleDot, ImageIcon, ChartColumn, Bell,
  }

  function labelToKey(label: string): string {
    return label.trim().toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '')
  }

  // Pre-populate: track which blocks have their value editor open
  const PREPOPULABLE_TYPES = ['text', 'checklist', 'list', 'select', 'number', 'rating', 'radio', 'checkbox_group'] as const
  function canPrepopulate(type: string): boolean {
    return (PREPOPULABLE_TYPES as readonly string[]).includes(type)
  }

  /* svelte-ignore state_referenced_locally */
  let prePopulate = $state<Record<string, boolean>>(
    Object.fromEntries(
      (template?.blocks ?? [])
        .filter(b => canPrepopulate(b.type))
        .filter(b => {
          if (b.type === 'text') return typeof b.value === 'string' && b.value.length > 0
          if (b.type === 'checklist') return Array.isArray(b.value) && b.value.length > 0
          if (b.type === 'list') return Array.isArray(b.value) && b.value.length > 0
          if (b.type === 'select') return (b.meta?.options?.length ?? 0) > 0 || b.meta?.multi
          if (b.type === 'number') return b.meta?.min != null || b.meta?.max != null || b.meta?.suffix
          if (b.type === 'rating') return b.meta?.max != null && b.meta.max !== 5
          if (b.type === 'radio') return (b.meta?.options?.length ?? 0) > 0
          if (b.type === 'checkbox_group') return (b.meta?.options?.length ?? 0) > 0
          return false
        })
        .map(b => [b.id, true])
    )
  )

  function togglePrePopulate(blockId: string, blockType: string) {
    const wasOn = !!prePopulate[blockId]
    prePopulate = { ...prePopulate, [blockId]: !wasOn }
    if (wasOn) {
      blocks = blocks.map(b => {
        if (b.id !== blockId) return b
        if (blockType === 'text') return { ...b, value: '' }
        if (blockType === 'checklist') return { ...b, value: [] }
        if (blockType === 'list') return { ...b, value: [] }
        if (blockType === 'select') return { ...b, meta: { ...b.meta, options: [], multi: undefined } }
        if (blockType === 'number') return { ...b, meta: { ...b.meta, min: undefined, max: undefined, suffix: undefined } }
        if (blockType === 'rating') return { ...b, meta: { ...b.meta, max: 5 } }
        if (blockType === 'radio') return { ...b, meta: { ...b.meta, options: [] } }
        if (blockType === 'checkbox_group') return { ...b, meta: { ...b.meta, options: [] } }
        return b
      })
    }
  }

  function updateBlockValue(blockId: string, value: Block['value']) {
    blocks = blocks.map(b => b.id === blockId ? { ...b, value } : b)
  }

  function updateBlockMeta(blockId: string, meta: BlockMeta) {
    blocks = blocks.map(b => b.id === blockId ? { ...b, meta: { ...b.meta, ...meta } } : b)
  }

  // Select options management for template editor
  let newSelectOptions = $state<Record<string, string>>({})

  function addSelectOption(blockId: string) {
    const text = (newSelectOptions[blockId] || '').trim()
    if (!text) return
    const block = blocks.find(b => b.id === blockId)
    if (!block) return
    const opts = [...(block.meta?.options || []), text]
    updateBlockMeta(blockId, { options: opts })
    newSelectOptions = { ...newSelectOptions, [blockId]: '' }
  }

  // Serial add-input per the keyboard entry contract: Enter commits and
  // re-arms, Escape discards the draft and ends the entry, blur keeps
  // the draft uncommitted. Keyed by block id since each pre-populate
  // editor has its own add-option row.
  let selectOptionInputEls = $state<Record<string, HTMLInputElement | null>>({})

  function cancelSelectOption(blockId: string) {
    newSelectOptions = { ...newSelectOptions, [blockId]: '' }
    selectOptionInputEls[blockId]?.blur()
  }

  function removeSelectOption(blockId: string, idx: number) {
    const block = blocks.find(b => b.id === blockId)
    if (!block) return
    const opts = (block.meta?.options || []).filter((_: string, i: number) => i !== idx)
    updateBlockMeta(blockId, { options: opts })
  }

  let showBlockPicker = $state(false)
  let addBlockBtnEl = $state<HTMLButtonElement | null>(null)
  let blockPickerEl = $state<HTMLDivElement | null>(null)
  let blockPickerPos = $state({ top: 0, left: 0 })

  function toggleBlockPicker() {
    if (!showBlockPicker && addBlockBtnEl) {
      const r = addBlockBtnEl.getBoundingClientRect()
      blockPickerPos = { top: r.top - 4, left: r.left }
    }
    showBlockPicker = !showBlockPicker
  }

  function handleWindowClick(e: MouseEvent) {
    const target = e.target as Node
    if (showBlockPicker) {
      if (!addBlockBtnEl?.contains(target) && !blockPickerEl?.contains(target)) showBlockPicker = false
    }
  }

  function defaultSelectOptions(): string[] {
    return [1, 2, 3].map(n => t('block.default_option', { n }))
  }

  function addBlock(blockType: string) {
    showBlockPicker = false
    const option = BLOCK_OPTIONS.find(o => o.type === blockType)
    const label = option?.label ?? blockType
    const id = `blk-${crypto.randomUUID().slice(0, 8)}`
    let value: Block['value'] = ''
    let meta: BlockMeta | undefined = undefined
    if (blockType === 'checklist') value = []
    else if (blockType === 'list') value = []
    else if (blockType === 'media') value = []
    else if (blockType === 'divider') value = null
    else if (blockType === 'select') { value = ''; meta = { options: defaultSelectOptions() } }
    else if (blockType === 'number') value = 0
    else if (blockType === 'date') value = ''
    else if (blockType === 'rating') { value = 0; meta = { max: 5 } }
    else if (blockType === 'checkbox') value = false
    else if (blockType === 'radio') { value = ''; meta = { options: defaultSelectOptions() } }
    else if (blockType === 'checkbox_group') { value = []; meta = { options: defaultSelectOptions() } }
    else if (blockType === 'image') value = null
    else if (blockType === 'progress') value = 0
    else if (blockType === 'alarm') { value = null; meta = { alarm_channels: 'in-app,system' } }
    blocks = [...blocks, { id, type: blockType as Block['type'], label, key: labelToKey(label), value, meta }]
  }

  // Drag-and-drop block reordering
  let draggingIdx = $state<number | null>(null)
  let dropIdx = $state<number | null>(null)

  function handleBlockDragStart(e: DragEvent, idx: number) {
    draggingIdx = idx
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move'
      e.dataTransfer.setData('text/plain', String(idx))
    }
  }

  function handleBlockDragEnd() { draggingIdx = null; dropIdx = null }

  function handleBlockDragOver(e: DragEvent, idx: number) {
    if (draggingIdx === null) return
    e.preventDefault()
    e.stopPropagation()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    dropIdx = e.clientY < rect.top + rect.height / 2 ? idx : idx + 1
  }

  function handleBlockDrop(e: DragEvent) {
    e.preventDefault()
    if (draggingIdx === null || dropIdx === null) { handleBlockDragEnd(); return }
    const from = draggingIdx
    let to = dropIdx
    const list = [...blocks]
    handleBlockDragEnd()
    const [moved] = list.splice(from, 1)
    if (to > from) to--
    list.splice(to, 0, moved)
    blocks = list
  }

  // Block label inline editing
  let editingLabelId = $state<string | null>(null)
  let labelDraft = $state('')

  function startEditLabel(block: Block) {
    editingLabelId = block.id
    labelDraft = block.label
  }

  function commitLabel(blockId: string) {
    if (!labelDraft.trim()) { editingLabelId = null; return }
    blocks = blocks.map(b => b.id === blockId
      ? { ...b, label: labelDraft.trim(), key: labelToKey(labelDraft) }
      : b
    )
    editingLabelId = null
  }

  function cancelLabel() {
    editingLabelId = null
  }

  // Template name field: Enter commits by blurring (the template itself
  // is saved via the dialog's Save button, not per-field), Escape reverts
  // to the last-loaded name and blurs.
  function commitName() {
    nameInputEl?.blur()
  }

  function cancelName() {
    name = template?.name ?? ''
    nameInputEl?.blur()
  }

  async function deleteBlock(blockId: string) {
    const block = blocks.find(b => b.id === blockId)
    const ok = await showConfirm(
      t('template_editor.confirm_delete_block', { name: block?.label ?? '' })
    )
    if (!ok) return
    blocks = blocks.filter(b => b.id !== blockId)
  }

  async function save() {
    if (!name.trim()) { showToast(t('template_editor.name_required'), 'error'); return }
    saving = true
    try {
      onSave({
        id: template?.id ?? '',
        name: name.trim(),
        blocks,
      })
    } finally {
      saving = false
    }
  }

  // Container side of the keyboard entry contract. Escape closes
  // (cancels, discarding local edits) only when nothing is being
  // edited — the block-label and add-option fields consume Escape
  // themselves via the inlineEdit action; this is the backstop for
  // presses that land while no field is focused. Ctrl+Enter commits any
  // in-flight edit then saves-and-closes via editScope.requestClose.
  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (editScope.hasActive()) return
      e.stopPropagation()
      onClose()
    } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      editScope.commitAll()
      void save()
    }
  }

  function handleOverlayClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }

  // Used-by labels for this template
  let usedBy = $derived(
    allTypes.filter(ct => ct.template_id === template?.id).map(ct => ct.label)
  )
</script>

<svelte:window onkeydown={handleKeydown} onclick={handleWindowClick} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" role="presentation" use:portal onclick={handleOverlayClick} out:fade={{ duration: 150 }}>
  <div class="dialog" use:draggable={{ handle: '.dialog-header' }} use:focusTrap>
    <div class="dialog-header">
      <div class="header-left">
        <span class="template-badge">{t('template_editor.banner')}</span>
        <h2>{template ? t('template_editor.title_edit') : t('template_editor.title_create')}</h2>
      </div>
      <button class="close-btn" onclick={onClose} title={t('common.close')}><X size={18} /></button>
    </div>

    <div class="dialog-body">
      <div class="field-row">
        <span class="field-label">{t('template_editor.name')}</span>
        <input
          class="field-input"
          bind:this={nameInputEl}
          bind:value={name}
          placeholder={t('template_editor.name_placeholder')}
          use:inlineEdit={{ onCommit: commitName, onCancel: cancelName }}
        />
      </div>

      {#if usedBy.length > 0}
        <div class="used-by-row">
          <span class="used-by-label">{t('template_editor.used_by')}</span>
          {#each usedBy as label}
            <span class="used-by-chip">{label}</span>
          {/each}
        </div>
      {/if}

      <div class="blocks-section">
        {#if blocks.length === 0}
          <p class="empty-hint">{t('template_editor.no_blocks')}</p>
        {:else}
          <ul
            class="blocks-list"
            role="list"
            ondragover={(e) => { if (blocks.length === 0) { e.preventDefault() } }}
            ondrop={handleBlockDrop}
          >
            {#each blocks as block, idx (block.id)}
              <li
                class="block-row"
                class:dragging={draggingIdx === idx}
                class:drop-above={dropIdx === idx && draggingIdx !== null && draggingIdx !== idx}
                class:drop-below={dropIdx === idx + 1 && draggingIdx !== null && draggingIdx !== idx}
                ondragover={(e) => handleBlockDragOver(e, idx)}
                ondrop={handleBlockDrop}
              >
                <div class="block-header">
                  <span
                    class="drag-handle"
                    role="button"
                    tabindex={-1}
                    draggable
                    ondragstart={(e) => handleBlockDragStart(e, idx)}
                    ondragend={handleBlockDragEnd}
                    aria-label={t('tooltip.drag_block')}
                  ><GripVertical size={14} /></span>

                  {#if editingLabelId === block.id}
                    <!-- svelte-ignore a11y_autofocus -->
                    <input
                      class="label-input"
                      bind:value={labelDraft}
                      autofocus
                      use:inlineEdit={{ onCommit: () => commitLabel(block.id), onCancel: cancelLabel, scope: editScope }}
                    />
                  {:else}
                    <button class="block-label-btn" onclick={() => startEditLabel(block)}>
                      {block.label}
                    </button>
                  {/if}
                  <span class="block-type-hint">{block.type}</span>
                  {#if canPrepopulate(block.type)}
                    <label class="prepopulate-toggle" title={t('template_editor.prepopulate')}>
                      <input
                        type="checkbox"
                        checked={!!prePopulate[block.id]}
                        onchange={() => togglePrePopulate(block.id, block.type)}
                      />
                      <span class="prepopulate-label">{t('template_editor.prepopulate')}</span>
                    </label>
                  {/if}
                  <button class="delete-btn" onclick={() => deleteBlock(block.id)} aria-label={t('tooltip.delete_block')}>
                    <Trash2 size={13} />
                  </button>
                </div>

                {#if prePopulate[block.id]}
                  <div class="prepopulate-editor">
                    {#if block.type === 'text'}
                      <textarea
                        class="prepopulate-textarea"
                        value={typeof block.value === 'string' ? block.value : ''}
                        oninput={(e) => updateBlockValue(block.id, (e.target as HTMLTextAreaElement).value)}
                        placeholder={t('template_editor.prepopulate_text_placeholder')}
                        rows={3}
                      ></textarea>
                    {:else if block.type === 'checklist'}
                      <EditableChecklist
                        items={Array.isArray(block.value) ? block.value as ChecklistItem[] : []}
                        placeholder={t('template_editor.prepopulate_checklist_placeholder')}
                        onUpdate={(updated) => updateBlockValue(block.id, updated)}
                      />
                    {:else if block.type === 'list'}
                      <EditableList
                        items={Array.isArray(block.value) ? block.value as ListItem[] : []}
                        placeholder={t('template_editor.prepopulate_list_placeholder')}
                        onUpdate={(updated) => updateBlockValue(block.id, updated)}
                      />
                    {:else if block.type === 'select'}
                      <div class="config-fields">
                        <label class="config-toggle">
                          <input
                            type="checkbox"
                            checked={!!block.meta?.multi}
                            onchange={() => updateBlockMeta(block.id, { multi: !block.meta?.multi })}
                          />
                          <span>{t('block.multi_select')}</span>
                        </label>
                        <div class="config-options-list">
                          {#each (block.meta?.options || []) as opt, i}
                            <div class="config-option-row">
                              <span class="config-option-text">{opt}</span>
                              <button class="config-option-remove" onclick={() => removeSelectOption(block.id, i)} title={t('block.remove_option')}>
                                <Trash2 size={12} />
                              </button>
                            </div>
                          {/each}
                          <div class="config-option-add">
                            <input
                              type="text"
                              class="config-option-input"
                              placeholder={t('block.option_placeholder')}
                              value={newSelectOptions[block.id] || ''}
                              oninput={(e) => { newSelectOptions = { ...newSelectOptions, [block.id]: (e.target as HTMLInputElement).value } }}
                              bind:this={selectOptionInputEls[block.id]}
                              use:inlineEdit={{ serial: true, onCommit: () => addSelectOption(block.id), onCancel: () => cancelSelectOption(block.id), scope: editScope }}
                            />
                            <button class="config-option-add-btn" onclick={() => addSelectOption(block.id)}>
                              <Plus size={12} />
                            </button>
                          </div>
                        </div>
                      </div>
                    {:else if block.type === 'radio' || block.type === 'checkbox_group'}
                      <div class="config-fields">
                        <div class="config-options-list">
                          {#each (block.meta?.options || []) as opt, i}
                            <div class="config-option-row">
                              <span class="config-option-text">{opt}</span>
                              <button class="config-option-remove" onclick={() => removeSelectOption(block.id, i)} title={t('block.remove_option')}>
                                <Trash2 size={12} />
                              </button>
                            </div>
                          {/each}
                          <div class="config-option-add">
                            <input
                              type="text"
                              class="config-option-input"
                              placeholder={t('block.option_placeholder')}
                              value={newSelectOptions[block.id] || ''}
                              oninput={(e) => { newSelectOptions = { ...newSelectOptions, [block.id]: (e.target as HTMLInputElement).value } }}
                              bind:this={selectOptionInputEls[block.id]}
                              use:inlineEdit={{ serial: true, onCommit: () => addSelectOption(block.id), onCancel: () => cancelSelectOption(block.id), scope: editScope }}
                            />
                            <button class="config-option-add-btn" onclick={() => addSelectOption(block.id)}>
                              <Plus size={12} />
                            </button>
                          </div>
                        </div>
                      </div>
                    {:else if block.type === 'number'}
                      <div class="config-fields">
                        <div class="config-row">
                          <span class="config-label">{t('block.min_value')}</span>
                          <input
                            type="number"
                            class="config-input-sm"
                            value={block.meta?.min ?? ''}
                            oninput={(e) => { const v = (e.target as HTMLInputElement).valueAsNumber; updateBlockMeta(block.id, { min: isNaN(v) ? undefined : v }) }}
                          />
                        </div>
                        <div class="config-row">
                          <span class="config-label">{t('block.max_value')}</span>
                          <input
                            type="number"
                            class="config-input-sm"
                            value={block.meta?.max ?? ''}
                            oninput={(e) => { const v = (e.target as HTMLInputElement).valueAsNumber; updateBlockMeta(block.id, { max: isNaN(v) ? undefined : v }) }}
                          />
                        </div>
                        <div class="config-row">
                          <span class="config-label">{t('block.suffix')}</span>
                          <input
                            type="text"
                            class="config-input-sm"
                            value={block.meta?.suffix ?? ''}
                            oninput={(e) => updateBlockMeta(block.id, { suffix: (e.target as HTMLInputElement).value || undefined })}
                            placeholder="%, kg, etc."
                          />
                        </div>
                      </div>
                    {:else if block.type === 'rating'}
                      <div class="config-fields">
                        <div class="config-row">
                          <span class="config-label">{t('block.max_stars')}</span>
                          <input
                            type="number"
                            class="config-input-sm"
                            value={block.meta?.max ?? 5}
                            min={1}
                            max={10}
                            oninput={(e) => { const v = (e.target as HTMLInputElement).valueAsNumber; updateBlockMeta(block.id, { max: isNaN(v) || v < 1 ? 5 : v }) }}
                          />
                        </div>
                      </div>
                    {/if}
                  </div>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}

        <div class="add-block-wrap">
          <button class="add-block-btn" bind:this={addBlockBtnEl} onclick={toggleBlockPicker}>
            <Plus size={14} /> {t('template_editor.add_block')}
          </button>
          {#if showBlockPicker}
            <div class="block-picker" bind:this={blockPickerEl} style="position:fixed; bottom:{window.innerHeight - blockPickerPos.top}px; left:{blockPickerPos.left}px;">
              {#each BLOCK_OPTIONS as opt}
                {@const Icon = BLOCK_ICON_MAP[opt.icon]}
                <button class="block-picker-option" onclick={() => addBlock(opt.type)} title={opt.label}>
                  {#if Icon}<Icon size={14} />{/if}
                  <span>{opt.label}</span>
                </button>
              {/each}
            </div>
          {/if}
        </div>
      </div>
    </div>

    <div class="dialog-footer">
      <button class="btn-secondary" onclick={onClose}>{t('common.cancel')}</button>
      <button class="btn-primary" onclick={save} disabled={saving}>
        {saving ? t('common.saving') : t('template_editor.save')}
      </button>
    </div>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 300;
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
    overflow: hidden;
    box-shadow: 0 8px 32px var(--shadow-lg);
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }
  .dialog-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
    gap: 0.75rem;
  }
  .header-left { display: flex; align-items: center; gap: 0.6rem; }
  .template-badge {
    font-size: 0.65rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    background: var(--accent);
    color: #fff;
    padding: 2px 8px;
    border-radius: 4px;
  }
  .dialog-header h2 { font-size: 1.1rem; font-weight: 600; margin: 0; }
  .close-btn { background: none; border: none; cursor: pointer; color: var(--text-muted); padding: 0.25rem; line-height: 1; border-radius: 4px; }
  .close-btn:hover { color: var(--text-primary); }
  .dialog-body { padding: 1.25rem; overflow-y: auto; flex: 1; display: flex; flex-direction: column; gap: 1rem; min-height: 0; }
  .dialog-footer { padding: 0.75rem 1.25rem; border-top: 1px solid var(--border-muted); display: flex; justify-content: flex-end; gap: 0.5rem; }

  .field-row { display: flex; flex-direction: column; gap: 0.35rem; }
  .field-label { font-size: 0.85rem; font-weight: 500; color: var(--text-muted); }
  .field-input {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0.5rem 0.6rem;
    color: var(--text-primary);
    font-size: 0.85rem;
    width: 100%;
    box-sizing: border-box;
  }
  .field-input:focus { outline: none; border-color: var(--accent); }

  .used-by-row { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
  .used-by-label { font-size: 0.7rem; color: var(--text-muted); }
  .used-by-chip { font-size: 0.7rem; background: var(--bg); border: 1px solid var(--border); border-radius: 999px; padding: 1px 8px; color: var(--text-primary); }

  .blocks-section { display: flex; flex-direction: column; gap: 0.5rem; }
  .empty-hint { font-size: 0.8rem; color: var(--text-muted); text-align: center; padding: 1.25rem 0; margin: 0; }

  .blocks-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
  .block-row {
    display: flex;
    flex-direction: column;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0.4rem 0.5rem;
    transition: opacity 0.15s;
  }
  .block-row.dragging { opacity: 0.4; }
  .block-row.drop-above { border-top: 2px solid var(--accent); }
  .block-row.drop-below { border-bottom: 2px solid var(--accent); }

  .block-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .drag-handle { cursor: grab; color: var(--text-muted); line-height: 1; padding: 0 2px; }
  .drag-handle:active { cursor: grabbing; }

  .block-label-btn {
    flex: 1;
    text-align: left;
    background: none;
    border: none;
    cursor: pointer;
    font-size: 0.85rem;
    color: var(--text-primary);
    padding: 0;
    min-width: 0;
  }
  .block-label-btn:hover { color: var(--accent); }

  .label-input {
    flex: 1;
    background: none;
    border: none;
    border-bottom: 1px solid var(--accent);
    font-size: 0.85rem;
    color: var(--text-primary);
    padding: 0 2px;
    outline: none;
    min-width: 0;
  }

  .block-type-hint { font-size: 0.65rem; color: var(--text-muted); text-transform: uppercase; flex-shrink: 0; }
  .delete-btn { background: none; border: none; cursor: pointer; color: var(--text-muted); padding: 2px; line-height: 1; border-radius: 3px; flex-shrink: 0; }
  .delete-btn:hover { color: var(--danger); }

  .prepopulate-toggle {
    display: flex;
    align-items: center;
    gap: 4px;
    cursor: pointer;
    flex-shrink: 0;
    font-size: 0.7rem;
    color: var(--text-muted);
    user-select: none;
  }
  .prepopulate-toggle input[type="checkbox"] {
    margin: 0;
    cursor: pointer;
  }
  .prepopulate-label { white-space: nowrap; }

  .prepopulate-editor {
    margin-top: 0.4rem;
    padding-left: 1.4rem;
  }
  .prepopulate-textarea {
    width: 100%;
    box-sizing: border-box;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0.5rem 0.6rem;
    color: var(--text-primary);
    font-size: 0.85rem;
    font-family: inherit;
    resize: vertical;
    min-height: 56px;
  }
  .prepopulate-textarea:focus { outline: none; border-color: var(--accent); }


  .add-block-btn {
    display: flex;
    align-items: center;
    gap: 6px;
    background: none;
    border: 1px dashed var(--border);
    border-radius: var(--radius);
    padding: 0.4rem 0.75rem;
    font-size: 0.8rem;
    color: var(--text-muted);
    cursor: pointer;
    width: 100%;
    justify-content: center;
  }
  .add-block-btn:hover { border-color: var(--accent); color: var(--accent); }

  .block-picker {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 4px 16px var(--shadow-lg);
    padding: 0.35rem;
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 2px;
    z-index: 9999;
    min-width: 220px;
  }
  .block-picker-option {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.6rem;
    border: none;
    border-radius: 5px;
    background: transparent;
    color: var(--text-primary);
    font-size: 0.8rem;
    cursor: pointer;
    white-space: nowrap;
  }
  .block-picker-option:hover { background: var(--bg-elevated); }

  .btn-primary {
    background: var(--accent);
    color: #fff;
    border: none;
    border-radius: var(--radius);
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
    border-radius: var(--radius);
    padding: 0.5rem 1rem;
    font-size: 0.85rem;
    color: var(--text-primary);
    cursor: pointer;
  }
  .btn-secondary:hover { background: var(--bg-hover); }

  .config-fields { display: flex; flex-direction: column; gap: 6px; }
  .config-row { display: flex; align-items: center; gap: 8px; }
  .config-label { font-size: 0.75rem; color: var(--text-muted); min-width: 40px; }
  .config-input-sm {
    width: 80px; padding: 3px 6px; border: 1px solid var(--border); border-radius: 4px;
    background: var(--bg-elevated); color: var(--text-primary); font-size: 0.8rem;
  }
  .config-input-sm:focus { outline: none; border-color: var(--accent); }
  .config-toggle { display: flex; align-items: center; gap: 6px; font-size: 0.8rem; color: var(--text-muted); cursor: pointer; }
  .config-toggle input[type="checkbox"] { margin: 0; cursor: pointer; }
  .config-options-list { display: flex; flex-direction: column; gap: 3px; }
  .config-option-row { display: flex; align-items: center; gap: 4px; padding: 2px 0; }
  .config-option-text { flex: 1; font-size: 0.8rem; color: var(--text-primary); }
  .config-option-remove { background: none; border: none; color: var(--text-muted); cursor: pointer; padding: 0; line-height: 1; }
  .config-option-remove:hover { color: var(--danger); }
  .config-option-add { display: flex; gap: 4px; }
  .config-option-input {
    flex: 1; padding: 3px 6px; border: 1px solid var(--border); border-radius: 4px;
    background: var(--bg-elevated); color: var(--text-primary); font-size: 0.8rem;
  }
  .config-option-input:focus { outline: none; border-color: var(--accent); }
  .config-option-add-btn { background: none; border: none; color: var(--accent); cursor: pointer; padding: 0; line-height: 1; }
</style>
