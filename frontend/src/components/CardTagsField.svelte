<script lang="ts">
  import { UpdateCardTags, GetProjectLabels, AddProjectLabel } from '@shared/api'
  import { X } from 'lucide-svelte'
  import DynamicIcon from './DynamicIcon.svelte'
  import { projectTags, nav, getTagColor, getTagIcon } from '../lib/store.svelte'
  import { getContext, onDestroy } from 'svelte'
  import { EDIT_SCOPE_KEY, type EditScope } from '@shared/editScope'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { floatingDropdown } from '../lib/actions'
  import type { Card, CardPin } from '@shared/types'

  // The card's tag chips + combined input/autocomplete picker. Owns
  // all tag mutation logic, including syncing new tags into the tag
  // definitions of every project the card is pinned to. The parent
  // provides `track` (save-indicator wrapper) and receives the updated
  // card via onCardUpdated.

  let { card, cardId, pinBreadcrumbs, track, onCardUpdated }: {
    card: Card
    cardId: string
    pinBreadcrumbs: CardPin[]
    track: <T>(promise: Promise<T>) => Promise<T>
    onCardUpdated: (card: Card) => void
  } = $props()

  let newTag = $state('')

  /** Ensure a tag exists in a project's tag definitions */
  async function ensureTagInProject(tagName: string, brandSlug: string, streamSlug: string, projectSlug: string) {
    try {
      const existing = await GetProjectLabels(brandSlug, streamSlug, projectSlug) || []
      if (!existing.some((t: { name: string }) => t.name.toLowerCase() === tagName.toLowerCase())) {
        await AddProjectLabel(brandSlug, streamSlug, projectSlug, tagName, '')
      }
    } catch { /* best-effort */ }
  }

  /** Sync a tag to the current project and all projects this card is pinned to */
  async function syncTagToProjects(tagName: string) {
    // Current project
    if (nav.brandSlug && nav.streamSlug && nav.projectSlug) {
      await ensureTagInProject(tagName, nav.brandSlug, nav.streamSlug, nav.projectSlug)
    }
    // All pinned projects
    for (const pin of pinBreadcrumbs) {
      if (pin.brandSlug === nav.brandSlug && pin.streamSlug === nav.streamSlug && pin.projectSlug === nav.projectSlug) continue
      await ensureTagInProject(tagName, pin.brandSlug, pin.streamSlug, pin.projectSlug)
    }
    // Refresh current project tags
    if (nav.brandSlug && nav.streamSlug && nav.projectSlug) {
      try { projectTags.list = await GetProjectLabels(nav.brandSlug, nav.streamSlug, nav.projectSlug) || [] } catch {}
    }
  }

  async function addTag() {
    await addTagBatch(newTag)
  }

  async function addTagBatch(rawInput: string) {
    const segments = rawInput.split(',').map(s => s.trim()).filter(Boolean)
    if (segments.length === 0) { newTag = ''; return }
    const existingLower = new Set((card.tags || []).map((t: string) => t.toLowerCase()))
    const incoming = segments.filter(s => !existingLower.has(s.toLowerCase()))
    if (incoming.length === 0) { newTag = ''; return }
    const tags = [...(card.tags || []), ...incoming]
    try {
      const updated = await track(UpdateCardTags(cardId, tags))
      newTag = ''
      // Reflect the change immediately — the per-project tag sync below
      // is a series of best-effort RPCs and shouldn't delay the chips.
      onCardUpdated(updated)
      for (const tag of incoming) await syncTagToProjects(tag)
    } catch (e) { showToast(t('error.tag_failed'), 'error') }
  }

  let suppressPickerUntil = 0

  async function removeTag(tag: string) {
    suppressPickerUntil = Date.now() + 200
    const tags = (card.tags || []).filter((t: string) => t !== tag)
    try {
      const updated = await track(UpdateCardTags(cardId, tags))
      onCardUpdated(updated)
    } catch (e) { showToast(t('error.tag_failed'), 'error') }
  }

  // Combined tag input + picker
  let showTagPicker = $state(false)
  let tagInputEl = $state<HTMLInputElement | null>(null)
  let highlightIdx = $state(-1)

  // Edit-scope registration (keyboard entry contract). The input is a
  // serial-style entry: registered while focused, so an idle empty tag
  // input never blocks Escape-closing the card. Hand-rolled rather than
  // the inlineEdit action because Enter here is overloaded by the
  // suggestion picker (highlight selection vs literal add).
  const editScope = getContext<EditScope | undefined>(EDIT_SCOPE_KEY) ?? null
  let unregisterEdit: (() => void) | null = null

  function registerEdit() {
    if (!editScope || unregisterEdit) return
    unregisterEdit = editScope.register({ commit: () => { void addTag() }, cancel: cancelTagEntry })
  }
  function deregisterEdit() {
    unregisterEdit?.()
    unregisterEdit = null
  }
  onDestroy(deregisterEdit)

  function cancelTagEntry() {
    newTag = ''
    showTagPicker = false
    highlightIdx = -1
    tagInputEl?.blur()
  }

  let filteredProjectTags = $derived(
    projectTags.list.filter(t =>
      !isProjectTagAssigned(t.name) &&
      (!newTag.trim() || t.name.toLowerCase().includes(newTag.trim().toLowerCase()))
    )
  )

  // Reset highlight when filter text changes
  $effect(() => { newTag; highlightIdx = -1 })

  function isProjectTagAssigned(tagName: string): boolean {
    return (card.tags || []).some((t: string) => t.toLowerCase() === tagName.toLowerCase())
  }

  async function toggleProjectTag(tagName: string) {
    const current = card.tags || []
    const isAssigned = current.some((t: string) => t.toLowerCase() === tagName.toLowerCase())
    const tags = isAssigned
      ? current.filter((t: string) => t.toLowerCase() !== tagName.toLowerCase())
      : [...current, tagName]
    try {
      const updated = await track(UpdateCardTags(cardId, tags))
      onCardUpdated(updated)
      if (!isAssigned) await syncTagToProjects(tagName)
    } catch (e) { showToast(t('error.tag_failed'), 'error') }
  }

  async function handleTagKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (filteredProjectTags.length > 0) {
        highlightIdx = Math.min(highlightIdx + 1, filteredProjectTags.length - 1)
      }
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      highlightIdx = Math.max(highlightIdx - 1, -1)
    } else if (e.key === 'Tab' && showTagPicker && filteredProjectTags.length > 0) {
      e.preventDefault()
      if (e.shiftKey) {
        highlightIdx = Math.max(highlightIdx - 1, 0)
      } else {
        highlightIdx = Math.min(highlightIdx + 1, filteredProjectTags.length - 1)
      }
    } else if (e.key === ',') {
      // Comma immediately commits whatever is typed before it as a tag
      const before = newTag.split(',')[0]?.trim()
      if (before) {
        e.preventDefault()
        await addTagBatch(before)
      }
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (e.ctrlKey || e.metaKey) {
        // Contract: Ctrl+Enter commits and closes the card.
        e.stopPropagation()
        await addTag()
        editScope?.requestClose?.()
        return
      }
      if (highlightIdx >= 0 && highlightIdx < filteredProjectTags.length) {
        toggleProjectTag(filteredProjectTags[highlightIdx].name)
        newTag = ''
        highlightIdx = -1
      } else {
        addTag()
      }
    } else if (e.key === 'Escape') {
      // Layered per the contract: first Escape closes the suggestion
      // picker (a picker closes unchosen), the next cancels the tag
      // entry itself. Only after that does Escape reach the card.
      e.preventDefault()
      e.stopPropagation()
      if (showTagPicker) {
        showTagPicker = false
        highlightIdx = -1
      } else {
        cancelTagEntry()
      }
    }
  }

  function handleTagInputFocus() {
    registerEdit()
    if (Date.now() < suppressPickerUntil) return
    showTagPicker = true
    highlightIdx = -1
  }

  function handleTagInputBlur(e: FocusEvent) {
    // Keep picker open if focus moves to picker items
    const related = e.relatedTarget as HTMLElement | null
    if (related?.closest('.tag-picker-dropdown')) return
    // Contract: blur keeps the typed draft uncommitted; the field just
    // stops counting as an active edit.
    deregisterEdit()
    // Small delay so click events on picker items fire first
    setTimeout(() => { showTagPicker = false; highlightIdx = -1 }, 150)
  }
