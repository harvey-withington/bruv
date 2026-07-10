<script lang="ts">
  // Reusable form for enrolling this device with a remote BRUV server.
  // Used by:
  //   - The connections dialog (desktop mode) to add a new server.
  //   - The browser-mode EnrolmentScreen as the empty-state form.
  // The component just does the form + the /auth/enrol POST. Persistence
  // is handed off to the parent via onEnrolled — different callers
  // store the result in different places (Wails-backed JSON file vs.
  // browser localStorage).

  import { t } from '../lib/i18n.svelte'

  let {
    initialURL = '',
    initialName = '',
    submitLabel,
    onEnrolled,
    onCancel,
  }: {
    initialURL?: string
    initialName?: string
    submitLabel?: string
    onEnrolled: (args: { name: string; url: string; deviceToken: string }) => void | Promise<void>
    onCancel?: () => void
  } = $props()

  // svelte-ignore state_referenced_locally
  let serverName = $state(initialName)
  // svelte-ignore state_referenced_locally
  let serverURL = $state(initialURL)
  let bootstrapToken = $state('')
  let submitting = $state(false)
  let errorMsg = $state<string | null>(null)

  function defaultDeviceName(): string {
    // navigator.platform is deprecated but still the best short device
    // hint; typed since TS still declares it.
    const platform = navigator.platform || 'device'
    return `${platform.toLowerCase().replace(/[^a-z0-9]/g, '')}`
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    errorMsg = null

    const url = serverURL.trim().replace(/\/+$/, '')
    const token = bootstrapToken.trim()
    const name = serverName.trim()

    if (!name) {
      errorMsg = t('connection.err_name_required')
      return
    }
    if (!url) {
      errorMsg = t('connection.err_url_required')
      return
    }
    if (!token) {
      errorMsg = t('connection.err_token_required')
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
          device_name: defaultDeviceName(),
        }),
      })
      if (!res.ok) {
        // Pull a readable error from the JSON body when possible.
        let detail = res.statusText
        try {
          const body = await res.json()
          if (body?.error) detail = String(body.error)
        } catch { /* fall through to status text */ }
        errorMsg = `${res.status} ${detail}`
        return
      }
      const body = (await res.json()) as { device_token?: string }
      if (!body.device_token) {
        errorMsg = t('connection.err_no_token_in_response')
        return
      }
      await onEnrolled({ name, url, deviceToken: body.device_token })
    } catch (err) {
      console.error('Failed to enrol connection:', err)
      errorMsg = t('connection.err_network')
    } finally {
      submitting = false
    }
  }
</script>

<form class="add-connection-form" onsubmit={submit}>
  <label>
    <span>{t('connection.field_name')}</span>
    <input
      type="text"
      bind:value={serverName}
      placeholder={t('connection.field_name_placeholder')}
      disabled={submitting}
      required
    />
    <small>{t('connection.field_name_hint')}</small>
  </label>

  <label>
    <span>{t('connection.field_url')}</span>
    <input
      type="url"
      bind:value={serverURL}
      placeholder="http://100.x.y.z:9870"
      disabled={submitting}
      required
    />
    <small>{t('connection.field_url_hint')}</small>
  </label>

  <label>
    <span>{t('connection.field_token')}</span>
    <input
      type="password"
      bind:value={bootstrapToken}
      placeholder={t('connection.field_token_placeholder')}
      disabled={submitting}
      autocomplete="off"
      spellcheck="false"
      required
    />
    <small>{t('connection.field_token_hint')}</small>
  </label>

  {#if errorMsg}
    <div class="error" role="alert">{errorMsg}</div>
  {/if}

  <div class="actions">
    {#if onCancel}
      <button type="button" class="btn-secondary" onclick={onCancel} disabled={submitting}>
        {t('common.cancel')}
      </button>
    {/if}
    <button type="submit" class="btn-primary" disabled={submitting}>
      {submitting ? t('connection.submitting') : (submitLabel ?? t('connection.submit'))}
    </button>
  </div>
</form>

<style>
  .add-connection-form {
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
  }

  label {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    font-size: 0.8rem;
    color: var(--text-strong);
  }

  label span {
    font-weight: 500;
  }

  input {
    padding: 0.5rem 0.65rem;
    background: var(--bg-base);
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--text-strong);
    font: inherit;
  }

  input:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 25%, transparent);
  }

  small {
    color: var(--text-muted);
    font-size: 0.7rem;
    line-height: 1.4;
  }

  .error {
    padding: 0.6rem 0.75rem;
    background: color-mix(in srgb, var(--danger) 15%, transparent);
    color: var(--danger-light, var(--danger));
    border: 1px solid var(--danger);
    border-radius: 4px;
    font-size: 0.8rem;
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    margin-top: 0.25rem;
  }

  .btn-primary,
  .btn-secondary {
    padding: 0.45rem 0.95rem;
    border-radius: 4px;
    font: inherit;
    font-weight: 500;
    cursor: pointer;
    border: 1px solid transparent;
  }

  .btn-primary {
    background: var(--accent);
    color: #fff;
  }
  .btn-primary:hover:not(:disabled) {
    background: var(--accent-hover, var(--accent));
  }

  .btn-secondary {
    background: transparent;
    color: var(--text-secondary);
    border-color: var(--border);
  }
  .btn-secondary:hover:not(:disabled) {
    color: var(--text-strong);
    background: var(--bg-elevated);
  }

  button:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
</style>
