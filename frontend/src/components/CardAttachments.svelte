<script lang="ts">
  import { Paperclip, Trash2, FileText, FileImage, FileVideo, File as FileIcon, Download, X, Eye } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { AddCardAttachment, RemoveCardAttachment, SignAttachmentURL } from '@shared/api'
  import type { Attachment, Card } from '@shared/types'
  import { marked } from 'marked'

  let {
    cardId,
    attachments = [],
    onCardUpdated,
  }: {
    cardId: string
    attachments: Attachment[]
    onCardUpdated: (card: Card) => void
  } = $props()

  let draggingOver = $state(false)

  function iconForMime(mime: string) {
    if (mime.startsWith('image/')) return FileImage
    if (mime.startsWith('video/')) return FileVideo
    if (mime.startsWith('text/') || mime.includes('pdf') || mime.includes('document')) return FileText
    return FileIcon
  }

  function formatSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  async function handleFiles(files: FileList | File[]) {
    for (const file of files) {
      try {
        const data = await fileToBase64(file)
        const updated = await AddCardAttachment(cardId, file.name, data) as Card
        onCardUpdated(updated)
      } catch {
        showToast(t('attachment.upload_failed'), 'error')
      }
    }
  }

  function fileToBase64(file: globalThis.File): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => {
        const result = reader.result as string
        // Strip data URL prefix
        const base64 = result.includes(',') ? result.split(',')[1] : result
        resolve(base64)
      }
      reader.onerror = reject
      reader.readAsDataURL(file)
    })
  }

  function handleDrop(e: DragEvent) {
    e.preventDefault()
    draggingOver = false
    if (e.dataTransfer?.files?.length) {
      handleFiles(e.dataTransfer.files)
    }
  }

  function handleDragOver(e: DragEvent) {
    e.preventDefault()
    draggingOver = true
  }

  function handleDragLeave() {
    draggingOver = false
  }

  let fileInputEl = $state<HTMLInputElement | null>(null)

  function openFilePicker() {
    fileInputEl?.click()
  }

  function handleFileInput(e: Event) {
    const input = e.target as HTMLInputElement
    if (input.files?.length) {
      handleFiles(input.files)
      input.value = ''
    }
  }

  // Open the attachment in a new tab via a freshly-signed URL.
  // Sign-on-click rather than render-time so the URL is fresh each
  // time (the 5-min HMAC TTL otherwise expires while the dialog is
  // open) and we don't N+1-RPC every list render.
  async function downloadAttachment(att: Attachment) {
    try {
      const url = await SignAttachmentURL(cardId, att.id)
      window.open(url, '_blank', 'noopener,noreferrer')
    } catch {
      showToast(t('attachment.download_failed'), 'error')
    }
  }

  let previewAttachment = $state<Attachment | null>(null)
  let previewUrl = $state('')
  let previewTextContent = $state('')
  let previewLoading = $state(false)
  let previewError = $state('')

  async function previewAtt(att: Attachment) {
    previewAttachment = att
    previewUrl = ''
    previewTextContent = ''
    previewError = ''
    previewLoading = true
    try {
      const url = await SignAttachmentURL(cardId, att.id)
      previewUrl = url

      const mime = att.mime.toLowerCase()
      const name = att.name.toLowerCase()
      const isText = mime.startsWith('text/') || 
                     name.endsWith('.md') || 
                     name.endsWith('.markdown') || 
                     name.endsWith('.txt') || 
                     name.endsWith('.json') || 
                     mime === 'application/json' ||
                     mime === 'application/javascript' ||
                     mime === 'text/javascript'

      if (isText) {
        const res = await fetch(url)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        previewTextContent = await res.text()
      }
    } catch (err) {
      console.error(err)
      previewError = 'Failed to load preview content.'
    } finally {
      previewLoading = false
    }
  }

  function closePreview() {
    previewAttachment = null
    previewUrl = ''
    previewTextContent = ''
    previewError = ''
  }

  async function removeAttachment(att: Attachment) {
    const ok = await showConfirm(t('attachment.remove_confirm').replace('{name}', att.name))
    if (!ok) return
    try {
      const updated = await RemoveCardAttachment(cardId, att.id) as Card
      onCardUpdated(updated)
    } catch {
      showToast(t('attachment.remove_failed'), 'error')
    }
  }
</script>

