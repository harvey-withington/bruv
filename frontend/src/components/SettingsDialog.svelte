<script lang="ts">
  // Settings dialog shell: header with search, tab bar, the active tab,
  // and Save. The working copy, per-section load/save and Save's side
  // effects live in lib/settingsForm.svelte.ts; each tab is its own
  // component; the field styles are global (.settings-form in style.css).
  import { X, Search } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { draggable } from '../lib/draggable'
  import { fade } from 'svelte/transition'
  import { focusTrap } from '../lib/actions'
  import { SettingsForm } from '../lib/settingsForm.svelte'
  import { SETTINGS_TABS, CAPTURE_KEYS, matchSettings, tabsMatching, type SettingsTabId } from '../lib/settingsSearch'
  import SettingsSectionGate from './SettingsSectionGate.svelte'
  import SettingsGeneralTab from './SettingsGeneralTab.svelte'
  import SettingsAITab from './SettingsAITab.svelte'
  import SettingsNotificationsTab from './SettingsNotificationsTab.svelte'
  import SlideTemplatePrefsSection from './SlideTemplatePrefsSection.svelte'
  import CaptureSettingsSection from './CaptureSettingsSection.svelte'

  let { onClose, initialTab }: { onClose: () => void; initialTab?: SettingsTabId } = $props()

  // initialTab is intentionally read once at mount time — the dialog is
  // re-created on every open, so reactive capture would be wrong here.
  // svelte-ignore state_referenced_locally
  let activeTab = $state<SettingsTabId>(initialTab ?? 'general')
  let searchQuery = $state('')
  let searchInputEl = $state<HTMLInputElement | null>(null)

  const form = new SettingsForm()
  $effect(() => { void form.load() })

  async function save() {
    const failed = await form.save()
    if (failed.length > 0) {
      // Keep the dialog open with the user's edits so they can retry.
      showToast(t('prefs.save_some_failed', { sections: failed.join(', ') }), 'error')
      return
    }
    onClose()
  }

  // --- Search ---
  let matchingKeys = $derived(matchSettings(searchQuery))
  let matchingTabs = $derived(tabsMatching(matchingKeys))
  const visible = (key: string): boolean => !matchingKeys || matchingKeys.has(key)

  // Auto-switch to the first matching tab when searching.
  $effect(() => {
    if (matchingTabs && !matchingTabs.has(activeTab)) {
      const first = SETTINGS_TABS.find(tab => matchingTabs!.has(tab.id))
      if (first) activeTab = first.id
    }
  })

  function handleOverlayClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" role="presentation" onclick={handleOverlayClick} out:fade={{ duration: 150 }}>
  <div class="dialog" use:draggable={{ handle: '.dialog-header' }} use:focusTrap>
    <div class="dialog-header">
      <h2>{t('prefs.title')}</h2>
      <div class="search-box">
        <Search size={14} />
        <input
          bind:this={searchInputEl}
          bind:value={searchQuery}
          type="text"
          placeholder={t('prefs.search_placeholder')}
          class="search-input"
        />
        {#if searchQuery}
          <button class="search-clear" onclick={() => { searchQuery = ''; searchInputEl?.focus() }}><X size={12} /></button>
        {/if}
      </div>
      <button class="close-btn" onclick={onClose} title={t('common.close')}><X size={18} /></button>
    </div>

    <div class="tab-bar">
      {#each SETTINGS_TABS as tab (tab.id)}
        <button
          class="tab-btn"
          class:active={activeTab === tab.id}
          class:dimmed={matchingTabs !== null && !matchingTabs.has(tab.id)}
          onclick={() => activeTab = tab.id}
        >{t(tab.labelKey)}</button>
      {/each}
    </div>

    <div class="dialog-body settings-form">
      {#if activeTab === 'general'}
        <SettingsSectionGate sections={form.sections} ids={['prefs', 'ui']} labels={form.labels}>
          <SettingsGeneralTab {form} {visible} />
        </SettingsSectionGate>
      {:else if activeTab === 'ai'}
        <SettingsSectionGate sections={form.sections} ids={['llmConfig', 'llmProviders']} labels={form.labels}>
          <SettingsAITab {form} {visible} />
        </SettingsSectionGate>
      {:else if activeTab === 'notifications'}
        <SettingsSectionGate sections={form.sections} ids={['notify', 'dueDate']} labels={form.labels}>
          <SettingsNotificationsTab {form} {visible} />
        </SettingsSectionGate>
      {:else if activeTab === 'templates'}
        <!-- Vault-level auto-matching prefs; the section persists its own
             changes (no dialog Save involved). -->
        {#if visible('slide_templates')}
          <SlideTemplatePrefsSection />
        {/if}
      {:else if activeTab === 'capture'}
        <!-- Vault-level capture defaults, saved with the dialog. -->
        <SettingsSectionGate sections={form.sections} ids={['capture']} labels={form.labels}>
          {#if CAPTURE_KEYS.some(visible)}
            <CaptureSettingsSection bind:prefs={form.capture} />
          {/if}
        </SettingsSectionGate>
      {/if}
    </div>

    <div class="dialog-footer">
      <button class="btn btn-ghost" onclick={onClose}>{t('common.cancel')}</button>
      <button class="btn btn-primary" onclick={save} disabled={form.saving || form.sections.anyLoading}>{t('common.save')}</button>
    </div>
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
    animation: fade-in var(--duration-normal) var(--ease-out);
  }

  .dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    width: 540px;
    max-height: 85vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px var(--shadow-lg);
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }

  .dialog-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
  }

  .dialog-header h2 {
    margin: 0;
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text-primary);
    white-space: nowrap;
  }

  .search-box {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0rem 0.6rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-muted);
  }
  .search-box:focus-within {
    border-color: var(--accent);
  }

  .search-input {
    flex: 1;
    padding: 0.45rem 0;
    border: none;
    background: transparent;
    color: var(--text-primary);
    font-size: 0.8rem;
    outline: none;
    min-width: 0;
  }
  .search-input::placeholder {
    color: var(--text-muted);
  }

  .search-clear {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0;
    display: flex;
    align-items: center;
  }
  .search-clear:hover { color: var(--text-primary); }

  .close-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
    border-radius: 4px;
    display: flex;
    align-items: center;
    flex-shrink: 0;
  }
  .close-btn:hover { color: var(--text-primary); }

  .tab-bar {
    display: flex;
    gap: 0;
    padding: 0 1.25rem;
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
  }

  .tab-btn {
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    color: var(--text-secondary);
    font-size: 0.82rem;
    font-weight: 500;
    padding: 0.6rem 1rem;
    cursor: pointer;
    transition: color var(--duration-normal), border-color var(--duration-normal);
  }
  .tab-btn:hover { color: var(--text-primary); }
  .tab-btn.active {
    color: var(--accent);
    border-bottom-color: var(--accent);
  }
  .tab-btn.dimmed {
    opacity: 0.35;
  }

  .dialog-body {
    padding: 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    overflow-y: auto;
    flex: 1;
    min-height: 0;
  }

  .dialog-footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.5rem;
    padding: 0.75rem 1.25rem;
    border-top: 1px solid var(--border-muted);
    flex-shrink: 0;
  }

  .btn {
    padding: 0.45rem 1rem;
    border-radius: 6px;
    font-size: 0.85rem;
    font-weight: 500;
    cursor: pointer;
    border: none;
    display: flex;
    align-items: center;
    gap: 0.35rem;
  }

  .btn-primary {
    background: var(--accent);
    color: #fff;
  }
  .btn-primary:hover:not(:disabled) { background: var(--accent-hover); }
  .btn-primary:disabled { opacity: 0.55; cursor: default; }

  .btn-ghost {
    background: transparent;
    color: var(--text-secondary);
  }
  .btn-ghost:hover { color: var(--text-primary); }
</style>
