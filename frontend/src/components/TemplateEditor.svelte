<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { showConfirm } from '../lib/confirm.svelte'
  import { focusTrap, portal } from '../lib/actions'
  import { draggable } from '../lib/draggable'
  import { GripVertical, Plus, Trash2, Type, ListChecks, List, Film, Link, Minus, X } from 'lucide-svelte'
  import EditableChecklist from './EditableChecklist.svelte'
  import EditableList from './EditableList.svelte'
  import type { CardTemplate, Block, BlockMeta, ChecklistItem, ListItem } from '../lib/types'
  import type { CardTypeInfo } from '../lib/types'

  let { template, allTypes, onSave, onClose }: {
    template?: CardTemplate
    allTypes: CardTypeInfo[]
    onSave: (t: CardTemplate) => void
    onClose: () => void
  } = $props()

  let name = $state(template?.name ?? '')
  let blocks = $state<Block[]>(template?.blocks ? JSON.parse(JSON.stringify(template.blocks)) : [])
  let saving = $state(false)

  const BLOCK_OPTIONS = [
    { type: 'text',      label: 'Text',      icon: 'Type' },
    { type: 'checklist', label: 'Checklist', icon: 'ListChecks' },
    { type: 'list',      label: 'List',      icon: 'List' },
    { type: 'media',     label: 'Media',     icon: 'Film' },
    { type: 'url',       label: 'Link',      icon: 'Link' },
    { type: 'divider',   label: 'Divider',   icon: 'Minus' },
  ] as const

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const BLOCK_ICON_MAP: Record<string, any> = {
    Type, ListChecks, List, Film, Link, Minus,
  }

  function labelToKey(label: string): string {
    return label.trim().toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '')
  }

  // Pre-populate: track which blocks have their value editor open
  const PREPOPULABLE_TYPES = ['text', 'checklist', 'list'] as const
  function canPrepopulate(type: string): boolean {
    return (PREPOPULABLE_TYPES as readonly string[]).includes(type)
  }

  let prePopulate = $state<Record<string, boolean>>(
    Object.fromEntries(
      (template?.blocks ?? [])
        .filter(b => canPrepopulate(b.type))
        .filter(b => {
          if (b.type === 'text') return typeof b.value === 'string' && b.value.length > 0
          if (b.type === 'checklist') return Array.isArray(b.value) && b.value.length > 0
          if (b.type === 'list') return Array.isArray(b.value) && b.value.length > 0
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
        return b
      })
    }
  }

  function updateBlockValue(blockId: string, value: Block['value']) {
    blocks = blocks.map(b => b.id === blockId ? { ...b, value } : b)
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

  async function deleteBlock(blockId: string) {
    const block = blocks.find(b => b.id === blockId)
    const ok = await showConfirm(
      t('template_editor.confirm_delete_block').replace('{name}', block?.label ?? '')
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

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.stopPropagation()
      onClose()
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
<div class="overlay" role="presentation" use:portal onclick={handleOverlayClick}>
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
        <label class="field-label">{t('template_editor.name')}</label>
        <input
          class="field-input"
          bind:value={name}
          placeholder={t('template_editor.name_placeholder')}
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
                      onblur={() => commitLabel(block.id)}
                      onkeydown={(e) => { if (e.key === 'Enter') commitLabel(block.id); if (e.key === 'Escape') { e.stopPropagation(); editingLabelId = null } }}
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

  .add-block-wrap { }
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
</style>
