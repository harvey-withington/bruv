<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { SignAttachmentURL } from '@shared/api'

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
    if (url && url.startsWith('att-')) {
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

  function save() {
    if (!urlDraft.trim()) return
    onUpdate({ url: urlDraft.trim(), caption: captionDraft.trim() || undefined })
    editing = false
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
        onkeydown={(e) => { if (e.key === 'Enter') save() }}
      />
      <input
        type="text"
        class="image-caption-input"
        placeholder={t('block.image_caption_placeholder')}
        bind:value={captionDraft}
        onkeydown={(e) => { if (e.key === 'Enter') save() }}
      />
      <button class="image-save-btn" onclick={save}>{t('common.save')}</button>
    </div>
  {:else}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="image-display" onclick={() => { urlDraft = imgData.url; captionDraft = imgData.caption || ''; editing = true }}>
      <img src={resolvedURL} alt={imgData.caption || ''} class="block-image" />
      {#if imgData.caption}
        <p class="image-caption">{imgData.caption}</p>
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
