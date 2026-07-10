<script lang="ts">
  import { onMount } from 'svelte'
  import { Lightbulb, SquareCheckBig, Trash2 } from 'lucide-svelte'
  import CardItem from './CardItem.svelte'
  import InboxRecentCards from './InboxRecentCards.svelte'
  import InboxActivity from './InboxActivity.svelte'
  import { board, search, inboxSearchFilters } from '../lib/store.svelte'
  import { ListActivityLog, ListRecentlyUpdatedCards, GetUIPreferences, DeleteCard } from '@shared/api'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { t } from '../lib/i18n.svelte'
  import type { ActivityEntry, RecentCard } from '@shared/types'

  let {
    onNewIdea,
    onCardClick,
  }: {
    onNewIdea: () => void
    onCardClick: (cardId: string) => void
  } = $props()

  // Multi-select state
  let selectMode = $state(false)
  let selectedIds = $state(new Set<string>())
  let deleting = $state(false)

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
    let recentLimit = 21
    let activityLimit = 25
    try {
      const p = await GetUIPreferences()
      recentLimit = p.inbox_recent_cards_limit || 21
      activityLimit = p.inbox_activity_limit || 25
    } catch { /* use defaults */ }

    recentLoading = true
    activityLoading = true
    try {
      const [recent, activity] = await Promise.all([
        ListRecentlyUpdatedCards(recentLimit).catch(() => [] as RecentCard[]),
        ListActivityLog(activityLimit).catch(() => [] as ActivityEntry[]),
      ])
      recentCards = recent
      activityEntries = activity
    } finally {
      recentLoading = false
      activityLoading = false
    }
  }

  function toggleSelectMode() {
    selectMode = !selectMode
    if (!selectMode) selectedIds = new Set()
  }

  function toggleCard(id: string) {
    const next = new Set(selectedIds)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    selectedIds = next
  }

  function toggleSelectAll() {
    const ids = filteredOrphaned.map(c => c.id)
    const allSelected = ids.length > 0 && ids.every(id => selectedIds.has(id))
    if (allSelected) {
      selectedIds = new Set()
    } else {
      selectedIds = new Set(ids)
    }
  }

  let allSelected = $derived(
    filteredOrphaned.length > 0 && filteredOrphaned.every(c => selectedIds.has(c.id))
  )

  async function bulkDelete() {
    if (selectedIds.size === 0) return
    const count = selectedIds.size
    const msg = t('inbox.confirm_bulk_delete', { count })
    if (!await showConfirm(msg)) return
    deleting = true
    try {
      for (const id of selectedIds) {
        await DeleteCard(id)
      }
      selectedIds = new Set()
      selectMode = false
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
      document.dispatchEvent(new CustomEvent('bruv:sidebar-changed'))
      showToast(t('inbox.bulk_deleted', { count }), 'success')
    } catch (e) {
      showToast(t('error.delete_failed'), 'error')
      console.error('Bulk delete failed:', e)
    } finally {
      deleting = false
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
  {#if search.query}
    <div class="filter-bar">
      <span class="filter-summary">
        {t('inbox.filter_showing', { cards: String(filteredRecent.length), totalCards: String(recentCards.length), activity: String(filteredActivity.length), totalActivity: String(activityEntries.length) })}
      </span>
      <button class="filter-clear" onclick={() => { search.query = '' }}>
        {t('inbox.filter_clear')}
      </button>
    </div>
  {/if}
  <div class="inbox-columns">
    <!-- Column 1: Orphaned cards -->
    <section class="inbox-col" aria-label={t('board.inbox_orphaned')}>
      <div class="col-heading-row">
        <h2 class="col-heading">{t('board.inbox_orphaned')}</h2>
        {#if filteredOrphaned.length > 0}
          <button class="select-toggle" class:active={selectMode} onclick={toggleSelectMode} title={t('inbox.select')}>
            <SquareCheckBig size={13} />
          </button>
        {/if}
      </div>
      {#if selectMode}
        <div class="select-toolbar">
          <label class="select-all-label">
            <input type="checkbox" checked={allSelected} onchange={toggleSelectAll} />
            {t('inbox.select_all')}
          </label>
          {#if selectedIds.size > 0}
            <button class="bulk-delete-btn" onclick={bulkDelete} disabled={deleting}>
              <Trash2 size={12} />
              {t('inbox.delete_selected', { count: selectedIds.size })}
            </button>
          {/if}
        </div>
      {/if}
      {#if filteredOrphaned.length === 0}
        <p class="col-empty">{search.query ? t('board.inbox_search_empty') : t('board.inbox_empty')}</p>
      {:else}
        <div class="cards-grid" role="list">
          {#each filteredOrphaned as card (card.id)}
            <div class="card-slot" class:selected={selectMode && selectedIds.has(card.id)} role="listitem">
              {#if selectMode}
                <input type="checkbox" class="card-checkbox" checked={selectedIds.has(card.id)} onchange={() => toggleCard(card.id)} />
              {/if}
              <CardItem {card} categoryId="__inbox__" onclick={() => selectMode ? toggleCard(card.id) : onCardClick(card.id)} />
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
      {:else if filteredRecent.length === 0 && search.query}
        <div class="col-empty-filtered">
          <p>{t('inbox.no_recent_match', { query: search.query })}</p>
          <button class="clear-search-btn" onclick={() => { search.query = '' }}>{t('inbox.filter_clear')}</button>
        </div>
      {:else}
        <InboxRecentCards cards={filteredRecent} onCardClick={onCardClick} />
      {/if}
    </section>

    <!-- Column 3: Activity log (narrower) -->
    <section class="inbox-col" aria-label={t('board.inbox_activity')}>
      <h2 class="col-heading">{t('board.inbox_activity')}</h2>
      {#if activityLoading}
        <p class="col-loading">{t('app.loading')}</p>
      {:else if filteredActivity.length === 0 && search.query}
        <div class="col-empty-filtered">
          <p>{t('inbox.no_activity_match', { query: search.query })}</p>
          <button class="clear-search-btn" onclick={() => { search.query = '' }}>{t('inbox.filter_clear')}</button>
        </div>
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
    display: flex;
    flex-direction: column;
  }

  .filter-bar {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.35rem 1rem;
    background: color-mix(in srgb, var(--accent) 8%, var(--bg-elevated));
    border-bottom: 1px solid color-mix(in srgb, var(--accent) 20%, var(--border-muted));
    flex-shrink: 0;
  }
  .filter-summary {
    font-size: 0.75rem;
    color: var(--text-muted);
  }
  .filter-clear {
    font-size: 0.72rem;
    padding: 0.1rem 0.5rem;
    border: 1px solid var(--border-muted);
    border-radius: 4px;
    background: var(--bg-surface);
    color: var(--accent);
    cursor: pointer;
    transition: background 0.15s;
  }
  .filter-clear:hover {
    background: var(--accent);
    color: white;
    border-color: var(--accent);
  }

  .col-empty-filtered {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.4rem;
    padding: 1.5rem 0.5rem;
    text-align: center;
  }
  .col-empty-filtered p {
    font-size: 0.8rem;
    color: var(--text-muted);
    margin: 0;
  }
  .clear-search-btn {
    font-size: 0.72rem;
    padding: 0.2rem 0.6rem;
    border: 1px solid var(--border-muted);
    border-radius: 4px;
    background: var(--bg-surface);
    color: var(--accent);
    cursor: pointer;
    transition: background 0.15s;
  }
  .clear-search-btn:hover {
    background: var(--accent);
    color: white;
    border-color: var(--accent);
  }

  .inbox-columns {
    display: grid;
    grid-template-columns: 1fr 3fr 1fr;
    gap: 0.75rem;
    flex: 1;
    overflow: hidden;
  }

  .inbox-col {
    display: flex;
    flex-direction: column;
    overflow-y: auto;
    overflow-x: hidden;
    padding-right: 0.35rem;
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
    animation: fade-in var(--duration-moderate) var(--ease-out);
  }

  .col-heading-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    position: sticky;
    top: 0;
    background: var(--bg-base);
    z-index: 1;
    padding-top: 0.25rem;
    padding-bottom: 0.4rem;
  }

  .col-heading-row .col-heading {
    position: static;
    padding: 0;
  }

  .select-toggle {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 3px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: none;
    color: var(--text-muted);
    cursor: pointer;
    transition: color 0.15s, border-color 0.15s, background 0.15s;
  }
  .select-toggle:hover {
    color: var(--text-primary);
    border-color: var(--accent);
  }
  .select-toggle.active {
    color: var(--accent);
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }

  .select-toolbar {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.25rem 0;
  }

  .select-all-label {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.75rem;
    color: var(--text-muted);
    cursor: pointer;
  }
  .select-all-label input {
    accent-color: var(--accent);
    cursor: pointer;
  }

  .bulk-delete-btn {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 3px 8px;
    border-radius: 4px;
    font-size: 0.72rem;
    font-weight: 500;
    cursor: pointer;
    border: 1px solid var(--danger, #ef4444);
    background: color-mix(in srgb, var(--danger, #ef4444) 8%, var(--bg-elevated));
    color: var(--danger, #ef4444);
    transition: background 0.15s;
  }
  .bulk-delete-btn:hover:not(:disabled) {
    background: color-mix(in srgb, var(--danger, #ef4444) 18%, var(--bg-elevated));
  }
  .bulk-delete-btn:disabled {
    opacity: 0.4;
    cursor: default;
  }

  .cards-grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: 0.5rem;
    align-content: start;
  }

  .card-slot {
    min-width: 0;
    display: flex;
    align-items: stretch;
    gap: 0.35rem;
  }
  .card-slot.selected {
    background: color-mix(in srgb, var(--accent) 8%, transparent);
    border-radius: 6px;
  }

  .card-checkbox {
    flex-shrink: 0;
    accent-color: var(--accent);
    cursor: pointer;
    margin-left: 2px;
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
