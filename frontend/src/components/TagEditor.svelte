<script lang="ts">
  import { tick, onMount } from 'svelte'
  import { projectTags, nav, loadBoard } from '../lib/store.svelte'
  import { AddProjectLabel, RemoveProjectLabel, UpdateProjectLabel, ListCardIDsByTag, GetCard, UpdateCardTags } from '../lib/api'
  import { X, Plus, Trash2, Palette } from 'lucide-svelte'

  const TAG_PALETTE = [
    '#61bd4f', '#f2d600', '#ff9f1a', '#eb5a46', '#c377e0',
    '#0079bf', '#00c2e0', '#51e898', '#ff78cb', '#344563',
    '#b3bac5', '#096dd9',
  ]

  let { onClose }: { onClose: () => void } = $props()

  let query = $state('')
  let editingId = $state<string | null>(null)
  let editingName = $state('')
  let editInputEl = $state<HTMLInputElement | null>(null)
  let colorPickerTagId = $state<string | null>(null)

  // Usage counts: tag name (lowercase) → number of cards
  let tagUsage = $state<Record<string, number>>({})

  // Delete confirmation
  let confirmDelete = $state<{ id: string; name: string; count: number } | null>(null)

  async function loadUsageCounts() {
    const counts: Record<string, number> = {}
    for (const tag of projectTags.list) {
      try {
        const ids = await ListCardIDsByTag(tag.name) || []
        counts[tag.name.toLowerCase()] = ids.length
      } catch { counts[tag.name.toLowerCase()] = 0 }
    }
    tagUsage = counts
  }

  onMount(() => { loadUsageCounts() })

  let filteredTags = $derived(
    projectTags.list.filter(t =>
      !query.trim() || t.name.toLowerCase().includes(query.toLowerCase())
    )
  )

  let canCreate = $derived(
    query.trim() && !projectTags.list.some(t => t.name.toLowerCase() === query.trim().toLowerCase())
  )

  async function createTag() {
    const name = query.trim()
    if (!name || !nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return
    try {
      projectTags.list = await AddProjectLabel(nav.brandSlug, nav.streamSlug, nav.projectSlug, name, '') || []
      tagUsage[name.toLowerCase()] = 0
      query = ''
    } catch (e) { console.error('Add tag:', e) }
  }

  async function requestRemoveTag(tagId: string) {
    const tag = projectTags.list.find(t => t.id === tagId)
    if (!tag) return
    const count = tagUsage[tag.name.toLowerCase()] || 0
    if (count > 0) {
      confirmDelete = { id: tagId, name: tag.name, count }
    } else {
      await doRemoveTag(tagId, tag.name)
    }
  }

  async function doRemoveTag(tagId: string, tagName: string) {
    if (!nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return
    try {
      // Remove tag from all cards that have it
      const cardIds = await ListCardIDsByTag(tagName) || []
      for (const cardId of cardIds) {
        try {
          const card = await GetCard(cardId)
          const newTags = (card.tags || []).filter((t: string) => t.toLowerCase() !== tagName.toLowerCase())
          await UpdateCardTags(cardId, newTags)
        } catch { /* best-effort */ }
      }
      // Remove from project
      projectTags.list = await RemoveProjectLabel(nav.brandSlug, nav.streamSlug, nav.projectSlug, tagId) || []
      confirmDelete = null
      await loadUsageCounts()
      // Refresh board so card items drop the removed tag
      if (nav.brandSlug && nav.streamSlug && nav.projectSlug) {
        await loadBoard(nav.brandSlug, nav.streamSlug, nav.projectSlug)
      }
    } catch (e) { console.error('Remove tag:', e) }
  }

  async function startEdit(tag: { id: string; name: string }) {
    editingId = tag.id
    editingName = tag.name
    await tick()
    editInputEl?.focus()
    editInputEl?.select()
  }

  async function commitEdit() {
    if (!editingId || !nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return
    const name = editingName.trim()
    if (!name) { editingId = null; return }
    const tag = projectTags.list.find(t => t.id === editingId)
    try {
      projectTags.list = await UpdateProjectLabel(nav.brandSlug, nav.streamSlug, nav.projectSlug, editingId, name, tag?.color || '') || []
      editingId = null
      editingName = ''
    } catch (e) { console.error('Edit tag:', e) }
  }

  async function changeColor(tagId: string, color: string) {
    if (!nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return
    const tag = projectTags.list.find(t => t.id === tagId)
    if (!tag) return
    try {
      projectTags.list = await UpdateProjectLabel(nav.brandSlug, nav.streamSlug, nav.projectSlug, tagId, tag.name, color) || []
      colorPickerTagId = null
    } catch (e) { console.error('Change color:', e) }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="modal-backdrop" role="presentation" onclick={onClose} onkeydown={handleKeydown}>
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="tag-editor" role="dialog" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={handleKeydown}>
    <div class="editor-header">
      <h2 class="editor-title">Project Tags</h2>
      <button class="close-btn" onclick={onClose}><X size={18} /></button>
    </div>

    <input
      type="text"
      class="search-input"
      bind:value={query}
      onkeydown={(e) => { if (e.key === 'Enter' && canCreate) createTag() }}
      placeholder="Search or create tag..."
    />

    <div class="tag-list">
      {#each filteredTags as tag (tag.id)}
        <div class="tag-row">
          {#if editingId === tag.id}
            <input
              type="text"
              class="edit-input"
              bind:this={editInputEl}
              bind:value={editingName}
              onkeydown={(e) => { if (e.key === 'Enter') commitEdit(); if (e.key === 'Escape') { editingId = null; e.stopPropagation() } }}
              onblur={commitEdit}
            />
          {:else}
            <span
              class="tag-chip"
              style:background={tag.color || 'var(--border)'}
              role="button"
              tabindex="0"
              onclick={() => startEdit(tag)}
              onkeydown={(e) => { if (e.key === 'Enter') startEdit(tag) }}
            >{tag.name}</span>
            {@const count = tagUsage[tag.name.toLowerCase()] || 0}
            <span class="tag-usage" class:unused={count === 0}>{count === 0 ? 'unused' : count}</span>
            <div class="tag-actions">
              <button class="action-btn color-btn" onclick={() => { colorPickerTagId = colorPickerTagId === tag.id ? null : tag.id }} title="Change color"><Palette size={12} /></button>
              <button class="action-btn delete-btn" onclick={() => requestRemoveTag(tag.id)} title="Remove from project"><Trash2 size={12} /></button>
            </div>
          {/if}
        </div>
        {#if colorPickerTagId === tag.id}
          <div class="color-picker">
            {#each TAG_PALETTE as color}
              <button
                class="color-swatch"
                class:active={tag.color === color}
                style:background={color}
                onclick={() => changeColor(tag.id, color)}
              ></button>
            {/each}
          </div>
        {/if}
      {/each}

      {#if filteredTags.length === 0 && !canCreate}
        <p class="empty-msg">No tags yet</p>
      {/if}

      {#if canCreate}
        <button class="create-row" onclick={createTag}>
          <Plus size={14} /> Create "{query.trim()}"
        </button>
      {/if}
    </div>
  </div>
</div>

{#if confirmDelete}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="confirm-backdrop" role="presentation" onclick={() => confirmDelete = null}>
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="confirm-dialog" role="dialog" tabindex="-1" onclick={(e) => e.stopPropagation()}>
      <h3 class="confirm-title">Delete tag "{confirmDelete.name}"?</h3>
      <p class="confirm-msg">This tag is used on {confirmDelete.count} card{confirmDelete.count === 1 ? '' : 's'}. It will be removed from all of them.</p>
      <div class="confirm-actions">
        <button class="btn-ghost" onclick={() => confirmDelete = null}>Cancel</button>
        <button class="btn-danger" onclick={() => confirmDelete && doRemoveTag(confirmDelete.id, confirmDelete.name)}>Delete</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 200;
  }

  .tag-editor {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 1.25rem;
    width: 380px;
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px var(--shadow-lg);
  }

  .editor-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.75rem;
  }

  .editor-title {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
    color: var(--text-primary);
  }

  .close-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
    border-radius: 4px;
    display: flex;
    align-items: center;
  }
  .close-btn:hover { color: var(--text-primary); }

  .search-input {
    width: 100%;
    font-size: 0.85rem;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-strong);
    padding: 0.4rem 0.6rem;
    outline: none;
    margin-bottom: 0.5rem;
    box-sizing: border-box;
  }
  .search-input:focus { border-color: var(--accent); }

  .tag-list {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-height: 60px;
    max-height: 400px;
  }

  .tag-row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.25rem 0.3rem;
    border-radius: 6px;
  }
  .tag-row:hover { background: var(--bg-elevated); }

  .tag-chip {
    font-size: 0.75rem;
    font-weight: 600;
    padding: 0.15rem 0.5rem;
    border-radius: 4px;
    color: #fff;
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    cursor: pointer;
  }
  .tag-chip:hover { color: #f0f0f0; }

  .tag-actions {
    display: flex;
    gap: 0.15rem;
    opacity: 0;
    transition: opacity 0.15s;
  }
  .tag-row:hover .tag-actions { opacity: 1; }

  .action-btn {
    background: none;
    border: none;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0.2rem;
    border-radius: 3px;
    display: flex;
    align-items: center;
    transition: color 0.15s;
  }
  .action-btn:hover { color: var(--accent); }
  .delete-btn:hover { color: var(--danger); }

  .edit-input {
    flex: 1;
    font-size: 0.85rem;
    background: var(--bg-elevated);
    border: 1px solid var(--accent);
    border-radius: 4px;
    color: var(--text-strong);
    padding: 0.25rem 0.4rem;
    outline: none;
  }

  .color-picker {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
    padding: 0.3rem 0.3rem 0.3rem 0.5rem;
  }

  .color-swatch {
    width: 22px;
    height: 22px;
    border-radius: 4px;
    border: 2px solid transparent;
    cursor: pointer;
    transition: border-color 0.15s, transform 0.1s;
  }
  .color-swatch:hover { transform: scale(1.15); }
  .color-swatch.active { border-color: var(--text-strong); }

  .create-row {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    width: 100%;
    background: none;
    border: 1px dashed var(--border);
    border-radius: 6px;
    color: var(--accent);
    font-size: 0.8rem;
    font-weight: 500;
    padding: 0.35rem 0.4rem;
    cursor: pointer;
    margin-top: 0.2rem;
    transition: background 0.15s, border-color 0.15s;
  }
  .create-row:hover {
    background: var(--bg-elevated);
    border-color: var(--accent);
  }

  .tag-usage {
    font-size: 0.7rem;
    color: var(--text-muted);
    white-space: nowrap;
    min-width: 2rem;
    text-align: right;
  }
  .tag-usage.unused {
    color: var(--text-faint);
    font-style: italic;
  }

  .empty-msg {
    font-size: 0.8rem;
    color: var(--text-muted);
    text-align: center;
    padding: 0.5rem;
    margin: 0;
  }

  .confirm-backdrop {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 300;
  }

  .confirm-dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 1.25rem;
    width: 320px;
    box-shadow: 0 8px 32px var(--shadow-lg);
  }

  .confirm-title {
    margin: 0 0 0.5rem;
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text-strong);
  }

  .confirm-msg {
    margin: 0 0 1rem;
    font-size: 0.8rem;
    color: var(--text-secondary);
    line-height: 1.4;
  }

  .confirm-actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
  }

  .btn-ghost {
    background: none;
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-secondary);
    font-size: 0.8rem;
    padding: 0.35rem 0.75rem;
    cursor: pointer;
    transition: background 0.15s;
  }
  .btn-ghost:hover {
    background: var(--bg-elevated);
  }

  .btn-danger {
    background: var(--danger);
    border: none;
    border-radius: 6px;
    color: #fff;
    font-size: 0.8rem;
    font-weight: 500;
    padding: 0.35rem 0.75rem;
    cursor: pointer;
    transition: opacity 0.15s;
  }
  .btn-danger:hover {
    opacity: 0.85;
  }
</style>
