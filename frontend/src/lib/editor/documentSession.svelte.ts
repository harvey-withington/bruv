import type { WorkspaceFileStamp } from '@shared/types'
import type { DocumentSource } from './documentSource'

/**
 * Decisions only the user can make, injected so the session stays free of
 * dialog code and testable. Each resolves true for the destructive-to-
 * local-edits choice named by the method.
 */
export interface DocumentSessionHooks {
  /** The file changed on disk while there are unsaved edits. true = reload and drop the edits. */
  confirmReload(): Promise<boolean>
  /** A save was refused because the file changed on disk. true = overwrite the disk version. */
  confirmOverwrite(): Promise<boolean>
  /** The file changed on disk and was reloaded silently (no local edits to lose). Ambient notice. */
  onReloaded?(): void
}

export type DocumentStatus = 'loading' | 'ready' | 'error'

const AUTOSAVE_MS = 1000

/**
 * The editor shell's model: one open document, its on-disk stamp, the
 * draft, autosave, and the external-change guard. Runes-based so the
 * components just read it; all disk talk goes through the DocumentSource.
 *
 * Divergence policy (plan §4.1 — never clobber silently):
 *  - focus-time stat shows a newer file and the draft is clean → reload
 *    quietly and tell the user ambiently;
 *  - draft is dirty → ask reload-or-keep; keeping pauses autosave so the
 *    prompt doesn't re-fire every keystroke, and the next explicit save
 *    (Ctrl+S, Ctrl+Enter, close) asks overwrite-or-keep;
 *  - a save the backend refuses as diverged asks overwrite-or-keep.
 */
export class DocumentSession {
  status = $state<DocumentStatus>('loading')
  loadError = $state('')
  /** The draft — what the editor shows. */
  text = $state('')
  /** What we last knew to be on disk (loaded or saved by us). */
  savedText = $state('')
  stamp = $state<WorkspaceFileStamp | null>(null)
  saving = $state(false)
  saveError = $state('')
  /** Autosave is off until the user resolves an external change explicitly. */
  paused = $state(false)

  readonly dirty = $derived(this.text !== this.savedText)

  private timer: ReturnType<typeof setTimeout> | null = null
  private prompting = false
  private disposed = false

  constructor(
    private readonly source: DocumentSource,
    private readonly hooks: DocumentSessionHooks,
    private readonly autosaveMs = AUTOSAVE_MS,
  ) {}

  async load(): Promise<void> {
    this.status = 'loading'
    this.loadError = ''
    try {
      const file = await this.source.open()
      if (this.disposed) return
      this.text = file.content
      this.savedText = file.content
      this.stamp = file.stamp
      this.status = 'ready'
    } catch (e) {
      this.loadError = e instanceof Error ? e.message : String(e)
      this.status = 'error'
    }
  }

  /** The editor's every change lands here; autosave follows after a quiet second. */
  edit(text: string): void {
    this.text = text
    this.saveError = ''
    this.scheduleAutosave()
  }

  private scheduleAutosave(): void {
    if (this.timer) clearTimeout(this.timer)
    if (this.paused) return
    this.timer = setTimeout(() => { this.timer = null; void this.save(false) }, this.autosaveMs)
  }

  /**
   * Save now if there is anything to save, asking about an external change
   * when needed. Resolves true when the draft is on disk afterwards.
   */
  async flush(): Promise<boolean> {
    if (this.timer) { clearTimeout(this.timer); this.timer = null }
    if (!this.dirty) return true
    if (this.saving) return false
    return this.save(false)
  }

  /**
   * One save attempt. `overwrite` skips the stamp check — the user chose
   * to replace whatever is on disk. Resolves true when the draft as of
   * the call is on disk.
   */
  private async save(overwrite: boolean): Promise<boolean> {
    if (this.status !== 'ready' || this.saving) return false
    const text = this.text
    this.saving = true
    try {
      const res = await this.source.save(text, overwrite ? '' : (this.stamp?.hash ?? ''))
      if (this.disposed) return false
      if (res.diverged) {
        this.saving = false
        return this.resolveDivergedSave()
      }
      this.stamp = res.stamp
      this.savedText = text
      this.saveError = ''
      this.paused = false
      return true
    } catch (e) {
      this.saveError = e instanceof Error ? e.message : String(e)
      return false
    } finally {
      this.saving = false
      // Typing during the save leaves the draft dirty again — keep going.
      if (!this.disposed && this.dirty && !this.paused && !this.prompting) this.scheduleAutosave()
    }
  }

  private async resolveDivergedSave(): Promise<boolean> {
    if (this.prompting) return false
    this.prompting = true
    this.paused = true
    try {
      const overwrite = await this.hooks.confirmOverwrite()
      if (this.disposed) return false
      if (!overwrite) return false
      return await this.save(true)
    } finally {
      this.prompting = false
    }
  }

  /**
   * Window-focus check: did another program change the file? Stat errors
   * are swallowed on purpose — a transient failure here must not interrupt
   * typing; the next save reports a real problem inline.
   */
  async checkExternal(): Promise<void> {
    if (this.status !== 'ready' || this.saving || this.prompting || !this.stamp) return
    let current: WorkspaceFileStamp
    try {
      current = await this.source.stat()
    } catch {
      return
    }
    if (this.disposed || this.prompting || !this.stamp || current.hash === this.stamp.hash) return
    if (!this.dirty) {
      await this.reload()
      this.hooks.onReloaded?.()
      return
    }
    this.prompting = true
    this.paused = true
    try {
      if (await this.hooks.confirmReload()) await this.reload()
    } finally {
      this.prompting = false
    }
  }

  /** Replace the draft with what is on disk now. */
  async reload(): Promise<void> {
    if (this.timer) { clearTimeout(this.timer); this.timer = null }
    const file = await this.source.open()
    if (this.disposed) return
    this.text = file.content
    this.savedText = file.content
    this.stamp = file.stamp
    this.saveError = ''
    this.paused = false
  }

  /** Stop timers; in-flight results are dropped. */
  dispose(): void {
    this.disposed = true
    if (this.timer) { clearTimeout(this.timer); this.timer = null }
  }
}
