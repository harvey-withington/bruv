<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { X, Eye, PencilLine, AlertTriangle } from 'lucide-svelte'
  import { documentFormatForPath, documentName } from '@shared/documentFormats'
  import { inlineEdit } from '@shared/inlineEdit'
  import { EditScope } from '@shared/editScope'
  import { createSerialSaver } from '@shared/serialSave'
  import type { WorkspaceFileStamp } from '@shared/types'
  import { t } from '../lib/i18n.svelte'
  import { autoGrow } from '../lib/actions/autoGrow'
  import type { DocumentSource } from '../lib/documentSource'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import EditorDoneButton from './EditorDoneButton.svelte'

  // A document on the phone: rendered preview by default (readable
  // typography, wrapped — never the raw bytes in a browser tab), a plain
  // textarea to edit. Same source contract and divergence guard as the
  // desktop editor: autosave presents the stamp it loaded; a refused save
  // asks overwrite-or-reload, never clobbers. Deliberate asymmetry with
  // desktop — no CodeMirror, no outline — the phone edits a paragraph,
  // it doesn't author a manuscript.
  let { source, onClose }: { source: DocumentSource; onClose: () => void } = $props()

  const name = $derived(documentName(source.path))
  const format = $derived(documentFormatForPath(source.path))

  let status = $state<'loading' | 'ready' | 'error'>('loading')
  let loadError = $state('')
  let text = $state('')
  let stamp = $state<WorkspaceFileStamp | null>(null)
  let mode = $state<'preview' | 'edit'>('preview')
  let saving = $state(false)
  let saveError = $state<string | null>(null)
  let savedFlash = $state(false)
  let diverged = $state(false)
  // A close whose save failed (offline, server error): ask before the
  // unsaved edits are dropped instead of trapping the user in the sheet.
  let confirmingDiscard = $state(false)
  let textareaEl: HTMLTextAreaElement | undefined = $state()
  let lastSaved = ''
  let saveTimer: ReturnType<typeof setTimeout> | null = null

  const editScope = new EditScope()
  editScope.requestClose = () => void requestClose()

  const html = $derived(status === 'ready' ? format.render(text, { name }) : '')

  // The sheet owns one history entry; Back pops it and asks to close.
  const pushSheetEntry = () => history.pushState({ documentSheet: true }, '')

  onMount(() => {
    pushSheetEntry()
    const onPop = () => {
      if (history.state?.documentSheet) return
      // Back while a prompt is up: the prompt stays in charge (Back =
      // Escape cancels the discard prompt; the diverged choice has no
      // safe default) and the sheet keeps its entry.
      if (confirmingDiscard || diverged) {
        confirmingDiscard = false
        pushSheetEntry()
        return
      }
      // A refused close (save failed or diverged) re-pushes the entry —
      // otherwise the NEXT Back would leave the page under the sheet and
      // silently drop the draft.
      void requestClose().then((closed) => { if (!closed) pushSheetEntry() })
    }
    window.addEventListener('popstate', onPop)
    void load()
    return () => {
      window.removeEventListener('popstate', onPop)
      if (history.state?.documentSheet) history.back()
      if (saveTimer) clearTimeout(saveTimer)
    }
  })

  async function load() {
    status = 'loading'
    try {
      const doc = await source.open()
      text = doc.content
      stamp = doc.stamp
      lastSaved = doc.content
      status = 'ready'
    } catch (e) {
      loadError = e instanceof Error ? e.message : String(e)
      status = 'error'
    }
  }

  function scheduleSave() {
    if (saveTimer) clearTimeout(saveTimer)
    saveTimer = setTimeout(() => void save(), 1000)
  }

  // One save in flight at a time (shared/serialSave); saves requested
  // meanwhile coalesce into one follow-up carrying the latest text. Each
  // persist reads the stamp when it RUNS, so a follow-up presents the
  // stamp its predecessor just wrote — overlapping saves used to send
  // the stale one and prompt "changed elsewhere" for the user's own edit
  // (desktop DocumentSession has the same one-at-a-time guard).
  const saver = createSerialSaver<{ content: string; overwrite: boolean }>(async ({ content, overwrite }) => {
    if (diverged && !overwrite) return // queued before the refusal — the prompt decides now
    const res = await source.save(content, overwrite ? '' : (stamp?.hash ?? ''))
    if (res.diverged) {
      diverged = true
      return
    }
    stamp = res.stamp
    lastSaved = content
    diverged = false
    savedFlash = true
    setTimeout(() => savedFlash = false, 1200)
  })

  /** Save the draft. Resolves true when the current draft is on disk. */
  async function save(overwrite = false): Promise<boolean> {
    if (saveTimer) { clearTimeout(saveTimer); saveTimer = null }
    if (status !== 'ready' || diverged && !overwrite) return false
    // Nothing new — unless a save is in flight, whose older content would
    // otherwise land after a revert back to the saved text.
    if (text === lastSaved && !overwrite && !saver.busy) return true
    saving = true
    saveError = null
    try {
      await saver.save({ content: text, overwrite })
    } catch (e) {
      saveError = e instanceof Error ? e.message : String(e)
      return false
    } finally {
      saving = saver.busy
    }
    return !diverged && text === lastSaved
  }

  async function reloadFromDisk() {
    diverged = false
    await load()
  }

  async function startEdit() {
    mode = 'edit'
    await tick()
    textareaEl?.focus()
  }

  function commitEdit() {
    mode = 'preview'
    void save()
  }

  /** Close once the draft is safely on disk. Resolves false when the
   *  close was refused: a diverged save (its prompt is up) or a failed
   *  one (the discard prompt is up). */
  async function requestClose(): Promise<boolean> {
    if (status === 'ready' && (text !== lastSaved || saver.busy)) {
      const ok = await save()
      if (!ok) {
        if (!diverged) confirmingDiscard = true
        return false
      }
    }
    onClose()
    return true
  }

  function discardAndClose() {
    confirmingDiscard = false
    onClose()
  }

  // Capture phase: the sheet overlays CardPage, which listens for
  // Escape/Ctrl+Enter on window bubble — the sheet must consume those
  // before the page underneath reacts (UI-CONVENTIONS §8.1).
  function onWindowKeydownCapture(e: KeyboardEvent) {
    if (diverged || confirmingDiscard) return // the ConfirmDialog owns the keys
    if (e.key === 'Escape') {
      if (editScope.hasActive()) return
      e.preventDefault()
      e.stopPropagation()
      void requestClose()
    } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      e.stopPropagation()
      editScope.commitAll()
      void requestClose()
    }
  }
