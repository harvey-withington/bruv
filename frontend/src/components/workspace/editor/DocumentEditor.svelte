<script lang="ts">
  import { onMount } from 'svelte'
  import { ExternalLink, FolderSearch } from 'lucide-svelte'
  import type { DocumentLayout } from '@shared/types'
  import { countWords, documentFormatForPath, documentName, type OutlineNode } from '@shared/documentFormats'
  import { t } from '../../../lib/i18n.svelte'
  import { showToast } from '../../../lib/toast.svelte'
  import { showConfirm } from '../../../lib/confirm.svelte'
  import type { DocumentSource } from '../../../lib/editor/documentSource'
  import { DocumentSession } from '../../../lib/editor/documentSession.svelte'
  import type { CodeMirrorApi, CursorPosition } from '../../../lib/editor/codemirror'
  import { documentLayout, loadDocumentPrefs, outlineShown, setDocumentLayout, setOutlineShown } from '../../../lib/editor/documentPrefs.svelte'
  import EditorToolbar from './EditorToolbar.svelte'
  import EditorPane from './EditorPane.svelte'
  import PreviewPane from './PreviewPane.svelte'
  import DocumentOutline from './DocumentOutline.svelte'
  import EditorStatusBar from './EditorStatusBar.svelte'

  // The document editor shell: one DocumentSource, one format module,
  // edit / split / preview layouts, outline, autosave with the external-
  // change guard (DocumentSession). Source-agnostic by design — a card
  // document is a different DocumentSource, not a different editor.
  let { source, onClose, onOpenExternal, onReveal }: {
    source: DocumentSource
    onClose: () => void
    /** Tier 1 fallbacks — shown when provided (workspace files have them). */
    onOpenExternal?: () => void
    onReveal?: () => void
  } = $props()

  const name = $derived(documentName(source.path))
  const format = $derived(documentFormatForPath(source.path))
  const formatLabel = $derived(t(`document.format_${format.id}`))

  // One session per source: swapping the source (another file) loads a
  // fresh one and disposes the old — the editor follows its prop.
  const session = $derived(new DocumentSession(source, {
    confirmReload: () => showConfirm(t('document.diverged_reload', { name })),
    confirmOverwrite: () => showConfirm(t('document.diverged_overwrite', { name })),
    onReloaded: () => showToast(t('document.reloaded', { name }), 'info'),
  }))
  $effect(() => {
    const s = session
    void s.load()
    return () => s.dispose()
  })

  let layout = $state<DocumentLayout>('split')
  let outline = $state(true)
  let cursor = $state<CursorPosition>({ line: 1, col: 1 })
  let editorApi: CodeMirrorApi | null = null
  let closing = false

  onMount(() => {
    loadDocumentPrefs()
      .then(() => { layout = documentLayout(format.id); outline = outlineShown() })
      .catch(() => { /* defaults stand; the layout still works, it just won't be remembered */ })
  })

  // Another program may have the file open (Obsidian, a terminal editor):
  // check the on-disk stamp whenever the app regains focus.
  $effect(() => {
    const check = () => { void session.checkExternal() }
    window.addEventListener('focus', check)
    return () => window.removeEventListener('focus', check)
  })

  // Outline + word count lag typing by a beat — both walk the whole text.
  let analysis = $state<{ outline: OutlineNode[]; words: number }>({ outline: [], words: 0 })
  $effect(() => {
    const text = session.text
    const h = setTimeout(() => {
      analysis = { outline: format.outline?.(text) ?? [], words: (format.wordCount ?? countWords)(text) }
    }, 250)
    return () => clearTimeout(h)
  })

  async function changeLayout(l: DocumentLayout) {
    layout = l
    try {
      await setDocumentLayout(format.id, l)
    } catch {
      showToast(t('document.prefs_save_failed'), 'error')
    }
  }

  async function toggleOutline() {
    outline = !outline
    try {
      await setOutlineShown(outline)
    } catch {
      showToast(t('document.prefs_save_failed'), 'error')
    }
  }

  /**
   * Print the rendered preview through the webview's paged media. The
   * preview must be on screen to print, so a pure edit layout flips to
   * split for the duration; style.css `body.printing-document` hides the
   * rest of the app and un-fixes the overlay so pages can flow.
   */
  async function print() {
    const previous = layout
    if (layout === 'edit') layout = 'split'
    await new Promise<void>(r => requestAnimationFrame(() => r()))
    document.body.classList.add('printing-document')
    const done = () => {
      document.body.classList.remove('printing-document')
      layout = previous
      window.removeEventListener('afterprint', done)
    }
    window.addEventListener('afterprint', done)
    window.print()
    // afterprint is not guaranteed everywhere; never leave the class behind.
    setTimeout(done, 60_000)
  }

  /**
   * Commit-and-close: flush the draft (which may ask about an external
   * change), and only then hand back. A draft that could not be saved is
   * never dropped silently.
   */
  export async function requestClose(): Promise<void> {
    if (closing) return
    closing = true
    try {
      if (session.status === 'ready') {
        const clean = await session.flush()
        if (!clean && session.dirty && !(await showConfirm(t('document.discard_confirm', { name })))) return
      }
      session.dispose()
      onClose()
    } finally {
      closing = false
    }
  }

  /**
   * Called by the hosting dialog for every keydown inside it. Every key
   * stays inside the editor — the board's single-key shortcuts and any
   * card dialog underneath must not see them (UI-CONVENTIONS §8.1). A key
   * CodeMirror already consumed (search-panel Escape, Ctrl+S, Ctrl+Enter)
   * is left alone.
   */
  export function onKeydown(e: KeyboardEvent): void {
    e.stopPropagation()
    if (e.defaultPrevented) return
    if (e.key === 'Escape' || (e.key === 'Enter' && (e.ctrlKey || e.metaKey))) {
      e.preventDefault()
      void requestClose()
    }
  }

  const showEditor = $derived(layout !== 'preview')
  const showPreview = $derived(layout !== 'edit')
  const showOutline = $derived(outline && format.outline !== undefined)
