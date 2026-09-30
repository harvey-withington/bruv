<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { renderInline } from '@shared/markdown'
  import { mentionable } from '../lib/mentions.svelte'
  import { SignAttachmentURL } from '@shared/api'
  import { parseAttachmentRef } from '@shared/attachmentRefs'
  import { getContext } from 'svelte'
  import { EDIT_SCOPE_KEY, type EditScope } from '@shared/editScope'
  import { inlineEdit } from '../lib/actions'

  let {
    value,
    cardId,
    onUpdate
  }: {
    value: string | { url: string; caption?: string } | null
    cardId: string
    onUpdate: (value: { url: string; caption?: string }) => void
  } = $props()

  // Normalize value -- can be plain string URL or {url, caption} object
  const imgData = $derived(
    typeof value === 'string' ? { url: value, caption: '' } :
    (value && typeof value === 'object' && 'url' in value) ? value as { url: string; caption?: string } :
    { url: '', caption: '' }
  )

  let resolvedURL = $state('')

  $effect(() => {
    const url = imgData.url
    // Two attachment-backed forms: a bare `att-` id (uploads write the
    // id with the block's own card implied) and a durable
    // `attachment:<cardID>/<attID>` ref (the capture pipeline — the ref
    // carries its own card id, use it).
    const ref = parseAttachmentRef(url)
    if (ref) {
      SignAttachmentURL(ref.cardID, ref.attachmentID)
        .then(path => {
          resolvedURL = path
        })
        .catch(() => {
          resolvedURL = ''
        })
    } else if (url && url.startsWith('att-')) {
      SignAttachmentURL(cardId, url)
        .then(path => {
          resolvedURL = path
        })
        .catch(() => {
          resolvedURL = ''
        })
    } else {
      resolvedURL = url
    }
  })

  // Draft state seeded once from the incoming value. The edit flow writes
  // back through onUpdate, so drafts don't need to track prop changes.
  // svelte-ignore state_referenced_locally
  let editing = $state(!imgData.url)
  // svelte-ignore state_referenced_locally
  let urlDraft = $state(imgData.url)
  // svelte-ignore state_referenced_locally
  let captionDraft = $state(imgData.caption || '')

  // Keyboard entry contract (UI-CONVENTIONS §8) via inlineEdit + the card's
  // EditScope: Enter saves, Escape cancels the edit (never the card),
  // Ctrl+Enter saves + closes. The hand-rolled Enter-only handler let Escape
  // close the card and drop the typed caption. Editing an existing image is
  // edit-in-place (blur out of the url+caption pair saves); an EMPTY block's
  // inputs are always mounted, so they behave like an add row (serial:
  // registered only while focused, blur keeps the draft) — otherwise they'd
  // hold the scope open and Escape could never close the card.
  const editScope = getContext<EditScope | undefined>(EDIT_SCOPE_KEY) ?? null
  const editParams = $derived({
    onCommit: save,
    onCancel: cancel,
    scope: editScope,
    container: '.image-edit',
    serial: !imgData.url,
  })

  function save() {
    const url = urlDraft.trim()
    if (!url) return
    const caption = captionDraft.trim() || undefined
    editing = false
    // Both inputs register with the scope — a Ctrl+Enter commitAll must not
    // write the same value twice.
    if (url === imgData.url && (caption ?? '') === (imgData.caption ?? '')) return
    onUpdate({ url, caption })
  }

  function cancel() {
    urlDraft = imgData.url
    captionDraft = imgData.caption || ''
    if (imgData.url) editing = false
  }
</script>

<div class="image-block">
  {#if editing || !imgData.url}
    <div class="image-edit">
      <input
        type="url"
        class="image-url-input"
        placeholder={t('block.image_url_placeholder')}
        bind:value={urlDraft}
        use:inlineEdit={editParams}
      />
      <input
        type="text"
        class="image-caption-input"
        placeholder={t('block.image_caption_placeholder')}
        use:mentionable
        bind:value={captionDraft}
        use:inlineEdit={editParams}
      />
      <button class="image-save-btn" onclick={save}>{t('common.save')}</button>
    </div>
  {:else}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="image-display" onclick={() => { urlDraft = imgData.url; captionDraft = imgData.caption || ''; editing = true }}>
      <img src={resolvedURL} alt={imgData.caption || ''} class="block-image" />
      {#if imgData.caption}
        <p class="image-caption">{@html renderInline(imgData.caption)}</p>
      {/if}
    </div>
  {/if}
</div>

<style>
  .image-edit { display: flex; flex-direction: column; gap: 6px; }
  .image-url-input, .image-caption-input {
    padding: 6px 10px; border: 1px solid var(--border); border-radius: 6px;
    background: var(--bg-surface); color: var(--text-primary); font-size: 0.9em;
  }
  .image-url-input:focus, .image-caption-input:focus { border-color: var(--accent); outline: none; }
  .image-save-btn {
    align-self: flex-end; padding: 4px 12px; border-radius: 4px;
    background: var(--accent); color: white; border: none; cursor: pointer; font-size: 0.85em;
  }
  .image-display { cursor: pointer; }
  .block-image {
    max-width: 100%; max-height: 300px; border-radius: 6px;
    object-fit: contain; display: block;
  }
  .image-caption { font-size: 0.8em; color: var(--text-muted); margin-top: 4px; font-style: italic; }
</style>
