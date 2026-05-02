<script lang="ts">
  import { onMount } from 'svelte'
  import { Share2 } from 'lucide-svelte'
  import { repoRPC } from '../lib/auth'
  import { navigate, cardURL } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
  import type { Card } from '@shared/types'

  // Landing page for shares from the Android system share sheet.
  //
  // The browser POSTs to /m/share with title/text/url; the Go handler
  // 303-redirects here with those fields as query params. We pre-fill
  // an editable preview so the user can tweak before saving — most
  // shares are URLs that the user wants to keep + maybe annotate.

  function readParam(name: string): string {
    if (typeof window === 'undefined') return ''
    return new URLSearchParams(window.location.search).get(name) ?? ''
  }

  // Capture once on mount so the inputs are seeded; let the user edit
  // freely after that.
  // svelte-ignore state_referenced_locally
  let title = $state(readParam('title'))
  // svelte-ignore state_referenced_locally
  let text = $state(readParam('text'))
  // svelte-ignore state_referenced_locally
  let url = $state(readParam('url'))

  let saving = $state(false)
  let errorMsg = $state<string | null>(null)
  let titleEl: HTMLInputElement | undefined = $state()

  onMount(() => {
    // If no title was supplied, derive a sensible one from the URL or
    // the first line of text. Real-world Android shares from Chrome
    // typically include a URL but no title.
    if (!title) {
      if (url) {
        try {
          title = new URL(url).hostname
        } catch {
          title = url
        }
      } else if (text) {
        title = text.split('\n')[0].slice(0, 80)
      }
    }
    queueMicrotask(() => titleEl?.focus())
  })

  function buildBody(): string {
    // Minimal filtering: take whatever's in the form's URL + text
    // fields and concatenate verbatim (preserving the multi-line
    // structure of the highlighted text). The user already saw and
    // edited these in the preview — we don't second-guess them.
    //
    // Only dedup case: text and URL are byte-identical (Brave's
    // "share this page" stuffs the URL into both). Everything else
    // including title overlap goes through as-is — better to keep
    // duplicate content than silently drop a paragraph.
    const u = url.trim()
    const tx = text.trim()
    if (u && tx && u !== tx) return `${u}\n\n${tx}`
    return tx || u
  }

  async function save() {
    const finalTitle = title.trim()
    if (!finalTitle || saving) return
    saving = true
    errorMsg = null
    try {
      const card = await repoRPC<Card>('CreateCard', ['', finalTitle])
      const body = buildBody()
      if (body) {
        // Persist the shared text as the card's intrinsic description.
        // Non-fatal on failure — the card exists either way; the user
        // can paste manually if the description save fails.
        try {
          await repoRPC('UpdateCardDescription', [card.id, body])
        } catch {
          /* leave description empty rather than blocking the navigate */
        }
      }
      navigate(cardURL(card.id))
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('share.err_save')
    } finally {
      saving = false
    }
  }

  function cancel() {
    navigate('/')
  }
</script>

<header class="topbar">
  <button type="button" class="back" onclick={cancel}>
    <span aria-hidden="true">‹</span> {t('common.cancel')}
  </button>
  <span class="topbar-title"><Share2 size={14} /> {t('share.title')}</span>
  <span class="spacer"></span>
</header>

<main>
  <p class="intro">{t('share.intro')}</p>

  <label class="field">
    <span class="field-label">{t('share.field_title')}</span>
    <input
      bind:this={titleEl}
      bind:value={title}
      type="text"
      placeholder={t('share.field_title_placeholder')}
      disabled={saving}
    />
  </label>

  {#if url}
    <label class="field">
      <span class="field-label">{t('share.field_url')}</span>
      <input bind:value={url} type="url" disabled={saving} />
    </label>
  {/if}

  {#if text}
    <label class="field">
      <span class="field-label">{t('share.field_text')}</span>
      <textarea bind:value={text} rows="4" disabled={saving}></textarea>
    </label>
  {/if}

  {#if errorMsg}
    <div class="error" role="alert">{errorMsg}</div>
  {/if}

  <div class="actions">
    <button type="button" class="ghost" onclick={cancel} disabled={saving}>
      {t('common.cancel')}
    </button>
    <button
      type="button"
      class="primary"
      onclick={save}
      disabled={saving || !title.trim()}
    >
      {saving ? t('share.saving') : t('share.save')}
    </button>
  </div>
</main>

<style>
  .topbar {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: center;
    padding: 0.75rem;
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    background: var(--bg);
    z-index: 10;
  }

  .topbar-title {
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text);
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    justify-self: center;
  }

  .back {
    background: transparent;
    border: none;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.9rem;
    cursor: pointer;
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    justify-self: start;
  }

  .back:hover,
  .back:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }

  main {
    max-width: 600px;
    margin: 0 auto;
    padding: 1.25rem 1rem 4rem;
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
  }

  .intro {
    margin: 0 0 0.5rem;
    color: var(--text-muted);
    font-size: 0.9rem;
    line-height: 1.5;
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

  .error {
    padding: 0.5rem 0.75rem;
    background: rgba(239, 68, 68, 0.12);
    color: #fca5a5;
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 6px;
    font-size: 0.85rem;
  }

  .actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.5rem;
  }

  .ghost,
  .primary {
    padding: 0.7rem 1.2rem;
    border-radius: 8px;
    font: inherit;
    font-size: 0.9rem;
    font-weight: 500;
    cursor: pointer;
    border: 1px solid transparent;
  }

  .ghost {
    background: transparent;
    color: var(--text-muted);
    border-color: var(--border);
  }
  .ghost:hover:not(:disabled),
  .ghost:focus-visible:not(:disabled) {
    color: var(--text);
    border-color: var(--text-muted);
    outline: none;
  }

  .primary {
    background: var(--accent);
    color: var(--bg);
    flex: 1;
    font-weight: 600;
  }
  .primary:hover:not(:disabled),
  .primary:focus-visible:not(:disabled) {
    filter: brightness(1.1);
    outline: none;
  }

  .ghost:disabled,
  .primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
