<script lang="ts">
  import { t } from '../../lib/i18n.svelte'
  import type { Block } from '@shared/types'
  import { asUrlValue, withValue } from './narrow'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  const value = $derived(asUrlValue(block.value))

  // Local drafts so debounced commits don't fight typing on either field.
  let urlDraft = $state(value.url)
  let captionDraft = $state(value.caption ?? '')
  let urlTimer: ReturnType<typeof setTimeout> | null = null
  let captionTimer: ReturnType<typeof setTimeout> | null = null

  function commit() {
    const next: { url: string; caption?: string } = { url: urlDraft }
    if (captionDraft.trim() !== '') next.caption = captionDraft
    onChange(withValue(block, next))
  }

  function handleUrl(e: Event) {
    urlDraft = (e.currentTarget as HTMLInputElement).value
    if (urlTimer) clearTimeout(urlTimer)
    urlTimer = setTimeout(() => {
      urlTimer = null
      commit()
    }, 400)
  }

  function handleCaption(e: Event) {
    captionDraft = (e.currentTarget as HTMLInputElement).value
    if (captionTimer) clearTimeout(captionTimer)
    captionTimer = setTimeout(() => {
      captionTimer = null
      commit()
    }, 400)
  }

  function handleBlur() {
    if (urlTimer) {
      clearTimeout(urlTimer)
      urlTimer = null
    }
    if (captionTimer) {
      clearTimeout(captionTimer)
      captionTimer = null
    }
    commit()
  }

  $effect(() => () => {
    if (urlTimer) clearTimeout(urlTimer)
    if (captionTimer) clearTimeout(captionTimer)
  })
</script>

<div class="url-block">
  <input
    type="url"
    inputmode="url"
    class="field"
    placeholder={t('block.url.url_placeholder')}
    value={urlDraft}
    oninput={handleUrl}
    onblur={handleBlur}
  />
  <input
    type="text"
    class="field"
    placeholder={t('block.url.caption_placeholder')}
    value={captionDraft}
    oninput={handleCaption}
    onblur={handleBlur}
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