</script>

<svelte:window onkeydowncapture={onWindowKeydownCapture} />

<div class="backdrop" role="presentation" onclick={() => void requestClose()}></div>

<div class="sheet" role="dialog" aria-label={name}>
  <div class="header">
    <span class="grabber" aria-hidden="true"></span>
    <span class="title" title={source.path}>{name}</span>
    {#if status === 'ready'}
      <span class="state" class:on={savedFlash || saving}>{saving ? t('document.saving') : savedFlash ? t('document.saved') : ''}</span>
      <button type="button" class="icon-btn" class:active={mode === 'edit'} onclick={() => mode === 'edit' ? commitEdit() : void startEdit()} aria-label={mode === 'edit' ? t('document.preview') : t('document.edit')}>
        {#if mode === 'edit'}<Eye size={16} />{:else}<PencilLine size={16} />{/if}
      </button>
    {/if}
    <button type="button" class="icon-btn" onclick={() => void requestClose()} aria-label={t('common.close')}>
      <X size={18} />
    </button>
  </div>

  {#if saveError}
    <div class="banner banner-error" role="alert"><AlertTriangle size={14} /> {t('document.save_failed', { error: saveError })}</div>
  {/if}

  <div class="body">
    {#if status === 'loading'}
      <p class="status">{t('common.loading')}</p>
    {:else if status === 'error'}
      <p class="status error">{t('document.open_failed', { error: loadError })}</p>
    {:else if mode === 'edit'}
      <textarea
        bind:this={textareaEl}
        bind:value={text}
        use:autoGrow
        use:inlineEdit={{ multiline: true, enterInsertsNewline: true, onCommit: commitEdit, onCancel: commitEdit, scope: editScope }}
        oninput={scheduleSave}
        class="editor"
        class:mono={format.id === 'fountain'}
        rows="12"
        spellcheck="true"
      ></textarea>
      <div class="editor-actions">
        <EditorDoneButton onDone={commitEdit} />
      </div>
    {:else}
      {#if format.styles}
        {@html `<style>${format.styles}</style>`}
      {/if}
      <div class="doc-preview prose {format.id}">{@html html}</div>
    {/if}
  </div>
</div>

{#if diverged}
  <ConfirmDialog
    title={t('document.diverged_title')}
    body={t('document.diverged_body', { name })}
    confirmLabel={t('document.overwrite')}
    cancelLabel={t('document.reload')}
    onConfirm={() => void save(true)}
    onCancel={() => void reloadFromDisk()}
  />
{/if}

{#if confirmingDiscard}
  <ConfirmDialog
    title={t('document.discard_title')}
    body={t('document.discard_body', { name, error: saveError ?? '' })}
    confirmLabel={t('document.discard')}
    cancelLabel={t('document.keep_editing')}
    destructive
    onConfirm={discardAndClose}
    onCancel={() => (confirmingDiscard = false)}
  />
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    z-index: 60;
    animation: fade-in 200ms ease forwards;
  }
  .sheet {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    height: 92vh;
    background: var(--bg);
    border-top-left-radius: 16px;
    border-top-right-radius: 16px;
    border-top: 1px solid var(--border);
    box-shadow: 0 -10px 30px rgba(0, 0, 0, 0.35);
    z-index: 61;
    display: flex;
    flex-direction: column;
    animation: slide-up 220ms cubic-bezier(0.16, 1, 0.3, 1) forwards;
    padding-bottom: env(safe-area-inset-bottom);
  }
  .header {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.9rem 0.6rem 0.5rem 0.9rem;
    border-bottom: 1px solid var(--border);
  }
  .grabber {
    position: absolute;
    top: 0.35rem;
    left: 50%;
    width: 36px;
    height: 4px;
    margin-left: -18px;
    border-radius: 2px;
    background: var(--border);
  }
  .title {
    flex: 1;
    min-width: 0;
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .state {
    font-size: 0.72rem;
    color: var(--text-faint);
    opacity: 0;
    transition: opacity 150ms;
  }
  .state.on { opacity: 1; }
  .icon-btn {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.4rem;
    border-radius: 8px;
    display: inline-flex;
  }
  .icon-btn.active { color: var(--accent); }
  .banner {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin: 0.5rem 0.85rem 0;
    padding: 0.5rem 0.7rem;
    border-radius: 8px;
    font-size: 0.82rem;
  }
  .banner-error {
    background: var(--danger-bg);
    border: 1px solid var(--danger-border);
    color: var(--danger-text);
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 0.85rem 0.95rem 2rem;
    -webkit-overflow-scrolling: touch;
  }
  .status { color: var(--text-muted); font-size: 0.9rem; }
  .status.error { color: var(--danger-text); }
  .editor {
    width: 100%;
    box-sizing: border-box;
    min-height: 60vh;
    background: var(--bg-elev-1);
    border: 1px solid var(--accent);
    border-radius: 8px;
    color: var(--text);
    font: inherit;
    font-size: 1rem;
    line-height: 1.55;
    padding: 0.65rem 0.75rem;
    resize: none;
    outline: none;
  }
  .editor.mono { font-family: 'Courier Prime', 'Courier New', Courier, monospace; }
  .editor-actions { display: flex; justify-content: flex-end; margin-top: 0.4rem; }
  /* Readable on a phone: the whole point of opening here rather than in
     the browser. Body-size text, wrapped, no horizontal scroll. */
  .prose {
    font-size: 1rem;
    line-height: 1.6;
    color: var(--text);
    overflow-wrap: anywhere;
  }
  .prose :global(p) { margin: 0 0 0.85rem; }
  .prose :global(h1) { font-size: 1.45rem; margin: 0.6rem 0 0.5rem; }
  .prose :global(h2) { font-size: 1.25rem; margin: 0.6rem 0 0.4rem; }
  .prose :global(h3) { font-size: 1.1rem; margin: 0.5rem 0 0.35rem; }
  .prose :global(a) { color: var(--accent); word-break: break-word; }
  .prose :global(code) {
    background: var(--bg-elev-1);
    padding: 0.1rem 0.3rem;
    border-radius: 3px;
    font-size: 0.9em;
  }
  .prose :global(pre) {
    background: var(--bg-elev-1);
    padding: 0.65rem;
    border-radius: 6px;
    overflow-x: auto;
    white-space: pre-wrap;
  }
  .prose :global(pre code) { background: transparent; padding: 0; }
  .prose :global(.doc-plain) { margin: 0; white-space: pre-wrap; word-break: break-word; font-size: 0.95rem; }
  .prose :global(blockquote) {
    margin: 0.5rem 0;
    padding-left: 0.75rem;
    border-left: 3px solid var(--border);
    color: var(--text-muted);
  }
  .prose :global(ul),
  .prose :global(ol) { padding-left: 1.25rem; margin: 0.5rem 0; }
  .prose :global(img) { max-width: 100%; height: auto; }
  .prose :global(table) { display: block; max-width: 100%; overflow-x: auto; }
  @keyframes fade-in { to { opacity: 1; } }
  @keyframes slide-up { from { transform: translateY(100%); } to { transform: translateY(0); } }
</style>
