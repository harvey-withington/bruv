<script lang="ts">
  // Shown when the boot health probe finds the active Remote
  // connection isn't responding. Three escape paths:
  //   - Test again (re-run the probe; reload on success)
  //   - Use Local (switch the active connection back to Local)
  //   - Manage connections (open the dialog to add/edit/remove)

  import { Server, AlertTriangle, RefreshCw, Monitor, Settings } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import BruvIcon from './BruvIcon.svelte'
  import { activeConnectionLabel, switchConnection, connections } from '../lib/connections.svelte'
  import { probeBackend } from '../lib/repos.svelte'

  let { onOpenConnections, onProbeOk }: {
    onOpenConnections?: () => void
    onProbeOk?: () => void
  } = $props()

  let testing = $state(false)
  let lastResult = $state<'idle' | 'fail' | 'ok'>('idle')

  const activeURL = $derived(
    connections.connections.find(c => c.id === connections.active)?.url ?? ''
  )

  async function testAgain() {
    testing = true
    try {
      const ok = await probeBackend()
      lastResult = ok ? 'ok' : 'fail'
      if (ok && onProbeOk) onProbeOk()
    } finally {
      testing = false
    }
  }

  async function useLocal() {
    await switchConnection('')
    // switchConnection reloads; nothing else to do.
  }
</script>

<div class="screen">
  <div class="card">
    <div class="header-icon">
      <BruvIcon size={64} />
    </div>
    <h1>{t('remote.unreachable_title').replace('{server}', activeConnectionLabel())}</h1>
    <p class="subtitle">
      {t('remote.unreachable_subtitle')
        .replace('{server}', activeConnectionLabel())
        .replace('{url}', activeURL)}
    </p>

    <div class="status">
      <Server size={14} class="server-icon" />
      <span class="status-text">
        {#if testing}
          {t('common.loading')}
        {:else if lastResult === 'fail'}
          <AlertTriangle size={12} /> {activeURL}
        {:else}
          {activeURL}
        {/if}
      </span>
    </div>

    <div class="actions">
      <button class="btn-primary" onclick={testAgain} disabled={testing}>
        <RefreshCw size={14} />
        {testing ? t('common.loading') : t('remote.test_again')}
      </button>
      <button class="btn-secondary" onclick={useLocal}>
        <Monitor size={14} /> {t('remote.switch_local')}
      </button>
      {#if onOpenConnections}
        <button class="btn-secondary" onclick={onOpenConnections}>
          <Settings size={14} /> {t('remote.manage_connections')}
        </button>
      {/if}
    </div>
  </div>
</div>

<style>
  .screen {
    position: fixed;
    inset: 0;
    display: grid;
    place-items: center;
    background: var(--bg-base);
    padding: 1.5rem;
  }
  .card {
    text-align: center;
    max-width: 480px;
    width: 100%;
    padding: 2rem;
  }
  .header-icon { margin-bottom: 0.75rem; opacity: 0.6; }
  h1 {
    margin: 0 0 0.5rem;
    font-size: 1.3rem;
    color: var(--text-strong);
  }
  .subtitle {
    margin: 0 0 1rem;
    color: var(--text-secondary);
    font-size: 0.85rem;
    line-height: 1.5;
  }
  .status {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.4rem 0.75rem;
    background: color-mix(in srgb, var(--danger) 12%, transparent);
    color: var(--danger-light, var(--danger));
    border-radius: 999px;
    font-size: 0.75rem;
    margin-bottom: 1.25rem;
  }
  .status-text { display: inline-flex; align-items: center; gap: 0.3rem; }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    justify-content: center;
  }
  .btn-primary, .btn-secondary {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.55rem 1rem;
    border-radius: 6px;
    font: inherit;
    font-weight: 500;
    cursor: pointer;
    border: 1px solid transparent;
  }
  .btn-primary { background: var(--accent); color: #fff; }
  .btn-primary:hover:not(:disabled) { background: var(--accent-hover, var(--accent)); }
  .btn-primary:disabled { opacity: 0.6; cursor: not-allowed; }
  .btn-secondary {
    background: transparent;
    color: var(--text-secondary);
    border-color: var(--border);
  }
  .btn-secondary:hover { color: var(--text-strong); background: var(--bg-elevated); }
</style>
