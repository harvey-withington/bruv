<script lang="ts">
  import { X } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { GetLLMConfig, SetLLMConfig } from '../lib/api'
  import { draggable } from '../lib/draggable'

  let { onClose }: { onClose: () => void } = $props()

  let config = $state({
    context: '',
  })
  let loaded = $state(false)
  let saved = $state(false)

  $effect(() => {
    load()
  })

  async function load() {
    try {
      const c = await GetLLMConfig()
      if (c) {
        config.context = c.context || ''
      }
    } catch { /* use defaults */ }
    loaded = true
  }

  async function save() {
    try {
      await SetLLMConfig(config)
      onClose()
    } catch (e) { console.error('SetLLMConfig:', e) }
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
<div class="overlay" role="presentation" onclick={handleOverlayClick}>
  <div class="dialog" use:draggable={{ handle: '.dialog-header' }}>
    <div class="dialog-header">
      <h2>{t('llm.title')}</h2>
      <button class="close-btn" onclick={onClose} title={t('common.close')}><X size={18} /></button>
    </div>

    {#if loaded}
      <div class="dialog-body">
        <label class="field">
          <span class="field-label">{t('llm.context')}</span>
          <textarea rows="6" bind:value={config.context} placeholder={t('llm.context_placeholder')}></textarea>
        </label>
      </div>

      <div class="dialog-footer">
        {#if saved}<span class="saved-msg">{t('llm.saved')}</span>{/if}
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
    width: 480px;
    max-height: 85vh;
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

  .field-label {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--text-secondary);
  }

  textarea {
    padding: 0.45rem 0.6rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    font-family: inherit;
    outline: none;
    resize: vertical;
  }
  textarea:focus {
    border-color: var(--accent);
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
