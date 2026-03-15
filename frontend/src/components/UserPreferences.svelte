<script lang="ts">
  import { X } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { GetPreferences, SetPreferences } from '../lib/api'
  import { theme, setTheme } from '../lib/theme.svelte'
  import { setLocale, availableLocales } from '../lib/i18n.svelte'
  import { nav } from '../lib/store.svelte'

  let { onClose }: { onClose: () => void } = $props()

  let prefs = $state({
    reopen_last_repo: false,
    theme: 'dark',
    locale: 'en',
    confirm_before_delete: true,
    sidebar_width: 260,
  })
  let loaded = $state(false)
  let saved = $state(false)

  $effect(() => {
    load()
  })

  async function load() {
    try {
      const p = await GetPreferences()
      if (p) {
        prefs.reopen_last_repo = p.reopen_last_repo ?? false
        prefs.theme = p.theme || 'dark'
        prefs.locale = p.locale || 'en'
        prefs.confirm_before_delete = p.confirm_before_delete ?? true
        prefs.sidebar_width = p.sidebar_width || 260
      }
    } catch { /* use defaults */ }
    loaded = true
  }

  async function save() {
    try {
      await SetPreferences(prefs)

      // Apply theme immediately
      setTheme(prefs.theme as 'dark' | 'light')

      // Apply locale immediately
      setLocale(prefs.locale)

      // Apply sidebar width
      nav.sidebarWidth = prefs.sidebar_width
      localStorage.setItem('bruv-sidebar-width', String(prefs.sidebar_width))

      onClose()
    } catch (e) { console.error('SavePreferences:', e) }
  }

  function handleOverlayClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
<div class="overlay" onclick={handleOverlayClick}>
  <div class="dialog">
    <div class="dialog-header">
      <h2>{t('prefs.title')}</h2>
      <button class="close-btn" onclick={onClose} title={t('common.close')}><X size={18} /></button>
    </div>

    {#if loaded}
      <div class="dialog-body">
        <label class="field toggle-field">
          <span class="field-label">{t('prefs.reopen_last_repo')}</span>
          <input type="checkbox" bind:checked={prefs.reopen_last_repo} />
        </label>

        <label class="field">
          <span class="field-label">{t('prefs.theme')}</span>
          <select bind:value={prefs.theme}>
            <option value="dark">{t('prefs.theme_dark')}</option>
            <option value="light">{t('prefs.theme_light')}</option>
          </select>
        </label>

        <label class="field">
          <span class="field-label">{t('prefs.locale')}</span>
          <select bind:value={prefs.locale}>
            {#each availableLocales() as loc}
              <option value={loc}>{loc.toUpperCase()}</option>
            {/each}
          </select>
        </label>

        <label class="field toggle-field">
          <span class="field-label">{t('prefs.confirm_delete')}</span>
          <input type="checkbox" bind:checked={prefs.confirm_before_delete} />
        </label>

        <label class="field">
          <span class="field-label">{t('prefs.sidebar_width')}</span>
          <div class="range-row">
            <input type="range" min="160" max="500" step="10" bind:value={prefs.sidebar_width} />
            <span class="range-value">{prefs.sidebar_width}px</span>
          </div>
        </label>
      </div>

      <div class="dialog-footer">
        {#if saved}<span class="saved-msg">{t('prefs.saved')}</span>{/if}
        <button class="btn btn-ghost" onclick={onClose}>{t('common.cancel')}</button>
        <button class="btn btn-primary" onclick={save}>{t('common.save')}</button>
      </div>
    {/if}
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }

  .dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    width: 440px;
    max-height: 80vh;
    overflow-y: auto;
    box-shadow: 0 8px 32px var(--shadow-lg);
  }

  .dialog-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
  }

  .dialog-header h2 {
    margin: 0;
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text-primary);
  }

  .close-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
    border-radius: 4px;
    display: flex;
    align-items: center;
  }
  .close-btn:hover { color: var(--text-primary); }

  .dialog-body {
    padding: 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .toggle-field {
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
  }

  .field-label {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--text-secondary);
  }

  select, input[type="text"], input[type="number"] {
    padding: 0.45rem 0.6rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    outline: none;
  }
  select:focus, input[type="text"]:focus, input[type="number"]:focus {
    border-color: var(--accent);
  }

  input[type="checkbox"] {
    accent-color: var(--accent);
    width: 16px;
    height: 16px;
    cursor: pointer;
  }

  input[type="range"] {
    flex: 1;
    accent-color: var(--accent);
    cursor: pointer;
  }

  .range-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }

  .range-value {
    font-size: 0.8rem;
    color: var(--text-muted);
    min-width: 48px;
    text-align: right;
  }

  .dialog-footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.5rem;
    padding: 0.75rem 1.25rem;
    border-top: 1px solid var(--border-muted);
  }

  .saved-msg {
    font-size: 0.8rem;
    color: var(--success);
    margin-right: auto;
  }

  .btn {
    padding: 0.45rem 1rem;
    border-radius: 6px;
    font-size: 0.85rem;
    font-weight: 500;
    cursor: pointer;
    border: none;
  }

  .btn-primary {
    background: var(--accent);
    color: #fff;
  }
  .btn-primary:hover { background: var(--accent-hover); }

  .btn-ghost {
    background: transparent;
    color: var(--text-secondary);
  }
  .btn-ghost:hover { color: var(--text-primary); }
</style>
