<script lang="ts">
  // Warns when the connected server runs older code than this desktop app
  // (shared/serverSkew.ts): its missing RPCs would otherwise surface one
  // feature at a time as unexplained load failures. Local connections
  // run this same binary, so they never trip it. Dismissing hides it
  // until either build changes.
  import { onMount } from 'svelte'
  import { TriangleAlert, X } from 'lucide-svelte'
  import { GetBuildInfo } from '@shared/api'
  import { resolveTransportInfo, httpBase } from '@shared/adapters/cloud'
  import { fetchServerBuild, serverIsOlder } from '@shared/serverSkew'
  import { t } from '../lib/i18n.svelte'

  const DISMISSED_KEY = 'bruv:serverSkewDismissed'

  let skew = $state<{ app: string; server: string } | null>(null)

  const dismissKey = (s: { app: string; server: string }) => `${s.app}|${s.server}`

  onMount(async () => {
    try {
      const [app, info] = await Promise.all([GetBuildInfo(), resolveTransportInfo()])
      const server = await fetchServerBuild(httpBase(info))
      if (!server || !serverIsOlder(app.commit_date, server.commit_date)) return
      const found = { app: app.version, server: server.version }
      if (localStorage.getItem(DISMISSED_KEY) === dismissKey(found)) return
      skew = found
    } catch {
      // No desktop shell (browser mode) or no transport — nothing to compare.
    }
  })

  function dismiss() {
    if (skew) localStorage.setItem(DISMISSED_KEY, dismissKey(skew))
    skew = null
  }
</script>

{#if skew}
  <div class="server-outdated" role="alert">
    <TriangleAlert size={16} />
    <span class="server-outdated-text">{t('server_skew.banner', { server: skew.server, app: skew.app })}</span>
    <button class="server-outdated-dismiss" onclick={dismiss} title={t('server_skew.dismiss')} aria-label={t('server_skew.dismiss')}>
      <X size={14} />
    </button>
  </div>
{/if}

<style>
  .server-outdated {
    position: fixed;
    bottom: 1.5rem;
    left: 1.5rem;
    z-index: 150; /* above the card dialog, below confirms and toasts */
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    max-width: 28rem;
    padding: 0.6rem 0.75rem;
    border-radius: 6px;
    background: var(--warning-bg);
    border: 1px solid var(--warning-border);
    color: var(--warning-text);
    font-size: 0.8rem;
    line-height: 1.4;
    box-shadow: 0 4px 16px var(--shadow-lg);
  }
  .server-outdated-text {
    flex: 1;
  }
  .server-outdated-dismiss {
    display: flex;
    flex-shrink: 0;
    padding: 0.1rem;
    border: none;
    border-radius: 4px;
    background: none;
    color: inherit;
    cursor: pointer;
  }
  .server-outdated-dismiss:hover {
    background: var(--warning-border);
  }
</style>
