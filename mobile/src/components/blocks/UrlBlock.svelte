<script lang="ts">
  import { untrack, getContext } from 'svelte'
  import { t } from '../../lib/i18n.svelte'
  import { EDIT_SCOPE_KEY, type EditScope } from '@shared/editScope'
  import type { Block } from '@shared/types'
  import { asUrlValue, withValue } from './narrow'
  import { draftEdit } from '../../lib/actions/draftEdit'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  const value = $derived(asUrlValue(block.value))

  // Local drafts, committed explicitly per the keyboard entry contract
  // (draftEdit action: Enter/blur commit, Escape reverts, Ctrl+Enter
  // commits + closes the page). Both fields commit together — the block
  // value is one {url, caption} object.
  // untrack: seed once from the prop; in-flight typing owns the field.
  let urlDraft = $state(untrack(() => value.url))
  let captionDraft = $state(untrack(() => value.caption ?? ''))
  // External-edit sync state: tracks the last value we ourselves
  // committed, so the $effect below can distinguish our own echoes
  // from genuinely-external changes.
  let lastSavedUrl = $state(untrack(() => value.url))
  let lastSavedCaption = $state(untrack(() => value.caption ?? ''))

  const editScope = getContext<EditScope | undefined>(EDIT_SCOPE_KEY) ?? null

  function commitDrafts() {
    if (urlDraft === lastSavedUrl && captionDraft === lastSavedCaption) return
    const next: { url: string; caption?: string } = { url: urlDraft }
    if (captionDraft.trim() !== '') next.caption = captionDraft
    lastSavedUrl = urlDraft
    lastSavedCaption = captionDraft
    onChange(withValue(block, next))
  }

  function revertDrafts() {
    urlDraft = lastSavedUrl
    captionDraft = lastSavedCaption
  }

  // Fresh block prop (re-keyed to a different block): drop stale
  // drafts and re-seed.
  let seededID = untrack(() => block.id)
  $effect(() => {
    if (block.id === seededID) return
    seededID = block.id
    untrack(() => {
      const next = asUrlValue(block.value)
      urlDraft = next.url
      captionDraft = next.caption ?? ''
      lastSavedUrl = urlDraft
      lastSavedCaption = captionDraft
    })
  })

  // External-edit sync. Track last committed url + caption to detect
  // external changes vs our own echos. Skip when the user has unsaved
  // typing in either field.
  $effect(() => {
    const next = asUrlValue(block.value)
    const incomingUrl = next.url
    const incomingCaption = next.caption ?? ''
    if (incomingUrl === lastSavedUrl && incomingCaption === lastSavedCaption) return
    if (urlDraft !== lastSavedUrl || captionDraft !== lastSavedCaption) return // mid-type
    lastSavedUrl = incomingUrl
    lastSavedCaption = incomingCaption
    urlDraft = incomingUrl
    captionDraft = incomingCaption
  })
</script>

<div class="url-block">
  <input
    type="url"
    inputmode="url"
    class="field"
    placeholder={t('block.url.url_placeholder')}
    bind:value={urlDraft}
    enterkeyhint="done"
    use:draftEdit={{ onCommit: commitDrafts, onCancel: revertDrafts, scope: editScope }}
  />
  <input
    type="text"
    class="field"
    placeholder={t('block.url.caption_placeholder')}
    bind:value={captionDraft}
    enterkeyhint="done"
    use:draftEdit={{ onCommit: commitDrafts, onCancel: revertDrafts, scope: editScope }}
  />
  {#if urlDraft}
    <a class="preview" href={urlDraft} target="_blank" rel="noopener noreferrer">
      {captionDraft || urlDraft}
    </a>
  {/if}
</div>

<style>
  .url-block {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }
  .field {
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    padding: 0.55rem 0.7rem;
  }
  .field:focus {
    outline: none;
    border-color: var(--accent);
  }
  .preview {
    align-self: flex-start;
    color: var(--accent);
    font-size: 0.85rem;
    word-break: break-all;
  }
</style>
