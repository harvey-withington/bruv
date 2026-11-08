<script lang="ts">
  // Where an offline card's edits stand, at the top of the card page:
  // saved on this phone, syncing, clashing with edits made at home
  // (→ resolve sheet), a failed sync (→ retry), or attachments kept twice.
  // Silent when synced.
  import { offline } from '../lib/offline.svelte'
  import { t } from '../lib/i18n.svelte'
  import { offlineCopyName } from '@shared/offline/engine.svelte'
  import OfflineConflictSheet from './OfflineConflictSheet.svelte'

  let { cardId }: { cardId: string } = $props()

  const summary = $derived(offline.summary(cardId))
  let resolving = $state(false)
</script>

{#if summary}
  {#if summary.status === 'conflict'}
    <div class="notice warn" role="status">
      <span>{t('offline.banner_conflict')}</span>
      <button type="button" onclick={() => (resolving = true)}>{t('offline.resolve')}</button>
    </div>
  {:else if summary.status === 'error'}
    <div class="notice warn" role="status">
      <span>{t('offline.banner_error', { error: summary.syncError ?? '' })}</span>
      <button type="button" onclick={() => void offline.sync()} disabled={offline.syncing || offline.working}>{t('common.retry')}</button>
    </div>
  {:else if summary.status === 'pending'}
    <div class="notice" role="status">
      <span>{offline.working ? t('offline.banner_saved_here') : t('offline.banner_syncing')}</span>
    </div>
  {/if}
  {#each summary.keptBoth as name (name)}
    <div class="notice" role="status">
      <span>{t('offline.kept_both', { name, copy: offlineCopyName(name) })}</span>
      <button type="button" onclick={() => void offline.dismissKeptBoth(cardId)}>{t('offline.dismiss')}</button>
    </div>
  {/each}
{/if}

{#if resolving}
  <OfflineConflictSheet {cardId} onClose={() => (resolving = false)} />
{/if}

<style>
  .notice {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.65rem 0.8rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text-muted);
    font-size: 0.85rem;
    line-height: 1.4;
  }
  .notice.warn {
    background: var(--warn-bg);
    border-color: var(--warn-border);
    color: var(--warn-text);
  }
  .notice span { flex: 1; min-width: 0; }
  .notice button {
    flex-shrink: 0;
    min-height: 36px;
    padding: 0 0.75rem;
    border-radius: 8px;
    border: 1px solid currentColor;
    background: transparent;
    color: inherit;
    font: inherit;
    font-weight: 600;
    cursor: pointer;
  }
  .notice button:disabled { opacity: 0.6; cursor: default; }
</style>
