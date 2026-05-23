<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { Check, Trash2, MapPin, X, Search } from 'lucide-svelte'
  import { repoRPC } from '../lib/auth'
  import { navigate, cardURL } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
  import { repoMeta } from '../lib/repoMeta.svelte'
  import { getCardTypeColor, getCardTypeTextColor, getCardTypeLabel } from '@shared/cardTypes'
  import { onEvent } from '../lib/events.svelte'
  import { longPress } from '../lib/actions/longPress.svelte'
  import PinPicker from '../components/PinPicker.svelte'
  import ConfirmDialog from '../components/ConfirmDialog.svelte'
  import SearchSheet from '../components/SearchSheet.svelte'
  import type { CardSummary } from '../lib/model'
  import type { RecentCard } from '@shared/types'

  // Inbox = orphaned cards (created but not yet pinned to any
  // category). Long-press a card to enter selection mode for bulk
  // operations: pin many to a single category, or delete many at
  // once. Short-tap navigates as before.

  let cards = $state<CardSummary[]>([])
  let loading = $state(true)
  let errorMsg = $state<string | null>(null)

  // Tabs: orphaned cards waiting for triage, vs recently-updated
  // cards across the vault. Same bulk-select gestures apply to both.
  type Tab = 'inbox' | 'recent'
  let tab = $state<Tab>('inbox')
  let recents = $state<RecentCard[]>([])
  let recentsLoaded = $state(false)
  let recentsLoading = $state(false)

  // Selection mode.
  let selecting = $state(false)
  let selected = $state<Record<string, boolean>>({})
  let pinPickerOpen = $state(false)
  let confirmingDelete = $state(false)
  let busy = $state(false)
  let searchOpen = $state(false)

  // Active list (orphans on inbox tab, recents on recent tab) so the
  // bulk-select handlers operate on whichever the user is looking at.
  const activeList = $derived<Array<CardSummary | RecentCard>>(tab === 'inbox' ? cards : recents)

  const selectedCount = $derived(Object.values(selected).filter(Boolean).length)

  function isSelected(id: string): boolean {
    return selected[id] === true
  }

  function startSelectionWith(id: string) {
    selecting = true
    selected = { ...selected, [id]: true }
  }

  function toggleSelection(id: string) {
    selected = { ...selected, [id]: !selected[id] }
    // Auto-exit selection mode when the last card is deselected —
    // saves the user a tap on Cancel.
    const anyLeft = Object.values({ ...selected, [id]: !selected[id] }).some(Boolean)
    if (!anyLeft) selecting = false
  }

  function exitSelection() {
    selecting = false
    selected = {}
  }

  function tapRow(card: { id: string }) {
    if (selecting) {
      toggleSelection(card.id)
    } else {
      navigate(cardURL(card.id))
    }
  }

  function setTab(next: Tab) {
    if (tab === next) return
    if (selecting) exitSelection()
    tab = next
    if (next === 'recent' && !recentsLoaded) void loadRecents()
  }

  async function loadRecents() {
    recentsLoading = true
    try {
      recents = (await repoRPC<RecentCard[]>('ListRecentlyUpdatedCards', [50])) ?? []
      recentsLoaded = true
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('inbox.err_load')
    } finally {
      recentsLoading = false
    }
  }

  async function reload() {
    try {
      const ids = (await repoRPC<string[]>('ListOrphanedCardIDs')) ?? []
      const fetched = await Promise.all(
        ids.map(async (id) => {
          try {
            return await repoRPC<CardSummary>('GetCard', [id])
          } catch {
            return null
          }
        }),
      )
      cards = fetched.filter((c): c is CardSummary => c !== null)
      cards.sort((a, b) => (a.updated_at < b.updated_at ? 1 : -1))
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('inbox.err_load')
    } finally {
      loading = false
    }
  }

  onMount(reload)

  let liveReloadTimer: ReturnType<typeof setTimeout> | null = null
  const unsubscribeEvents = onEvent((ev) => {
    if (ev.topic === 'card:created' || ev.topic === 'card:updated' || ev.topic === 'card:deleted') {
      if (liveReloadTimer) clearTimeout(liveReloadTimer)
      liveReloadTimer = setTimeout(() => {
        liveReloadTimer = null
        void reload()
        if (recentsLoaded) void loadRecents()
      }, 150)
    }
  })
  onDestroy(() => {
    if (liveReloadTimer) clearTimeout(liveReloadTimer)
    unsubscribeEvents()
  })

  // --- Bulk actions ---

  async function bulkPin(sel: { project: { id: string }; category: { id: string } }) {
    pinPickerOpen = false
    busy = true
    const ids = Object.entries(selected).filter(([, v]) => v).map(([k]) => k)
    let failed = 0
    for (const id of ids) {
      try {
        await repoRPC('PinCard', [id, sel.category.id])
      } catch {
        failed++
      }
    }
    if (failed > 0) errorMsg = t('inbox.bulk_pin_failed', { n: failed })
    busy = false
    exitSelection()
    void reload()
  }

  async function performBulkDelete() {
    confirmingDelete = false
    busy = true
    const ids = Object.entries(selected).filter(([, v]) => v).map(([k]) => k)
    let failed = 0
    for (const id of ids) {
      try {
        await repoRPC('DeleteCard', [id])
      } catch {
        failed++
      }
    }
    if (failed > 0) errorMsg = t('inbox.bulk_delete_failed', { n: failed })
    busy = false
    exitSelection()
    void reload()
  }