</script>

<div class="doc-editor">
  <EditorToolbar
    path={source.path}
    {layout}
    onLayout={changeLayout}
    {outline}
    canOutline={format.outline !== undefined}
    onToggleOutline={toggleOutline}
    saving={session.saving}
    onPrint={format.printable ? print : undefined}
    {onOpenExternal}
    {onReveal}
    onClose={() => { void requestClose() }}
  />

  {#if session.status === 'error'}
    <div class="state">
      <p class="error">{session.loadError}</p>
      <div class="error-actions">
        {#if onOpenExternal}<button class="btn subtle" onclick={onOpenExternal}><ExternalLink size={13} /> {t('workspace.open_external')}</button>{/if}
        {#if onReveal}<button class="btn subtle" onclick={onReveal}><FolderSearch size={13} /> {t('workspace.reveal')}</button>{/if}
      </div>
    </div>
  {:else if session.status === 'loading'}
    <div class="state"><p class="loading">{t('common.loading')}</p></div>
  {:else}
    <div class="body" class:with-outline={showOutline} class:split={showEditor && showPreview}>
      {#if showOutline}
        <DocumentOutline nodes={analysis.outline} cursorLine={cursor.line} onSelect={(line) => editorApi?.goToLine(line)} />
      {/if}
      <!-- The editor stays mounted in preview mode (hidden, not removed) so
           undo history and the cursor survive a layout flip. -->
      <div class="pane" hidden={!showEditor}>
        <EditorPane
          text={session.text}
          format={format.id}
          onChange={(text) => session.edit(text)}
          onCursor={(pos) => cursor = pos}
          onSave={() => { void session.flush() }}
          onCommit={() => { void requestClose() }}
          onReady={(api) => editorApi = api}
        />
      </div>
      {#if showPreview}
        <!-- preview-host: the one pane that stays in the DOM when printing (style.css). -->
        <div class="pane preview-host">
          <PreviewPane text={session.text} {format} {name} />
        </div>
      {/if}
    </div>
    <EditorStatusBar
      {formatLabel}
      words={analysis.words}
      {cursor}
      dirty={session.dirty}
      saving={session.saving}
      saveError={session.saveError}
      paused={session.paused}
      onSaveNow={() => { void session.flush() }}
    />
  {/if}
</div>

<style>
  .doc-editor {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }
  .body {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: minmax(0, 1fr);
  }
  .body.split { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
  .body.with-outline { grid-template-columns: 13rem minmax(0, 1fr); }
  .body.with-outline.split { grid-template-columns: 13rem minmax(0, 1fr) minmax(0, 1fr); }
  .pane {
    min-height: 0;
    min-width: 0;
    height: 100%;
  }
  .pane + .pane { border-left: 1px solid var(--border-muted); }
  .state {
    flex: 1;
    padding: 1rem 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }
  .error {
    margin: 0;
    color: var(--danger);
    font-size: 0.85rem;
  }
  .error-actions {
    display: flex;
    gap: 0.5rem;
  }
  .loading {
    margin: 0;
    color: var(--text-faint);
    font-size: 0.85rem;
  }
</style>
