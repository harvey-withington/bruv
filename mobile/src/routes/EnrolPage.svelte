<script lang="ts">
  import { enrol } from '../lib/auth'
  import { replace } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'

  // Mobile enrolment. Two entry paths:
  //
  //   1. Manual paste — user types/pastes the bootstrap token from the
  //      desktop's enrolment screen.
  //   2. QR scan (future) — desktop renders a QR encoding
  //      /m/enrol?token=<bootstrap>; the user scans, lands here with
  //      the token pre-filled, and we auto-submit.
  //
  // Server URL is always the current page origin — the phone can't
  // reach a *different* server than the one serving this bundle (CORS,
  // SW scope, Tailscale routing all assume same-origin).

  const serverURL = typeof window !== 'undefined' ? window.location.origin : ''

  // Pull the token (and optional device name) from the URL on first
  // mount, so a QR/share-link arrival can auto-fill.
  function readQueryParam(name: string): string {
    if (typeof window === 'undefined') return ''
    return new URLSearchParams(window.location.search).get(name) ?? ''
  }

  let bootstrapToken = $state(readQueryParam('token'))
  let deviceName = $state(readQueryParam('name'))
  let submitting = $state(false)
  let errorMsg = $state<string | null>(null)
  let showAdvanced = $state(false)

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    errorMsg = null
    submitting = true
    try {
      await enrol({
        serverURL,
        bootstrapToken,
        deviceName,
      })
      // Replace, not push — back button shouldn't drop the user back
      // onto the enrol form once they're paired.
      replace('/')
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('enrol.err_generic')
    } finally {
      submitting = false
    }
  }
</script>

<main>
  <div class="card">
    <h1>{t('enrol.title')}</h1>
    <p class="subtitle">{t('enrol.subtitle')}</p>

    <form onsubmit={submit}>
      <label>
        <span>{t('enrol.field_token')}</span>
        <input
          type="password"
          bind:value={bootstrapToken}
          placeholder={t('enrol.field_token_placeholder')}
          autocomplete="off"
          spellcheck="false"
          inputmode="text"
          disabled={submitting}
          required
        />
      </label>

      <button
        type="button"
        class="advanced-toggle"
        onclick={() => (showAdvanced = !showAdvanced)}
      >
        {showAdvanced ? t('enrol.hide_advanced') : t('enrol.show_advanced')}
      </button>

      {#if showAdvanced}
        <label>
          <span>{t('enrol.field_device_name')}</span>
          <input
            type="text"
            bind:value={deviceName}
            placeholder={t('enrol.field_device_name_placeholder')}
            autocomplete="off"
            disabled={submitting}
          />
          <small>{t('enrol.field_device_name_hint')}</small>
        </label>

        <p class="server-info">
          {t('enrol.server_label')} <code>{serverURL}</code>
        </p>
      {/if}

      {#if errorMsg}
        <div class="error" role="alert">{errorMsg}</div>
      {/if}

      <button type="submit" class="primary" disabled={submitting || !bootstrapToken.trim()}>
        {submitting ? t('enrol.submitting') : t('enrol.submit')}
      </button>
    </form>
  </div>
</main>

<style>
  main {
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 1.5rem;
  }

  .card {
    width: 100%;
    max-width: 420px;
    padding: 1.75rem 1.5rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 12px;
    box-shadow: 0 10px 30px rgba(0, 0, 0, 0.4);
  }

  h1 {
    margin: 0 0 0.5rem;
    font-size: 1.5rem;
    color: var(--text);
  }

  .subtitle {
    margin: 0 0 1.25rem;
    font-size: 0.9rem;
    color: var(--text-muted);
    line-height: 1.5;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
  }

  label {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    font-size: 0.85rem;
    color: var(--text);
  }

  label span {
    font-weight: 500;
  }

  input {
    padding: 0.65rem 0.75rem;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text);
    font: inherit;
    font-size: 1rem;
  }

  input:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 25%, transparent);
  }

  small {
    color: var(--text-faint);
    font-size: 0.75rem;
  }

  .advanced-toggle {
    align-self: flex-start;
    background: transparent;
    border: none;
    color: var(--text-muted);
    font-size: 0.8rem;
    padding: 0.25rem 0;
    cursor: pointer;
    text-decoration: underline;
  }

  .advanced-toggle:hover {
    color: var(--text);
  }

  .server-info {
    margin: 0;
    font-size: 0.8rem;
    color: var(--text-muted);
    word-break: break-all;
  }

  .server-info code {
    color: var(--text);
    font-size: 0.8rem;
  }

  .error {
    padding: 0.6rem 0.75rem;
    background: rgba(239, 68, 68, 0.15);
    color: #fca5a5;
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 6px;
    font-size: 0.85rem;
  }

  .primary {
    margin-top: 0.5rem;
    padding: 0.85rem 1rem;
    background: var(--accent);
    color: #18181b;
    border: none;
    border-radius: 8px;
    font: inherit;
    font-weight: 600;
    font-size: 1rem;
    cursor: pointer;
  }

  .primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .primary:not(:disabled):active {
    transform: scale(0.98);
  }
</style>
