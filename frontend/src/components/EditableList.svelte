<script lang="ts">
  import { Trash2 } from 'lucide-svelte'
  import EditableText from './EditableText.svelte'
  import { t } from '../lib/i18n.svelte'

  type ListItem = { id: string; text: string }

  let {
    items = [],
    placeholder = '',
    onUpdate,
  }: {
    items?: ListItem[]
    placeholder?: string
    onUpdate?: (items: ListItem[]) => void
  } = $props()

  let newText = $state('')

  function emit(updated: ListItem[]) {
    onUpdate?.(updated)
  }

  function removeItem(id: string) {
    emit(items.filter(item => item.id !== id))
  }

  function addItem() {
    const text = newText.trim()
    if (!text) return
    const id = `li-${crypto.randomUUID().slice(0, 8)}`
    emit([...items, { id, text }])
    newText = ''
  }

  function saveItemText(id: string, text: string) {
    if (!text) return
    emit(items.map(item => item.id === id ? { ...item, text } : item))
  }

  function focusDeleteButton(itemId: string) {
    setTimeout(() => {
      const row = document.querySelector(`.li-item[data-item-id="${itemId}"]`)
      const removeBtn = row?.querySelector('.li-remove') as HTMLElement
      removeBtn?.focus()
    }, 0)
  }

  function handleAddKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') addItem()
    else if (e.key === 'Escape') { e.stopPropagation(); (e.target as HTMLElement)?.blur() }
  }
</script>

<div class="li-items">
  {#each items as item (item.id)}
    <div class="li-item action-reveal-parent" data-item-id={item.id}>
      <span class="li-bullet">&#8226;</span>
      <EditableText
        value={item.text}
        inlineMarkdown
        class="li-text"
        onSave={(text) => saveItemText(item.id, text)}
        onTab={() => focusDeleteButton(item.id)}
      />
      <button class="action-reveal action-reveal--danger li-remove" onclick={() => removeItem(item.id)} title={t('tooltip.remove_checklist_item')}><Trash2 size={12} /></button>
    </div>
  {/each}
</div>

<div class="li-add">
  <input
    type="text"
    bind:value={newText}
    onkeydown={handleAddKeydown}
    placeholder={placeholder || t('block.list_placeholder')}
    class="li-add-input"
  />
  <button class="li-add-btn" onclick={addItem}>{t('block.list_add')}</button>
</div>

<style>
  .li-items {
    display: flex;
    flex-direction: column;
  }

  .li-item {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 2px 0;
    border-radius: 4px;
  }

  .li-bullet {
    color: var(--text-muted);
    font-size: 1rem;
    line-height: 1;
    flex-shrink: 0;
    width: 16px;
    text-align: center;
  }

  :global(.li-text) {
    flex: 1;
    font-size: 0.85rem;
    color: var(--text-body);
  }

  .li-remove {
    font-size: 0.75rem;
  }

  .li-add {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.4rem;
  }

  .li-add-input {
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
  .li-add-input:focus { border-color: var(--accent); }

  .li-add-btn {
    padding: 0.3rem 0.6rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-secondary);
    font-size: 0.8rem;
    cursor: pointer;
    white-space: nowrap;
  }
  .li-add-btn:hover {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
  }
</style>
