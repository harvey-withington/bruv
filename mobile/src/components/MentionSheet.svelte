<script lang="ts">
  import { onMount } from 'svelte'
  import { repoRPC } from '../lib/auth'
  import { t } from '../lib/i18n.svelte'
  import { renderInline } from '@shared/markdown'
  import { cardMentionLink, mentionMarkdown } from '@shared/mentions'
  import BottomSheet from './BottomSheet.svelte'

  // The mobile @mention picker: a sheet with a search field, recents on
  // an empty query, tap a card to insert `[Title](bruv:card:id)` into the
  // field that typed the `@` (lib/mentions.svelte.ts owns the splice).
  // Cards only for now; project mentions stay desktop (v1).
  let { onSelect, onClose }: {
    onSelect: (markdown: string) => void
    onClose: () => void
  } = $props()

  type CardHit = { CardID: string; Title: string; ProjectContext: string; Type?: string }

  let query = $state('')
  let results = $state<CardHit[]>([])
  let loading = $state(true)
  let errorMsg = $state<string | null>(null)
  let inputEl: HTMLInputElement | null = $state(null)
  let timer: ReturnType<typeof setTimeout> | null = null
  let seq = 0

  async function run(q: string) {
    const mine = ++seq
    loading = true
    errorMsg = null
    try {
      const trimmed = q.trim()
      const hits = (trimmed
        ? await repoRPC<CardHit[]>('SearchCards', [trimmed, 12])
        : await repoRPC<CardHit[]>('RecentCards', [10])) ?? []
      if (mine !== seq) return
      results = hits
    } catch (err) {
      if (mine !== seq) return
      errorMsg = err instanceof Error ? err.message : t('mention.load_failed')
    } finally {
      if (mine === seq) loading = false
    }
  }

  function onInput(e: Event) {
    query = (e.currentTarget as HTMLInputElement).value
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => { timer = null; void run(query) }, 200)
  }

  function pick(hit: CardHit) {
    onSelect(mentionMarkdown(hit.Title || t('inbox.untitled'), cardMentionLink(hit.CardID)))
  }

  onMount(() => {
    void run('')
    // The sheet's own field takes focus deliberately: the editor's blur is
    // suspended for the sheet's lifetime (shared/mentions.ts).
    setTimeout(() => inputEl?.focus(), 50)
  })
</script>

<BottomSheet title={t('mention.title')} historyKey="mentionSheet" {onClose}>
  <input
    bind:this={inputEl}
    class="search"
    type="search"
    placeholder={t('mention.placeholder')}
    value={query}
    oninput={onInput}
    enterkeyhint="search"
    autocomplete="off"
  />
  {#if errorMsg}
    <p class="error">{errorMsg}</p>
  {/if}
  <ul class="results">
    {#if loading && results.length === 0}
      <li class="status">{t('common.loading')}</li>
    {:else if results.length === 0}
      <li class="status">{t('mention.no_results')}</li>
    {:else}
      {#each results as hit (hit.CardID)}
        <li>
          <button type="button" class="hit" onclick={() => pick(hit)}>
            <span class="title">{@html renderInline(hit.Title || t('inbox.untitled'))}</span>
            {#if hit.ProjectContext}<span class="crumb">{hit.ProjectContext}</span>{/if}
          </button>
        </li>
      {/each}
    {/if}
  </ul>
</BottomSheet>

<style>
  .search {
    width: 100%;
    box-sizing: border-box;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 10px;
    color: var(--text);
    font: inherit;
    font-size: 1rem;
    padding: 0.6rem 0.75rem;
  }
  .search:focus { outline: none; border-color: var(--accent); }
  .results {
    list-style: none;
    margin: 0;
    padding: 0;
    max-height: 50vh;
    overflow: auto;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  .hit {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.15rem;
    width: 100%;
    min-height: 44px;
    padding: 0.5rem 0.65rem;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: var(--bg);
    color: var(--text);
    font: inherit;
    text-align: left;
  }
  .hit:active { border-color: var(--accent); }
  .title { font-size: 0.95rem; font-weight: 500; }
  .crumb { font-size: 0.75rem; color: var(--text-faint); }
  .status { padding: 0.75rem 0.25rem; color: var(--text-faint); font-size: 0.9rem; }
  .error { margin: 0; color: var(--danger-text); font-size: 0.85rem; }
</style>