</script>

<div class="tags-list">
  {#each (card.tags || []) as tag}
    {@const icon = getTagIcon(tag)}
    <span class="tag-chip" style:background={getTagColor(tag)}>
      {#if icon}
        <span class="tag-chip-icon"><DynamicIcon name={icon} size={11} /></span>
      {/if}
      <span class="tag-label">{tag}</span>
      <button class="tag-remove" onclick={() => removeTag(tag)} title={t('tooltip.remove_tag')}><X size={12} /></button>
    </span>
  {/each}
  <input
    type="text"
    bind:this={tagInputEl}
    bind:value={newTag}
    onkeydown={handleTagKeydown}
    onfocus={handleTagInputFocus}
    onblur={handleTagInputBlur}
    placeholder={t('card.tags_placeholder')}
    class="tag-input tag-input-inline"
  />
</div>

{#if showTagPicker && tagInputEl && filteredProjectTags.length > 0}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="tag-picker-dropdown" role="listbox" use:floatingDropdown={{ trigger: tagInputEl }}>
    {#each filteredProjectTags as ptag, i (ptag.id)}
      <button
        class="tag-picker-item"
        class:highlighted={i === highlightIdx}
        tabindex="-1"
        onclick={() => { toggleProjectTag(ptag.name); newTag = ''; tagInputEl?.focus() }}
      >
        <span class="tag-picker-chip" style:background={ptag.color || 'var(--border)'}>{ptag.name}</span>
      </button>
    {/each}
    {#if newTag.trim() && !projectTags.list.some(t => t.name.toLowerCase() === newTag.trim().toLowerCase())}
      <div class="tag-picker-create">
        {t('card.tag_create_hint', { name: newTag.trim() })}
      </div>
    {/if}
  </div>
{:else if showTagPicker && tagInputEl && newTag.trim() && !projectTags.list.some(t => t.name.toLowerCase() === newTag.trim().toLowerCase())}
  <div class="tag-picker-dropdown" use:floatingDropdown={{ trigger: tagInputEl }}>
    <div class="tag-picker-create">
      {t('card.tag_create_hint', { name: newTag.trim() })}
    </div>
  </div>
{/if}

<style>
  .tags-list {
    display: flex;
    gap: 0.3rem;
    flex-wrap: wrap;
    align-items: center;
  }

  .tag-chip {
    font-size: 0.75rem;
    padding: 0.15rem 0.5rem;
    border-radius: 4px;
    color: var(--on-color);
    display: flex;
    align-items: center;
    gap: 0.3rem;
  }

  .tag-chip-icon {
    display: inline-flex;
    align-items: center;
    flex-shrink: 0;
  }

  .tag-label {
    white-space: nowrap;
  }

  .tag-remove {
    background: none;
    border: none;
    color: var(--on-color-muted);
    cursor: pointer;
    font-size: 0.7rem;
    padding: 0;
    line-height: 1;
    display: flex;
    align-items: center;
  }
  .tag-remove:hover { color: var(--danger-light); }

  .tag-input {
    padding: 0.25rem 0.4rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.75rem;
    outline: none;
  }
  .tag-input:focus { border-color: var(--accent); }
  .tag-input-inline {
    width: 100px;
    flex-shrink: 1;
  }

  /* :global because floatingDropdown re-parents the dropdown out of
     this component's scope boundary. */
  :global(.tag-picker-dropdown) {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.3rem;
    box-shadow: 0 8px 32px var(--shadow-lg);
    min-width: 180px;
    max-height: 220px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  :global(.tag-picker-item) {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    cursor: pointer;
    padding: 0.25rem 0.35rem;
    border-radius: 4px;
    font-size: 0.8rem;
    background: none;
    border: none;
    color: var(--text-primary);
    width: 100%;
    text-align: left;
  }
  :global(.tag-picker-item:hover),
  :global(.tag-picker-item.highlighted) {
    background: var(--bg-surface);
  }

  :global(.tag-picker-chip) {
    font-size: 0.7rem;
    font-weight: 600;
    padding: 0.1rem 0.4rem;
    border-radius: 3px;
    color: var(--on-color);
    line-height: 1.4;
  }

  :global(.tag-picker-create) {
    font-size: 0.75rem;
    color: var(--text-muted);
    padding: 0.3rem 0.35rem;
    font-style: italic;
  }
</style>
