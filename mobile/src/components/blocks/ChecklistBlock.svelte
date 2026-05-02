<script lang="ts">
  import { Square, CheckSquare, X, Plus } from 'lucide-svelte'
  import { t } from '../../lib/i18n.svelte'
  import type { Block, ChecklistItem } from '@shared/types'
  import { asChecklist, withValue, newID } from './narrow'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  const items = $derived(asChecklist(block.value))

  // Local drafts so typing doesn't fight upstream re-renders. Keyed
  // by item.id so reorder/delete don't shift drafts onto wrong rows.
  let drafts = $state<Record<string, string>>({})

  function commitItems(next: ChecklistItem[]) {
    onChange(withValue(block, next))
  }

  function toggle(id: string) {
    commitItems(items.map((it) => (it.id === id ? { ...it, done: !it.done } : it)))
  }

  function deleteItem(id: string) {
    commitItems(items.filter((it) => it.id !== id))
    delete drafts[id]
  }

  function commitText(id: string, next: string) {
    const cur = items.find((it) => it.id === id)
    if (!cur) return
    if (cur.text === next) return
    commitItems(items.map((it) => (it.id === id ? { ...it, text: next } : it)))
  }

  function addItem() {
    const id = newID()
    commitItems([...items, { id, text: '', done: false }])
    // Defer focus until the new <input> mounts.
    queueMicrotask(() => {
      const el = document.querySelector<HTMLInputElement>(
        `[data-checklist-input="${id}"]`,
      )
      el?.focus()
    })
  }
</script>

<ul class="list">
  {#each items as item (item.id)}
    <li class="row" class:done={item.done}>
      <button
        type="button"
        class="check"
        onclick={() => toggle(item.id)}
        aria-pressed={item.done}
        aria-label={t('block.checkbox.toggle')}
      >
        {#if item.done}
          <CheckSquare size={20} />
        {:else}
          <Square size={20} />
        {/if}
      </button>
      <input
        class="text"
        type="text"
        data-checklist-input={item.id}
        value={drafts[item.id] ?? item.text}
        oninput={(e) => (drafts[item.id] = (e.currentTarget as HTMLInputElement).value)}
        onblur={(e) => {
          const v = (e.currentTarget as HTMLInputElement).value
          delete drafts[item.id]
          commitText(item.id, v)
        }}
        onkeydown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            ;(e.currentTarget as HTMLInputElement).blur()
            addItem()
          }
        }}
      />
      <button
        type="button"
        class="del"
        onclick={() => deleteItem(item.id)}
        aria-label={t('block.checklist.delete')}
      >
        <X size={14} />
      </button>
    </li>
  {/each}
  <li class="row add-row">
    <button type="button" class="add" onclick={addItem}>
      <Plus size={14} />
      <span>{t('block.checklist.add')}</span>
    </button>
  </li>
</ul>

<style>
  .list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.2rem 0;
  }
  .row.done .text {
    text-decoration: line-through;
    color: var(--text-muted);
  }
  .check {
    background: transparent;
    border: none;
    color: var(--text-muted);
    padding: 0.4rem;
    cursor: pointer;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 36px;
    min-height: 36px;
    flex-shrink: 0;
  }
  .check:hover,
  .check:focus-visible {
    color: var(--accent);
    background: var(--bg-elev-1);
    outline: none;
  }
  .row.done .check {
    color: var(--accent);
  }
  .text {
    flex: 1;
    min-width: 0;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    padding: 0.4rem 0.5rem;
  }
  .text:hover {
    border-color: var(--border);
  }
  .text:focus {
    outline: none;
    border-color: var(--accent);
    background: var(--bg-elev-1);
  }
  .del {
    background: transparent;
    border: none;
    color: var(--text-faint);
    padding: 0.45rem;
    cursor: pointer;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 36px;
    min-height: 36px;
    flex-shrink: 0;
  }
  .del:hover,
  .del:focus-visible {
    color: #ef4444;
    background: rgba(239, 68, 68, 0.1);
    outline: none;
  }
  .add-row {
    margin-top: 0.2rem;
  }
  .add {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    background: transparent;
    border: 1px dashed var(--border);
    color: var(--text-muted);
    font: inherit;
    font-size: 0.85rem;
    padding: 0.4rem 0.7rem;
    border-radius: 6px;
    cursor: pointer;
  }
  .add:hover,
  .add:focus-visible {
    color: var(--text);
    border-color: var(--text-muted);
    outline: none;
  }
</style>
