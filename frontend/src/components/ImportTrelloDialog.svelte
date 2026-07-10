<script lang="ts">
  import { X, FileJson, UploadCloud } from 'lucide-svelte'
  import { fade } from 'svelte/transition'
  import { t } from '../lib/i18n.svelte'
  import { focusTrap } from '../lib/actions'
  import { showToast } from '../lib/toast.svelte'
  import { ImportTrelloBoardFromJSON, GetPreferences, SetPreferences } from '@shared/api'
  import type { TrelloArchiveMode, TrelloImportResult } from '@shared/types'

  let {
    brandSlug,
    streamSlug,
    onClose,
    onImported,
  }: {
    brandSlug: string
    streamSlug: string
    onClose: () => void
    onImported: (brandSlug: string, streamSlug: string, result: TrelloImportResult) => void
  } = $props()

  // Read the file's bytes client-side and ship the JSON content to
  // the backend (rather than a filesystem path). Two reasons:
  //   1. Path-passing only works when the backend can see the same
  //      filesystem as the user. Remote BRUV servers can't open
  //      C:\Users\harve\Downloads\foo.json.
  //   2. Drag-and-drop uses the browser's File API which gives us
  //      bytes directly. Unifying picker + drop on the same code
  //      path means we always ship JSON, never paths.
  let fileContent = $state<string | null>(null)
  let fileName = $state('')
  let fileInputEl = $state<HTMLInputElement | null>(null)
  let archiveMode = $state<TrelloArchiveMode>('archive')
  let apiKey = $state('')
  let apiToken = $state('')
  let importing = $state(false)
  let errorMsg = $state('')
  let dragActive = $state(false)

  // Load saved credentials from backend preferences on mount
  $effect(() => {
    loadPrefs()
  })

  async function loadPrefs() {
    try {
      const prefs = await GetPreferences()
      if (prefs) {
        apiKey = prefs.trello_api_key || ''
        apiToken = prefs.trello_api_token || ''
      }
    } catch (e) {
      console.error('Failed to load preferences', e)
      showToast(t('import.trello.prefs_load_failed'), 'error')
    }
  }

  const ARCHIVE_CHOICES: Array<{ value: TrelloArchiveMode; label: string; hint: string }> = [
    { value: 'archive', label: t('import.trello.archive.archive'), hint: t('import.trello.archive.archive_hint') },
    { value: 'skip',    label: t('import.trello.archive.skip'),    hint: t('import.trello.archive.skip_hint') },
    { value: 'inline',  label: t('import.trello.archive.inline'),  hint: t('import.trello.archive.inline_hint') },
  ]

  // Programmatically click the hidden <input type="file"> so the
  // "Choose File" button keeps the same UX without us owning the
  // OS-dialog plumbing. Browser File API works everywhere — Wails
  // webview, browser-mode against a remote, all the same.
  function pickFile() {
    fileInputEl?.click()
  }

  async function handleFileInput(e: Event) {
    const input = e.currentTarget as HTMLInputElement
    const file = input.files?.[0]
    if (!file) return
    await loadFile(file)
    // Reset the input so picking the same file twice in a row still fires onchange.
    input.value = ''
  }

  async function loadFile(file: File) {
    if (!file.name.toLowerCase().endsWith('.json')) {
      errorMsg = t('import.trello.parse_failed')
      return
    }
    try {
      const text = await file.text()
      fileContent = text
      fileName = file.name
      errorMsg = ''
    } catch (e) {
      console.error('read file', e)
      errorMsg = t('import.trello.read_failed')
    }
  }

  function handleDragOver(e: DragEvent) {
    e.preventDefault()
    e.stopPropagation()
    dragActive = true
  }

  function handleDragLeave(e: DragEvent) {
    e.preventDefault()
    e.stopPropagation()
    dragActive = false
  }

  async function handleDrop(e: DragEvent) {
    e.preventDefault()
    e.stopPropagation()
    dragActive = false
    const file = e.dataTransfer?.files?.[0]
    if (!file) return
    await loadFile(file)
  }

  async function runImport() {
    if (importing) return
    if (!fileContent) {
      errorMsg = t('import.trello.no_file')
      return
    }
    if (!brandSlug || !streamSlug) {
      errorMsg = t('import.trello.no_target')
      return
    }
    importing = true
    errorMsg = ''
    try {
      const result = await ImportTrelloBoardFromJSON(brandSlug, streamSlug, fileContent, archiveMode, apiKey || undefined, apiToken || undefined)
      
      // Save credentials back to backend preferences
      try {
        const prefs = await GetPreferences() || {}
        prefs.trello_api_key = apiKey
        prefs.trello_api_token = apiToken
        await SetPreferences(prefs)
      } catch (pe) {
        console.error('Failed to save preferences', pe)
      }

      showToast(
        t('import.trello.success', {
          project: result.project_name,
          cards: result.cards,
          categories: result.categories,
        }),
        'success',
      )
      onImported(brandSlug, streamSlug, result)
    } catch (e) {
      console.error('ImportTrelloBoard', e)
      const msg = e instanceof Error ? e.message : String(e)
      errorMsg = msg.includes('trello') || msg.includes('board')
        ? t('import.trello.parse_failed')
        : `${t('import.trello.failed')}: ${msg}`
    } finally {
      importing = false
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && !importing) {
      e.preventDefault()
      onClose()
    }
  }

  const sourceLabel = $derived(fileName)
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="import-backdrop" role="presentation" onclick={() => !importing && onClose()} out:fade={{ duration: 150 }}>
  <div class="import-dialog" role="dialog" aria-modal="true" tabindex="-1" onclick={(e) => e.stopPropagation()} use:focusTrap>
    <header class="import-header">
      <FileJson size={16} />
      <h2 class="import-title">{t('import.trello.title')}</h2>
      <button class="import-close" onclick={onClose} disabled={importing} title={t('common.close')}>
        <X size={14} />
      </button>
    </header>

    <div class="import-body">
      <div class="import-field">
        <span class="import-label">{t('import.trello.source')}</span>
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="import-dropzone"
          class:active={dragActive}
          class:has-file={!!sourceLabel}
          ondragover={handleDragOver}
          ondragleave={handleDragLeave}
          ondrop={handleDrop}
        >
          <UploadCloud size={20} />
          {#if sourceLabel}
            <p class="import-dropzone-file">{sourceLabel}</p>
          {:else}
            <p class="import-dropzone-hint">{t('import.trello.drop_hint')}</p>
          {/if}
          <button class="import-pick-btn" onclick={pickFile} disabled={importing}>
            {t('import.trello.choose_file')}
          </button>
          <input
            type="file"
            accept=".json,application/json"
            bind:this={fileInputEl}
            onchange={handleFileInput}
            class="import-file-input"
          />
        </div>
      </div>

      <div class="import-field">
        <span class="import-label">{t('import.trello.creds_label')}</span>
        <div class="import-creds-row">
          <input
            type="text"
            placeholder={t('import.trello.creds_api_key_placeholder')}
            bind:value={apiKey}
            class="import-input"
            disabled={importing}
          />
          <input
            type="password"
            placeholder={t('import.trello.creds_api_token_placeholder')}
            bind:value={apiToken}
            class="import-input"
            disabled={importing}
          />
        </div>
        <p class="import-help-text">
          {@html t('import.trello.creds_help')}
        </p>
      </div>

      <div class="import-field">
        <span class="import-label">{t('import.trello.archive')}</span>
        <div class="import-archive-choices">
          {#each ARCHIVE_CHOICES as choice}
            <label class="import-archive-choice" class:active={archiveMode === choice.value}>
              <input
                type="radio"
                name="archive-mode"
                value={choice.value}
                checked={archiveMode === choice.value}
                onchange={() => archiveMode = choice.value}
              />
              <div class="import-archive-text">
                <span class="import-archive-label">{choice.label}</span>
                <span class="import-archive-hint">{choice.hint}</span>
              </div>
            </label>
          {/each}
        </div>
      </div>

      {#if errorMsg}
        <p class="import-error">{errorMsg}</p>
      {/if}
    </div>

    <footer class="import-footer">
      <button class="import-cancel" onclick={onClose} disabled={importing}>
        {t('import.trello.cancel_btn')}
      </button>
      <button class="import-submit" onclick={runImport} disabled={importing || !fileContent}>
        {importing ? t('import.trello.importing') : t('import.trello.import_btn')}
      </button>
    </footer>
  </div>
</div>

<style>
  .import-backdrop {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 99990;
    animation: fade-in var(--duration-normal) var(--ease-out);
  }

  .import-dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    width: min(520px, 90vw);
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    box-shadow: 0 12px 48px rgba(0, 0, 0, 0.35);
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }

  .import-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--border);
    color: var(--text-primary);
  }
  .import-title {
    margin: 0;
    font-size: 0.95rem;
    font-weight: 600;
    flex: 1;
  }
  .import-close {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 0.2rem;
    border-radius: 4px;
    display: flex;
    align-items: center;
  }
  .import-close:hover { color: var(--text-primary); background: var(--bg-elevated); }
  .import-close:disabled { opacity: 0.4; cursor: not-allowed; }

  .import-body {
    padding: 1rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
    overflow-y: auto;
  }

  .import-field { display: flex; flex-direction: column; gap: 0.35rem; }

  .import-label {
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted);
  }

  .import-creds-row {
    display: flex;
    gap: 0.5rem;
  }

  .import-input {
    flex: 1;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 0.4rem 0.6rem;
    font-size: 0.85rem;
    color: var(--text-primary);
    font-family: inherit;
    transition: border-color 0.15s;
  }

  .import-input:focus {
    border-color: var(--accent);
    outline: none;
  }

  .import-help-text {
    margin: 0.2rem 0 0;
    font-size: 0.75rem;
    color: var(--text-muted);
  }

  .import-help-text :global(a) {
    color: var(--accent);
    text-decoration: none;
  }

  .import-help-text :global(a:hover) {
    text-decoration: underline;
  }

  .import-dropzone {
    border: 2px dashed var(--border);
    border-radius: 6px;
    padding: 1rem;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.5rem;
    color: var(--text-muted);
    background: var(--bg-elevated);
    transition: border-color 0.15s, background 0.15s;
  }
  .import-dropzone.active {
    border-color: var(--accent);
    background: var(--bg-surface);
  }
  .import-dropzone.has-file {
    border-style: solid;
    border-color: var(--accent);
  }

  .import-dropzone-hint {
    margin: 0;
    font-size: 0.8rem;
    color: var(--text-muted);
  }
  .import-dropzone-file {
    margin: 0;
    font-size: 0.85rem;
    color: var(--text-primary);
    font-weight: 500;
    word-break: break-all;
    text-align: center;
  }

  .import-pick-btn {
    padding: 0.35rem 0.8rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-surface);
    color: var(--text-body);
    font-size: 0.8rem;
    font-family: inherit;
    cursor: pointer;
  }
  .import-pick-btn:hover { background: var(--accent); color: #fff; border-color: var(--accent); }
  .import-pick-btn:disabled { opacity: 0.5; cursor: not-allowed; }

  /* Hidden file input — triggered programmatically by the Choose File
     button. Display:none breaks accessibility tooling on some
     browsers, so use opacity + position trickery instead. */
  .import-file-input {
    position: absolute;
    width: 1px;
    height: 1px;
    opacity: 0;
    pointer-events: none;
  }

  .import-archive-choices {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .import-archive-choice {
    display: flex;
    gap: 0.5rem;
    align-items: flex-start;
    padding: 0.55rem 0.7rem;
    border: 1px solid var(--border);
    border-radius: 6px;
    cursor: pointer;
    background: var(--bg-elevated);
  }
  .import-archive-choice:hover { border-color: var(--accent); }
  .import-archive-choice.active {
    border-color: var(--accent);
    background: var(--bg-surface);
  }
  .import-archive-choice input[type="radio"] {
    margin-top: 0.25rem;
    accent-color: var(--accent);
  }

  .import-archive-text {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }
  .import-archive-label {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--text-primary);
  }
  .import-archive-hint {
    font-size: 0.75rem;
    color: var(--text-muted);
  }

  .import-error {
    margin: 0;
    padding: 0.5rem 0.7rem;
    background: color-mix(in srgb, var(--danger, #e53935) 15%, transparent);
    border: 1px solid var(--danger, #e53935);
    color: var(--danger, #e53935);
    border-radius: 4px;
    font-size: 0.8rem;
  }

  .import-footer {
    padding: 0.75rem 1rem;
    border-top: 1px solid var(--border);
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    background: var(--bg-elevated);
  }

  .import-cancel,
  .import-submit {
    padding: 0.4rem 1rem;
    border-radius: 4px;
    font-size: 0.85rem;
    font-family: inherit;
    cursor: pointer;
    border: 1px solid var(--border);
  }
  .import-cancel {
    background: var(--bg-surface);
    color: var(--text-body);
  }
  .import-cancel:hover { background: var(--bg-elevated); }
  .import-submit {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
  }
  .import-submit:hover:not(:disabled) { filter: brightness(1.1); }
  .import-cancel:disabled,
  .import-submit:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
