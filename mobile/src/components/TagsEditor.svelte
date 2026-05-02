<script lang="ts">
  import { X, Plus } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { repoMeta } from '../lib/repoMeta.svelte'

  // Chip list with X-to-remove on each tag and an inline add input.
  // The parent owns the array; each mutation calls onChange with the
  // full new list, so the parent can persist via repoRPC and update
  // its local state from the server's authoritative response.

  let {
    tags,
    projectKey,
    onChange,
  }: {
    tags: string[]
    /** Optional brand/stream/project key — when set, project-defined tag
     *  colours win over the global map. Leave undefined for orphaned
     *  cards (Inbox) or anywhere project context isn't known. */
    projectKey?: string
    onChange: (next: string[]) => void | Promise<void>
  } = $props()

  let adding = $state(false)
  let draft = $state('')
  let inputEl: HTMLInputElement | undefined = $state()

  function startAdd() {
    draft = ''
    adding = true
    queueMicrotask(() => inputEl?.focus())
  }

  function commitAdd() {
    const tag = draft.trim()
    adding = false
    if (!tag) return
    if (tags.includes(tag)) return
    onChange([...tags, tag])
  }

  function cancelAdd() {
    adding = false
    draft = ''
  }

  function remove(tag: string) {
    onChange(tags.filter((existing) => existing !== tag))
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault()
      commitAdd()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      cancelAdd()
    }
  }
</script>

<div class="tags">
  {#each tags as tag (tag)}
    <span class="chip coloured" style:background={repoMeta.tagColor(tag, projectKey)}>
      <span class="chip-text">{tag}</span>
      <button type="button" class="chip-remove" onclick={() => remove(tag)} aria-label={`Remove ${tag}`}>
        <X size={10} />
      </button>
    </span>
  {/each}

  {#if adding}
    <input
      bind:this={inputEl}
      bind:value={draft}
      onblur={commitAdd}
      onkeydown={onKey}
      placeholder={t('tags.placeholder')}
      class="chip-input"
    />
  {:else}
    <button type="button" class="chip add" onclick={startAdd} aria-label={t('tags.add')}>
      <Plus size={12} /> {t('tags.add')}
    </button>
  {/if}
</div>

<style>
  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    align-items: center;
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.2rem 0.5rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 4px;
    font-size: 0.75rem;
    color: var(--text);
  }

  /* Coloured (existing tag) chips: vivid background + white text for
     contrast against typical tag colours. The X stays white-ish too. */
  .chip.coloured {
    color: #fff;
    border-color: transparent;
  }

  .chip-text {
    line-height: 1;
  }

  .chip-remove {
    background: transparent;
    border: none;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0;
    line-height: 1;
    display: inline-flex;
  }

  .chip.coloured .chip-remove {
    color: rgba(255, 255, 255, 0.65);
  }

  .chip-remove:hover,
  .chip-remove:focus-visible {
    color: var(--text);
    outline: none;
  }

  .chip.coloured .chip-remove:hover,
  .chip.coloured .chip-remove:focus-visible {
    color: #fff;
  }

  .add {
    background: transparent;
    border-style: dashed;
    color: var(--text-muted);
    cursor: pointer;
    font: inherit;
    font-size: 0.75rem;
  }

  .add:hover,
  .add:focus-visible {
    color: var(--text);
    border-color: var(--text-muted);
    outline: none;
  }

  .chip-input {
    background: var(--bg);
    border: 1px solid var(--accent);
    border-radius: 4px;
    padding: 0.2rem 0.45rem;
    font: inherit;
    font-size: 0.75rem;
    color: var(--text);
    outline: none;
    min-width: 5rem;
  }
</style>
