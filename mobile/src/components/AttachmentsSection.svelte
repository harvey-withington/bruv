<script lang="ts">
  import { Paperclip, Trash2, FileText, FileImage, FileVideo, File as FileIcon, Download, Eye, X } from 'lucide-svelte'
  import { repoRPC } from '../lib/auth'
  import { t } from '../lib/i18n.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import type { Attachment, Card } from '@shared/types'

  let {
    cardId,
    attachments = [],
    onCardUpdated,
  }: {
    cardId: string
    attachments: Attachment[]
    onCardUpdated: (card: Card) => void
  } = $props()

  let uploading = $state(false)
  let errorMsg = $state<string | null>(null)
  let fileInputEl = $state<HTMLInputElement | null>(null)
  let confirmingDelete = $state<Attachment | null>(null)

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

  async function openFilePicker() {
    fileInputEl?.click()
  }

  async function handleFileInput(e: Event) {
    const input = e.target as HTMLInputElement
    if (input.files?.length) {
      await handleFiles(input.files)
      input.value = ''
    }
  }

  async function handleFiles(files: FileList | File[]) {
    uploading = true
    errorMsg = null
    for (const file of files) {
      try {
        const data = await fileToBase64(file)
        const updated = await repoRPC<Card>('AddCardAttachment', [cardId, file.name, data])
        if (updated) onCardUpdated(updated)
      } catch (err) {
        errorMsg = err instanceof Error ? err.message : t('attachment.upload_failed')
      }
    }
    uploading = false
  }

  function fileToBase64(file: globalThis.File): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => {
        const result = reader.result as string
        const base64 = result.includes(',') ? result.split(',')[1] : result
        resolve(base64)
      }
      reader.onerror = reject
      reader.readAsDataURL(file)
    })
  }

  async function previewAttachment(att: Attachment) {
    errorMsg = null
    try {
      const url = await repoRPC<string>('SignAttachmentURL', [cardId, att.id])
      if (url) {
        window.open(url, '_blank', 'noopener,noreferrer')
      }
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('attachment.download_failed')
    }
  }

  async function removeAttachment(att: Attachment) {
    confirmingDelete = null
    errorMsg = null
    try {
      const updated = await repoRPC<Card>('RemoveCardAttachment', [cardId, att.id])
      if (updated) onCardUpdated(updated)
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('attachment.remove_failed')
    }
  }
</script>

<section class="attachments">
  {#if errorMsg}
    <p class="error">{errorMsg}</p>
  {/if}

  {#if attachments.length > 0}
    <ul class="list">
      {#each attachments as att (att.id)}
        {@const Icon = iconForMime(att.mime)}
        <li class="attachment-item">
          <button type="button" class="attachment-info" onclick={() => previewAttachment(att)}>
            <Icon size={16} class="file-icon" />
            <span class="name">{att.name}</span>
            <span class="size">{formatSize(att.size)}</span>
          </button>
          <div class="actions-row">
            <button type="button" class="ghost-btn" onclick={() => previewAttachment(att)} aria-label="Preview">
              <Eye size={14} />
            </button>
            <button type="button" class="ghost-btn danger" onclick={() => (confirmingDelete = att)} aria-label="Delete">
              <Trash2 size={14} />
            </button>
          </div>
        </li>
      {/each}
    </ul>
  {:else}
    <p class="empty">{t('attachment.empty')}</p>
  {/if}

  <div class="composer">
    <button type="button" class="attach-btn" onclick={openFilePicker} disabled={uploading}>
      <Paperclip size={14} />
      <span>{uploading ? t('common.working') : t('attachment.title')}</span>
    </button>
    <input
      bind:this={fileInputEl}
      type="file"
      multiple
      class="hidden-file-input"
      onchange={handleFileInput}
    />
  </div>
</section>

{#if confirmingDelete}
  <ConfirmDialog
    title={t('comments.delete_title')}
    body={t('attachment.remove_confirm').replace('{name}', confirmingDelete.name)}
    confirmLabel={t('attachment.remove')}
    destructive
    onConfirm={() => removeAttachment(confirmingDelete!)}
    onCancel={() => (confirmingDelete = null)}
  />
{/if}

<style>
  .attachments {
    margin-bottom: 1.5rem;
  }

  .list {
    list-style: none;
    padding: 0;
    margin: 0 0 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .attachment-item {
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0.65rem 0.85rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }
  .attachment-info {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex: 1;
    min-width: 0;
    cursor: pointer;
    /* Reset button defaults — this used to be a <div>; the change to
       <button> is purely for the click affordance + a11y. */
    background: transparent;
    border: none;
    padding: 0;
    color: inherit;
    font: inherit;
    text-align: left;
  }
  :global(.file-icon) {
    color: var(--text-muted);
    flex-shrink: 0;
  }
  .name {
    font-size: 0.9rem;
    font-weight: 500;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }
  .name:hover {
    color: var(--accent);
    text-decoration: underline;
  }
  .size {
    font-size: 0.7rem;
    color: var(--text-faint);
    flex-shrink: 0;
    margin-left: 0.25rem;
  }

  .actions-row {
    display: flex;
    align-items: center;
    gap: 0.15rem;
  }

  .ghost-btn {
    background: transparent;
    border: 1px solid transparent;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0.3rem 0.4rem;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .ghost-btn:hover,
  .ghost-btn:focus-visible {
    color: var(--text);
    background: var(--bg);
    outline: none;
  }
  .ghost-btn.danger:hover,
  .ghost-btn.danger:focus-visible {
    color: #ef4444;
  }

  .composer {
    display: flex;
    align-items: center;
  }

  .attach-btn {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.4rem;
    padding: 0.65rem;
    background: transparent;
    border: 1px dashed var(--border);
    border-radius: 8px;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
    transition: color 0.15s, border-color 0.15s;
  }
  .attach-btn:hover,
  .attach-btn:focus-visible {
    color: var(--text);
    border-color: var(--text-muted);
    outline: none;
  }
  .attach-btn:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .hidden-file-input {
    display: none;
  }

  .empty {
    color: var(--text-faint);
    font-size: 0.85rem;
    margin: 0 0 0.75rem;
    font-style: italic;
  }
  .error {
    margin: 0 0 0.75rem;
    padding: 0.5rem 0.75rem;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 6px;
    color: #fca5a5;
    font-size: 0.85rem;
  }
</style>
