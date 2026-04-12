<script lang="ts">
  import { optionsEditorState, resolveOptionsEditor } from '../lib/optionsEditor.svelte'
  import { fade } from 'svelte/transition'
  import { t } from '../lib/i18n.svelte'
  import { focusTrap, focusOnMount } from '../lib/actions'
  import { GripVertical, Trash2, Plus, Pencil, Check, X } from 'lucide-svelte'
  import type { BlockMeta } from '../lib/types'

  // Local working copies
  let options = $state<string[]>([])
  let meta = $state<BlockMeta>({})
  let newOption = $state('')
  let editingIdx = $state<number | null>(null)
  let editDraft = $state('')

  // Drag state
  let dragIdx = $state<number | null>(null)
  let dropIdx = $state<number | null>(null)

  // Sync from global state when dialog opens
  $effect(() => {
    if (optionsEditorState.visible) {
      options = [...optionsEditorState.options]
      meta = { ...optionsEditorState.meta }
      newOption = ''
      editingIdx = null
    }
  })

  function addOption() {
    const val = newOption.trim()
    if (!val || options.includes(val)) return
    options = [...options, val]
    newOption = ''
  }

  function removeOption(idx: number) {
    options = options.filter((_, i) => i !== idx)
  }

  function startEdit(idx: number) {
    editingIdx = idx
    editDraft = options[idx]
  }

  function commitEdit() {
    if (editingIdx === null) return
    const val = editDraft.trim()
    if (val && !options.some((o, i) => o === val && i !== editingIdx)) {
      options = options.map((o, i) => i === editingIdx ? val : o)
    }
    editingIdx = null
  }

  function cancelEdit() {
    editingIdx = null
  }

  function handleDragStart(idx: number) {
    dragIdx = idx
  }

  function handleDragOver(e: DragEvent, idx: number) {
    e.preventDefault()
    dropIdx = idx
  }

  function handleDrop() {
    if (dragIdx !== null && dropIdx !== null && dragIdx !== dropIdx) {
      const reordered = [...options]
      const [moved] = reordered.splice(dragIdx, 1)
      reordered.splice(dropIdx, 0, moved)
      options = reordered
    }
    dragIdx = null
    dropIdx = null
  }

  function handleDragEnd() {
    dragIdx = null
    dropIdx = null
  }

  function save() {
    resolveOptionsEditor({ options, meta })
  }

  function cancel() {
    resolveOptionsEditor(null)
  }

  function handleKeydown(e: KeyboardEvent) {
    if (!optionsEditorState.visible) return
    if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); cancel() }
    if (e.key === 'Enter' && e.ctrlKey) { e.preventDefault(); e.stopPropagation(); save() }
  }

  // Derived: which toolbar options to show
  const blockType = $derived(optionsEditorState.blockType)
  const showMultiToggle = $derived(blockType === 'select')
  const showOrientation = $derived(blockType === 'radio' || blockType === 'checkbox_group')
</script>

<svelte:window onkeydown={handleKeydown} />