<section class="attachments-section">
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="attachments-body"
    class:drag-over={draggingOver}
    role="region"
    aria-label="Attachments drop zone"
    ondrop={handleDrop}
    ondragover={handleDragOver}
    ondragleave={handleDragLeave}
  >
    {#if attachments.length > 0}
      <div class="attachments-list">
        {#each attachments as att (att.id)}
          {@const Icon = iconForMime(att.mime)}
          <div class="attachment-item action-reveal-parent">
            <Icon size={14} />
            <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
            <span class="attachment-name clickable-preview" onclick={() => previewAtt(att)} title="Preview {att.name}">{att.name}</span>
            <span class="attachment-size">{formatSize(att.size)}</span>
            <button class="action-reveal attachment-action" onclick={() => previewAtt(att)} title="Preview"><Eye size={11} /></button>
            <button class="action-reveal attachment-action" onclick={() => downloadAttachment(att)} title={t('attachment.download')}><Download size={11} /></button>
            <button class="action-reveal action-reveal--danger attachment-remove" onclick={() => removeAttachment(att)} title={t('attachment.remove')}><Trash2 size={11} /></button>
          </div>
        {/each}
      </div>
    {/if}

    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="attachment-drop-zone" onclick={openFilePicker}>
      <Paperclip size={14} />
      <span>{t('attachment.drop_hint')}</span>
    </div>
    <input
      bind:this={fileInputEl}
      type="file"
      multiple
      class="hidden-file-input"
      onchange={handleFileInput}
    />
  </div>
</section>

{#if previewAttachment}
  <div class="preview-overlay">
    <button type="button" class="preview-backdrop-btn" onclick={closePreview} aria-label="Close preview"></button>
    <div class="preview-modal" style="position: relative; z-index: 1;">
      <header class="preview-header">
        <span class="preview-title" title={previewAttachment.name}>{previewAttachment.name}</span>
        <button class="preview-close-btn" onclick={closePreview} title={t('common.close')}>
          <X size={14} />
        </button>
      </header>
      <div class="preview-content">
        {#if previewLoading}
          <div class="preview-loader">
            <span class="spinner"></span>
            <span>Loading preview...</span>
          </div>
        {:else if previewError}
          <div class="preview-error-box">
            <p>{previewError}</p>
          </div>
        {:else}
          {@const mime = previewAttachment.mime.toLowerCase()}
          {@const name = previewAttachment.name.toLowerCase()}
          {#if mime.startsWith('image/')}
            <div class="preview-image-container">
              <img src={previewUrl} alt={previewAttachment.name} />
            </div>
          {:else if mime === 'text/html' || name.endsWith('.html')}
            <iframe src={previewUrl} title={previewAttachment.name} sandbox="allow-scripts allow-same-origin"></iframe>
          {:else if name.endsWith('.pdf') || mime.includes('pdf')}
            <iframe src={previewUrl} title={previewAttachment.name}></iframe>
          {:else if name.endsWith('.md') || name.endsWith('.markdown') || mime === 'text/markdown'}
            <div class="preview-markdown-body">
              {@html marked.parse(previewTextContent)}
            </div>
          {:else if mime.startsWith('text/') || name.endsWith('.txt') || name.endsWith('.json') || mime === 'application/json'}
            <pre class="preview-text-box"><code>{previewTextContent}</code></pre>
          {:else}
            <div class="preview-fallback">
              <p>No preview available for this file type.</p>
              <button class="btn" onclick={() => downloadAttachment(previewAttachment!)}>
                <Download size={14} />
                Download to View
              </button>
            </div>
          {/if}
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .attachments-section {
    padding-top: 0.25rem;
    height: 100%;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .attachments-body {
    padding: 0.35rem 0;
    border-radius: 6px;
    transition: background var(--duration-normal);
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
    gap: 0.35rem;
  }
  .attachments-body.drag-over {
    background: color-mix(in srgb, var(--accent) 8%, transparent);
    outline: 2px dashed var(--accent);
    outline-offset: -2px;
    border-radius: 6px;
  }

  .attachments-list {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    margin-bottom: 0.35rem;
    flex: 1;
    overflow-y: auto;
    padding-right: 0.25rem;
  }

  .attachment-item {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.25rem 0.4rem;
    border-radius: 4px;
    font-size: 0.8rem;
    color: var(--text-body);
  }
  .attachment-item:hover { background: var(--bg-elevated); }

  .attachment-name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .attachment-size {
    font-size: 0.7rem;
    color: var(--text-faint);
    flex-shrink: 0;
  }

  .attachment-remove {
    font-size: 0.7rem;
  }

  .attachment-drop-zone {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.4rem;
    padding: 0.5rem;
    border: 1px dashed var(--border);
    border-radius: 6px;
    color: var(--text-muted);
    font-size: 0.75rem;
    cursor: pointer;
    transition: border-color var(--duration-normal), color var(--duration-normal);
  }
  .attachment-drop-zone:hover {
    border-color: var(--accent);
    color: var(--accent);
  }

  .hidden-file-input {
    display: none;
  }

  .clickable-preview {
    cursor: pointer;
    transition: color var(--duration-fast);
  }
  .clickable-preview:hover {
    color: var(--accent);
    text-decoration: underline;
  }

  .preview-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.75);
    backdrop-filter: blur(4px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 99999;
    animation: fade-in 0.2s ease-out;
  }

  .preview-backdrop-btn {
    position: absolute;
    inset: 0;
    background: transparent;
    border: none;
    cursor: default;
    width: 100%;
    height: 100%;
    padding: 0;
  }

  .preview-modal {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    width: min(960px, 95vw);
    height: 85vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 20px 50px rgba(0, 0, 0, 0.5);
    overflow: hidden;
  }

  .preview-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--border-muted);
    background: var(--bg-elevated);
  }

  .preview-title {
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 80%;
  }

  .preview-close-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 0.25rem;
    border-radius: 4px;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: background 0.15s, color 0.15s;
  }
  .preview-close-btn:hover {
    color: var(--text-primary);
    background: var(--bg-elevated);
  }

  .preview-content {
    flex: 1;
    overflow: auto;
    position: relative;
    display: flex;
    flex-direction: column;
    background: var(--bg-surface);
  }

  .preview-loader, .preview-error-box, .preview-fallback {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.75rem;
    padding: 2rem;
    margin: auto;
    color: var(--text-secondary);
  }

  .preview-error-box p {
    color: var(--danger, #e53935);
    font-weight: 500;
  }

  .preview-image-container {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    height: 100%;
    padding: 1rem;
    box-sizing: border-box;
  }

  .preview-image-container img {
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
    border-radius: 4px;
    box-shadow: 0 4px 16px rgba(0,0,0,0.2);
  }

  .preview-markdown-body {
    padding: 1.5rem;
    font-size: 0.9rem;
    line-height: 1.6;
    color: var(--text-body);
    overflow-y: auto;
    height: 100%;
    box-sizing: border-box;
  }

  .preview-markdown-body :global(h1),
  .preview-markdown-body :global(h2),
  .preview-markdown-body :global(h3) {
    margin-top: 1.5rem;
    margin-bottom: 0.75rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .preview-markdown-body :global(h1) { font-size: 1.4rem; border-bottom: 1px solid var(--border-muted); padding-bottom: 0.3rem; }
  .preview-markdown-body :global(h2) { font-size: 1.2rem; }
  .preview-markdown-body :global(h3) { font-size: 1.05rem; }
  .preview-markdown-body :global(p) { margin-bottom: 1rem; }
  .preview-markdown-body :global(pre) {
    background: var(--bg-elevated);
    border: 1px solid var(--border-muted);
    border-radius: 6px;
    padding: 0.75rem;
    overflow-x: auto;
    margin-bottom: 1rem;
  }
  .preview-markdown-body :global(code) {
    font-family: var(--font-mono, monospace);
    font-size: 0.85rem;
    background: var(--bg-elevated);
    padding: 0.15rem 0.3rem;
    border-radius: 3px;
  }
  .preview-markdown-body :global(pre code) {
    padding: 0;
    background: none;
  }

  .preview-text-box {
    margin: 0;
    padding: 1.25rem;
    font-family: var(--font-mono, monospace);
    font-size: 0.85rem;
    line-height: 1.45;
    color: var(--text-body);
    background: var(--bg-surface);
    overflow: auto;
    height: 100%;
    box-sizing: border-box;
    white-space: pre-wrap;
    word-break: break-all;
  }

  .preview-content iframe {
    width: 100%;
    height: 100%;
    border: none;
    background: white;
  }

  .spinner {
    width: 24px;
    height: 24px;
    border: 2px solid var(--border-muted);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
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
    background: var(--accent);
    color: #fff;
    transition: filter 0.15s;
  }
  .btn:hover {
    filter: brightness(1.1);
  }
</style>
