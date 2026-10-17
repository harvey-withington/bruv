<script lang="ts">
  // The offline view: the cards kept on this device. Each opens in the
  // normal card page, answered from the device's copy. Back online, edits
  // sync on their own; "Back to browse" returns to the live app.
  import { ChevronLeft, CloudOff, RefreshCw } from 'lucide-svelte'
  import { offline } from '../lib/offline.svelte'
  import { connectivity, retryNow } from '../lib/connectivity.svelte'
  import { navigate, replace, cardURL } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
  import { offlineStatusText, formatBytes, placesText } from '@shared/offline/labels'

  const totalBytes = $derived(offline.cards.reduce((n, c) => n + c.bytes, 0))
  const allSynced = $derived(offline.cards.every((c) => c.status === 'synced'))
</script>

<header class="topbar">
  <button type="button" class="back" onclick={() => (offline.working ? history.back() : replace('/'))} aria-label={t('common.back')}>
    <ChevronLeft size={20} />
  </button>
  <h1>{offline.working ? t('offline.view_title') : t('offline.view_title_online')}</h1>
  <span class="spacer"></span>
</header>

<main>
  <section class="summary" role="status">
    <CloudOff size={18} />
    <p>
      {#if offline.working}
        {t('offline.view_subtitle_mobile', { count: offline.cards.length, size: formatBytes(totalBytes) })}
      {:else if offline.syncing}
        {t('offline.view_syncing_mobile')}
      {:else if allSynced}
        {t('offline.view_synced_mobile')}
      {:else}
        {t('offline.view_needs_attention')}
      {/if}
    </p>
  </section>

  <div class="actions">
    {#if offline.working}
      <button type="button" class="action" onclick={() => void retryNow()} disabled={connectivity.checking}>
        <RefreshCw size={16} /> {connectivity.checking ? t('offline.reconnecting') : t('offline.try_connection')}
      </button>
    {:else}
      <button type="button" class="action primary" onclick={() => replace('/')} disabled={offline.syncing}>
        {t('offline.back_to_browse')}
      </button>
    {/if}
  </div>

  {#if offline.cards.length === 0}
    <p class="status">{t('offline.view_empty')}</p>
  {:else}
    <ul class="cards">
      {#each offline.cards as c (c.cardId)}
        <li>
          <button type="button" class="card" onclick={() => navigate(cardURL(c.cardId), { fromOffline: true })}>
            <span class="title">{c.title}</span>
            {#if c.places}
              <span class="place">{placesText(c.places, t)}</span>
            {/if}
            <span class="state" class:pending={c.status === 'pending'} class:warn={c.status === 'conflict' || c.status === 'error'}>
              {offlineStatusText(c, t, offline.working)}
            </span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</main>

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
  .topbar h1 { margin: 0; font-size: 1rem; font-weight: 600; color: var(--text); text-align: center; }
  .back {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.4rem;
    border-radius: 6px;
    justify-self: start;
    display: inline-flex;
  }
  .back:hover, .back:focus-visible { color: var(--text); background: var(--bg-elev-1); outline: none; }
  main { padding: 0.75rem 0.85rem 4rem; max-width: 600px; margin: 0 auto; }
  .summary {
    display: flex;
    align-items: flex-start;
    gap: 0.6rem;
    color: var(--text-muted);
    font-size: 0.9rem;
    line-height: 1.45;
  }
  .summary p { margin: 0; }
  .actions { margin: 0.85rem 0 1rem; }
  .action {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    min-height: 44px;
    padding: 0 1rem;
    border-radius: 10px;
    border: 1px solid var(--border);
    background: var(--bg-elev-1);
    color: var(--text);
    font: inherit;
    cursor: pointer;
  }
  .action.primary { border-color: var(--accent); color: var(--accent); font-weight: 600; }
  .action:disabled { opacity: 0.6; cursor: default; }
  .status { color: var(--text-muted); text-align: center; margin-top: 2rem; }
  .cards { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 0.4rem; }
  .card {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.2rem;
    width: 100%;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0.75rem 0.85rem;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
    touch-action: manipulation;
  }
  .card:hover, .card:focus-visible { border-color: var(--accent); outline: none; }
  .title { font-weight: 500; overflow-wrap: anywhere; }
  .place { font-size: 0.8rem; color: var(--text-muted); overflow-wrap: anywhere; white-space: pre-line; }
  .state { font-size: 0.8rem; color: var(--text-muted); }
  .state.pending { color: var(--accent); }
  .state.warn { color: var(--warn-text); }
</style>