{#if optionsEditorState.visible}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="oe-backdrop" role="presentation" onclick={cancel} out:fade={{ duration: 150 }}>
    <div class="oe-dialog" role="dialog" aria-modal="true" tabindex="-1" onclick={(e) => e.stopPropagation()} use:focusTrap>
      <div class="oe-header">
        <h3 class="oe-title">{optionsEditorState.title}</h3>
        <button class="oe-close" onclick={cancel}><X size={16} /></button>
      </div>

      <!-- Toolbar: block-type-specific settings -->
      <div class="oe-toolbar">
        {#if showMultiToggle}
          <label class="oe-toggle">
            <input type="checkbox" checked={meta.multi || false} onchange={() => { meta = { ...meta, multi: !meta.multi } }} />
            <span>{t('options_editor.multi_select')}</span>
          </label>
        {/if}
        {#if showOrientation}
          <div class="oe-orientation">
            <span class="oe-orientation-label">{t('options_editor.layout')}</span>
            <button class="oe-orient-btn" class:active={(meta.orientation || 'vertical') === 'vertical'} onclick={() => { meta = { ...meta, orientation: 'vertical' } }}>{t('options_editor.vertical')}</button>
            <button class="oe-orient-btn" class:active={meta.orientation === 'horizontal'} onclick={() => { meta = { ...meta, orientation: 'horizontal' } }}>{t('options_editor.horizontal')}</button>
          </div>
        {/if}
      </div>

      <!-- Options list -->
      <div class="oe-list" role="list">
        {#each options as opt, i}
          <div
            class="oe-item"
            class:dragging={dragIdx === i}
            class:drop-target={dropIdx === i && dragIdx !== i}
            role="listitem"
            ondragover={(e) => handleDragOver(e, i)}
            ondrop={handleDrop}
          >
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <span
              class="oe-drag-handle"
              draggable={true}
              ondragstart={() => handleDragStart(i)}
              ondragend={handleDragEnd}
            >
              <GripVertical size={14} />
            </span>
            {#if editingIdx === i}
              <input
                class="oe-edit-input"
                use:focusOnMount={true}
                bind:value={editDraft}
                onkeydown={(e) => { if (e.key === 'Enter') commitEdit(); if (e.key === 'Escape') cancelEdit() }}
                onblur={commitEdit}
              />
              <button class="oe-item-btn" onclick={commitEdit}><Check size={14} /></button>
            {:else}
              <!-- svelte-ignore a11y_no_static_element_interactions -->
              <span class="oe-item-text" role="button" tabindex="-1" ondblclick={() => startEdit(i)}>{opt}</span>
              <span class="oe-item-actions">
                <button class="oe-item-btn oe-edit" onclick={() => startEdit(i)}><Pencil size={12} /></button>
                <button class="oe-item-btn oe-delete" onclick={() => removeOption(i)}><Trash2 size={12} /></button>
              </span>
            {/if}
          </div>
        {/each}
        {#if options.length === 0}
          <div class="oe-empty">{t('options_editor.empty')}</div>
        {/if}
      </div>

      <!-- Add new option -->
      <div class="oe-add-row">
        <input
          class="oe-add-input"
          type="text"
          placeholder={t('options_editor.add_placeholder')}
          bind:value={newOption}
          onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addOption() } }}
        />
        <button class="oe-add-btn" onclick={addOption} disabled={!newOption.trim()}>
          <Plus size={14} />
          <span>{t('options_editor.add')}</span>
        </button>
      </div>

      <!-- Footer -->
      <div class="oe-footer">
        <span class="oe-count">{options.length} {options.length === 1 ? t('options_editor.item') : t('options_editor.items')}</span>
        <div class="oe-footer-actions">
          <button class="oe-btn-cancel" onclick={cancel}>{t('common.cancel')}</button>
          <button class="oe-btn-save" onclick={save}>{t('common.save')}</button>
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  .oe-backdrop {
    position: fixed; inset: 0; z-index: 99990;
    background: var(--bg-overlay);
    display: flex; align-items: center; justify-content: center;
    animation: fade-in var(--duration-normal) var(--ease-out);
  }
  .oe-dialog {
    background: var(--bg-surface); border: 1px solid var(--border); border-radius: 10px;
    width: 440px; max-height: 80vh; display: flex; flex-direction: column;
    box-shadow: 0 8px 32px var(--shadow-lg);
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }
  .oe-header {
    display: flex; align-items: center; justify-content: space-between;
    padding: 1rem 1.25rem 0.75rem; border-bottom: 1px solid var(--border-muted);
  }
  .oe-title { margin: 0; font-size: 1rem; font-weight: 600; color: var(--text-primary); }
  .oe-close { background: none; border: none; color: var(--text-muted); cursor: pointer; padding: 4px; border-radius: 4px; }
  .oe-close:hover { color: var(--text-primary); background: var(--bg-hover); }

  /* Toolbar */
  .oe-toolbar {
    display: flex; align-items: center; gap: 1rem; padding: 0.6rem 1.25rem;
    border-bottom: 1px solid var(--border-muted); min-height: 0;
  }
  .oe-toolbar:empty { display: none; }
  .oe-toggle { display: flex; align-items: center; gap: 6px; font-size: 0.8rem; color: var(--text-secondary); cursor: pointer; }
  .oe-toggle input { accent-color: var(--accent); }
  .oe-orientation { display: flex; align-items: center; gap: 4px; }
  .oe-orientation-label { font-size: 0.8rem; color: var(--text-muted); margin-right: 4px; }
  .oe-orient-btn {
    padding: 2px 10px; font-size: 0.75rem; border: 1px solid var(--border); border-radius: 4px;
    background: none; color: var(--text-muted); cursor: pointer;
  }
  .oe-orient-btn.active { background: var(--accent); color: #fff; border-color: var(--accent); }
  .oe-orient-btn:hover:not(.active) { border-color: var(--accent); color: var(--accent); }

  /* Options list */
  .oe-list { flex: 1; overflow-y: auto; padding: 0.5rem 0; min-height: 100px; max-height: 340px; }
  .oe-item {
    display: flex; align-items: center; gap: 6px;
    padding: 0.35rem 1.25rem; transition: background 0.1s;
  }
  .oe-item:hover { background: var(--bg-hover); }
  .oe-item.dragging { opacity: 0.4; }
  .oe-item.drop-target { border-top: 2px solid var(--accent); }
  .oe-drag-handle { color: var(--text-faint); cursor: grab; flex-shrink: 0; display: flex; }
  .oe-drag-handle:active { cursor: grabbing; }
  .oe-item-text { flex: 1; font-size: 0.9rem; color: var(--text-primary); cursor: default; user-select: none; }
  .oe-edit-input {
    flex: 1; padding: 3px 8px; font-size: 0.9rem; border: 1px solid var(--accent); border-radius: 4px;
    background: var(--bg-surface); color: var(--text-primary); outline: none;
  }
  .oe-item-actions { display: flex; gap: 2px; opacity: 0; transition: opacity 0.1s; }
  .oe-item:hover .oe-item-actions { opacity: 1; }
  .oe-item-btn {
    background: none; border: none; color: var(--text-muted); cursor: pointer;
    padding: 3px; border-radius: 3px; display: flex;
  }
  .oe-item-btn.oe-edit:hover { color: var(--accent); }
  .oe-item-btn.oe-delete:hover { color: var(--danger); }
  .oe-empty { padding: 1.5rem; text-align: center; color: var(--text-muted); font-style: italic; font-size: 0.85rem; }

  /* Add row */
  .oe-add-row {
    display: flex; gap: 6px; padding: 0.6rem 1.25rem;
    border-top: 1px solid var(--border-muted);
  }
  .oe-add-input {
    flex: 1; padding: 6px 10px; border: 1px solid var(--border); border-radius: 6px;
    background: var(--bg-surface); color: var(--text-primary); font-size: 0.9rem;
  }
  .oe-add-input:focus { border-color: var(--accent); outline: none; }
  .oe-add-btn {
    display: flex; align-items: center; gap: 4px;
    padding: 6px 12px; border-radius: 6px; border: none;
    background: var(--accent); color: #fff; font-size: 0.85rem;
    cursor: pointer; white-space: nowrap;
  }
  .oe-add-btn:disabled { opacity: 0.4; cursor: default; }
  .oe-add-btn:hover:not(:disabled) { filter: brightness(1.1); }

  /* Footer */
  .oe-footer {
    display: flex; align-items: center; justify-content: space-between;
    padding: 0.75rem 1.25rem; border-top: 1px solid var(--border-muted);
  }
  .oe-count { font-size: 0.75rem; color: var(--text-faint); }
  .oe-footer-actions { display: flex; gap: 0.5rem; }
  .oe-btn-cancel {
    padding: 0.4rem 0.9rem; border-radius: 5px; border: 1px solid var(--border);
    background: var(--bg-elevated); color: var(--text-body); font-size: 0.85rem; cursor: pointer;
  }
  .oe-btn-cancel:hover { background: var(--bg-surface); }
  .oe-btn-save {
    padding: 0.4rem 0.9rem; border-radius: 5px; border: none;
    background: var(--accent); color: #fff; font-size: 0.85rem; font-weight: 500; cursor: pointer;
  }
  .oe-btn-save:hover { filter: brightness(1.1); }
</style>
