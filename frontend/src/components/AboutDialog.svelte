<script lang="ts">
  import { X, Github, Bug, FolderOpen, Heart } from 'lucide-svelte'
  import { onMount } from 'svelte'
  import { t } from '../lib/i18n.svelte'
  import { GetBuildInfo, OpenConfigFolder, OpenBugReportURL } from '../lib/api'
  import type { BuildInfo } from '../lib/types'
  import { draggable } from '../lib/draggable'
  import { focusTrap } from '../lib/actions'
  import { showToast } from '../lib/toast.svelte'

  let { onClose }: { onClose: () => void } = $props()

  let info = $state<BuildInfo | null>(null)

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
<div class="overlay" role="presentation" onclick={handleOverlayClick}>
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
</style>
