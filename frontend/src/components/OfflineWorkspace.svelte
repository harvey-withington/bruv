<script lang="ts">
  // The offline view: the cards kept on this device, each opening in the
  // usual card dialog (answered from the device's copy). It stays up after
  // the connection returns — edits sync on their own and the user goes
  // back to their projects when ready, which reloads so every view starts from
  // the server again.
  import { CloudOff, RefreshCw, ArrowLeft } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { offline } from '../lib/offline.svelte'
  import { connectivity, retryNow } from '../lib/connectivity.svelte'
  import { activeConnectionLabel } from '../lib/connections.svelte'
  import { offlineStatusText, formatBytes, placesText } from '@shared/offline/labels'
  import CardDetail from './CardDetail.svelte'
  import NewOfflineCard from './NewOfflineCard.svelte'

  let openCardId = $state<string | null>(null)
  const server = $derived(activeConnectionLabel())
  const totalBytes = $derived(offline.cards.reduce((n, c) => n + c.bytes, 0))
  const allSynced = $derived(offline.cards.every((c) => c.status === 'synced'))

  function backToBoard() {
    window.location.reload()
  }
</script>

<div class="offline-workspace">
  <header class="ow-header">
    <CloudOff size={20} />
    <div class="ow-heading">
      <h1>{offline.working ? t('offline.view_title') : t('offline.view_title_online')}</h1>
      <p>
        {#if offline.working}
          {t('offline.view_subtitle', { count: offline.cards.length, server, size: formatBytes(totalBytes) })}
        {:else if offline.syncing}
          {t('offline.view_syncing', { server })}
        {:else if allSynced}
          {t('offline.view_synced', { server })}
        {:else}
          {t('offline.view_needs_attention')}
        {/if}
      </p>
    </div>
    <div class="ow-actions">
      {#if offline.working}
        <button class="btn" onclick={() => void retryNow()} disabled={connectivity.checking}>
          <RefreshCw size={14} /> {connectivity.checking ? t('connection.reconnecting') : t('offline.try_connection')}
        </button>
      {:else}
        <button class="btn primary" onclick={backToBoard} disabled={offline.syncing}>
          <ArrowLeft size={14} /> {t('offline.back_to_projects')}
        </button>
      {/if}
    </div>
  </header>

  {#if offline.working}
    <NewOfflineCard onCreated={(card) => openCardId = card.id} />
  {/if}

  {#if offline.cards.length === 0}
    <p class="ow-empty">{t('offline.view_empty')}</p>
  {:else}
    <ul class="ow-list">
      {#each offline.cards as c (c.cardId)}
        <li>
          <button class="ow-row" onclick={() => openCardId = c.cardId}>
            <span class="ow-text">
              <span class="ow-title">{c.title}</span>
              {#if c.places}
                <span class="ow-place">{placesText(c.places, t)}</span>
              {/if}
            </span>
            <span class="ow-status" class:warn={c.status === 'conflict' || c.status === 'error'} class:pending={c.status === 'pending'}>
              {offlineStatusText(c, t, offline.working)}
            </span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

{#if openCardId}
  <CardDetail cardId={openCardId} onClose={() => openCardId = null} />
{/if}

<style>
  .offline-workspace {
    height: 100vh;
    overflow-y: auto;
    background: var(--bg-base);
    padding: 2rem clamp(1rem, 6vw, 4rem);
    box-sizing: border-box;
  }
  .ow-header {
    display: flex;
    align-items: center;
    gap: 0.9rem;
    max-width: 760px;
    margin: 0 auto 1.5rem;
    color: var(--text-secondary);
  }
  .ow-heading { flex: 1; min-width: 0; }
  .ow-heading h1 { margin: 0; font-size: 1.2rem; font-weight: 600; color: var(--text-primary); }
  .ow-heading p { margin: 0.2rem 0 0; font-size: 0.82rem; color: var(--text-muted); }
  .ow-actions { flex-shrink: 0; }
  .ow-empty { max-width: 760px; margin: 0 auto; color: var(--text-muted); font-size: 0.85rem; }
  .ow-list {
    list-style: none;
    margin: 0 auto;
    padding: 0;
    max-width: 760px;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }
  .ow-row {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.75rem 1rem;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text-primary);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .ow-row:hover, .ow-row:focus-visible { background: var(--bg-hover); }
  .ow-text { display: flex; flex-direction: column; gap: 0.15rem; min-width: 0; }
  .ow-title { font-size: 0.9rem; font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ow-place { font-size: 0.75rem; color: var(--text-muted); white-space: pre-line; overflow-wrap: anywhere; }
  .ow-status { flex-shrink: 0; font-size: 0.75rem; color: var(--text-muted); }
  .ow-status.pending { color: var(--accent); }
  .ow-status.warn { color: var(--warning); }
</style>
