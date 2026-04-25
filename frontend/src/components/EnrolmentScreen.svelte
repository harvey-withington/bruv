<script lang="ts">
  // First-run enrolment wizard for browser-mode and remote-shell
  // users. Invoked by main.ts when the cloud adapter's
  // resolveTransport throws NeedsEnrolmentError — i.e. no Wails
  // Shell API, no env vars, no saved credentials in localStorage.
  //
  // Flow:
  //
  //   1. User pastes the bootstrap token the server operator shared
  //      out-of-band (usually from <server-configdir>/bootstrap-token.txt).
  //   2. User confirms the server URL (defaulted to the origin they
  //      loaded from, which is almost always correct).
  //   3. POST /auth/enrol with the bootstrap token → server returns
  //      a long-lived device token.
  //   4. Save URL + device token to localStorage and reload; the
  //      cloud adapter now finds them and boots normally.
  //
  // The bootstrap token is only used here. It's never stored; only
  // the derived device token is. Server-side, tokens are hashed at
  // rest — see transport/http/devices.go.

  import { t } from '../lib/i18n.svelte'
  import { saveEnrolment } from '../lib/adapters/cloud'

  let serverURL = $state(window.location.origin)
  let bootstrapToken = $state('')
  let deviceName = $state(defaultDeviceName())
  let submitting = $state(false)
  let errorMsg = $state<string | null>(null)

  function defaultDeviceName(): string {
    // Navigator platform gives a usable hint for "this device" names:
    // "Win32" / "MacIntel" / "Linux x86_64". The user can edit before
    // submitting, so imperfect defaults are fine.
    const platform = (navigator as any).platform || 'device'
    return `browser-${platform.toLowerCase().replace(/[^a-z0-9]/g, '')}`
  }

  async function enrol(event: SubmitEvent) {
    event.preventDefault()
    errorMsg = null

    const url = serverURL.trim().replace(/\/+$/, '') // strip trailing slashes
    const token = bootstrapToken.trim()
    const name = deviceName.trim() || 'Unnamed device'

    if (!url) {
      errorMsg = t('enrol.err_url_required')
      return
    }
    if (!token) {
      errorMsg = t('enrol.err_token_required')
      return
    }

    submitting = true
    try {
      const res = await fetch(`${url}/auth/enrol`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          bootstrap_token: token,
          device_name: name,
        }),
      })
      if (!res.ok) {
        // Try to pull a readable error from the JSON body; fall
        // back to status text so a broken server still shows
        // something actionable.
        let detail = res.statusText
        try {
          const body = await res.json()
          if (body?.error) detail = String(body.error)
        } catch {}
        errorMsg = `${res.status} ${detail}`
        return
      }
      const body = (await res.json()) as { device_token?: string }
      if (!body.device_token) {
        errorMsg = t('enrol.err_no_token_in_response')
        return
      }
      saveEnrolment(url, body.device_token)
      // Reload so the freshly-saved credentials take effect through
      // the normal cloud-adapter bootstrap path. Simpler than trying
      // to re-run initBackend in place.
      window.location.reload()
    } catch (err) {
      errorMsg = (err as Error).message || t('enrol.err_network')
    } finally {
      submitting = false
    }
  }
</script>

<div class="screen">
  <form class="card" onsubmit={enrol}>
    <h1>{t('enrol.title')}</h1>
    <p class="subtitle">{t('enrol.subtitle')}</p>

    <label>
      <span>{t('enrol.server_url')}</span>
      <input
        type="url"
        bind:value={serverURL}
        placeholder="https://home.tailnet.ts.net:9870"
        required
        disabled={submitting}
      />
      <small>{t('enrol.server_url_hint')}</small>
    </label>

    <label>
      <span>{t('enrol.bootstrap_token')}</span>
      <input
        type="password"
        bind:value={bootstrapToken}
        placeholder={t('enrol.bootstrap_token_placeholder')}
        required
        disabled={submitting}
        autocomplete="off"
        spellcheck="false"
      />
      <small>{t('enrol.bootstrap_token_hint')}</small>
    </label>

    <label>
      <span>{t('enrol.device_name')}</span>
      <input
        type="text"
        bind:value={deviceName}
        placeholder={t('enrol.device_name_placeholder')}
        disabled={submitting}
      />
      <small>{t('enrol.device_name_hint')}</small>
    </label>

    {#if errorMsg}
      <div class="error" role="alert">{errorMsg}</div>
    {/if}

    <button type="submit" disabled={submitting}>
      {submitting ? t('enrol.submitting') : t('enrol.submit')}
    </button>
  </form>
</div>

<style>
  .screen {
    position: fixed;
    inset: 0;
    display: grid;
    place-items: center;
    background: var(--bg-base, #18181b);
    padding: var(--space-6, 24px);
  }

  .card {
    width: 100%;
    max-width: 440px;
    display: flex;
    flex-direction: column;
    gap: var(--space-4, 16px);
    padding: var(--space-6, 24px);
    background: var(--bg-raised, #27272a);
    border: 1px solid var(--border-default, #3f3f46);
    border-radius: var(--radius-md, 8px);
    box-shadow: var(--shadow-lg, 0 10px 30px rgba(0, 0, 0, 0.4));
  }

  h1 {
    margin: 0;
    font-size: var(--font-size-xl, 20px);
    color: var(--fg-strong, #fafafa);
  }

  .subtitle {
    margin: 0;
    font-size: var(--font-size-sm, 13px);
    color: var(--fg-muted, #a1a1aa);
    line-height: 1.5;
  }

  label {
    display: flex;
    flex-direction: column;
    gap: var(--space-1, 4px);
    font-size: var(--font-size-sm, 13px);
    color: var(--fg-strong, #fafafa);
  }

  label span {
    font-weight: 500;
  }

  input {
    padding: var(--space-2, 8px) var(--space-3, 12px);
    background: var(--bg-base, #18181b);
    border: 1px solid var(--border-default, #3f3f46);
    border-radius: var(--radius-sm, 4px);
    color: var(--fg-strong, #fafafa);
    font: inherit;
  }

  input:focus {
    outline: none;
    border-color: var(--accent-default, #6366f1);
    box-shadow: 0 0 0 3px var(--accent-soft, rgba(99, 102, 241, 0.2));
  }

  small {
    color: var(--fg-muted, #a1a1aa);
    font-size: var(--font-size-xs, 11px);
    line-height: 1.4;
  }

  button {
    padding: var(--space-3, 12px);
    background: var(--accent-default, #6366f1);
    color: white;
    border: none;
    border-radius: var(--radius-sm, 4px);
    font: inherit;
    font-weight: 500;
    cursor: pointer;
    transition: background 0.15s ease;
  }

  button:hover:not(:disabled) {
    background: var(--accent-hover, #4f46e5);
  }

  button:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .error {
    padding: var(--space-3, 12px);
    background: var(--danger-soft, rgba(239, 68, 68, 0.15));
    color: var(--danger-default, #ef4444);
    border: 1px solid var(--danger-default, #ef4444);
    border-radius: var(--radius-sm, 4px);
    font-size: var(--font-size-sm, 13px);
  }
</style>
