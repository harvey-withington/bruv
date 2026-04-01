<script lang="ts">
  import { Square, CheckSquare, Trash2 } from 'lucide-svelte'
  import EditableText from './EditableText.svelte'
  import { t } from '../lib/i18n.svelte'

  type ChecklistItem = { id: string; text: string; done: boolean }

  let {
    items = [],
    placeholder = '',
    onUpdate,
  }: {
    items?: ChecklistItem[]
    placeholder?: string
    onUpdate?: (items: ChecklistItem[]) => void
  } = $props()

  let newText = $state('')

  function emit(updated: ChecklistItem[]) {
    onUpdate?.(updated)
  }

  function toggleItem(id: string) {
    emit(items.map(item => item.id === id ? { ...item, done: !item.done } : item))
  }

  function removeItem(id: string) {
    emit(items.filter(item => item.id !== id))
  }

  function addItem() {
    const text = newText.trim()
    if (!text) return
    const id = `ck-${crypto.randomUUID().slice(0, 8)}`
    emit([...items, { id, text, done: false }])
    newText = ''
  }

  function saveItemText(id: string, text: string) {
    if (!text) return
    emit(items.map(item => item.id === id ? { ...item, text } : item))
  }

  function focusDeleteButton(itemId: string) {
    setTimeout(() => {
      const row = document.querySelector(`.cl-item[data-item-id="${itemId}"]`)
      const removeBtn = row?.querySelector('.cl-remove') as HTMLElement
      removeBtn?.focus()
    }, 0)
  }

  function handleAddKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') addItem()
    else if (e.key === 'Escape') { e.stopPropagation(); (e.target as HTMLElement)?.blur() }
  }
</script>

{#if items.length > 0}
  <div class="cl-bar">
    <div
      class="cl-bar-fill"
      style="width: {(items.filter(c => c.done).length / items.length) * 100}%"
    ></div>
  </div>
{/if}

<div class="cl-items">
  {#each items as item (item.id)}
    <div class="cl-item action-reveal-parent" class:done={item.done} data-item-id={item.id}>
      <button class="cl-checkbox" onclick={() => toggleItem(item.id)} title={t('tooltip.toggle_checklist')}>
        {#if item.done}<CheckSquare size={16} />{:else}<Square size={16} />{/if}
      </button>
      <EditableText
        value={item.text}
        inlineMarkdown
        class="cl-text"
        onSave={(text) => saveItemText(item.id, text)}
        onTab={() => focusDeleteButton(item.id)}
      />
      <button class="action-reveal action-reveal--danger cl-remove" onclick={() => removeItem(item.id)} title={t('tooltip.remove_checklist_item')}><Trash2 size={12} /></button>
    </div>
  {/each}
</div>

<div class="cl-add">
  <input
    type="text"
    bind:value={newText}
    onkeydown={handleAddKeydown}
    placeholder={placeholder || t('card.checklist_placeholder')}
    class="cl-add-input"
  />
  <button class="cl-add-btn" onclick={addItem}>{t('card.checklist_add')}</button>
</div>

<style>
  .cl-bar {
    height: 4px;
    background: var(--bg-elevated);
    border-radius: 2px;
    margin-bottom: 0.4rem;
    overflow: hidden;
  }
  .cl-bar-fill {
    height: 100%;
    background: var(--success);
    border-radius: 2px;
    transition: width 0.2s;
  }

  .cl-items {
    display: flex;
    flex-direction: column;
  }

  .cl-item {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 2px 0;
    border-radius: 4px;
  }
  .cl-item.done .cl-checkbox { color: var(--success); }
  .cl-item.done :global(.cl-text) { text-decoration: line-through; color: var(--text-faint); }

  .cl-checkbox {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 1rem;
    padding: 0;
  }

  :global(.cl-text) {
    flex: 1;
    font-size: 0.85rem;
    color: var(--text-body);
  }

  .cl-remove {
    font-size: 0.75rem;
  }

  .cl-add {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.4rem;
  }

  .cl-add-input {
    flex: 1;
    padding: 0.3rem 0.5rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    font-family: inherit;
    outline: none;
  }
  .cl-add-input:focus { border-color: var(--accent); }

  .cl-add-btn {
    padding: 0.3rem 0.6rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-secondary);
    font-size: 0.8rem;
    cursor: pointer;
    white-space: nowrap;
  }
  .cl-add-btn:hover {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
  }
</style>
