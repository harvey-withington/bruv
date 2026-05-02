<script lang="ts">
  import { renderMarkdown, renderInline } from '@shared/markdown'
  import { t } from '../lib/i18n.svelte'

  let {
    value = '',
    placeholder = t('tooltip.click_to_edit'),
    multiline = false,
    markdown = false,
    inlineMarkdown = false,
    rows = 4,
    class: className = '',
    onSave,
    onCancel,
    onTab,
  }: {
    value?: string
    placeholder?: string
    multiline?: boolean
    markdown?: boolean
    inlineMarkdown?: boolean
    rows?: number
    class?: string
    onSave?: (value: string) => void
    onCancel?: () => void
    onTab?: () => void
  } = $props()

  let editing = $state(false)
  let draft = $state('')
  let inputEl = $state<HTMLInputElement | HTMLTextAreaElement | null>(null)

  $effect(() => { if (!editing) draft = value })

  $effect(() => {
    if (editing && inputEl) {
      inputEl.focus()
      if ('select' in inputEl) inputEl.select()
    }
  })

  function startEdit() {
    draft = value
    editing = true
  }

  function save() {
    if (!editing) return
    editing = false
    const trimmed = draft.trim()
    if (trimmed !== value) {
      onSave?.(trimmed)
    }
  }

  function cancel() {
    editing = false
    draft = value
    onCancel?.()
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.stopPropagation()
      cancel()
    } else if (e.key === 'Enter' && !multiline) {
      e.preventDefault()
      save()
    } else if (e.key === 'Tab' && !multiline) {
      e.preventDefault()
      save()
      onTab?.()
    }
  }

  export function startEditing() { startEdit() }
  export function isEditing() { return editing }
</script>

{#if editing}
  {#if multiline}
    <textarea
      class="inline-edit-input {className}"
      bind:this={inputEl}
      bind:value={draft}
      onkeydown={handleKeydown}
      onblur={save}
      {rows}
    ></textarea>
  {:else}
    <input
      class="inline-edit-input {className}"
      bind:this={inputEl}
      bind:value={draft}
      onkeydown={handleKeydown}
      onblur={save}
    />
  {/if}
{:else}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <span
    class="editable-display {className}"
    role="button"
    tabindex="0"
    onclick={(e) => { if ((e.target as HTMLElement).closest('a')) return; startEdit() }}
    onfocus={startEdit}
    onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); startEdit() } }}
    title={placeholder}
  >
    {#if value}
      {#if markdown}
        <div class="markdown-content">{@html renderMarkdown(value)}</div>
      {:else if inlineMarkdown}
        {@html renderInline(value)}
      {:else}
        {value}
      {/if}
    {:else}
      <span class="editable-placeholder">{placeholder}</span>
    {/if}
  </span>
{/if}

<style>
  .editable-placeholder {
    color: var(--text-muted);
    font-style: italic;
  }
</style>
