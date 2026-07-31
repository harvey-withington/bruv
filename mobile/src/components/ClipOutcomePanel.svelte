<script lang="ts">
  import { MonitorUp } from 'lucide-svelte'
  import { navigate, cardURL } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'

  // Shown instead of a success toast when a capture came back pending:
  // the platform blocked the server, so the card (and the deck slide, if
  // one was asked for) exists holding just the link, and the clip gets
  // finished from the desktop browser extension. A half-done clip that
  // silently navigated away like a success would be a lie.

  let {
    platform,
    cardId,
    slideAppended,
  }: {
    /** Display label for the platform, e.g. "Truthsocial". */
    platform: string
    cardId: string
    slideAppended: boolean
  } = $props()
</script>

<section class="panel" role="status">
  <span class="icon" aria-hidden="true"><MonitorUp size={22} /></span>
  <h2>{t('share.pending_title')}</h2>
  <p>{t('share.pending_body', { platform })}</p>
  {#if slideAppended}
    <p class="note">{t('share.pending_slide_note')}</p>
  {/if}
  <div class="actions">
    <button type="button" class="ghost" onclick={() => navigate('/')}>{t('share.done')}</button>
    <button type="button" class="primary" onclick={() => navigate(cardURL(cardId))}>
      {t('share.view_card')}
    </button>
  </div>
</section>

<style>
  .panel {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    padding: 1rem 1.1rem 1.25rem;
    background: var(--warn-bg);
    border: 1px solid var(--warn-border);
    border-radius: 12px;
    color: var(--warn-text);
  }

  .icon {
    display: inline-flex;
    color: var(--warn-border);
  }

  h2 {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
  }

  p {
    margin: 0;
    font-size: 0.9rem;
    line-height: 1.5;
  }

  .note {
    font-size: 0.85rem;
    opacity: 0.85;
  }

  .actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.4rem;
  }

  .ghost,
  .primary {
    padding: 0.7rem 1.2rem;
    border-radius: 8px;
    font: inherit;
    font-size: 0.9rem;
    font-weight: 500;
    cursor: pointer;
    border: 1px solid transparent;
    touch-action: manipulation;
  }

  .ghost {
    background: transparent;
    color: inherit;
    border-color: var(--warn-border);
  }
  .ghost:hover,
  .ghost:focus-visible {
    background: color-mix(in srgb, var(--warn-border) 20%, transparent);
    outline: none;
  }

  .primary {
    flex: 1;
    background: var(--accent);
    color: var(--bg);
    font-weight: 600;
  }
  .primary:hover,
  .primary:focus-visible {
    filter: brightness(1.1);
    outline: none;
  }
</style>
