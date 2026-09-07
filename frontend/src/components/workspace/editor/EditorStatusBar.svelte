<script lang="ts">
  import { AlertTriangle, Save } from 'lucide-svelte'
  import { t } from '../../../lib/i18n.svelte'
  import type { CursorPosition } from '../../../lib/editor/codemirror'

  // Ambient status line (UI-CONVENTIONS §9: save state stays inline, never
  // a toast per autosave). Left: what the document is; right: where the
  // draft stands relative to disk.
  let { formatLabel, words, cursor, dirty, saving, saveError, paused, onSaveNow }: {
    formatLabel: string
    words: number
    cursor: CursorPosition
    dirty: boolean
    saving: boolean
    saveError: string
    /** Autosave is off until the user resolves an external change. */
    paused: boolean
    onSaveNow: () => void
  } = $props()
</script>

<footer class="status-bar">
  <span class="cell format">{formatLabel}</span>
  <span class="cell">{t('document.words', { n: words })}</span>
  <span class="cell">{t('document.position', { line: cursor.line, col: cursor.col })}</span>
  <span class="spacer"></span>
  {#if saveError}
    <span class="cell error" title={saveError}><AlertTriangle size={12} /> {t('document.save_failed', { error: saveError })}</span>
    <button class="btn subtle small" onclick={onSaveNow}><Save size={12} /> {t('document.save_now')}</button>
  {:else if paused && dirty}
    <span class="cell warning"><AlertTriangle size={12} /> {t('document.autosave_paused')}</span>
    <button class="btn subtle small" onclick={onSaveNow}><Save size={12} /> {t('document.save_now')}</button>
  {:else if saving}
    <span class="cell muted">{t('common.saving')}</span>
  {:else if dirty}
    <span class="cell muted">{t('document.unsaved')}</span>
  {:else}
    <span class="cell muted">{t('common.saved')}</span>
  {/if}
</footer>

<style>
  .status-bar {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 0.3rem 0.85rem;
    border-top: 1px solid var(--border-muted);
    background: var(--bg-elevated);
    font-size: 0.72rem;
    color: var(--text-muted);
    min-height: 1.9rem;
  }
  .cell {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    white-space: nowrap;
  }
  .format { color: var(--text-secondary); font-weight: 500; }
  .spacer { flex: 1; }
  .error { color: var(--danger); max-width: 40ch; overflow: hidden; text-overflow: ellipsis; }
  .warning { color: var(--warning); }
  .btn.small { padding: 0.15rem 0.5rem; font-size: 0.72rem; }
</style>
