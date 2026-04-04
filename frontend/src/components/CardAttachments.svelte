<script lang="ts">
  import { Paperclip, Trash2, ChevronDown, ChevronRight, FileText, FileImage, FileVideo, File as FileIcon } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { AddCardAttachment, RemoveCardAttachment } from '../lib/api'
  import type { Attachment, Card } from '../lib/types'

  let {
    cardId,
    attachments = [],
    onCardUpdated,
  }: {
    cardId: string
    attachments: Attachment[]
    onCardUpdated: (card: Card) => void
  } = $props()

  let collapsed = $state(false)
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
  <button class="attachments-header" onclick={() => collapsed = !collapsed}>
    {#if collapsed}
      <ChevronRight size={14} />
    {:else}
      <ChevronDown size={14} />
    {/if}
    <Paperclip size={13} />
    <span class="attachments-title">{t('attachment.title')}</span>
    {#if attachments.length > 0}
      <span class="attachments-count">{attachments.length}</span>
    {/if}
  </button>

  {#if !collapsed}
    <div
      class="attachments-body"
      class:drag-over={draggingOver}
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
              <span class="attachment-name" title={att.name}>{att.name}</span>
              <span class="attachment-size">{formatSize(att.size)}</span>
              <button class="action-reveal action-reveal--danger attachment-remove" onclick={() => removeAttachment(att)} title="Remove"><Trash2 size={11} /></button>
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
  {/if}
</section>

<style>
  .attachments-section {
    border-top: 1px solid var(--border-muted);
    padding-top: 0.5rem;
  }

  .attachments-header {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-secondary);
    font-size: 0.8rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.25rem 0;
    width: 100%;
    text-align: left;
  }
  .attachments-header:hover { color: var(--text-primary); }

  .attachments-count {
    font-size: 0.65rem;
    font-weight: 600;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 999px;
    padding: 0 0.35rem;
    color: var(--text-muted);
    margin-left: 0.25rem;
  }

  .attachments-title {
    flex: 1;
  }

  .attachments-body {
    padding: 0.35rem 0;
    border-radius: 6px;
    transition: background 0.15s;
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
    transition: border-color 0.15s, color 0.15s;
  }
  .attachment-drop-zone:hover {
    border-color: var(--accent);
    color: var(--accent);
  }

  .hidden-file-input {
    display: none;
  }
</style>
