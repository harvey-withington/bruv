<script lang="ts">
  // Settings → Notifications: system notification test, email (SMTP) and
  // webhook delivery, and due-date reminders.
  //
  // No in-app master toggle for system notifications: per-agent /
  // per-alarm channel selection is authoritative; Windows Settings →
  // Notifications → BRUV mutes everything.
  import { Bell, Eye, EyeOff } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { TestSystemNotification } from '@shared/api'
  import type { SettingsForm } from '../lib/settingsForm.svelte'
  import type { FieldVisible } from '../lib/settingsSearch'

  let { form, visible }: { form: SettingsForm; visible: FieldVisible } = $props()
  const n = $derived(form.notify)
  const dd = $derived(form.dueDate)

  let showSmtpPassword = $state(false)
  let testingSystem = $state(false)

  async function testSystemNotif() {
    testingSystem = true
    try {
      await TestSystemNotification()
      showToast(t('notifications.test_system_ok'), 'success')
    } catch {
      showToast(t('notifications.test_system_fail'), 'error')
    }
    testingSystem = false
  }
</script>

{#if visible('system_enabled')}
  <div class="field-row">
    <span class="field-label">{t('notifications.system_enabled')}</span>
    <div class="field-value field-value--action">
      <button class="btn" onclick={testSystemNotif} disabled={testingSystem}>
        <Bell size={14} />
        {testingSystem ? '…' : t('notifications.test_system')}
      </button>
    </div>
  </div>
{/if}

{#if visible('smtp')}
  <div class="field-section-label">{t('notifications.email_title')}</div>
  <div class="field-row">
    <span class="field-label">{t('notifications.smtp_host')}</span>
    <div class="field-value"><input type="text" bind:value={n.smtp_host} placeholder="smtp.gmail.com" /></div>
  </div>
  <div class="field-row">
    <span class="field-label">{t('notifications.smtp_port')}</span>
    <div class="field-value"><input type="number" class="field-input-short" bind:value={n.smtp_port} /></div>
  </div>
  <div class="field-row">
    <span class="field-label">{t('notifications.smtp_username')}</span>
    <div class="field-value"><input type="text" bind:value={n.smtp_username} /></div>
  </div>
  <div class="field-row">
    <span class="field-label">{t('notifications.smtp_password')}</span>
    <div class="field-value">
      <div class="key-row">
        <input type={showSmtpPassword ? 'text' : 'password'} bind:value={n.smtp_password} />
        <button class="icon-toggle" onclick={() => showSmtpPassword = !showSmtpPassword} title={showSmtpPassword ? t('llm.hide_key') : t('llm.show_key')}>
          {#if showSmtpPassword}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
        </button>
      </div>
    </div>
  </div>
  <div class="field-row">
    <span class="field-label">{t('notifications.smtp_from')}</span>
    <div class="field-value"><input type="email" bind:value={n.smtp_from_addr} placeholder="bruv@example.com" /></div>
  </div>
  <div class="field-row">
    <span class="field-label">{t('notifications.smtp_to')}</span>
    <div class="field-value"><input type="email" bind:value={n.smtp_to_addr} placeholder="you@example.com" /></div>
  </div>
  <div class="field-row">
    <span class="field-label">{t('notifications.smtp_tls')}</span>
    <div class="field-value"><input type="checkbox" bind:checked={n.smtp_tls} /></div>
  </div>
{/if}

{#if visible('webhook')}
  <div class="field-section-label">{t('notifications.webhook_title')}</div>
  <div class="field-row">
    <span class="field-label">{t('notifications.webhook_url')}</span>
    <div class="field-value"><input type="url" bind:value={n.webhook_url} placeholder="https://example.com/webhook" /></div>
  </div>
  <div class="field-row">
    <span class="field-label">{t('notifications.webhook_auth')}</span>
    <div class="field-value"><input type="text" bind:value={n.webhook_auth_header} placeholder="Bearer token..." /></div>
  </div>
{/if}

{#if visible('due_date')}
  <div class="field-section-label">{t('prefs.due_date_title')}</div>
  <div class="field-row">
    <span class="field-label">{t('prefs.due_date_enabled')}</span>
    <div class="field-value"><input type="checkbox" bind:checked={dd.enabled} /></div>
  </div>
  {#if dd.enabled}
    <div class="field-row">
      <span class="field-label">{t('prefs.due_date_thresholds')}</span>
      <div class="field-value checkbox-group">
        <label class="toggle"><input type="checkbox" bind:checked={dd.thresholds['24h']} /> {t('prefs.due_date_24h')}</label>
        <label class="toggle"><input type="checkbox" bind:checked={dd.thresholds['1h']} /> {t('prefs.due_date_1h')}</label>
        <label class="toggle"><input type="checkbox" bind:checked={dd.thresholds['0']} /> {t('prefs.due_date_at_due')}</label>
        <label class="toggle"><input type="checkbox" bind:checked={dd.thresholds['overdue']} /> {t('prefs.due_date_overdue')}</label>
      </div>
    </div>
    <div class="field-row">
      <span class="field-label">{t('prefs.due_date_channels')}</span>
      <div class="field-value checkbox-group">
        <label class="toggle"><input type="checkbox" checked disabled /> {t('agent.channel_inapp')}</label>
        <label class="toggle"><input type="checkbox" bind:checked={dd.channels['system']} /> {t('agent.channel_system')}</label>
        <label class="toggle"><input type="checkbox" bind:checked={dd.channels['email']} /> {t('agent.channel_email')}</label>
        <label class="toggle"><input type="checkbox" bind:checked={dd.channels['webhook']} /> {t('agent.channel_webhook')}</label>
        <span class="field-hint">{t('notifications.channels_hint')}</span>
      </div>
    </div>
  {/if}
{/if}

<style>
  .icon-toggle {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
    display: flex;
    align-items: center;
  }
  .icon-toggle:hover { color: var(--text-primary); }
</style>
