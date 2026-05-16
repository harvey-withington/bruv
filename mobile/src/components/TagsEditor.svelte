<script lang="ts">
  import { X, Plus } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { repoMeta } from '../lib/repoMeta.svelte'

  // Chip list with X-to-remove on each tag and an inline add input.
  // The parent owns the array; each mutation calls onChange with the
  // full new list, so the parent can persist via repoRPC and update
  // its local state from the server's authoritative response.
  //
  // While the user is typing a new tag, an autocomplete dropdown shows
  // matching known tags from `repoMeta.knownTags(projectKey)` — pulled
  // from the project's tag definitions plus the global colour map.
  // Tap a suggestion to commit it.

  let {
    tags,
    projectKey,
    onChange,
  }: {
    tags: string[]
    projectKey?: string
    onChange: (next: string[]) => void | Promise<void>
  } = $props()

  let adding = $state(false)
  let draft = $state('')
  let inputEl: HTMLInputElement | undefined = $state()
  let suggestionIdx = $state(-1)
  // True for the brief window between blurring the input and applying
  // the click on a suggestion. Without this, blur fires first, the
  // dropdown unmounts, and the click never lands. Acts as a debounce.
  let suppressBlurCommit = $state(false)

  // Show every known tag (minus the ones already on this card) as soon
  // as the input opens, so a tap on "+ Add tag" reveals the picker
  // without the user having to type first. Typing narrows the list.
  // Matches the desktop CardDetail behaviour.
  const suggestions = $derived.by(() => {
    const q = draft.trim().toLowerCase()
    const all = repoMeta.knownTags(projectKey)
    return all.filter(
      (name) => !tags.includes(name) && (!q || name.toLowerCase().includes(q)),
    )
  })

  function startAdd() {
    draft = ''
    suggestionIdx = -1
    adding = true
    queueMicrotask(() => inputEl?.focus())
  }

  function commitAdd(value?: string) {
    const tag = (value ?? draft).trim()
    adding = false
    suggestionIdx = -1
    if (!tag) return
    if (tags.includes(tag)) return
    onChange([...tags, tag])
  }

  function pickSuggestion(name: string) {
    suppressBlurCommit = true
    commitAdd(name)
    // Reset shortly after so the next add cycle works normally.
    queueMicrotask(() => { suppressBlurCommit = false })
  }

  function cancelAdd() {
    adding = false
    draft = ''
    suggestionIdx = -1
  }

  function remove(tag: string) {
    onChange(tags.filter((existing) => existing !== tag))
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault()
      // Enter with a highlighted suggestion picks it; otherwise commit
      // the literal draft.
      if (suggestionIdx >= 0 && suggestionIdx < suggestions.length) {
        commitAdd(suggestions[suggestionIdx])
      } else {
        commitAdd()
      }
    } else if (e.key === 'Escape') {
      e.preventDefault()
      cancelAdd()
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (suggestions.length === 0) return
      suggestionIdx = (suggestionIdx + 1) % suggestions.length
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (suggestions.length === 0) return
      suggestionIdx = suggestionIdx <= 0 ? suggestions.length - 1 : suggestionIdx - 1
    }
  }

  function onBlur() {
    if (suppressBlurCommit) return
    commitAdd()
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
    <div class="add-wrap">
      <input
        bind:this={inputEl}
        bind:value={draft}
        oninput={() => (suggestionIdx = -1)}
        onblur={onBlur}
        onkeydown={onKey}
        placeholder={t('tags.placeholder')}
        class="chip-input"
        autocomplete="off"
        autocapitalize="off"
        spellcheck="false"
      />
      {#if suggestions.length > 0}
        <ul class="suggest" role="listbox">
          {#each suggestions as name, i (name)}
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <li
              class="suggest-item"
              class:active={suggestionIdx === i}
              role="option"
              aria-selected={suggestionIdx === i}
              onpointerdown={(e) => { e.preventDefault(); pickSuggestion(name) }}
            >
              <span class="dot" style:background={repoMeta.tagColor(name, projectKey)}></span>
              <span class="name">{name}</span>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
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
    position: relative;
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

  .add-wrap {
    position: relative;
    display: inline-block;
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
    min-width: 6rem;
  }

  .suggest {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    z-index: 20;
    list-style: none;
    margin: 0;
    padding: 0.25rem;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 8px 20px rgba(0, 0, 0, 0.35);
    min-width: 12rem;
    max-height: 14rem;
    overflow-y: auto;
  }
  .suggest-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.45rem 0.55rem;
    border-radius: 6px;
    cursor: pointer;
    color: var(--text);
    font-size: 0.85rem;
  }
  .suggest-item.active,
  .suggest-item:hover {
    background: var(--bg-elev-1);
  }
  .dot {
    width: 12px;
    height: 12px;
    border-radius: 3px;
    flex-shrink: 0;
  }
  .name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
