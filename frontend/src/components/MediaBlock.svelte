<script lang="ts">
  import { Trash2, ExternalLink } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'

  type MediaItem = { id: string; url: string; caption?: string; mime?: string }

  let {
    items = [],
    onUpdate,
  }: {
    items?: MediaItem[]
    onUpdate?: (items: MediaItem[]) => void
  } = $props()

  let newUrl = $state('')
  let expandedId = $state<string | null>(null)

  function emit(updated: MediaItem[]) {
    onUpdate?.(updated)
  }

  function addMedia() {
    const url = newUrl.trim()
    if (!url) return
    const id = `med-${crypto.randomUUID().slice(0, 8)}`
    const mime = guessMime(url)
    emit([...items, { id, url, mime }])
    newUrl = ''
  }

  function removeMedia(id: string) {
    emit(items.filter(item => item.id !== id))
  }

  function updateCaption(id: string, caption: string) {
    emit(items.map(item => item.id === id ? { ...item, caption } : item))
  }

  function guessMime(url: string): string {
    const lower = url.toLowerCase()
    if (lower.match(/\.(jpg|jpeg|png|webp|svg|bmp|ico)(\?|$)/)) return 'image'
    if (lower.match(/\.(gif)(\?|$)/)) return 'image/gif'
    if (lower.match(/\.(mp4|webm|ogg|mov)(\?|$)/)) return 'video'
    // Default to image for common image hosts
    if (lower.includes('imgur') || lower.includes('giphy') || lower.includes('unsplash')) return 'image'
    return 'image'
  }

  function isVideo(item: MediaItem): boolean {
    return item.mime === 'video' || (item.url?.match(/\.(mp4|webm|ogg|mov)(\?|$)/i) != null)
  }

  function handleAddKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') addMedia()
    else if (e.key === 'Escape') { e.stopPropagation(); (e.target as HTMLElement)?.blur() }
  }
</script>

{#if items.length === 0}
  <p class="media-empty">{t('block.media_empty')}</p>
{:else}
  <div class="media-grid">
    {#each items as item (item.id)}
      <div class="media-item action-reveal-parent">
        <div class="media-preview" class:expanded={expandedId === item.id}>
          {#if isVideo(item)}
            <!-- svelte-ignore a11y_media_has_caption -->
            <video src={item.url} controls preload="metadata" class="media-content"></video>
          {:else}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <img
              src={item.url}
              alt={item.caption || ''}
              class="media-content"
              loading="lazy"
              onclick={() => expandedId = expandedId === item.id ? null : item.id}
            />
          {/if}
        </div>
        <div class="media-meta">
          <input
            class="media-caption"
            type="text"
            value={item.caption || ''}
            onchange={(e) => updateCaption(item.id, (e.target as HTMLInputElement).value)}
            placeholder={t('block.media_caption_placeholder')}
          />
          <div class="media-actions">
            <a href={item.url} target="_blank" rel="noopener" class="media-action-btn" title="Open"><ExternalLink size={12} /></a>
            <button class="media-action-btn action-reveal action-reveal--danger" onclick={() => removeMedia(item.id)} title="Remove"><Trash2 size={12} /></button>
          </div>
        </div>
      </div>
    {/each}
  </div>
{/if}

<div class="media-add">
  <input
    type="text"
    bind:value={newUrl}
    onkeydown={handleAddKeydown}
    placeholder={t('block.add_media')}
    class="media-add-input"
  />
  <button class="media-add-btn" onclick={addMedia}>{t('block.add_media_btn')}</button>
</div>

<style>
  .media-empty {
    font-size: 0.8rem;
    color: var(--text-muted);
    font-style: italic;
    margin: 0;
    padding: 0.5rem 0;
  }

  .media-grid {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .media-item {
    border: 1px solid var(--border);
    border-radius: 6px;
    overflow: hidden;
    background: var(--bg-elevated);
  }

  .media-preview {
    max-height: 200px;
    overflow: hidden;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg);
    cursor: pointer;
    transition: max-height var(--duration-moderate) ease;
  }
  .media-preview.expanded {
    max-height: none;
  }

  .media-content {
    max-width: 100%;
    max-height: 200px;
    object-fit: contain;
    display: block;
  }
  .media-preview.expanded .media-content {
    max-height: none;
  }

  .media-meta {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.25rem 0.4rem;
  }

  .media-caption {
    flex: 1;
    font-size: 0.75rem;
    color: var(--text-body);
    background: none;
    border: none;
    outline: none;
    padding: 0.15rem 0.3rem;
    border-radius: 3px;
    font-family: inherit;
  }
  .media-caption:focus {
    background: var(--bg);
    border: 1px solid var(--accent);
  }
  .media-caption::placeholder {
    color: var(--text-faint);
    font-style: italic;
  }

  .media-actions {
    display: flex;
    gap: 0.15rem;
    flex-shrink: 0;
  }

  .media-action-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 0.15rem;
    line-height: 1;
    display: flex;
    align-items: center;
    border-radius: 3px;
    text-decoration: none;
  }
  .media-action-btn:hover { color: var(--text-primary); }

  .media-add {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.4rem;
  }

  .media-add-input {
    flex: 1;
    padding: 0.3rem 0.5rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    font-family: inherit;
    outline: none;
  }
  .media-add-input:focus { border-color: var(--accent); }

  .media-add-btn {
    padding: 0.3rem 0.6rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-secondary);
    font-size: 0.8rem;
    cursor: pointer;
    white-space: nowrap;
  }
  .media-add-btn:hover {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
  }
</style>
