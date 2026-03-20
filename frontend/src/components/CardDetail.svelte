<script lang="ts">
  import { GetCard, UpdateCardTitle, UpdateCardFields, UpdateCardTags, UpdateCardDueDate,
    AddChecklistItem, ToggleChecklistItem, RemoveChecklistItem, DeleteCard,
    AssignTagColor, SetTagColor, GetTagColors } from '../lib/api'
  import { tagColors } from '../lib/store.svelte'
  import { X, Trash2, Square, CheckSquare, Palette, Save } from 'lucide-svelte'
  import { renderMarkdown, renderInline } from '../lib/markdown'
  import { t } from '../lib/i18n.svelte'
  import MentionPicker from './MentionPicker.svelte'

  const TAG_PALETTE = [
    '#61bd4f', '#f2d600', '#ff9f1a', '#eb5a46', '#c377e0',
    '#0079bf', '#00c2e0', '#51e898', '#ff78cb', '#344563',
    '#b3bac5', '#096dd9',
  ]

  let colorPickerTag = $state<string | null>(null)

  let { cardId, onClose, onUpdated, autoEditTitle }: {
    cardId: string
    onClose: (opts?: { escaped?: boolean }) => void
    onUpdated?: () => void
    autoEditTitle?: boolean
  } = $props()

  let card = $state<any>(null)
  let loading = $state(true)
  let editingTitle = $state(false)
  let titleDraft = $state('')
  let titleInputEl = $state<HTMLInputElement | null>(null)
  let newTag = $state('')
  let newChecklistText = $state('')
  let descriptionDraft = $state('')
  let editingDescription = $state(false)
  let descTextareaEl = $state<HTMLTextAreaElement | null>(null)
  let checklistInputEl = $state<HTMLInputElement | null>(null)

  // @ mention picker state
  let mentionVisible = $state(false)
  let mentionAnchor = $state<{ top: number; left: number } | null>(null)
  let mentionTarget = $state<'desc' | 'checklist' | null>(null)
  let mentionTriggerPos = $state<number>(0)

  const typeColors: Record<string, string> = {
    feature: '#6366f1',
    task: '#22c55e',
    brainstorm: '#f59e0b',
    episode: '#ec4899',
    reference: '#06b6d4',
  }

  $effect(() => {
    loadCard()
  })

  $effect(() => {
    if (editingTitle && titleInputEl) {
      titleInputEl.focus()
      titleInputEl.select()
    }
  })

  $effect(() => {
    if (editingDescription && descTextareaEl) {
      descTextareaEl.focus()
    }
  })

  async function loadCard() {
    loading = true
    try {
      card = await GetCard(cardId)
      titleDraft = card.title
      descriptionDraft = card.fields?.description || ''
      if (autoEditTitle) {
        editingTitle = true
      }
    } catch (e) {
      console.error('Failed to load card:', e)
    }
    loading = false
  }

  async function saveTitle() {
    if (!titleDraft.trim() || titleDraft === card.title) {
      editingTitle = false
      return
    }
    try {
      card = await UpdateCardTitle(cardId, titleDraft.trim())
      editingTitle = false
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function handleTitleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      await saveAndClose()
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      saveTitle()
      editingDescription = true
    } else if (e.key === 'Escape') {
      editingTitle = false
      titleDraft = card.title
    }
  }

  async function saveDescription() {
    const fields = { ...(card.fields || {}), description: descriptionDraft }
    try {
      card = await UpdateCardFields(cardId, fields)
      editingDescription = false
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function handleDescKeydown(e: KeyboardEvent) {
    if (mentionVisible) return
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      await saveAndClose()
    } else if (e.key === 'Escape') {
      editingDescription = false
      descriptionDraft = card.fields?.description || ''
    }
  }

  function handleDescInput(e: Event) {
    const el = e.target as HTMLTextAreaElement
    checkForMention(el, 'desc')
  }

  function handleChecklistInput(e: Event) {
    const el = e.target as HTMLInputElement
    checkForMention(el, 'checklist')
  }

  function checkForMention(el: HTMLTextAreaElement | HTMLInputElement, target: 'desc' | 'checklist') {
    const pos = el.selectionStart ?? 0
    const text = el.value
    // Look for @ at current position (just typed)
    if (pos > 0 && text[pos - 1] === '@') {
      // Only trigger if @ is at start or preceded by whitespace
      if (pos === 1 || /\s/.test(text[pos - 2])) {
        mentionTriggerPos = pos - 1
        mentionTarget = target
        // Calculate anchor position from the element's bounding rect
        const rect = el.getBoundingClientRect()
        mentionAnchor = { top: rect.bottom + 4, left: rect.left }
        mentionVisible = true
        return
      }
    }
  }

  function handleMentionSelect(markdown: string) {
    if (mentionTarget === 'desc' && descTextareaEl) {
      const before = descriptionDraft.slice(0, mentionTriggerPos)
      const after = descriptionDraft.slice(descTextareaEl.selectionStart ?? mentionTriggerPos + 1)
      descriptionDraft = before + markdown + after
      mentionVisible = false
      mentionTarget = null
      // Restore focus and cursor
      const newPos = before.length + markdown.length
      setTimeout(() => { descTextareaEl?.focus(); descTextareaEl?.setSelectionRange(newPos, newPos) }, 0)
    } else if (mentionTarget === 'checklist' && checklistInputEl) {
      const before = newChecklistText.slice(0, mentionTriggerPos)
      const after = newChecklistText.slice(checklistInputEl.selectionStart ?? mentionTriggerPos + 1)
      newChecklistText = before + markdown + after
      mentionVisible = false
      mentionTarget = null
      const newPos = before.length + markdown.length
      setTimeout(() => { checklistInputEl?.focus(); checklistInputEl?.setSelectionRange(newPos, newPos) }, 0)
    }
  }

  function handleMentionClose() {
    mentionVisible = false
    mentionTarget = null
  }

  async function addTag() {
    const tag = newTag.trim().toLowerCase()
    if (!tag || card.tags?.includes(tag)) { newTag = ''; return }
    const tags = [...(card.tags || []), tag]
    try {
      card = await UpdateCardTags(cardId, tags)
      // Auto-assign a color from the palette
      const colors = await AssignTagColor(tag)
      tagColors.map = colors || {}
      newTag = ''
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function changeTagColor(tag: string, color: string) {
    try {
      const colors = await SetTagColor(tag, color)
      tagColors.map = colors || {}
      colorPickerTag = null
    } catch (e) { console.error(e) }
  }

  async function removeTag(tag: string) {
    const tags = (card.tags || []).filter((t: string) => t !== tag)
    try {
      card = await UpdateCardTags(cardId, tags)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  function handleTagKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') addTag()
  }

  async function handleDueDateChange(e: Event) {
    const value = (e.target as HTMLInputElement).value
    try {
      card = await UpdateCardDueDate(cardId, value)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function addChecklist() {
    if (!newChecklistText.trim()) return
    try {
      card = await AddChecklistItem(cardId, newChecklistText.trim())
      newChecklistText = ''
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function toggleChecklist(itemId: string) {
    try {
      card = await ToggleChecklistItem(cardId, itemId)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function removeChecklist(itemId: string) {
    try {
      card = await RemoveChecklistItem(cardId, itemId)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  function handleChecklistKeydown(e: KeyboardEvent) {
    if (mentionVisible) return
    if (e.key === 'Enter') addChecklist()
  }

  async function handleDelete() {
    if (!confirm(t('card.delete_confirm'))) return
    try {
      await DeleteCard(cardId)
      onUpdated?.()
      onClose()
    } catch (e) { console.error(e) }
  }

  async function saveAndClose() {
    if (editingTitle && titleDraft.trim() && titleDraft !== card.title) {
      await saveTitle()
    }
    if (editingDescription) {
      await saveDescription()
    }
    onClose()
  }

  function handleBackdropClick(e: MouseEvent) {
    if ((e.target as HTMLElement).classList.contains('modal-backdrop')) {
      onClose()
    }
  }

  function handleBackdropKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      onClose({ escaped: true })
    } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      saveAndClose()
    }
  }
</script>

<svelte:window onkeydown={handleBackdropKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="modal-backdrop" role="presentation" onclick={handleBackdropClick}>
  <div class="modal">
    {#if loading}
      <div class="modal-loading">{t('app.loading')}</div>
    {:else if card}
      <div class="modal-header">
        <span class="type-badge" style="background: {typeColors[card.type] || '#71717a'}">{card.type}</span>

        {#if editingTitle}
          <input
            class="title-input"
            bind:this={titleInputEl}
            bind:value={titleDraft}
            onkeydown={handleTitleKeydown}
            onblur={saveTitle}
          />
        {:else}
          <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <h2 class="modal-title" onclick={() => { editingTitle = true }} title={t('tooltip.edit_title')}>
            {@html renderInline(card.title)}
          </h2>
        {/if}

        <button class="close-btn" onclick={() => onClose()} title={t('tooltip.close_card')}><X size={18} /></button>
      </div>

      <div class="modal-body">
        <!-- Description -->
        <section class="section">
          <h3 class="section-title">{t('card.description')}</h3>
          {#if editingDescription}
            <textarea
              class="desc-textarea"
              bind:this={descTextareaEl}
              bind:value={descriptionDraft}
              onkeydown={handleDescKeydown}
              oninput={handleDescInput}
              rows="4"
            ></textarea>
            <div class="section-actions">
              <button class="btn-cancel-sm" onclick={() => { editingDescription = false; descriptionDraft = card.fields?.description || '' }}>Cancel</button>
            </div>
          {:else}
            <div class="desc-display" role="button" tabindex="0" onclick={(e) => { if ((e.target as HTMLElement).closest('a')) return; editingDescription = true }} title={t('tooltip.edit_description')}>
              {#if card.fields?.description}
                <div class="markdown-content">{@html renderMarkdown(card.fields.description)}</div>
              {:else}
                <p class="placeholder">{t('card.description_placeholder')}</p>
              {/if}
            </div>
          {/if}
        </section>

        <!-- Due Date -->
        <section class="section">
          <h3 class="section-title">{t('card.due_date')}</h3>
          <input
            type="date"
            class="date-input"
            value={card.due_date ? card.due_date.slice(0, 10) : ''}
            onchange={handleDueDateChange}
          />
        </section>

        <!-- Tags -->
        <section class="section">
          <h3 class="section-title">{t('card.tags')}</h3>
          <div class="tags-list">
            {#each (card.tags || []) as tag}
              <span class="tag-chip" style:background={tagColors.map[tag] || 'var(--border)'}>
                <span class="tag-label">{tag}</span>
                <button class="tag-color-btn" onclick={() => colorPickerTag = colorPickerTag === tag ? null : tag} title={t('tooltip.change_tag_color')}><Palette size={10} /></button>
                <button class="tag-remove" onclick={() => removeTag(tag)} title={t('tooltip.remove_tag')}><X size={12} /></button>
              </span>
              {#if colorPickerTag === tag}
                <div class="color-picker">
                  {#each TAG_PALETTE as color}
                    <button
                      class="color-swatch"
                      class:active={tagColors.map[tag] === color}
                      style:background={color}
                      onclick={() => changeTagColor(tag, color)}
                    ></button>
                  {/each}
                </div>
              {/if}
            {/each}
          </div>
          <div class="tag-add">
            <input
              type="text"
              bind:value={newTag}
              onkeydown={handleTagKeydown}
              placeholder={t('card.tags_placeholder')}
              class="tag-input"
            />
          </div>
        </section>

        <!-- Checklist -->
        <section class="section">
          <h3 class="section-title">
            {t('card.checklist')}
            {#if card.checklist?.length > 0}
              <span class="checklist-progress">
                {card.checklist.filter((c: any) => c.done).length}/{card.checklist.length}
              </span>
            {/if}
          </h3>

          {#if card.checklist?.length > 0}
            <div class="checklist-bar">
              <div
                class="checklist-bar-fill"
                style="width: {(card.checklist.filter((c: any) => c.done).length / card.checklist.length) * 100}%"
              ></div>
            </div>
          {/if}

          <div class="checklist-items">
            {#each (card.checklist || []) as item}
              <div class="checklist-item" class:done={item.done}>
                <button class="checkbox" onclick={() => toggleChecklist(item.id)} title={t('tooltip.toggle_checklist')}>
                  {#if item.done}<CheckSquare size={16} />{:else}<Square size={16} />{/if}
                </button>
                <span class="checklist-text">{@html renderInline(item.text)}</span>
                <button class="checklist-remove" onclick={() => removeChecklist(item.id)} title={t('tooltip.remove_checklist_item')}><Trash2 size={12} /></button>
              </div>
            {/each}
          </div>

          <div class="checklist-add">
            <input
              type="text"
              bind:this={checklistInputEl}
              bind:value={newChecklistText}
              onkeydown={handleChecklistKeydown}
              oninput={handleChecklistInput}
              placeholder={t('card.checklist_placeholder')}
              class="checklist-input"
            />
            <button class="btn-add-sm" onclick={addChecklist}>{t('card.checklist_add')}</button>
          </div>
        </section>
      </div>

      <div class="modal-footer">
        <button class="btn-delete" onclick={handleDelete} title={t('tooltip.delete_card')}><Trash2 size={14} /> {t('card.delete')}</button>
        <span class="meta">Created {card.created_at?.slice(0, 10) || '—'}</span>
        <div class="footer-actions">
          <button class="btn-close" onclick={() => onClose()} title={t('tooltip.cancel')}>{t('common.cancel')}</button>
          <button class="btn-save" onclick={saveAndClose}><Save size={14} /> {t('card.save')}</button>
        </div>
      </div>
    {/if}
  </div>
</div>

<MentionPicker
  visible={mentionVisible}
  anchor={mentionAnchor}
  onSelect={handleMentionSelect}
  onClose={handleMentionClose}
/>

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: flex-start;
    justify-content: center;
    padding-top: 3rem;
    z-index: 100;
    overflow-y: auto;
  }

  .modal {
    background: var(--bg-surface);
    border-radius: 10px;
    width: 600px;
    max-width: 95vw;
    max-height: 85vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    box-shadow: 0 8px 32px var(--shadow-lg);
  }

  .modal-loading {
    padding: 2rem;
    text-align: center;
    color: var(--text-muted);
  }

  .modal-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
  }

  .type-badge {
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.15rem 0.5rem;
    border-radius: 3px;
    color: #fff;
    flex-shrink: 0;
  }

  .modal-title {
    margin: 0;
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text-primary);
    flex: 1;
    cursor: pointer;
    line-height: 1.3;
  }
  .modal-title:hover {
    color: var(--accent-light);
  }

  .title-input {
    flex: 1;
    font-size: 1.1rem;
    font-weight: 600;
    background: var(--bg-elevated);
    border: 1px solid var(--accent);
    border-radius: 4px;
    color: var(--text-primary);
    padding: 0.3rem 0.5rem;
    outline: none;
  }

  .close-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    font-size: 1.2rem;
    cursor: pointer;
    padding: 0.25rem;
    flex-shrink: 0;
  }
  .close-btn:hover { color: var(--text-primary); }

  .modal-body {
    flex: 1;
    overflow-y: auto;
    padding: 1rem 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
  }

  .section-title {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin: 0 0 0.5rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .desc-display {
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 6px;
    transition: background 0.1s;
  }
  .desc-display:hover { background: var(--bg-elevated); }
  .desc-display p { margin: 0; color: var(--text-body); font-size: 0.9rem; line-height: 1.5; }
  .desc-display .placeholder { white-space: pre-wrap; }
  .desc-display .placeholder { color: var(--text-faint); font-style: italic; }

  .desc-textarea {
    width: 100%;
    padding: 0.5rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.9rem;
    font-family: inherit;
    resize: vertical;
    outline: none;
    box-sizing: border-box;
  }
  .desc-textarea:focus { border-color: var(--accent); }

  .section-actions {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.4rem;
  }

  .date-input {
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    outline: none;
    color-scheme: dark light;
  }
  :global([data-theme="dark"]) .date-input { color-scheme: dark; }
  :global([data-theme="light"]) .date-input { color-scheme: light; }
  .date-input:focus { border-color: var(--accent); }

  .tags-list {
    display: flex;
    gap: 0.3rem;
    flex-wrap: wrap;
    margin-bottom: 0.4rem;
  }

  .tag-chip {
    font-size: 0.75rem;
    padding: 0.15rem 0.5rem;
    border-radius: 4px;
    color: #fff;
    display: flex;
    align-items: center;
    gap: 0.3rem;
  }

  .tag-label {
    white-space: nowrap;
  }

  .tag-color-btn {
    background: none;
    border: none;
    color: rgba(255, 255, 255, 0.6);
    cursor: pointer;
    padding: 0;
    line-height: 1;
    display: flex;
    align-items: center;
  }
  .tag-color-btn:hover { color: #fff; }

  .tag-remove {
    background: none;
    border: none;
    color: rgba(255, 255, 255, 0.6);
    cursor: pointer;
    font-size: 0.7rem;
    padding: 0;
    line-height: 1;
    display: flex;
    align-items: center;
  }
  .tag-remove:hover { color: var(--danger-light); }

  .color-picker {
    display: flex;
    gap: 0.25rem;
    flex-wrap: wrap;
    padding: 0.4rem;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    width: 100%;
  }

  .color-swatch {
    width: 22px;
    height: 22px;
    border-radius: 4px;
    border: 2px solid transparent;
    cursor: pointer;
    transition: border-color 0.1s, transform 0.1s;
  }
  .color-swatch:hover {
    transform: scale(1.15);
  }
  .color-swatch.active {
    border-color: #fff;
  }

  .tag-input {
    padding: 0.35rem 0.5rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.8rem;
    outline: none;
    width: 150px;
  }
  .tag-input:focus { border-color: var(--accent); }

  .checklist-progress {
    font-size: 0.7rem;
    color: var(--text-muted);
    font-weight: 400;
  }

  .checklist-bar {
    height: 4px;
    background: var(--border);
    border-radius: 2px;
    overflow: hidden;
    margin-bottom: 0.5rem;
  }
  .checklist-bar-fill {
    height: 100%;
    background: var(--success);
    border-radius: 2px;
    transition: width 0.2s;
  }

  .checklist-items {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .checklist-item {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.25rem 0;
  }

  .checkbox {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 1rem;
    padding: 0;
  }
  .checklist-item.done .checkbox { color: var(--success); }
  .checklist-item.done .checklist-text { text-decoration: line-through; color: var(--text-faint); }

  .checklist-text {
    flex: 1;
    font-size: 0.85rem;
    color: var(--text-body);
  }

  .checklist-remove {
    background: none;
    border: none;
    color: transparent;
    cursor: pointer;
    font-size: 0.75rem;
    padding: 0.15rem 0.3rem;
  }
  .checklist-item:hover .checklist-remove { color: var(--text-muted); }
  .checklist-remove:hover { color: var(--danger-light) !important; }

  .checklist-add {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.4rem;
  }

  .checklist-input {
    flex: 1;
    padding: 0.35rem 0.5rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.8rem;
    outline: none;
  }
  .checklist-input:focus { border-color: var(--accent); }

  .btn-save {
    padding: 0.3rem 0.7rem;
    border: none;
    border-radius: 4px;
    background: var(--accent);
    color: #fff;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .btn-save:hover { background: var(--accent-hover); }

  .btn-cancel-sm {
    padding: 0.3rem 0.7rem;
    border: none;
    border-radius: 4px;
    background: transparent;
    color: var(--text-muted);
    font-size: 0.8rem;
    cursor: pointer;
  }
  .btn-cancel-sm:hover { color: var(--text-primary); }

  .btn-add-sm {
    padding: 0.3rem 0.6rem;
    border: none;
    border-radius: 4px;
    background: var(--border);
    color: var(--text-body);
    font-size: 0.8rem;
    cursor: pointer;
  }
  .btn-add-sm:hover { background: var(--border-hover); }

  .modal-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.75rem 1.25rem;
    border-top: 1px solid var(--border-muted);
  }

  .footer-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .meta {
    font-size: 0.75rem;
    color: var(--text-faint);
  }

  .btn-close {
    padding: 0.3rem 0.7rem;
    border: none;
    border-radius: 4px;
    background: transparent;
    color: var(--text-muted);
    font-size: 0.8rem;
    cursor: pointer;
  }
  .btn-close:hover { color: var(--text-primary); }

  .btn-delete {
    padding: 0.3rem 0.7rem;
    border: none;
    border-radius: 4px;
    background: transparent;
    color: var(--text-muted);
    font-size: 0.8rem;
    cursor: pointer;
  }
  .btn-delete:hover { color: var(--danger-light); background: rgba(248, 113, 113, 0.1); }
</style>
