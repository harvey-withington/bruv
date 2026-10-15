<script lang="ts">
  // Settings → General: display and behaviour preferences (per device)
  // plus the vault's default category name.
  import { t, availableLocales } from '../lib/i18n.svelte'
  import { theme, setTheme } from '../lib/theme.svelte'
  import type { SettingsForm } from '../lib/settingsForm.svelte'
  import type { FieldVisible } from '../lib/settingsSearch'

  let { form, visible }: { form: SettingsForm; visible: FieldVisible } = $props()
  const prefs = $derived(form.prefs)
</script>

{#if visible('reopen_last_repo')}
  <label class="field toggle-field">
    <span class="field-label">{t('prefs.reopen_last_repo')}</span>
    <input type="checkbox" bind:checked={prefs.reopen_last_repo} />
  </label>
{/if}

{#if visible('theme')}
  <label class="field">
    <span class="field-label">{t('prefs.theme')}</span>
    <!-- Bound to the live theme store rather than prefs.theme so the
         dropdown reflects the current theme even when the sidebar toggle
         changed it after this dialog opened. setTheme applies + persists
         locally at once; Save syncs theme.mode into the prefs on disk. -->
    <select value={theme.mode} onchange={(e) => setTheme((e.currentTarget as HTMLSelectElement).value as 'dark' | 'light')}>
      <option value="dark">{t('prefs.theme_dark')}</option>
      <option value="light">{t('prefs.theme_light')}</option>
    </select>
  </label>
{/if}

{#if visible('locale')}
  <label class="field">
    <span class="field-label">{t('prefs.locale')}</span>
    <select bind:value={prefs.locale}>
      {#each availableLocales() as loc (loc)}
        <option value={loc}>{loc.toUpperCase()}</option>
      {/each}
    </select>
  </label>
{/if}

{#if visible('confirm_before_delete')}
  <label class="field toggle-field">
    <span class="field-label">{t('prefs.confirm_delete')}</span>
    <input type="checkbox" bind:checked={prefs.confirm_before_delete} />
  </label>
{/if}

{#if visible('sidebar_width')}
  <label class="field">
    <span class="field-label">{t('prefs.sidebar_width')}</span>
    <div class="range-row">
      <input type="range" min="160" max="500" step="10" bind:value={prefs.sidebar_width} />
      <span class="range-value">{prefs.sidebar_width}px</span>
    </div>
  </label>
{/if}

{#if visible('type_badge_display')}
  <label class="field">
    <span class="field-label">{t('prefs.type_badges')}</span>
    <select bind:value={prefs.type_badge_display}>
      <option value="text">{t('prefs.type_badge_text')}</option>
      <option value="color">{t('prefs.type_badge_color')}</option>
      <option value="hidden">{t('prefs.type_badge_hidden')}</option>
    </select>
  </label>
{/if}

{#if visible('default_category_name')}
  <label class="field">
    <span class="field-label">{t('prefs.default_category')}</span>
    <input type="text" bind:value={prefs.default_category_name} placeholder={t('prefs.default_category_placeholder')} />
    <span class="field-hint">{t('prefs.default_category_hint')}</span>
  </label>
{/if}

{#if visible('inbox_recent_cards_limit')}
  <label class="field">
    <span class="field-label">{t('prefs.inbox_recent_limit')}</span>
    <input type="number" min="1" max="100" bind:value={prefs.inbox_recent_cards_limit} class="number-input" />
    <span class="field-hint">{t('prefs.inbox_recent_limit_hint')}</span>
  </label>
{/if}

{#if visible('inbox_activity_limit')}
  <label class="field">
    <span class="field-label">{t('prefs.inbox_activity_limit')}</span>
    <input type="number" min="1" max="500" bind:value={prefs.inbox_activity_limit} class="number-input" />
    <span class="field-hint">{t('prefs.inbox_activity_limit_hint')}</span>
  </label>
{/if}

{#if visible('sidebar_collapse_default')}
  <label class="field toggle-field">
    <span class="field-label">{t('prefs.sidebar_collapse_default')}</span>
    <input type="checkbox" bind:checked={prefs.sidebar_collapse_default} />
  </label>
{/if}

{#if visible('local_server_port')}
  <label class="field">
    <span class="field-label">{t('prefs.local_server_port')}</span>
    <input type="number" min="0" max="65535" bind:value={prefs.local_server_port} class="number-input" />
    <span class="field-hint">{t('prefs.local_server_port_hint')}</span>
  </label>
{/if}
