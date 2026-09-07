<script lang="ts">
  import { OpenWorkspacePath, RevealWorkspacePath } from '@shared/api'
  import { t } from '../../lib/i18n.svelte'
  import { showToast } from '../../lib/toast.svelte'
  import { focusTrap, portal } from '../../lib/actions'
  import { workspaceDocumentSource } from '../../lib/editor/documentSource'
  import DocumentEditor from './editor/DocumentEditor.svelte'

  // Tier 2: text files open in BRUV's document editor (CodeMirror +
  // format modules — see components/workspace/editor/). Everything else
  // is Tier 1: the backend refuses the read and the open/reveal fallbacks
  // take over (root is the workspace's on-disk folder for those shell
  // actions). This component is only the overlay; the editor is source-
  // agnostic and gets a workspace-backed DocumentSource.
  let { brandSlug, streamSlug, projectSlug, root, path, onClose }: {
    brandSlug: string
    streamSlug: string
    projectSlug: string
    root: string
    path: string
    onClose: () => void
  } = $props()

  const source = $derived(workspaceDocumentSource(brandSlug, streamSlug, projectSlug, path))
  let editor = $state<DocumentEditor | null>(null)

  async function openExternal() {
    try {
      await OpenWorkspacePath(root, path)
    } catch (e) {
      showToast(t('workspace.open_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    }
  }

  async function reveal() {
    try {
      await RevealWorkspacePath(root, path)
    } catch (e) {
      showToast(t('workspace.open_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    }
  }
</script>

<!-- Backdrop and keys route through the editor so an unsaved draft is
     flushed (or explicitly discarded) before the dialog goes away.
     Portaled to <body>: printing turns the overlay static, and inside the
     side panel it would flow in the panel's column, right of the board. -->
<div class="viewer-overlay" role="presentation" use:portal onclick={(e) => { if (e.target === e.currentTarget) void editor?.requestClose() }}>
  <div class="viewer" role="dialog" aria-label={path} tabindex="-1" use:focusTrap onkeydown={(e) => editor?.onKeydown(e)}>
    <DocumentEditor bind:this={editor} {source} {onClose} onOpenExternal={openExternal} onReveal={reveal} />
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
    z-index: 90;
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
