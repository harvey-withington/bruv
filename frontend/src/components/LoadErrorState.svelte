<script lang="ts">
  // The "couldn't load" state for a panel whose data failed to load:
  // localized headline, the underlying error underneath (so the cause is
  // visible — "DescribeAgent is not supported by BRUV server …" beats a
  // bare "Couldn't load"), and Retry. Rendering this instead of a default
  // form also keeps Save from overwriting real data with blanks.
  import type { ComponentType } from 'svelte'
  import type { Icon as LucideIcon } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { errorDetail } from '@shared/errors'

  let { message, error, onRetry, icon: Icon, fill = false }: {
    message: string
    error?: unknown
    onRetry: () => void
    icon?: ComponentType<LucideIcon>
    fill?: boolean
  } = $props()

  const detail = $derived(errorDetail(error))
</script>

<div class="load-error" class:fill role="alert">
  {#if Icon}<Icon size={32} strokeWidth={1} />{/if}
  <span class="load-error-text">{message}</span>
  {#if detail && detail !== message}
    <span class="load-error-detail">{detail}</span>
  {/if}
  <button class="retry-btn" onclick={onRetry}>{t('agent.load_retry')}</button>
</div>

<style>
  .load-error {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    padding: 2rem;
    color: var(--text-muted);
    text-align: center;
  }
  .load-error.fill {
    height: 100%;
  }
  .load-error-text {
    color: var(--danger);
  }
  .load-error-detail {
    max-width: 36rem;
    font-size: 0.75rem;
    color: var(--text-muted);
    user-select: text;
    overflow-wrap: anywhere;
  }
  .retry-btn {
    padding: 0.3rem 0.75rem;
    background: var(--bg-surface);
    border: 1px solid var(--border-muted);
    border-radius: 5px;
    color: var(--text-body);
    font-size: 0.75rem;
    cursor: pointer;
    transition: border-color var(--duration-normal), color var(--duration-normal);
  }
  .retry-btn:hover {
    border-color: var(--accent);
    color: var(--accent);
  }
</style>
