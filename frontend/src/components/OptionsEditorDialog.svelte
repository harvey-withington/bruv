<script lang="ts">
  import { optionsEditorState, resolveOptionsEditor } from '../lib/optionsEditor.svelte'
  import { fade } from 'svelte/transition'
  import { t } from '../lib/i18n.svelte'
  import { focusTrap, focusOnMount, inlineEdit } from '../lib/actions'
  import { setContext } from 'svelte'
  import { EditScope, EDIT_SCOPE_KEY } from '@shared/editScope'
  import { GripVertical, Trash2, Plus, Pencil, Check, X } from 'lucide-svelte'
  import type { BlockMeta } from '@shared/types'
  import { computeReorder, wouldReorder, DROP_END } from '../lib/reorder'

  // This dialog can be opened from inside CardDetail (editing a select /
  // radio / checkbox_group block's options) while CardDetail's own
  // EditScope is live — it's a singleton mounted once at the App root
  // (see App.svelte), not a child of CardDetail, so its Escape handling
  // can't rely on DOM-ancestor stopPropagation to shield CardDetail's
  // window keydown listener (two separate `<svelte:window>` listeners on
  // the same target don't stop each other via stopPropagation, only
  // stopImmediatePropagation would — see CardDetail.svelte's guard on
  // optionsEditorState.visible for the other half of this fix). This
  // dialog gets its own EditScope so cancelling a row edit no longer also
  // closes the whole card dialog underneath it.
  const editScope = new EditScope()
  // The dialog's affirmative "commit + close" action is Save (there's no
  // per-field auto-save here — everything is staged in local `options` /
  // `meta` until Save is clicked), so Ctrl+Enter from any registered field
  // commits then saves-and-closes via requestClose.
  editScope.requestClose = () => save()
  setContext(EDIT_SCOPE_KEY, editScope)

  // Local working copy. Options are plain strings with no natural
  // stable key, so each row gets an ephemeral drag/edit-scoped id on
  // dialog open (CLAUDE.md: never key mutable state by array index —
  // deleting or reordering a different row while one is mid-edit must
  // not drop or misassign the draft). Unwrapped back to string[] on
  // save.
  type OptionRow = { id: string; value: string }
  let rows = $state<OptionRow[]>([])
  let meta = $state<BlockMeta>({})
  let newOption = $state('')
  let newOptionInputEl = $state<HTMLInputElement | null>(null)
  let editingId = $state<string | null>(null)
  let editDraft = $state('')

  // Drag state — id-keyed, mirroring EditableChecklist's reference DnD.
  let draggingId = $state<string | null>(null)
  let dropBeforeId = $state<string | typeof DROP_END | null>(null)

  // Sync from global state when dialog opens
  $effect(() => {
    if (optionsEditorState.visible) {
      rows = optionsEditorState.options.map(value => ({ id: crypto.randomUUID(), value }))
      meta = { ...optionsEditorState.meta }
      newOption = ''
      editingId = null
    }
  })

  function addOption() {
    const val = newOption.trim()
    if (!val || rows.some(r => r.value === val)) return
    rows = [...rows, { id: crypto.randomUUID(), value: val }]
    newOption = ''
  }

  // Serial add-input per the keyboard entry contract: Enter commits and
  // re-arms, Escape discards the draft and ends the entry, blur keeps
  // the draft uncommitted.
  function cancelAddOption() {
    newOption = ''
    newOptionInputEl?.blur()
  }

  function removeOption(id: string) {
    rows = rows.filter(r => r.id !== id)
  }

  function startEdit(id: string) {
    const row = rows.find(r => r.id === id)
    if (!row) return
    editingId = id
    editDraft = row.value
  }

  function commitEdit() {
    if (editingId === null) return
    const val = editDraft.trim()
    if (val && !rows.some(r => r.value === val && r.id !== editingId)) {
      rows = rows.map(r => r.id === editingId ? { ...r, value: val } : r)
    }
    editingId = null
  }

  function cancelEdit() {
    editingId = null
  }

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
      const next = rows[idx + 1]
      candidate = next ? next.id : DROP_END
    }
    dropBeforeId = wouldReorder(rows, draggingId, candidate, 'move') ? candidate : null
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
    const reordered = computeReorder(rows, draggingId, dropBeforeId, { mode: 'move' })
    handleDragEnd()
    if (reordered !== rows) rows = reordered
  }

  function save() {
    resolveOptionsEditor({ options: rows.map(r => r.value), meta })
  }

  function cancel() {
    resolveOptionsEditor(null)
  }

  // Container side of the keyboard entry contract. Escape closes
  // (cancels) only when nothing is being edited — an active row edit or
  // the add-option input consumes Escape itself via the inlineEdit
  // action (preventDefault + stopPropagation), so this is the backstop
  // for Escape presses that land while no field is focused. Ctrl+Enter
  // commits any in-flight edit then saves-and-closes.
  function handleKeydown(e: KeyboardEvent) {
    if (!optionsEditorState.visible) return
    if (e.key === 'Escape') {
      if (editScope.hasActive()) return
      e.preventDefault()
      e.stopPropagation()
      cancel()
    } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      e.stopPropagation()
      editScope.commitAll()
      save()
    }
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
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="oe-list"
        role="list"
        ondrop={handleDrop}
        ondragover={(e) => { if (draggingId !== null) e.preventDefault() }}
      >
        {#each rows as row, i (row.id)}
          {#if draggingId !== null && dropBeforeId === row.id}
            <div class="oe-drop-indicator"></div>
          {/if}
          <div
            class="oe-item"
            class:dragging={draggingId === row.id}
            role="listitem"
            ondragover={(e) => handleDragOver(e, row.id, i)}
          >
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <span
              class="oe-drag-handle"
              draggable={true}
              ondragstart={(e) => handleDragStart(e, row.id)}
              ondragend={handleDragEnd}
              role="button"
              tabindex="-1"
              aria-label={t('tooltip.drag_option')}
              title={t('tooltip.drag_option')}
            >
              <GripVertical size={14} />
            </span>
            {#if editingId === row.id}
              <input
                class="oe-edit-input"
                use:focusOnMount={true}
                bind:value={editDraft}
                use:inlineEdit={{ onCommit: commitEdit, onCancel: cancelEdit, scope: editScope }}
              />
              <button class="oe-item-btn" onclick={commitEdit}><Check size={14} /></button>
            {:else}
              <!-- svelte-ignore a11y_no_static_element_interactions -->
              <span class="oe-item-text" role="button" tabindex="-1" ondblclick={() => startEdit(row.id)}>{row.value}</span>
              <span class="oe-item-actions">
                <button class="oe-item-btn oe-edit" onclick={() => startEdit(row.id)}><Pencil size={12} /></button>
                <button class="oe-item-btn oe-delete" onclick={() => removeOption(row.id)}><Trash2 size={12} /></button>
              </span>
            {/if}
          </div>
        {/each}
        {#if draggingId !== null && dropBeforeId === DROP_END}
          <div class="oe-drop-indicator"></div>
        {/if}
        {#if rows.length === 0}
          <div class="oe-empty">{t('options_editor.empty')}</div>
        {/if}
      </div>

      <!-- Add new option -->
      <div class="oe-add-row">
        <input
          class="oe-add-input"
          type="text"
          placeholder={t('options_editor.add_placeholder')}
          bind:this={newOptionInputEl}
          bind:value={newOption}
          use:inlineEdit={{ serial: true, onCommit: addOption, onCancel: cancelAddOption, scope: editScope }}
        />
        <button class="oe-add-btn" onclick={addOption} disabled={!newOption.trim()}>
          <Plus size={14} />
          <span>{t('common.add')}</span>
        </button>
      </div>

      <!-- Footer -->
      <div class="oe-footer">
        <span class="oe-count">{rows.length} {rows.length === 1 ? t('options_editor.item') : t('options_editor.items')}</span>
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
  .oe-drop-indicator {
    height: 2px;
    background: var(--accent);
    border-radius: 1px;
    margin: 1px 0;
  }
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
