// EditScope — the containment half of BRUV's keyboard entry contract
// (see UI-CONVENTIONS "Keyboard entry"). One scope per closable
// container (card dialog, modal dialog, mobile sheet). Active inline
// edits register themselves; the container's window-level handlers ask
// the scope before acting:
//
//   Escape      → close the container ONLY when nothing is being edited
//                 (an active edit's own handler cancels + consumes Esc)
//   Ctrl+Enter  → commit every active edit, then close the container
//
// The scope also feeds "don't clobber my edit" guards (e.g. skipping a
// silent card reload while the user is typing) — previously a
// hand-plumbed hasActiveEdit()/commitActiveEdit() pair threaded through
// bind:this, which silently missed checklist items, tags, and comments.

export interface ActiveEdit {
  /** Commit the in-flight value (used by the Ctrl+Enter chord). */
  commit(): void
  /** Cancel without committing. */
  cancel(): void
}

export class EditScope {
  private active = new Set<ActiveEdit>()

  /**
   * Set by the container: how to close it (e.g. the card dialog's
   * onClose). Invoked by the Ctrl+Enter chord from any registered
   * field, and by the container's own Escape handling.
   */
  requestClose: (() => void) | null = null

  /** Register an in-flight edit. Returns the unregister function. */
  register(edit: ActiveEdit): () => void {
    this.active.add(edit)
    return () => this.active.delete(edit)
  }

  hasActive(): boolean {
    return this.active.size > 0
  }

  /** Commit every active edit (Ctrl+Enter chord, pre-close saves). */
  commitAll(): void {
    for (const edit of [...this.active]) edit.commit()
  }

  /**
   * Cancel every active edit without committing. Used by the mobile
   * Back = Escape layering (UI-CONVENTIONS §8): a Back activation
   * cancels in-flight edits instead of navigating away mid-edit.
   */
  cancelAll(): void {
    for (const edit of [...this.active]) edit.cancel()
  }

  /**
   * Window-keydown helper implementing the container side of the
   * contract. Containers with extra conditions (child dialogs open,
   * etc.) can guard before delegating here.
   */
  handleWindowKeydown(e: KeyboardEvent): void {
    if (e.key === 'Escape') {
      // An active edit's own Escape handler consumes the event before
      // it reaches the window; this check is the backstop for edits
      // whose events don't bubble through the container.
      if (this.hasActive()) return
      this.requestClose?.()
    } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      this.commitAll()
      this.requestClose?.()
    }
  }
}

/**
 * Svelte context key — containers setContext(EDIT_SCOPE_KEY, scope),
 * entry fields getContext<EditScope>(EDIT_SCOPE_KEY) and pass it to
 * the inlineEdit action. Fields outside any container simply get
 * undefined and skip registration.
 */
export const EDIT_SCOPE_KEY = Symbol('bruv-edit-scope')
