<script lang="ts">
  import SearchBar from './SearchBar.svelte'
  import CardTypesTab from './CardTypesTab.svelte'
  import { Tags, SlidersHorizontal, BotMessageSquare, Inbox, Layers } from 'lucide-svelte'
  import { nav } from '../lib/store.svelte'
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
    <SearchBar {onSelectCard} />
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
