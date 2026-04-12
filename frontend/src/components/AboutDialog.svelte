<script lang="ts">
  import { X, Github, Bug, FolderOpen, Heart, RefreshCw, CircleCheck, CircleAlert, Download } from 'lucide-svelte'
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { t } from '../lib/i18n.svelte'
  import { GetBuildInfo, OpenConfigFolder, OpenBugReportURL, CheckForUpdates } from '../lib/api'
  import type { BuildInfo, UpdateCheckResult } from '../lib/types'
  import { draggable } from '../lib/draggable'
  import { focusTrap } from '../lib/actions'
  import { showToast } from '../lib/toast.svelte'

  let { onClose }: { onClose: () => void } = $props()

  let info = $state<BuildInfo | null>(null)
  let updateResult = $state<UpdateCheckResult | null>(null)
  let checkingUpdates = $state(false)

  async function checkForUpdates() {
    checkingUpdates = true
    updateResult = null
    try {
      updateResult = await CheckForUpdates()
    } catch (e) {
      updateResult = { status: 'error', current_version: info?.version ?? '', error: String(e) }
    } finally {
      checkingUpdates = false
    }
  }

  function openReleaseURL() {
    if (updateResult?.release_url) {
      window.open(updateResult.release_url, '_blank', 'noopener')
    }
  }

  onMount(async () => {
    try {
      info = await GetBuildInfo()
    } catch (e) {
      showToast(t('about.load_error'), 'error')
    }
  })

  async function openConfig() {
    try {
      await OpenConfigFolder()
    } catch {
      showToast(t('about.open_config_error'), 'error')
    }
  }

  async function reportBug() {
    try {
      await OpenBugReportURL()
    } catch {
      showToast(t('about.report_bug_error'), 'error')
    }
  }

  function openRepo() {
    window.open('https://github.com/harvey-withington/bruv', '_blank', 'noopener')
  }

  function handleOverlayClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="overlay" role="presentation" onclick={handleOverlayClick} out:fade={{ duration: 150 }}>
  <div class="dialog" use:draggable={{ handle: '.dialog-header' }} use:focusTrap>
    <div class="dialog-header">
      <h2>{t('about.title')}</h2>
      <button class="close-btn" onclick={onClose} title={t('common.close')} aria-label={t('common.close')}>
        <X size={18} />
      </button>
    </div>

    <div class="dialog-body">
      <div class="brand">
        <img src="/bruv-icon.svg" alt="BRUV" class="logo" />
        <div class="brand-text">
          <h1>BRUV</h1>
          <p class="tagline">{t('about.tagline')}</p>
        </div>
      </div>

      <p class="description">{t('about.description')}</p>

      {#if info}
        <div class="info-grid">
          <div class="info-row">
            <span class="info-label">{t('about.version')}</span>
            <span class="info-value mono">{info.version}</span>
          </div>
          <div class="info-row">
            <span class="info-label">{t('about.build_date')}</span>
            <span class="info-value mono">{info.build_date}</span>
          </div>
          <div class="info-row">
            <span class="info-label">{t('about.platform')}</span>
            <span class="info-value mono">{info.os}/{info.arch}</span>
          </div>
          <div class="info-row">
            <span class="info-label">{t('about.go_version')}</span>
            <span class="info-value mono">{info.go_version}</span>
          </div>
        </div>
      {/if}

      <div class="actions">
        <button type="button" class="action-btn" onclick={checkForUpdates} disabled={checkingUpdates}>
          <RefreshCw size={16} class={checkingUpdates ? 'spinning' : ''} />
          <span>{checkingUpdates ? t('about.checking_updates') : t('about.check_updates')}</span>
        </button>
        <button type="button" class="action-btn" onclick={openRepo}>
          <Github size={16} />
          <span>{t('about.view_source')}</span>
        </button>
        <button type="button" class="action-btn" onclick={openConfig}>
          <FolderOpen size={16} />
          <span>{t('about.open_config_folder')}</span>
        </button>
        <button type="button" class="action-btn" onclick={reportBug}>
          <Bug size={16} />
          <span>{t('about.report_bug')}</span>
        </button>
      </div>

      {#if updateResult}
        <div class="update-result update-{updateResult.status}">
          {#if updateResult.status === 'up_to_date'}
            <CircleCheck size={16} />
            <div class="update-body">
              <p class="update-headline">{t('about.update_up_to_date')}</p>
              <p class="update-sub">{t('about.update_up_to_date_sub', { version: updateResult.current_version })}</p>
            </div>
          {:else if updateResult.status === 'update_available'}
            <Download size={16} />
            <div class="update-body">
              <p class="update-headline">{t('about.update_available', { version: updateResult.latest_version ?? '' })}</p>
              <p class="update-sub">{t('about.update_current', { version: updateResult.current_version })}</p>
              {#if updateResult.release_notes}
                <details class="update-notes">
                  <summary>{t('about.update_release_notes')}</summary>
                  <pre class="release-notes-body">{updateResult.release_notes}</pre>
                </details>
              {/if}
              <button type="button" class="update-download-btn" onclick={openReleaseURL}>
                <Download size={14} />
                <span>{t('about.update_download')}</span>
              </button>
            </div>
          {:else}
            <CircleAlert size={16} />
            <div class="update-body">
              <p class="update-headline">{t('about.update_error')}</p>
              <p class="update-sub">{updateResult.error ?? ''}</p>
            </div>
          {/if}
        </div>
      {/if}

      <p class="credit">
        {t('about.made_with')}
        <Heart size={12} class="heart-inline" />
        {t('about.by_harvey_claude')}
      </p>

      <p class="license">
        {@html t('about.license_html')}
      </p>
    </div>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--overlay-bg, rgba(0, 0, 0, 0.5));
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    animation: fade-in var(--duration-normal) var(--ease-out);
  }

  .dialog {
    background: var(--bg-panel);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.4);
    width: 440px;
    max-width: 90vw;
    max-height: 90vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }

  .dialog-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--border);
    cursor: move;
  }

  .dialog-header h2 {
    margin: 0;
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text-primary);
  }

  .close-btn {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
    border-radius: 4px;
    display: flex;
    align-items: center;
  }

  .close-btn:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }

  .dialog-body {
    padding: 1.25rem 1.5rem 1.5rem;
    overflow-y: auto;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 1rem;
    margin-bottom: 1rem;
  }

  .logo {
    width: 64px;
    height: 64px;
    flex-shrink: 0;
  }

  .brand-text h1 {
    margin: 0;
    font-size: 1.75rem;
    font-weight: 700;
    letter-spacing: 0.02em;
    color: var(--text-primary);
  }

  .tagline {
    margin: 0.15rem 0 0;
    color: var(--text-muted);
    font-size: 0.85rem;
    font-style: italic;
  }

  .description {
    margin: 0 0 1.25rem;
    color: var(--text-secondary);
    font-size: 0.85rem;
    line-height: 1.5;
  }

  .info-grid {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    padding: 0.75rem 0.9rem;
    background: var(--bg-base);
    border: 1px solid var(--border);
    border-radius: 6px;
    margin-bottom: 1.25rem;
  }

  .info-row {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    font-size: 0.8rem;
  }

  .info-label {
    color: var(--text-muted);
  }

  .info-value {
    color: var(--text-primary);
  }

  .mono {
    font-family: ui-monospace, 'SFMono-Regular', Consolas, monospace;
    font-size: 0.75rem;
  }

  .actions {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    margin-bottom: 1.25rem;
  }

  .action-btn {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.55rem 0.85rem;
    background: var(--bg-base);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-primary);
    font-size: 0.82rem;
    cursor: pointer;
    text-align: left;
    transition: background 0.12s, border-color 0.12s;
  }

  .action-btn:hover {
    background: var(--bg-hover);
    border-color: var(--accent);
  }

  .credit {
    margin: 0 0 0.5rem;
    text-align: center;
    color: var(--text-muted);
    font-size: 0.78rem;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.3rem;
  }

  .credit :global(.heart-inline) {
    color: var(--accent);
  }

  .license {
    margin: 0;
    text-align: center;
    color: var(--text-muted);
    font-size: 0.72rem;
  }

  .license :global(a) {
    color: var(--accent);
    text-decoration: none;
  }

  .license :global(a:hover) {
    text-decoration: underline;
  }

  .action-btn :global(.spinning) {
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .update-result {
    display: flex;
    gap: 0.6rem;
    padding: 0.7rem 0.9rem;
    border-radius: 6px;
    margin-bottom: 1rem;
    border: 1px solid var(--border);
    align-items: flex-start;
  }

  .update-result.update-up_to_date {
    background: color-mix(in srgb, var(--accent) 8%, transparent);
    border-color: color-mix(in srgb, var(--accent) 30%, var(--border));
    color: var(--text-primary);
  }

  .update-result.update-update_available {
    background: color-mix(in srgb, #f59e0b 10%, transparent);
    border-color: color-mix(in srgb, #f59e0b 40%, var(--border));
    color: var(--text-primary);
  }

  .update-result.update-error {
    background: color-mix(in srgb, #ef4444 10%, transparent);
    border-color: color-mix(in srgb, #ef4444 40%, var(--border));
    color: var(--text-primary);
  }

  .update-body {
    flex: 1;
    min-width: 0;
  }

  .update-headline {
    margin: 0 0 0.2rem;
    font-size: 0.82rem;
    font-weight: 600;
  }

  .update-sub {
    margin: 0;
    font-size: 0.75rem;
    color: var(--text-muted);
  }

  .update-notes {
    margin-top: 0.5rem;
  }

  .update-notes summary {
    font-size: 0.75rem;
    color: var(--text-muted);
    cursor: pointer;
    user-select: none;
  }

  .release-notes-body {
    margin: 0.4rem 0 0;
    max-height: 120px;
    overflow-y: auto;
    padding: 0.5rem 0.6rem;
    background: var(--bg-base);
    border: 1px solid var(--border);
    border-radius: 4px;
    font-size: 0.72rem;
    font-family: ui-monospace, 'SFMono-Regular', Consolas, monospace;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .update-download-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    margin-top: 0.55rem;
    padding: 0.4rem 0.75rem;
    background: var(--accent);
    color: white;
    border: none;
    border-radius: 4px;
    font-size: 0.78rem;
    cursor: pointer;
  }

  .update-download-btn:hover {
    filter: brightness(1.1);
  }
</style>
