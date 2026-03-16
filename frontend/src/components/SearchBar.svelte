<script lang="ts">
  import { search } from '../lib/store.svelte'
  import { SearchCards } from '../lib/api'
  import { Search, X } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'

  function clearSearch() {
    search.query = ''
    search.results = []
    search.open = false
    search.matchingIds = new Set()
    inputEl?.focus()
  }

  let inputEl: HTMLInputElement | undefined = $state()
  let debounceTimer: ReturnType<typeof setTimeout> | undefined

  let { onSelectCard }: { onSelectCard?: (cardId: string) => void } = $props()

  const typeColors: Record<string, string> = {
    feature: '#6366f1',
    task: '#22c55e',
    brainstorm: '#f59e0b',
    episode: '#ec4899',
    reference: '#06b6d4',
  }

  function handleInput() {
    clearTimeout(debounceTimer)
    if (!search.query.trim()) {
      search.results = []
      search.open = false
      return
    }
    debounceTimer = setTimeout(async () => {
      try {
        const results = await SearchCards(search.query, 20)
        search.results = results || []
        search.open = search.results.length > 0
        search.matchingIds = new Set(search.results.map(r => r.CardID))
      } catch {
        search.results = []
        search.open = false
        search.matchingIds = new Set()
      }
    }, 250)
  }

  function handleFocus() {
    if (search.query.trim() && search.results.length > 0) {
      search.open = true
    }
  }

  function handleBlur() {
    // Delay closing so click on result registers
    setTimeout(() => { search.open = false }, 200)
  }

  function selectResult(cardId: string) {
    search.open = false
    search.query = ''
    search.results = []
    search.matchingIds = new Set()
    onSelectCard?.(cardId)
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      search.open = false
      search.query = ''
      search.matchingIds = new Set()
      inputEl?.blur()
    }
  }
</script>

<div class="search-container">
  <div class="search-box">
    <span class="search-icon"><Search size={14} /></span>
    <input
      bind:this={inputEl}
      type="text"
      bind:value={search.query}
      oninput={handleInput}
      onfocus={handleFocus}
      onblur={handleBlur}
      onkeydown={handleKeydown}
      placeholder={t('search.placeholder')}
      class="search-input"
    />
    {#if search.query}
      <button class="search-clear" onclick={clearSearch}><X size={12} /></button>
    {/if}
  </div>

  {#if search.open}
    <div class="search-results">
      {#each search.results as result}
        <button class="search-result" onclick={() => selectResult(result.CardID)}>
          <span class="result-badge" style="background: {typeColors[result.Type] || '#71717a'}">{result.Type}</span>
          <span class="result-title">{result.Title}</span>
        </button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .search-container {
    position: relative;
  }

  .search-clear {
    background: none;
    border: none;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0.2rem;
    display: flex;
    align-items: center;
    flex-shrink: 0;
    transition: color 0.1s;
  }
  .search-clear:hover {
    color: var(--text-strong);
  }

  .search-box {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.3rem 0.6rem;
    transition: border-color 0.15s;
  }

  .search-box:focus-within {
    border-color: var(--accent);
  }

  .search-icon {
    font-size: 0.75rem;
    flex-shrink: 0;
  }

  .search-input {
    background: none;
    border: none;
    color: var(--text-primary);
    font-size: 0.8rem;
    outline: none;
    width: 200px;
  }

  .search-input::placeholder {
    color: var(--text-faint);
  }

  .search-results {
    position: absolute;
    top: 100%;
    left: 0;
    right: 0;
    margin-top: 4px;
    background: color-mix(in srgb, var(--bg-surface) 50%, transparent);
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 8px 24px var(--shadow-lg);
    max-height: 300px;
    overflow-y: auto;
    z-index: 50;
  }

  .search-result {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.5rem 0.75rem;
    background: none;
    border: none;
    border-bottom: 1px solid var(--border-muted);
    color: var(--text-body);
    font-size: 0.85rem;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
  }

  .search-result:last-child {
    border-bottom: none;
  }

  .search-result:hover {
    background: var(--bg-elevated);
  }

  .result-badge {
    font-size: 0.6rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.1rem 0.35rem;
    border-radius: 3px;
    color: #fff;
    flex-shrink: 0;
  }

  .result-title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
