<script lang="ts">
  import { onMount } from 'svelte'
  import { ChevronLeft, Bell, BellOff, Sun, Moon, Smartphone, LogOut, Database, Activity } from 'lucide-svelte'
  import { navigate } from '../lib/router.svelte'
  import { clearEnrolment, readEnrolment } from '../lib/auth'
  import { stopEvents } from '../lib/events.svelte'
  import { theme, setTheme, type Theme } from '../lib/theme.svelte'
  import * as push from '../lib/push.svelte'
  import { t } from '../lib/i18n.svelte'
  import ConfirmDialog from '../components/ConfirmDialog.svelte'

  // Mobile settings page. Phone-shaped: a flat scroll of grouped rows,
  // tap-to-act. No nested screens — every setting fits on this page.
  // Sections: Display / Notifications / Account / About.

  let pushStatus = $state<push.PushStatus>('unsupported')
  let pushBusy = $state(false)
  let pushError = $state<string | null>(null)
  let confirmingUnpair = $state(false)
  let serverURL = $state<string>('')

  onMount(async () => {
    const enrol = readEnrolment()
    serverURL = enrol?.serverURL ?? ''
    pushStatus = await push.getStatus()
  })

  const themes: { id: Theme; label: string; icon: typeof Sun }[] = [
    { id: 'auto', label: t('settings.theme_auto'), icon: Smartphone },
    { id: 'light', label: t('settings.theme_light'), icon: Sun },
    { id: 'dark', label: t('settings.theme_dark'), icon: Moon },
  ]

  async function togglePush() {
    if (pushBusy) return
    pushBusy = true
    pushError = null
    try {
      if (pushStatus === 'subscribed') {
        await push.disable()
        pushStatus = 'unsubscribed'
      } else {
        await push.enable()
        pushStatus = 'subscribed'
      }
    } catch (err) {
      pushError = err instanceof Error ? err.message : t('settings.push_err')
      pushStatus = await push.getStatus()
    } finally {
      pushBusy = false
    }
  }

  function performUnpair() {
    confirmingUnpair = false
    stopEvents()
    clearEnrolment()
    navigate('/enrol')
  }
</script>

<header class="topbar">
  <button type="button" class="back" onclick={() => history.back()} aria-label={t('common.back')}>
    <ChevronLeft size={20} />
  </button>
  <h1>{t('settings.title')}</h1>
  <span class="spacer"></span>
</header>

