<script lang="ts">
  import { onMount } from 'svelte'
  import { Image as ImageIcon, Upload } from 'lucide-svelte'
  import { repoRPC, readEnrolment } from '../../lib/auth'
  import { t } from '../../lib/i18n.svelte'
  import type { Block, Card } from '@shared/types'
  import { asString, withValue } from './narrow'

  let {
    block,
    cardId,
    onChange,
  }: {
    block: Block
    cardId: string
    onChange: (next: Block) => void
  } = $props()

  // The image block's value is an attachment ID (string) — the
  // resolved URL is fetched via SignAttachmentURL on mount.
  const attachmentID = $derived(asString(block.value))

  let resolvedURL = $state<string | null>(null)
  let uploading = $state(false)
  let uploadError = $state<string | null>(null)
  let fileInput: HTMLInputElement | null = $state(null)

  async function resolve() {
    if (!attachmentID) {
      resolvedURL = null
      return
    }
    try {
      const path = await repoRPC<string>('SignAttachmentURL', [cardId, attachmentID])
      // The signed URL from the runtime is server-relative; prepend
      // the enrolled server origin so <img src> can load it.
      const enrol = readEnrolment()
      resolvedURL = enrol ? `${enrol.serverURL}${path}` : path
    } catch {
      resolvedURL = null
    }
  }

  onMount(resolve)
  $effect(() => {
    void attachmentID
    void resolve()
  })

  async function handleFile(e: Event) {
    const input = e.currentTarget as HTMLInputElement
    const file = input.files?.[0]
    if (!file) return
    uploading = true
    uploadError = null
    try {
      // Upload via the existing AddCardAttachment RPC. The desktop
      // pipeline encodes file bytes as base64 in JSON; mobile mirrors
      // that — the bytes don't go on the wire as multipart because
      // the RPC dispatcher only speaks JSON-RPC. Acceptable on phone
      // too — image sizes are small and the cost is minor.
      const dataB64 = await fileToBase64(file)
      const updated = await repoRPC<Card>('AddCardAttachment', [cardId, file.name, dataB64])
      // The card's attachments list now has a new entry; the new
      // attachment is the last one. Find it and write the ID into
      // this block.
      const attachments = updated?.file_attachments ?? []
      const newest = attachments[attachments.length - 1]
      if (newest?.id) {
        onChange(withValue(block, newest.id))
      }
    } catch (err) {
      uploadError = err instanceof Error ? err.message : t('block.image.err_upload')
    } finally {
      uploading = false
      input.value = ''
    }
  }

  function fileToBase64(file: File): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => {
        const result = reader.result as string
        // dataURL → strip "data:<mime>;base64," prefix.
        const idx = result.indexOf(',')
        resolve(idx >= 0 ? result.slice(idx + 1) : result)
      }
      reader.onerror = () => reject(reader.error)
      reader.readAsDataURL(file)
    })
  }
</script>

<div class="image-block">
  {#if resolvedURL}
    <img src={resolvedURL} alt={block.label} class="img" />
  {:else if attachmentID}
    <div class="placeholder">
      <ImageIcon size={20} />
    </div>
  {/if}

  <input
    type="file"
    accept="image/*"
    capture="environment"
    bind:this={fileInput}
    onchange={handleFile}
    class="hidden"
  />

  <button type="button" class="btn" disabled={uploading} onclick={() => fileInput?.click()}>
    <Upload size={14} />
    {#if uploading}
      {t('block.image.uploading')}
    {:else if attachmentID}
      {t('block.image.replace')}
    {:else}
      {t('block.image.upload')}
    {/if}
  </button>

  {#if uploadError}
    <p class="error" role="alert">{uploadError}</p>
  {/if}
</div>

<style>
  .image-block {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    align-items: flex-start;
  }
  .img {
    max-width: 100%;
    max-height: 60vh;
    border-radius: 8px;
    border: 1px solid var(--border);
    display: block;
  }
  .placeholder {
    width: 100%;
    height: 8rem;
    background: var(--bg-elev-1);
    border: 1px dashed var(--border);
    border-radius: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-faint);
  }
  .hidden {
    display: none;
  }
  .btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    color: var(--text);
    font: inherit;
    font-size: 0.85rem;
    padding: 0.45rem 0.75rem;
    border-radius: 6px;
    cursor: pointer;
  }
  .btn:hover:not(:disabled),
  .btn:focus-visible {
    border-color: var(--accent);
    color: var(--accent);
    outline: none;
  }
  .btn:disabled {
    opacity: 0.6;
    cursor: default;
  }
  .error {
    color: #fca5a5;
    font-size: 0.85rem;
    margin: 0;
  }
</style>
