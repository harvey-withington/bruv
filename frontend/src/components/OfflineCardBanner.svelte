<script lang="ts">
  // Where an offline card's edits stand, inside the card dialog: saved on
  // this device, syncing, clashing with edits made at home (→ resolve), a
  // failed sync (→ retry), or attachments kept twice. Working offline it
  // also says where the card is pinned.
  import { offline } from '../lib/offline.svelte'
  import { t } from '../lib/i18n.svelte'
  import { offlineCopyName } from '@shared/offline/engine.svelte'
  import { placesText } from '@shared/offline/labels'
  import OfflineConflictDialog from './OfflineConflictDialog.svelte'

  let { cardId }: { cardId: string } = $props()

  const summary = $derived(offline.summary(cardId))
  let resolving = $state(false)
</script>

{#if summary}
  <!-- Working offline the pin panel is hidden (moving needs the server),
       so say where the card is pinned. -->
  {#if offline.working && summary.places}
    <div class="offline-banner" role="note">
      <span class="place">{t('offline.card_place', { place: placesText(summary.places, t) })}</span>
    </div>
  {/if}
  {#if summary.status === 'conflict'}
    <div class="offline-banner warn" role="status">
      <span>{t('offline.banner_conflict')}</span>
      <button class="btn" onclick={() => resolving = true}>{t('offline.resolve')}</button>
    </div>
  {:else if summary.status === 'error'}
    <div class="offline-banner warn" role="status">
      <span>{t('offline.banner_error', { error: summary.syncError ?? '' })}</span>
      <button class="btn" onclick={() => void offline.sync()} disabled={offline.syncing || offline.working}>{t('common.retry')}</button>
    </div>
  {:else if summary.status === 'pending'}
    <div class="offline-banner" role="status">
      <span>{offline.working ? t('offline.banner_saved_here') : t('offline.banner_syncing')}</span>
    </div>
  {/if}
  {#each summary.keptBoth as name (name)}
    <div class="offline-banner" role="status">
      <span>{t('offline.kept_both', { name, copy: offlineCopyName(name) })}</span>
      <button class="btn subtle" onclick={() => void offline.dismissKeptBoth(cardId)}>{t('offline.dismiss')}</button>
    </div>
  {/each}
{/if}

{#if resolving}
  <OfflineConflictDialog {cardId} onClose={() => resolving = false} />
{/if}

<style>
  .offline-banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 0.5rem 0.9rem;
    margin: 0 1.25rem 0.5rem;
    border-radius: 6px;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    color: var(--text-secondary);
    font-size: 0.8rem;
  }
  .offline-banner.warn {
    background: var(--warning-bg);
    border-color: var(--warning-border);
    color: var(--warning-text);
  }
  .offline-banner .btn { flex-shrink: 0; }
  .place { white-space: pre-line; }
</style>
