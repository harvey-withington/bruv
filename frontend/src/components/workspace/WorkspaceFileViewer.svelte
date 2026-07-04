<script lang="ts">
  import { X } from 'lucide-svelte'
  import { ReadWorkspaceFile, WriteWorkspaceFile } from '@shared/api'
  import { renderMarkdown } from '@shared/markdown'
  import { t } from '../../lib/i18n.svelte'
  import { showToast } from '../../lib/toast.svelte'
  import { focusTrap } from '../../lib/actions'

  // Tier 2: markdown / plain-text files open in BRUV's built-in editor —
  // the same markdown-textarea model card bodies use. Everything else is
  // refused by the backend with an "open externally" message.
  let { brandSlug, streamSlug, projectSlug, path, onClose }: {
    brandSlug: string
    streamSlug: string
    projectSlug: string
    path: string
    onClose: () => void
  } = $props()

  const isMarkdown = $derived(path.toLowerCase().endsWith('.md'))

  let content = $state<string | null>(null)
  let draft = $state('')
  let editing = $state(false)
  let saving = $state(false)
  let loadError = $state('')

  $effect(() => {
    content = null
    loadError = ''
    editing = false
    ReadWorkspaceFile(brandSlug, streamSlug, projectSlug, path)
      .then(c => { content = c })
      .catch((e: unknown) => { loadError = e instanceof Error ? e.message : String(e) })
  })

  function startEdit() {
    if (content === null) return
    draft = content
    editing = true
  }

  async function save() {
    if (!editing) return
    saving = true
    try {
      await WriteWorkspaceFile(brandSlug, streamSlug, projectSlug, path, draft)
      content = draft
      editing = false
      showToast(t('workspace.file_saved'), 'success')
    } catch (e) {
      showToast(t('workspace.file_save_failed', { error: e instanceof Error ? e.message : String(e) }), 'error')
    } finally {
      saving = false
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (editing) {
        editing = false
      } else {
        onClose()
      }
    } else if (e.key === 'Enter' && e.ctrlKey && editing) {
      e.preventDefault()
      save()
    }
  }
</script>

<div class="viewer-overlay" role="presentation" onclick={(e) => { if (e.target === e.currentTarget) onClose() }}>
  <div class="viewer" role="dialog" aria-label={path} tabindex="-1" use:focusTrap onkeydown={onKeydown}>
    <header>
      <span class="path" title={path}>{path}</span>
      <div class="header-actions">
        {#if editing}
          <button class="btn subtle" onclick={() => editing = false}>{t('common.cancel')}</button>
          <button class="btn primary" disabled={saving} onclick={save}>{saving ? t('common.saving') : t('common.save')}</button>
        {:else if content !== null}
          <button class="btn subtle" onclick={startEdit}>{t('common.edit')}</button>
        {/if}
        <button class="icon-btn" onclick={onClose} title={t('common.close')} aria-label={t('common.close')}><X size={16} /></button>
      </div>
    </header>

    <div class="body">
      {#if loadError}
        <p class="error">{loadError}</p>
      {:else if content === null}
        <p class="loading">{t('common.loading')}</p>
      {:else if editing}
        <!-- svelte-ignore a11y_autofocus -->
        <textarea bind:value={draft} spellcheck="false" autofocus></textarea>
      {:else if isMarkdown}
        <div class="markdown">{@html renderMarkdown(content)}</div>
      {:else}
        <pre>{content}</pre>
      {/if}
    </div>
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
    width: min(860px, 92vw);
    height: min(80vh, 900px);
    display: flex;
    flex-direction: column;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 10px;
    box-shadow: var(--shadow-lg, 0 12px 40px rgba(0, 0, 0, 0.4));
    overflow: hidden;
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.55rem 0.85rem;
    border-bottom: 1px solid var(--border-muted);
  }
  .path {
    font-size: 0.8rem;
    color: var(--text-muted);
    font-family: var(--font-mono, monospace);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .header-actions {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    flex-shrink: 0;
  }
  .body {
    flex: 1;
    overflow: auto;
    padding: 1rem 1.25rem;
    display: flex;
    flex-direction: column;
  }
  textarea {
    flex: 1;
    width: 100%;
    resize: none;
    background: var(--bg-base);
    color: var(--text-primary);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.75rem;
    font-family: var(--font-mono, monospace);
    font-size: 0.85rem;
    line-height: 1.5;
  }
  textarea:focus {
    outline: none;
    border-color: var(--accent);
  }
  pre {
    margin: 0;
    font-size: 0.82rem;
    white-space: pre-wrap;
    word-break: break-word;
    color: var(--text-secondary);
  }
  .markdown {
    font-size: 0.88rem;
    line-height: 1.55;
  }
  .error {
    color: var(--danger, #ef4444);
    font-size: 0.85rem;
  }
  .loading {
    color: var(--text-faint);
    font-size: 0.85rem;
  }
  .btn {
    border: 1px solid var(--border);
    background: var(--bg-base);
    color: var(--text-secondary);
    border-radius: 6px;
    padding: 0.3rem 0.7rem;
    font-size: 0.78rem;
    cursor: pointer;
  }
  .btn:hover { color: var(--text-primary); background: var(--bg-subtle-hover); }
  .btn.primary {
    background: var(--accent);
    border-color: var(--accent);
    color: white;
  }
  .btn.primary:disabled { opacity: 0.6; cursor: default; }
  .icon-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.3rem;
    border-radius: 6px;
    display: flex;
  }
  .icon-btn:hover { color: var(--text-primary); background: var(--bg-subtle-hover); }
</style>
