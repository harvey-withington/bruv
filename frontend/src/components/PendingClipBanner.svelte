<script lang="ts">
  import { RetryCapture, GetCard } from '@shared/api'
  import { CLIP_PENDING_TAG, type Card } from '@shared/types'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'

  // Shown on cards captured from a phone whose server-side capture was
  // blocked (tagged CLIP_PENDING_TAG). Points the user at the browser
  // extension to finish the clip, or offers a server-side retry —
  // success drops the tag server-side, so re-fetching the card
  // unmounts this banner automatically.

  let { card, onCardUpdated }: {
    card: Card
    onCardUpdated: (card: Card) => void
  } = $props()

  let retrying = $state(false)

  async function handleRetry() {
    retrying = true
    try {
      await RetryCapture(card.id)
      showToast(t('capture.retry_success'), 'success')
      const fresh = await GetCard(card.id)
      if (fresh) onCardUpdated(fresh)
    } catch (e) {
      showToast(e instanceof Error ? e.message : String(e), 'error')
    } finally {
      retrying = false
    }
  }
</script>

{#if card.tags?.includes(CLIP_PENDING_TAG)}
  <div class="pending-clip-banner" role="status">
    <div class="pending-clip-text">
      <strong>{t('capture.pending_banner')}</strong>
      <span class="pending-clip-detail">{t('capture.pending_banner_detail')}</span>
    </div>
    <button class="pending-clip-retry" type="button" onclick={handleRetry} disabled={retrying}>
      {retrying ? t('capture.retrying') : t('capture.retry')}
    </button>
  </div>
{/if}

<style>
  .pending-clip-banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 0.6rem 0.9rem;
    margin: 0 1.25rem 0.5rem;
    border-radius: 6px;
    background: var(--warning-bg);
    border: 1px solid var(--warning-border);
    color: var(--warning-text);
  }

  .pending-clip-text {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    min-width: 0;
  }

  .pending-clip-text strong {
    font-size: 0.85rem;
    font-weight: 600;
  }

  .pending-clip-detail {
    font-size: 0.75rem;
    opacity: 0.85;
  }

  .pending-clip-retry {
    flex-shrink: 0;
    padding: 0.35rem 0.75rem;
    border-radius: 4px;
    border: 1px solid var(--warning-border);
    background: transparent;
    color: var(--warning-text);
    font: inherit;
    font-size: 0.8rem;
    font-weight: 500;
    cursor: pointer;
  }

  .pending-clip-retry:hover:not(:disabled) {
    background: var(--warning-border);
  }

  .pending-clip-retry:disabled {
    cursor: default;
    opacity: 0.7;
  }
</style>
