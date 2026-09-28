<script lang="ts">
  import type { Attachment } from '@shared/types'
  import { focusTrap, portal } from '../lib/actions'
  import { attachmentDocumentSource } from '../lib/editor/documentSource'
  import DocumentEditor from './workspace/editor/DocumentEditor.svelte'

  // A text attachment in the document editor. Same overlay shape as
  // WorkspaceFileViewer, different DocumentSource: the bytes are a file
  // the card owns in the vault, so there is no folder to reveal and
  // nothing to open externally — those toolbar buttons simply don't show.
  let { cardId, attachment, onClose }: {
    cardId: string
    attachment: Attachment
    onClose: () => void
  } = $props()

  const source = $derived(attachmentDocumentSource(cardId, attachment.id, attachment.name))
  let editor = $state<DocumentEditor | null>(null)
</script>

<!-- Backdrop and keys route through the editor so an unsaved draft is
     flushed (or explicitly discarded) before the dialog goes away.
     Portaled to <body> so printing and the card dialog's own stacking
     don't get in the way. -->
<div class="viewer-overlay" role="presentation" use:portal onclick={(e) => { if (e.target === e.currentTarget) void editor?.requestClose() }}>
  <div class="viewer" role="dialog" aria-label={attachment.name} tabindex="-1" use:focusTrap onkeydown={(e) => editor?.onKeydown(e)}>
    <DocumentEditor bind:this={editor} {source} {onClose} />
  </div>
</div>

<style>
  .viewer-overlay {
    position: fixed;
    inset: 0;
    background: color-mix(in srgb, var(--bg-base) 60%, transparent);
    backdrop-filter: blur(2px);
    display: flex;
    align-items: center;
    justify-content: center;
    /* Above CardDetail (100): the editor opens FROM a card and is portaled
       to <body>, so it competes with the card at root level. Below
       ConfirmDialog (99990), whose prompts it raises. */
    z-index: 110;
  }
  .viewer {
    width: min(1280px, 94vw);
    height: min(88vh, 1000px);
    display: flex;
    flex-direction: column;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 10px;
    box-shadow: 0 12px 40px var(--shadow-lg);
    overflow: hidden;
  }
</style>
