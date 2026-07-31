<script lang="ts">
  import { onMount } from 'svelte'
  import { Clipboard } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'

  // The editable part of the share page. The URL field (with its paste
  // button) is shown in both modes; title + text only in plain mode —
  // a clip takes its title and body from the resolved post, so offering
  // them here would just be fields the server overwrites.

  let {
    platformLabel,
    title = $bindable(),
    text = $bindable(),
    url = $bindable(),
    disabled = false,
    onPasteError,
  }: {
    /** Capitalized platform id when the URL is clippable, '' otherwise. */
    platformLabel: string
    title: string
    text: string
    url: string
    disabled?: boolean
    onPasteError: (message: string | null) => void
  } = $props()

  let titleEl = $state<HTMLInputElement | null>(null)
  let urlEl = $state<HTMLInputElement | null>(null)

  onMount(() => {
    // Arriving from the home tile there's nothing to paste into yet, so
    // start in the URL field; an Android share already carries a URL, so
    // the title is the thing worth editing.
    queueMicrotask(() => (url ? titleEl?.focus() : urlEl?.focus()))
  })

  async function paste() {
    onPasteError(null)
    if (!navigator.clipboard?.readText) {
      onPasteError(t('share.paste_unsupported'))
      return
    }
    try {
      const clip = (await navigator.clipboard.readText()).trim()
      if (!clip) return
      url = clip
    } catch {
      // iOS Safari throws on denied permission; Android Chrome throws
      // when the page isn't focused. Long-press paste still works.
      onPasteError(t('share.paste_failed'))
    }
  }
</script>

{#if platformLabel}
  <div class="platform-row">
    <span class="platform-chip">{platformLabel}</span>
    <span class="platform-hint">{t('share.clip_hint')}</span>
  </div>
{/if}

<div class="field">
  <label class="field-label" for="share-url">{t('share.field_url')}</label>
  <div class="url-row">
    <input
      bind:this={urlEl}
      bind:value={url}
      id="share-url"
      type="url"
      inputmode="url"
      placeholder={t('share.field_url_placeholder')}
      autocomplete="off"
      autocapitalize="off"
      spellcheck="false"
      {disabled}
    />
    <button type="button" class="paste" onclick={paste} {disabled}>
      <Clipboard size={14} />
      {t('share.paste')}
    </button>
  </div>
</div>

{#if !platformLabel}
  <label class="field">
    <span class="field-label">{t('share.field_title')}</span>
    <input
      bind:this={titleEl}
      bind:value={title}
      type="text"
      placeholder={t('share.field_title_placeholder')}
      {disabled}
    />
  </label>

  <label class="field">
    <span class="field-label">{t('share.field_text')}</span>
    <textarea bind:value={text} rows="4" {disabled}></textarea>
  </label>
{/if}

<style>
  .platform-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .platform-chip {
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.25rem 0.6rem;
    border-radius: 999px;
    color: var(--accent);
    background: color-mix(in srgb, var(--accent) 15%, transparent);
    border: 1px solid color-mix(in srgb, var(--accent) 40%, transparent);
  }

  .platform-hint {
    font-size: 0.8rem;
    color: var(--text-muted);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .field-label {
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .url-row {
    display: flex;
    align-items: stretch;
    gap: 0.4rem;
  }
  .url-row input {
    flex: 1;
    min-width: 0;
  }

  input,
  textarea {
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text);
    font: inherit;
    font-size: 1rem;
    padding: 0.65rem 0.85rem;
    outline: none;
    resize: vertical;
  }

  input:focus,
  textarea:focus {
    border-color: var(--accent);
  }

  .paste {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.85rem;
    padding: 0 0.85rem;
    cursor: pointer;
    touch-action: manipulation;
  }
  .paste:hover:not(:disabled),
  .paste:focus-visible:not(:disabled) {
    color: var(--text);
    border-color: var(--accent);
    outline: none;
  }
  .paste:disabled,
  input:disabled,
  textarea:disabled {
    opacity: 0.6;
  }
</style>
