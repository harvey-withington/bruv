<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { Search, X } from 'lucide-svelte'
  import { repoRPC } from '../lib/auth'
  import { navigate, cardURL } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
  import { renderInline } from '@shared/markdown'
  import { repoMeta } from '../lib/repoMeta.svelte'
  import { getCardTypeColor, getCardTypeTextColor, getCardTypeLabel } from '@shared/cardTypes'

  // Slide-up search sheet, accessed from the home top-bar. 200ms
  // debounced server-side SearchCards. Result list of card rows;
  // tap to open. Empty input shows nothing — start typing to search.

  // Server-side SearchResult shape — Go's default field naming, NOT
  // the CardSummary lower-case shape. Mirrors the desktop's MentionPicker
  // contract so any future search-result UI sees the same fields.
  type SearchHit = {
    CardID: string
    Title: string
    Type: string
    Rank: number
    ProjectContext: string
  }

  let { onClose }: { onClose: () => void } = $props()

  let input = $state('')
  let results = $state<SearchHit[]>([])
  let loading = $state(false)
  let errorMsg = $state<string | null>(null)
  let inputEl: HTMLInputElement | null = $state(null)

  let timer: ReturnType<typeof setTimeout> | null = null
  let lastQuery = ''

  async function runSearch(q: string) {
    if (!q.trim()) {
      results = []
      loading = false
      return
    }
    loading = true
    errorMsg = null
    lastQuery = q
    try {
      const r = (await repoRPC<SearchHit[]>('SearchCards', [q, 50])) ?? []
      // Drop stale responses if the user has typed more in the meantime.
      if (q !== lastQuery) return
      results = r
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('search.err')
    } finally {
      if (q === lastQuery) loading = false
    }
  }

  function onInput(e: Event) {
    input = (e.currentTarget as HTMLInputElement).value
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => {
      timer = null
      void runSearch(input)
    }, 200)
  }

  function clearInput() {
    input = ''
    results = []
    inputEl?.focus()
  }

  function open(hit: SearchHit) {
    onClose()
    navigate(cardURL(hit.CardID))
  }

  onMount(() => {
    history.pushState({ search: true }, '')
    const onPop = () => onClose()
    window.addEventListener('popstate', onPop)
    void tick().then(() => inputEl?.focus())
    return () => {
      window.removeEventListener('popstate', onPop)
      if (timer) clearTimeout(timer)
      if (history.state?.search) history.back()
    }
  })
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="backdrop" onclick={onClose}></div>

<div class="sheet" role="dialog" aria-label={t('search.title')}>
  <header class="header">
    <span class="search-icon" aria-hidden="true"><Search size={18} /></span>
    <input
      bind:this={inputEl}
      type="text"
      class="input"
      placeholder={t('search.placeholder')}
      value={input}
      oninput={onInput}
      autocomplete="off"
      autocapitalize="off"
      spellcheck="false"
      enterkeyhint="search"
    />
    {#if input}
      <button type="button" class="icon-btn" onclick={clearInput} aria-label={t('common.cancel')}>
        <X size={16} />
      </button>
    {/if}
    <button type="button" class="cancel-btn" onclick={onClose}>{t('common.cancel')}</button>
  </header>

  <div class="body">
    {#if !input.trim()}
      <p class="hint">{t('search.hint')}</p>
    {:else if loading && results.length === 0}
      <p class="status">{t('common.loading')}</p>
    {:else if errorMsg}
      <p class="error">{errorMsg}</p>
    {:else if results.length === 0}
      <p class="status">{t('search.no_results')}</p>
    {:else}
      <ul class="results">
        {#each results as hit (hit.CardID)}
          <li>
            <button type="button" class="result" onclick={() => open(hit)}>
              <div class="result-text">
                <span class="result-title">{@html renderInline(hit.Title || t('inbox.untitled'))}</span>
                {#if hit.ProjectContext}
                  <span class="result-context">{hit.ProjectContext}</span>
                {/if}
              </div>
              {#if hit.Type}
                <span
                  class="card-type"
                  style:background={getCardTypeColor(hit.Type, repoMeta.cardTypes)}
                  style:color={getCardTypeTextColor(hit.Type)}
                >
                  {getCardTypeLabel(hit.Type, repoMeta.cardTypes)}
                </span>
              {/if}
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    z-index: 60;
    animation: fade-in 180ms ease forwards;
  }
  .sheet {
    position: fixed;
    left: 0; right: 0; top: 0;
    bottom: 0;
    background: var(--bg);
    z-index: 61;
    display: flex;
    flex-direction: column;
    animation: slide-down 220ms cubic-bezier(0.16, 1, 0.3, 1) forwards;
    padding-top: env(safe-area-inset-top);
    padding-bottom: env(safe-area-inset-bottom);
  }
  .header {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.6rem 0.75rem;
    border-bottom: 1px solid var(--border);
  }
  .search-icon {
    color: var(--text-faint);
    display: inline-flex;
  }
  .input {
    flex: 1;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    padding: 0.55rem 0.7rem;
    -webkit-appearance: none;
    appearance: none;
  }
  .input:focus {
    outline: none;
    border-color: var(--accent);
  }
  .icon-btn {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.45rem;
    border-radius: 6px;
    display: inline-flex;
  }
  .icon-btn:hover,
  .icon-btn:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }
  .cancel-btn {
    background: transparent;
    border: none;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.9rem;
    padding: 0.4rem 0.5rem;
    cursor: pointer;
  }
  .cancel-btn:hover,
  .cancel-btn:focus-visible {
    color: var(--accent);
    outline: none;
  }

  .body {
    flex: 1;
    overflow-y: auto;
    padding: 0.5rem 0.85rem 1rem;
  }

  .results {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .result {
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    width: 100%;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0.75rem 1rem;
    color: var(--text);
    font: inherit;
    cursor: pointer;
    text-align: left;
    touch-action: manipulation;
  }
  .result:hover,
  .result:focus-visible {
    border-color: var(--accent);
    outline: none;
  }

  .result-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }
  .result-title {
    font-size: 0.95rem;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .result-context {
    font-size: 0.75rem;
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .card-type {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    padding: 0.2rem 0.5rem;
    border-radius: 4px;
    font-weight: 500;
    flex-shrink: 0;
  }

  .hint {
    color: var(--text-faint);
    text-align: center;
    margin: 2rem 1rem;
    font-size: 0.9rem;
    font-style: italic;
  }
  .status {
    color: var(--text-muted);
    text-align: center;
    margin: 2rem 0;
  }
  .error {
    margin: 2rem 0;
    padding: 1rem;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 8px;
    color: #fca5a5;
    text-align: center;
  }

  @keyframes fade-in {
    from { opacity: 0; }
    to { opacity: 1; }
  }
  @keyframes slide-down {
    from { transform: translateY(-30px); opacity: 0; }
    to { transform: translateY(0); opacity: 1; }
  }
  @media (prefers-reduced-motion: reduce) {
    .backdrop, .sheet { animation: fade-in 120ms ease forwards; }
  }
</style>