</script>

<header class="topbar">
  {#if selecting}
    <button type="button" class="back" onclick={exitSelection} aria-label={t('common.cancel')}>
      <X size={18} />
    </button>
    <h1>{t('inbox.selected_n', { n: selectedCount })}</h1>
    <span class="spacer"></span>
  {:else}
    <button type="button" class="back" onclick={() => navigate('/')}>
      <span aria-hidden="true">‹</span> {t('common.back')}
    </button>
    <h1>{t('inbox.title')}</h1>
    <button type="button" class="topbar-search" onclick={() => (searchOpen = true)} aria-label={t('browse.search')} title={t('browse.search')}>
      <Search size={18} />
    </button>
  {/if}
</header>

<div class="tabs" role="tablist" aria-label={t('inbox.tabs')}>
  <button
    type="button"
    class="tab"
    class:active={tab === 'inbox'}
    role="tab"
    aria-selected={tab === 'inbox'}
    onclick={() => setTab('inbox')}
  >
    {t('inbox.tab_inbox')}{cards.length > 0 ? ` (${cards.length})` : ''}
  </button>
  <button
    type="button"
    class="tab"
    class:active={tab === 'recent'}
    role="tab"
    aria-selected={tab === 'recent'}
    onclick={() => setTab('recent')}
  >
    {t('inbox.tab_recent')}
  </button>
</div>

<main class:has-bar={selecting}>
  {#if tab === 'inbox'}
    {#if loading}
      <p class="status">{t('common.loading')}</p>
    {:else if errorMsg && cards.length === 0}
      <p class="error">{errorMsg}</p>
    {:else if cards.length === 0}
      <div class="empty">
        <h2>{t('inbox.empty_title')}</h2>
        <p>{t('inbox.empty_body')}</p>
      </div>
    {:else}
      {#if !selecting}
        <p class="hint">{t('inbox.long_press_hint')}</p>
      {/if}
      <ul class="cards">
        {#each cards as card (card.id)}
          <li>
            <button
              type="button"
              class="card-row"
              class:selected={isSelected(card.id)}
              style:view-transition-name={`card-${card.id}`}
              onclick={() => tapRow(card)}
              use:longPress={() => startSelectionWith(card.id)}
            >
              {#if selecting}
                <span class="checkbox" class:checked={isSelected(card.id)} aria-hidden="true">
                  {#if isSelected(card.id)}<Check size={14} />{/if}
                </span>
              {/if}
              <div class="card-main">
                <span class="card-title">{card.title || t('inbox.untitled')}</span>
                {#if card.tags?.length}
                  <div class="tags">
                    {#each card.tags as tag}
                      <span class="tag" style:background={repoMeta.tagColor(tag)}>{tag}</span>
                    {/each}
                  </div>
                {/if}
              </div>
              {#if card.type}
                <span
                  class="card-type"
                  style:background={getCardTypeColor(card.type, repoMeta.cardTypes)}
                  style:color={getCardTypeTextColor(card.type)}
                >
                  {getCardTypeLabel(card.type, repoMeta.cardTypes)}
                </span>
              {/if}
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  {:else}
    {#if recentsLoading && recents.length === 0}
      <p class="status">{t('common.loading')}</p>
    {:else if recents.length === 0}
      <div class="empty">
        <h2>{t('inbox.recent_empty_title')}</h2>
        <p>{t('inbox.recent_empty_body')}</p>
      </div>
    {:else}
      {#if !selecting}
        <p class="hint">{t('inbox.long_press_hint')}</p>
      {/if}
      <ul class="cards">
        {#each recents as card (card.id)}
          <li>
            <button
              type="button"
              class="card-row"
              class:selected={isSelected(card.id)}
              onclick={() => tapRow(card)}
              use:longPress={() => startSelectionWith(card.id)}
            >
              {#if selecting}
                <span class="checkbox" class:checked={isSelected(card.id)} aria-hidden="true">
                  {#if isSelected(card.id)}<Check size={14} />{/if}
                </span>
              {/if}
              <div class="card-main">
                <span class="card-title">{card.title || t('inbox.untitled')}</span>
                {#if card.breadcrumb}
                  <span class="card-breadcrumb">{card.breadcrumb}</span>
                {/if}
                {#if card.tags?.length}
                  <div class="tags">
                    {#each card.tags as tag}
                      <span class="tag" style:background={repoMeta.tagColor(tag)}>{tag}</span>
                    {/each}
                  </div>
                {/if}
              </div>
              {#if card.type}
                <span
                  class="card-type"
                  style:background={getCardTypeColor(card.type, repoMeta.cardTypes)}
                  style:color={getCardTypeTextColor(card.type)}
                >
                  {getCardTypeLabel(card.type, repoMeta.cardTypes)}
                </span>
              {/if}
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}

  {#if errorMsg && activeList.length > 0}
    <p class="error inline">{errorMsg}</p>
  {/if}
</main>

{#if selecting && selectedCount > 0}
  <div class="action-bar" role="toolbar" aria-label={t('inbox.bulk_actions')}>
    <button type="button" class="bar-btn" onclick={() => (pinPickerOpen = true)} disabled={busy}>
      <MapPin size={16} />
      {t('inbox.bulk_pin')}
    </button>
    <button type="button" class="bar-btn danger" onclick={() => (confirmingDelete = true)} disabled={busy}>
      <Trash2 size={16} />
      {t('inbox.bulk_delete')}
    </button>
  </div>
{/if}

{#if pinPickerOpen}
  <PinPicker onSelect={bulkPin} onClose={() => (pinPickerOpen = false)} />
{/if}

{#if confirmingDelete}
  <ConfirmDialog
    title={t('inbox.bulk_delete_title')}
    body={t('inbox.bulk_delete_body', { n: selectedCount })}
    confirmLabel={t('inbox.bulk_delete')}
    destructive
    onConfirm={performBulkDelete}
    onCancel={() => (confirmingDelete = false)}
  />
{/if}

{#if searchOpen}
  <SearchSheet onClose={() => (searchOpen = false)} />
{/if}

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
  .topbar h1 {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
    color: var(--text);
    text-align: center;
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
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
  }
  .back:hover,
  .back:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }
  .topbar-search {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 8px;
    justify-self: end;
    min-width: 40px;
    min-height: 40px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .topbar-search:hover,
  .topbar-search:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }

  main {
    padding: 0.75rem 0.85rem 4rem;
    max-width: 600px;
    margin: 0 auto;
  }
  main.has-bar {
    padding-bottom: calc(5rem + env(safe-area-inset-bottom));
  }

  .tabs {
    display: flex;
    gap: 0.25rem;
    padding: 0.5rem 0.85rem 0;
    max-width: 600px;
    margin: 0 auto;
    border-bottom: 1px solid var(--border);
  }
  .tab {
    background: transparent;
    border: none;
    border-bottom: 2px solid transparent;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.85rem;
    font-weight: 500;
    padding: 0.55rem 0.9rem;
    cursor: pointer;
    margin-bottom: -1px;
    touch-action: manipulation;
  }
  .tab:hover,
  .tab:focus-visible {
    color: var(--text);
    outline: none;
  }
  .tab.active {
    color: var(--accent);
    border-bottom-color: var(--accent);
  }

  .card-breadcrumb {
    font-size: 0.72rem;
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .hint {
    color: var(--text-faint);
    font-size: 0.78rem;
    text-align: center;
    margin: 0 0 0.6rem;
    font-style: italic;
  }

  .cards {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .card-row {
    width: 100%;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0.85rem 1rem;
    color: var(--text);
    font: inherit;
    cursor: pointer;
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    text-align: left;
    transition: border-color 120ms ease, background 120ms ease;
    -webkit-user-select: none;
    user-select: none;
    -webkit-touch-callout: none;
    touch-action: manipulation;
  }
  .card-row:hover,
  .card-row:focus-visible {
    border-color: var(--accent);
    outline: none;
  }
  .card-row.selected {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, var(--bg-elev-1));
  }

  .checkbox {
    width: 22px;
    height: 22px;
    border-radius: 50%;
    border: 2px solid var(--text-muted);
    background: var(--bg);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    margin-top: 2px;
    color: #fff;
    transition: background 120ms ease, border-color 120ms ease;
  }
  .checkbox.checked {
    background: var(--accent);
    border-color: var(--accent);
  }

  .card-main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }
  .card-title {
    font-size: 0.95rem;
    font-weight: 500;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
  }
  .tag {
    font-size: 0.7rem;
    color: #fff;
    padding: 0.1rem 0.45rem;
    border-radius: 4px;
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

  .empty {
    text-align: center;
    margin-top: 4rem;
    color: var(--text-muted);
  }
  .empty h2 {
    margin: 0 0 0.5rem;
    font-size: 1.1rem;
    color: var(--text);
  }
  .empty p {
    margin: 0;
    font-size: 0.9rem;
    line-height: 1.5;
  }

  .status {
    color: var(--text-muted);
    text-align: center;
    margin: 2rem 0;
    font-size: 0.95rem;
  }
  .error {
    margin: 2rem 0;
    padding: 1rem;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 8px;
    color: #fca5a5;
    text-align: center;
    font-size: 0.9rem;
  }
  .error.inline {
    margin: 1rem 0 0;
    font-size: 0.85rem;
  }

  .action-bar {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.5rem;
    padding: 0.65rem 0.85rem calc(0.65rem + env(safe-area-inset-bottom));
    background: var(--bg);
    border-top: 1px solid var(--border);
    z-index: 50;
  }
  .bar-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.4rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    color: var(--text);
    border-radius: 10px;
    padding: 0.75rem 0.5rem;
    font: inherit;
    font-size: 0.9rem;
    font-weight: 500;
    cursor: pointer;
    min-height: 48px;
  }
  .bar-btn:hover,
  .bar-btn:focus-visible {
    border-color: var(--accent);
    outline: none;
  }
  .bar-btn.danger {
    color: #ef4444;
  }
  .bar-btn.danger:hover,
  .bar-btn.danger:focus-visible {
    border-color: #ef4444;
    background: color-mix(in srgb, #ef4444 8%, var(--bg-elev-1));
  }
  .bar-btn:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
