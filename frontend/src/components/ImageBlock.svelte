<script lang="ts">
  import { t } from '../lib/i18n.svelte'

  let {
    value,
    onUpdate
  }: {
    value: string | { url: string; caption?: string } | null
    onUpdate: (value: { url: string; caption?: string }) => void
  } = $props()

  // Normalize value -- can be plain string URL or {url, caption} object
  const imgData = $derived(
    typeof value === 'string' ? { url: value, caption: '' } :
    (value && typeof value === 'object' && 'url' in value) ? value as { url: string; caption?: string } :
    { url: '', caption: '' }
  )

  let editing = $state(!imgData.url)
  let urlDraft = $state(imgData.url)
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
    <div class="image-display" onclick={() => { urlDraft = imgData.url; captionDraft = imgData.caption || ''; editing = true }}>
      <img src={imgData.url} alt={imgData.caption || ''} class="block-image" />
      {#if imgData.caption}
        <p class="image-caption">{imgData.caption}</p>
      {/if}
    </div>
  {/if}
</div>

<style>
  .image-block { }
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