<main>
  <!-- Display -->
  <section class="group">
    <h2 class="group-title">{t('settings.display')}</h2>
    <div class="theme-row">
      {#each themes as opt}
        {@const Icon = opt.icon}
        <button
          type="button"
          class="theme-btn"
          class:active={theme.current === opt.id}
          onclick={() => setTheme(opt.id)}
        >
          <Icon size={16} />
          <span>{opt.label}</span>
        </button>
      {/each}
    </div>
  </section>

  <!-- Notifications -->
  <section class="group">
    <h2 class="group-title">{t('settings.notifications')}</h2>
    <button
      type="button"
      class="row"
      onclick={togglePush}
      disabled={pushBusy || pushStatus === 'unsupported' || pushStatus === 'denied'}
    >
      <span class="row-icon">
        {#if pushStatus === 'subscribed'}
          <Bell size={18} />
        {:else}
          <BellOff size={18} />
        {/if}
      </span>
      <div class="row-text">
        <span class="row-title">
          {pushStatus === 'subscribed' ? t('settings.push_on') : t('settings.push_off')}
        </span>
        <span class="row-sub">
          {#if pushStatus === 'unsupported'}
            {t('settings.push_unsupported')}
          {:else if pushStatus === 'denied'}
            {t('settings.push_denied')}
          {:else if pushStatus === 'subscribed'}
            {t('settings.push_on_sub')}
          {:else}
            {t('settings.push_off_sub')}
          {/if}
        </span>
      </div>
      {#if pushStatus !== 'unsupported' && pushStatus !== 'denied'}
        <span class="toggle" class:on={pushStatus === 'subscribed'}>
          <span class="thumb"></span>
        </span>
      {/if}
    </button>
    {#if pushError}
      <p class="error" role="alert">{pushError}</p>
    {/if}
  </section>

  <!-- Activity & history -->
  <section class="group">
    <h2 class="group-title">{t('settings.activity')}</h2>
    <button type="button" class="row" onclick={() => navigate('/activity')}>
      <span class="row-icon"><Activity size={18} /></span>
      <div class="row-text">
        <span class="row-title">{t('settings.activity_log')}</span>
        <span class="row-sub">{t('settings.activity_log_sub')}</span>
      </div>
      <span class="row-arrow" aria-hidden="true">›</span>
    </button>
  </section>

  <!-- Account -->
  <section class="group">
    <h2 class="group-title">{t('settings.account')}</h2>
    <button type="button" class="row" onclick={() => navigate('/repos')}>
      <span class="row-icon"><Database size={18} /></span>
      <div class="row-text">
        <span class="row-title">{t('settings.switch_repo')}</span>
        <span class="row-sub">{t('settings.switch_repo_sub')}</span>
      </div>
      <span class="row-arrow" aria-hidden="true">›</span>
    </button>
    <button type="button" class="row danger" onclick={() => (confirmingUnpair = true)}>
      <span class="row-icon"><LogOut size={18} /></span>
      <div class="row-text">
        <span class="row-title">{t('settings.unpair')}</span>
        <span class="row-sub">{serverURL || t('settings.unpair_sub')}</span>
      </div>
    </button>
  </section>
</main>

{#if confirmingUnpair}
  <ConfirmDialog
    title={t('settings.unpair_confirm_title')}
    body={t('settings.unpair_confirm_body')}
    confirmLabel={t('settings.unpair')}
    destructive
    onConfirm={performUnpair}
    onCancel={() => (confirmingUnpair = false)}
  />
{/if}

<style>
  .topbar {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: center;
    padding: 0.75rem;
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    background: var(--bg);
    z-index: 10;
  }
  .topbar h1 {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
    color: var(--text);
    text-align: center;
  }
  .back {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.4rem;
    border-radius: 6px;
    justify-self: start;
    display: inline-flex;
  }
  .back:hover,
  .back:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }

  main {
    padding: 1rem 0.85rem 4rem;
    max-width: 600px;
    margin: 0 auto;
  }

  .group {
    margin-bottom: 1.75rem;
  }
  .group-title {
    margin: 0 0.25rem 0.5rem;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-faint);
  }

  .theme-row {
    display: grid;
    grid-template-columns: 1fr 1fr 1fr;
    gap: 0.4rem;
  }
  .theme-btn {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.35rem;
    padding: 0.85rem 0.5rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.8rem;
    cursor: pointer;
    min-height: 64px;
  }
  .theme-btn:hover,
  .theme-btn:focus-visible {
    color: var(--text);
    border-color: var(--text-muted);
    outline: none;
  }
  .theme-btn.active {
    color: var(--accent);
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, var(--bg-elev-1));
  }

  .row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    width: 100%;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0.85rem 1rem;
    color: var(--text);
    font: inherit;
    cursor: pointer;
    text-align: left;
    margin-bottom: 0.4rem;
    min-height: 56px;
    /* Don't preventDefault scroll on settings rows — they're tap-only. */
    touch-action: manipulation;
  }
  .row:hover,
  .row:focus-visible {
    border-color: var(--accent);
    outline: none;
  }
  .row:disabled {
    opacity: 0.55;
    cursor: default;
  }
  .row.danger {
    color: #ef4444;
  }
  .row.danger:hover,
  .row.danger:focus-visible {
    border-color: #ef4444;
    background: color-mix(in srgb, #ef4444 8%, var(--bg-elev-1));
  }

  .row-icon {
    color: var(--text-muted);
    display: inline-flex;
    flex-shrink: 0;
  }
  .row.danger .row-icon {
    color: #ef4444;
  }
  .row-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }
  .row-title {
    font-size: 0.95rem;
    font-weight: 500;
  }
  .row-sub {
    font-size: 0.78rem;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .row-arrow {
    color: var(--text-faint);
    font-size: 1.1rem;
    flex-shrink: 0;
  }

  .toggle {
    width: 38px;
    height: 22px;
    background: var(--border);
    border-radius: 999px;
    position: relative;
    flex-shrink: 0;
    transition: background 160ms ease;
  }
  .toggle.on {
    background: var(--accent);
  }
  .thumb {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: #fff;
    transition: transform 160ms cubic-bezier(0.16, 1, 0.3, 1);
  }
  .toggle.on .thumb {
    transform: translateX(16px);
  }

  .error {
    margin: 0.4rem 0.25rem 0;
    color: #fca5a5;
    font-size: 0.82rem;
  }
</style>
