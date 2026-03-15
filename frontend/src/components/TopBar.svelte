<script lang="ts">
  import SearchBar from './SearchBar.svelte'
  import ThemeToggle from './ThemeToggle.svelte'
  import { nav } from '../lib/store.svelte'
  import { t } from '../lib/i18n.svelte'

  let { onSelectCard }: { onSelectCard?: (cardId: string) => void } = $props()
</script>

<header class="topbar">
  <div class="breadcrumb">
    {#if nav.brandSlug}
      <span class="crumb">{nav.brandSlug}</span>
      {#if nav.streamSlug}
        <span class="sep">/</span>
        <span class="crumb">{nav.streamSlug}</span>
        {#if nav.projectSlug}
          <span class="sep">/</span>
          <span class="crumb active">{nav.projectSlug}</span>
        {/if}
      {/if}
    {:else}
      <span class="crumb empty">{t('app.no_project_breadcrumb')}</span>
    {/if}
  </div>

  <div class="topbar-actions">
    <SearchBar {onSelectCard} />
    <ThemeToggle />
  </div>
</header>

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

  .topbar-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .breadcrumb {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.8rem;
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

  .sep {
    color: var(--text-faint);
  }
</style>
