<script lang="ts">
  import { tick } from 'svelte'
  import { t } from '../../lib/i18n.svelte'
  import type { Block } from '@shared/types'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  // svelte-ignore state_referenced_locally
  let editing = $state(false)
  // svelte-ignore state_referenced_locally
  let draft = $state(block.label ?? '')
  let inputEl: HTMLInputElement | null = $state(null)

  async function startEdit() {
    editing = true
    await tick()
    inputEl?.focus()
  }

  function commit() {
    editing = false
    if (draft === block.label) return
    onChange({ ...block, label: draft })
  }
</script>

{#if editing}
  <input
    bind:this={inputEl}
    class="edit"
    type="text"
    value={draft}
    placeholder={t('block.divider.label_placeholder')}
    oninput={(e) => (draft = (e.currentTarget as HTMLInputElement).value)}
    onblur={commit}
    onkeydown={(e) => {
      if (e.key === 'Enter') {
        e.preventDefault()
        ;(e.currentTarget as HTMLInputElement).blur()
      }
      if (e.key === 'Escape') {
        draft = block.label ?? ''
        editing = false
      }
    }}
  />
{:else if block.label}
  <button type="button" class="div-with-label" onclick={startEdit}>
    <span>{block.label}</span>
  </button>
{:else}
  <button
    type="button"
    class="div-bare"
    onclick={startEdit}
    aria-label={t('block.divider.label_placeholder')}
  >
    <hr class="rule" />
  </button>
{/if}

<style>
  .edit {
    width: 100%;
    background: var(--bg-elev-1);
    border: 1px dashed var(--border);
    border-radius: 6px;
    color: var(--text);
    font: inherit;
    font-size: 0.85rem;
    padding: 0.4rem 0.6rem;
    margin: 0.5rem 0;
  }
  .edit:focus {
    outline: none;
    border-color: var(--accent);
  }
  .div-with-label {
    display: flex;
    align-items: center;
    gap: 0.65rem;
    margin: 0.75rem 0;
    color: var(--text-muted);
    font-size: 0.8rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    background: transparent;
    border: none;
    cursor: pointer;
    width: 100%;
  }
  .div-with-label::before,
  .div-with-label::after {
    content: '';
    flex: 1;
    border-top: 1px solid var(--border);
  }
  .div-bare {
    background: transparent;
    border: none;
    padding: 0;
    width: 100%;
    cursor: pointer;
  }
  .rule {
    border: none;
    border-top: 1px solid var(--border);
    margin: 0.75rem 0;
  }
</style>
