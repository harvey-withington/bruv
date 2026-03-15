<script lang="ts">
  import SearchBar from './SearchBar.svelte'
  import ThemeToggle from './ThemeToggle.svelte'
  import { Settings, UserCircle } from 'lucide-svelte'
  import { nav } from '../lib/store.svelte'
  import { t } from '../lib/i18n.svelte'

  let { onSelectCard, onOpenPrefs, onOpenProfile }: {
    onSelectCard?: (cardId: string) => void
    onOpenPrefs?: () => void
    onOpenProfile?: () => void
  } = $props()
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
    <button class="icon-btn" onclick={onOpenPrefs} title={t('prefs.title')}><Settings size={16} /></button>
    <button class="icon-btn" onclick={onOpenProfile} title={t('profile.title')}><UserCircle size={16} /></button>
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
