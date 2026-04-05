<script lang="ts">
  import SearchBar from './SearchBar.svelte'
  import CardTypesTab from './CardTypesTab.svelte'
  import { Tags, SlidersHorizontal, BotMessageSquare, Inbox, Layers, Type, User, FolderOpen, Check } from 'lucide-svelte'
  import { nav, inboxSearchFilters, boardSearchFilters } from '../lib/store.svelte'
  import { t } from '../lib/i18n.svelte'

  let { onSelectCard, onOpenTagEditor, onOpenProjectSettings, onCreateAIChat }: {
    onSelectCard?: (cardId: string) => void
    onOpenTagEditor?: () => void
    onOpenProjectSettings?: () => void
    onCreateAIChat?: () => void
  } = $props()

  let showCardTypes = $state(false)
</script>

<header class="topbar">
  <div class="breadcrumb">
    {#if nav.inboxMode}
      <span class="crumb active inbox-crumb"><Inbox size={14} /> {t('sidebar.inbox')}</span>
    {:else if nav.brandSlug}
      <span class="crumb">{nav.brandName || nav.brandSlug}</span>
      {#if nav.streamSlug}
        <span class="sep">›</span>
        <span class="crumb">{nav.streamName || nav.streamSlug}</span>
        {#if nav.projectSlug}
          <span class="sep">›</span>
          <span class="crumb active">{nav.projectName || nav.projectSlug}</span>
        {/if}
      {/if}
    {:else}
      <span class="crumb empty">{t('app.no_project_breadcrumb')}</span>
    {/if}
  </div>

  <div class="topbar-center">
    {#if nav.inboxMode}
      <div class="search-filter-group">
        <SearchBar {onSelectCard} grouped />
        <div class="inbox-filters">
        <button
          class="filter-btn"
          class:active={inboxSearchFilters.title}
          onclick={() => inboxSearchFilters.title = !inboxSearchFilters.title}
          title={t('inbox.filter_title')}
        >
          <Type size={13} />
          {#if inboxSearchFilters.title}<span class="check"><Check size={10} strokeWidth={3} /></span>{/if}
        </button>
        <button
          class="filter-btn"
          class:active={inboxSearchFilters.type}
          onclick={() => inboxSearchFilters.type = !inboxSearchFilters.type}
          title={t('inbox.filter_type')}
        >
          <Layers size={13} />
          {#if inboxSearchFilters.type}<span class="check"><Check size={10} strokeWidth={3} /></span>{/if}
        </button>
        <button
          class="filter-btn"
          class:active={inboxSearchFilters.tags}
          onclick={() => inboxSearchFilters.tags = !inboxSearchFilters.tags}
          title={t('inbox.filter_tags')}
        >
          <Tags size={13} />
          {#if inboxSearchFilters.tags}<span class="check"><Check size={10} strokeWidth={3} /></span>{/if}
        </button>
        <button
          class="filter-btn"
          class:active={inboxSearchFilters.actor}
          onclick={() => inboxSearchFilters.actor = !inboxSearchFilters.actor}
          title={t('inbox.filter_actor')}
        >
          <User size={13} />
          {#if inboxSearchFilters.actor}<span class="check"><Check size={10} strokeWidth={3} /></span>{/if}
        </button>
        <button
          class="filter-btn"
          class:active={inboxSearchFilters.project}
          onclick={() => inboxSearchFilters.project = !inboxSearchFilters.project}
          title={t('inbox.filter_project')}
        >
          <FolderOpen size={13} />
          {#if inboxSearchFilters.project}<span class="check"><Check size={10} strokeWidth={3} /></span>{/if}
        </button>
        </div>
      </div>
    {:else if nav.projectSlug}
      <div class="search-filter-group">
        <SearchBar {onSelectCard} grouped />
        <div class="board-filters">
          <button
            class="filter-btn"
            class:active={boardSearchFilters.title}
            onclick={() => boardSearchFilters.title = !boardSearchFilters.title}
            title={t('board.filter_title')}
          >
            <Type size={13} />
            {#if boardSearchFilters.title}<span class="check"><Check size={10} strokeWidth={3} /></span>{/if}
          </button>
          <button
            class="filter-btn"
            class:active={boardSearchFilters.type}
            onclick={() => boardSearchFilters.type = !boardSearchFilters.type}
            title={t('board.filter_type')}
          >
            <Layers size={13} />
            {#if boardSearchFilters.type}<span class="check"><Check size={10} strokeWidth={3} /></span>{/if}
          </button>
          <button
            class="filter-btn"
            class:active={boardSearchFilters.tags}
            onclick={() => boardSearchFilters.tags = !boardSearchFilters.tags}
            title={t('board.filter_tags')}
          >
            <Tags size={13} />
            {#if boardSearchFilters.tags}<span class="check"><Check size={10} strokeWidth={3} /></span>{/if}
          </button>
        </div>
      </div>
    {:else}
      <SearchBar {onSelectCard} />
    {/if}
  </div>

  <div class="topbar-actions">
    <button class="icon-btn" onclick={() => showCardTypes = true} title={t('toolbar.card_types')}><Layers size={16} /></button>
    {#if nav.projectSlug}
      <button class="icon-btn" onclick={onCreateAIChat} title={t('toolbar.ai_chat')}><BotMessageSquare size={16} /></button>
      <button class="icon-btn" onclick={onOpenTagEditor} title={t('toolbar.tags')}><Tags size={16} /></button>
      <button class="icon-btn" onclick={onOpenProjectSettings} title={t('toolbar.project_settings')}><SlidersHorizontal size={16} /></button>
    {/if}
  </div>
</header>

{#if showCardTypes}
  <CardTypesTab onClose={() => showCardTypes = false} />
{/if}

<style>
  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.5rem 1rem;
    background: var(--bg-base);
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
  }

  .topbar-center {
    flex: 1;
    display: flex;
    justify-content: center;
    padding: 0 1rem;
    min-width: 0;
  }

  .search-filter-group {
    display: flex;
    align-items: center;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-elevated);
    overflow: hidden;
    transition: border-color 0.15s;
  }
  .search-filter-group:focus-within {
    border-color: var(--accent);
  }

  .inbox-filters,
  .board-filters {
    display: flex;
    align-items: center;
    border-left: 1px solid var(--border);
  }

  .filter-btn {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border: none;
    border-right: 1px solid var(--border);
    background: none;
    color: var(--text-faint);
    cursor: pointer;
    transition: color 0.15s, background 0.15s;
    flex-shrink: 0;
  }
  .filter-btn:last-child {
    border-right: none;
  }
  .filter-btn:hover {
    color: var(--text-muted);
    background: var(--bg-subtle-hover);
  }
  .filter-btn.active {
    color: var(--text-secondary);
  }

  .check {
    position: absolute;
    bottom: 2px;
    right: 2px;
    display: flex;
    color: #4caf7d;
    line-height: 1;
  }

  .topbar-actions {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    flex-shrink: 0;
  }

  .breadcrumb {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.8rem;
    flex-shrink: 0;
  }

  .crumb {
    color: var(--text-muted);
  }

  .crumb.active {
    color: var(--text-strong);
    font-weight: 500;
  }

  .crumb.empty {
    color: var(--text-faint);
    font-style: italic;
  }

  .inbox-crumb {
    display: flex;
    align-items: center;
    gap: 0.3rem;
  }

  .sep {
    color: var(--text-faint);
  }

  .icon-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.35rem;
    border-radius: 6px;
    display: flex;
    align-items: center;
    transition: color 0.15s, background 0.15s;
  }
  .icon-btn:hover {
    color: var(--text-primary);
    background: var(--bg-subtle-hover);
  }
</style>
