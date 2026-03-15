<script lang="ts">
  import SearchBar from './SearchBar.svelte'
  import { nav } from '../lib/store.svelte'

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
      <span class="crumb empty">No project selected</span>
    {/if}
  </div>

  <SearchBar {onSelectCard} />
</header>

<style>
  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.5rem 1rem;
    background: #18181b;
    border-bottom: 1px solid #2e2e32;
    flex-shrink: 0;
  }

  .breadcrumb {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.8rem;
  }

  .crumb {
    color: #71717a;
  }

  .crumb.active {
    color: #e4e4e7;
    font-weight: 500;
  }

  .crumb.empty {
    color: #3f3f46;
    font-style: italic;
  }

  .sep {
    color: #3f3f46;
  }
</style>
