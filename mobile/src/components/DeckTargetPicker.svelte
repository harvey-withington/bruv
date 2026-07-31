<script lang="ts">
  import { onMount } from 'svelte'
  import { Layers } from 'lucide-svelte'
  import { repoRPC } from '../lib/auth'
  import { t } from '../lib/i18n.svelte'
  import { renderInline } from '@shared/markdown'
  import type { Card } from '@shared/types'
  import type { DeckTarget } from '../lib/capturePrefs'
  import BottomSheet from './BottomSheet.svelte'
  import NewDeckRow from './NewDeckRow.svelte'

  // Deck target picker: which slide deck should clipped slides land in?
  //
  // Search (or the recents list on an empty query) → tap a card → its
  // first slide_deck block becomes the target. Multi-deck cards are
  // rare; the desktop editor is the place to sort those out.

  let {
    onSelect,
    onClose,
  }: {
    onSelect: (target: DeckTarget) => void
    onClose: () => void
  } = $props()

  // Server-side SearchResult shape (Go field naming) — same contract as
  // SearchSheet's result rows.
  type CardHit = { CardID: string; Title: string; ProjectContext: string }

  let query = $state('')
  let results = $state<CardHit[]>([])
  let loading = $state(true)
  let errorMsg = $state<string | null>(null)
  let busy = $state(false)

  let timer: ReturnType<typeof setTimeout> | null = null
  let seq = 0

  async function runSearch(q: string) {
    const mine = ++seq
    loading = true
    errorMsg = null
    try {
      const trimmed = q.trim()
      const hits =
        (trimmed
          ? await repoRPC<CardHit[]>('SearchCards', [trimmed, 8])
          : await repoRPC<CardHit[]>('RecentCards', [8])) ?? []
      // Drop stale responses — the user has typed on since this one left.
      if (mine !== seq) return
      results = hits
    } catch (err) {
      if (mine !== seq) return
      errorMsg = err instanceof Error ? err.message : t('clip.err_search')
    } finally {
      if (mine === seq) loading = false
    }
  }

  function onInput(e: Event) {
    query = (e.currentTarget as HTMLInputElement).value
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => {
      timer = null
      void runSearch(query)
    }, 250)
  }

  async function pick(hit: CardHit) {
    if (busy) return
    busy = true
    errorMsg = null
    try {
      const card = await repoRPC<Card>('GetCard', [hit.CardID])
      const deck = (card?.blocks ?? []).find((b) => b.type === 'slide_deck')
      if (!deck) {
        errorMsg = t('clip.no_deck_block')
        return
      }
      onSelect({ cardID: hit.CardID, blockID: deck.id, name: hit.Title || t('inbox.untitled') })
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('clip.err_pick')
    } finally {
      busy = false
    }
  }

  onMount(() => {
    void runSearch('')
    return () => {
      if (timer) clearTimeout(timer)
    }
  })
</script>

<BottomSheet
  title={t('clip.deck_picker_title')}
  subtitle={t('clip.deck_picker_subtitle')}
  historyKey="deckPicker"
  {onClose}
>
  <input
    class="search"
    type="text"
    value={query}
    oninput={onInput}
    placeholder={t('clip.deck_search_placeholder')}
    aria-label={t('clip.deck_search_placeholder')}
    autocomplete="off"
    autocapitalize="off"
    spellcheck="false"
    enterkeyhint="search"
  />

  {#if errorMsg}
    <p class="error" role="alert">{errorMsg}</p>
  {/if}

  <div class="results">
    {#if loading && results.length === 0}
      <p class="status">{t('common.loading')}</p>
    {:else if results.length === 0}
      <p class="status">{t('clip.no_results')}</p>
    {:else}
      {#if !query.trim()}
        <p class="group-label">{t('clip.recent_cards')}</p>
      {/if}
      <ul>
        {#each results as hit (hit.CardID)}
          <li>
            <button type="button" class="row" onclick={() => pick(hit)} disabled={busy}>
              <Layers size={14} />
              <span class="row-text">
                <span class="row-title">{@html renderInline(hit.Title || t('inbox.untitled'))}</span>
                {#if hit.ProjectContext}
                  <span class="row-path">{hit.ProjectContext}</span>
                {/if}
              </span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <NewDeckRow disabled={busy} onCreated={onSelect} onError={(msg) => (errorMsg = msg)} />
</BottomSheet>

<style>
  .search {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    padding: 0.55rem 0.7rem;
    outline: none;
  }
  .search:focus {
    border-color: var(--accent);
  }

  .results {
    overflow-y: auto;
    flex: 1;
    min-height: 4rem;
  }

  ul {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .group-label {
    margin: 0 0 0.35rem;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-faint);
  }

  .row {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    width: 100%;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.6rem 0.7rem;
    color: var(--text);
    font: inherit;
    font-size: 0.9rem;
    cursor: pointer;
    text-align: left;
    touch-action: manipulation;
  }
  .row:hover:not(:disabled),
  .row:focus-visible:not(:disabled) {
    border-color: var(--accent);
    outline: none;
  }
  .row:disabled {
    opacity: 0.6;
    cursor: default;
  }

  .row-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }
  .row-title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .row-path {
    font-size: 0.75rem;
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .status {
    margin: 0.75rem 0.25rem;
    color: var(--text-muted);
    font-size: 0.85rem;
  }

  .error {
    margin: 0;
    padding: 0.5rem 0.75rem;
    background: rgba(239, 68, 68, 0.12);
    color: #fca5a5;
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 6px;
    font-size: 0.85rem;
  }
</style>
