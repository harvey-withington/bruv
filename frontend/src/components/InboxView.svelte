<script lang="ts">
  import { onMount } from 'svelte'
  import { Lightbulb } from 'lucide-svelte'
  import CardItem from './CardItem.svelte'
  import InboxRecentCards from './InboxRecentCards.svelte'
  import InboxActivity from './InboxActivity.svelte'
  import { board, search, inboxSearchFilters } from '../lib/store.svelte'
  import { ListActivityLog, ListRecentlyUpdatedCards, GetPreferences } from '../lib/api'
  import { t } from '../lib/i18n.svelte'
  import type { ActivityEntry, RecentCard } from '../lib/types'

  let {
    onNewIdea,
    onCardClick,
  }: {
    onNewIdea: () => void
    onCardClick: (cardId: string) => void
  } = $props()

  // Orphaned cards come from board store (populated by Sidebar.selectInbox)
  let orphanedCards = $derived(board.categories[0]?.cards || [])

  // Recently updated cards
  let recentCards = $state<RecentCard[]>([])
  let recentLoading = $state(false)

  // Activity log
  let activityEntries = $state<ActivityEntry[]>([])
  let activityLoading = $state(false)

  // Filter using the existing top-bar search query, respecting active filter fields
  function matchesCard(query: string, card: { title: string; type: string; tags: string[] }): boolean {
    const q = query.toLowerCase()
    return (
      (inboxSearchFilters.title && card.title.toLowerCase().includes(q)) ||
      (inboxSearchFilters.type && card.type.toLowerCase().includes(q)) ||
      (inboxSearchFilters.tags && card.tags.some(tag => tag.toLowerCase().includes(q)))
    )
  }

  let filteredOrphaned = $derived(
    search.query ? orphanedCards.filter(c => matchesCard(search.query, c)) : orphanedCards
  )
  let filteredRecent = $derived(
    search.query ? recentCards.filter(c => matchesCard(search.query, c)) : recentCards
  )
  let filteredActivity = $derived(
    search.query
      ? activityEntries.filter(e => {
          const q = search.query.toLowerCase()
          return (
            (inboxSearchFilters.title && (e.card_title || '').toLowerCase().includes(q)) ||
            (inboxSearchFilters.actor && e.actor.toLowerCase().includes(q)) ||
            (inboxSearchFilters.project && (e.project_name || '').toLowerCase().includes(q)) ||
            (inboxSearchFilters.project && (e.category_name || '').toLowerCase().includes(q))
          )
        })
      : activityEntries
  )

  async function loadRecentAndActivity() {
    let limit = 15
    try {
      const p = await GetPreferences()
      limit = p.inbox_recent_cards_limit || 15
    } catch { /* use default */ }

    recentLoading = true
    activityLoading = true
    try {
      const [recent, activity] = await Promise.all([
        ListRecentlyUpdatedCards(limit).catch(() => [] as RecentCard[]),
        ListActivityLog(50).catch(() => [] as ActivityEntry[]),
      ])
      recentCards = recent
      activityEntries = activity
    } finally {
      recentLoading = false
      activityLoading = false
    }
  }

  onMount(() => {
    loadRecentAndActivity()
    function handleInboxChanged() { loadRecentAndActivity() }
    document.addEventListener('bruv:inbox-changed', handleInboxChanged)
    return () => document.removeEventListener('bruv:inbox-changed', handleInboxChanged)
  })
</script>

<div class="inbox-view">
  <div class="inbox-columns">
    <!-- Column 1: Orphaned cards -->
    <section class="inbox-col" aria-label={t('board.inbox_orphaned')}>
      <h2 class="col-heading">{t('board.inbox_orphaned')}</h2>
      {#if filteredOrphaned.length === 0}
        <p class="col-empty">{search.query ? t('board.inbox_search_empty') : t('board.inbox_empty')}</p>
      {:else}
        <div class="cards-grid" role="list">
          {#each filteredOrphaned as card (card.id)}
            <div class="card-slot" role="listitem">
              <CardItem {card} categoryId="__inbox__" onclick={() => onCardClick(card.id)} />
            </div>
          {/each}
        </div>
      {/if}
    </section>

    <!-- Column 2: Recently updated -->
    <section class="inbox-col" aria-label={t('board.inbox_recent')}>
      <h2 class="col-heading">{t('board.inbox_recent')}</h2>
      {#if recentLoading}
        <p class="col-loading">{t('app.loading')}</p>
      {:else}
        <InboxRecentCards cards={filteredRecent} onCardClick={onCardClick} />
      {/if}
    </section>

    <!-- Column 3: Activity log (narrower) -->
    <section class="inbox-col" aria-label={t('board.inbox_activity')}>
      <h2 class="col-heading">{t('board.inbox_activity')}</h2>
      {#if activityLoading}
        <p class="col-loading">{t('app.loading')}</p>
      {:else}
        <InboxActivity entries={filteredActivity} onCardClick={onCardClick} />
      {/if}
    </section>
  </div>

  <!-- FAB: New Idea — top-right, overlaying content -->
  <button class="fab" onclick={onNewIdea} title={t('tooltip.new_idea')}>
    <Lightbulb size={22} />
  </button>
</div>

<style>
  .inbox-view {
    position: relative;
    height: 100%;
    overflow: hidden;
  }

  .inbox-columns {
    display: grid;
    grid-template-columns: 1fr 3fr 1fr;
    gap: 1.5rem;
    height: 100%;
    overflow: hidden;
  }

  .inbox-col {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    overflow-y: auto;
    overflow-x: hidden;
    padding-right: 0.75rem;
    min-width: 0;
    padding-bottom: 1rem;
  }

  .col-heading {
    margin: 0;
    font-size: 0.8rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-secondary);
    padding: 0 0.1rem;
    position: sticky;
    top: 0;
    background: var(--bg-base);
    z-index: 1;
    padding-top: 0.25rem;
    padding-bottom: 0.4rem;
  }

  .col-empty,
  .col-loading {
    color: var(--text-muted);
    font-size: 0.88rem;
    margin: 0;
  }

  .cards-grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: 0.5rem;
    align-content: start;
  }

  .card-slot {
    min-width: 0;
  }

  /* FAB — top-right corner of the inbox view, floating above content */
  .fab {
    position: absolute;
    top: 0;
    right: 0;
    width: 44px;
    height: 44px;
    border-radius: 50%;
    border: none;
    background: var(--accent);
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    box-shadow: 0 3px 12px var(--shadow-lg);
    transition: background 0.15s, transform 0.15s, box-shadow 0.15s;
    z-index: 10;
  }
  .fab:hover {
    background: var(--accent-hover, var(--accent));
    transform: scale(1.06);
    box-shadow: 0 5px 18px var(--shadow-lg);
  }
  .fab:active {
    transform: scale(0.97);
  }
</style>
