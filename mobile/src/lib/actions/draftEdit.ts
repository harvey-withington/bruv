// draftEdit — the always-mounted complement to @shared/inlineEdit.
//
// Mobile block fields (text / url / number / survey answers) render
// their input permanently rather than swapping display ↔ input the way
// desktop does, so the shared inlineEdit action's assumptions don't
// hold for them: its non-serial mode registers with the EditScope for
// the element's whole lifetime (an idle block would block Escape-close
// of the card page) and latches after the first commit (a second edit
// session could never commit again).
//
// draftEdit treats each focus → blur cycle as one edit session:
//
//   focus        register with the scope (the field now counts as an
//                active edit)
//   Enter        commit + end the edit (blur). Multiline fields insert
//                a newline on Shift+Enter instead.
//   Escape       cancel (caller reverts the draft) + end the edit;
//                consumed — never bubbles to the container
//   Ctrl+Enter   commit, then ask the containing scope to close
//   blur         commit (unless Enter/Escape already settled the
//                session) + deregister
//
// Mobile-surface variant (UI-CONVENTIONS §8): multiline fields pass
// `enterInsertsNewline` — plain Enter AND Shift+Enter then insert a
// native newline instead of committing; commit happens via blur / the
// ✓ Done affordance / the Ctrl+Enter chord. Escape keeps its contract
// behaviour.
//
// The caller owns the draft state: onCommit persists the draft when it
// changed, onCancel reverts it to the last committed value.

import type { EditScope } from '@shared/editScope'

export interface DraftEditParams {
  /** Persist the draft (no-op when unchanged — caller's decision). */
  onCommit: () => void
  /** Revert the draft to the last committed value. */
  onCancel: () => void
  /** Shift+Enter inserts a newline instead of committing. */
  multiline?: boolean
  /**
   * Mobile-surface variant (only meaningful with `multiline`): plain
   * Enter AND Shift+Enter insert a native newline — neither commits.
   * Ctrl+Enter and Escape keep their contract behaviour.
   */
  enterInsertsNewline?: boolean
  /** Containing EditScope; pass from getContext(EDIT_SCOPE_KEY). */
  scope?: EditScope | null
  /**
   * Whether the Ctrl+Enter chord also closes the scope's container.
   * Defaults to true per the contract.
   */
  closeOnCtrlEnter?: boolean
}

export function draftEdit(
  node: HTMLInputElement | HTMLTextAreaElement,
  params: DraftEditParams,
) {
  let unregister: (() => void) | null = null
  // True once Enter/Escape settled the current focus session — the
  // Enter-triggered blur must not commit a second time. Reset on focus.
  let settled = false

  function register() {
    if (!params.scope || unregister) return
    unregister = params.scope.register({ commit, cancel })
  }

  function deregister() {
    unregister?.()
    unregister = null
  }

  function commit() {
    if (settled) return
    settled = true
    deregister()
    params.onCommit()
  }

  function cancel() {
    if (settled) return
    settled = true
    deregister()
    params.onCancel()
  }

  function handleFocus() {
    settled = false
    register()
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.isComposing) return // IME composition — Enter selects a candidate
    if (e.key === 'Enter') {
      if (e.ctrlKey || e.metaKey) {
        e.preventDefault()
        e.stopPropagation()
        const close = params.closeOnCtrlEnter !== false ? params.scope?.requestClose : null
        commit()
        close?.()
        return
      }
      if (params.multiline && (e.shiftKey || params.enterInsertsNewline)) return // newline
      e.preventDefault()
      commit()
      node.blur()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      cancel()
      node.blur()
    }
  }

  function handleBlur() {
    commit() // no-op when Enter/Escape already settled the session
    deregister()
  }

  node.addEventListener('keydown', handleKeydown as EventListener)
  node.addEventListener('focus', handleFocus as EventListener)
  node.addEventListener('blur', handleBlur as EventListener)

  // Autofocused fields are already active when the action attaches.
  if (document.activeElement === node) register()

  return {
    update(newParams: DraftEditParams) {
      params = newParams
    },
    destroy() {
      node.removeEventListener('keydown', handleKeydown as EventListener)
      node.removeEventListener('focus', handleFocus as EventListener)
      node.removeEventListener('blur', handleBlur as EventListener)
      deregister()
    },
  }
}

