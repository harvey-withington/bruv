// inlineEdit — the field half of BRUV's keyboard entry contract
// (see UI-CONVENTIONS "Keyboard entry"). One Svelte action implements
// the whole contract so every data-entry surface behaves identically:
//
//   Enter        commit + end the edit
//                (multiline: Shift+Enter inserts a newline instead)
//   Escape       cancel without committing; consumed (never bubbles)
//   Ctrl+Enter   commit, then ask the containing scope to close
//   blur         commit (edit-in-place) — serial/composer fields keep
//                their draft uncommitted instead
//
// Serial mode is for "add another" inputs (checklist/list/media/option
// add rows): Enter commits the item and re-arms for the next, blur
// never commits, and the field registers with the scope only while
// focused (an idle-but-mounted add row must not block Escape-close).
//
// Mobile-surface variant (UI-CONVENTIONS §8): multiline fields on the
// touch-first PWA pass `enterInsertsNewline` — plain Enter AND
// Shift+Enter then insert a native newline instead of committing;
// commit happens via blur / an explicit ✓ Done affordance / the
// Ctrl+Enter chord. Escape keeps its contract behaviour. Default off —
// desktop never sets it.

import type { EditScope } from './editScope'

export interface InlineEditParams {
  onCommit: () => void
  onCancel: () => void
  /** Shift+Enter inserts a newline instead of committing. */
  multiline?: boolean
  /**
   * Mobile-surface variant (only meaningful with `multiline`): plain
   * Enter AND Shift+Enter insert a native newline — neither commits.
   * Ctrl+Enter and Escape keep their contract behaviour; commit
   * otherwise happens on blur or via an explicit ✓ Done affordance.
   */
  enterInsertsNewline?: boolean
  /**
   * "Add another" semantics: Enter commits without ending the edit
   * (caller clears the draft and keeps focus), blur preserves the
   * draft uncommitted.
   */
  serial?: boolean
  /**
   * Commit when focus leaves the field. Defaults to true for
   * edit-in-place fields; forced off by serial mode. Composers that
   * keep their draft on blur pass false.
   */
  blurCommits?: boolean
  /**
   * CSS selector — blur is ignored while focus stays inside the
   * matching ancestor (e.g. a name+description rename group).
   */
  container?: string
  /** Containing EditScope; pass from getContext(EDIT_SCOPE_KEY). */
  scope?: EditScope | null
  /**
   * Whether the Ctrl+Enter chord also closes the scope's container.
   * Defaults to true per the contract; the chat composer passes false
   * (send-and-close would hide the reply).
   */
  closeOnCtrlEnter?: boolean
}

export function inlineEdit(
  node: HTMLInputElement | HTMLTextAreaElement,
  params: InlineEditParams,
) {
  let committed = false
  let unregister: (() => void) | null = null

  function register() {
    if (!params.scope || unregister) return
    unregister = params.scope.register({ commit, cancel })
  }

  function deregister() {
    unregister?.()
    unregister = null
  }

  function commit() {
    if (params.serial) {
      // Serial fields re-arm: no latch, stay registered while focused.
      params.onCommit()
      return
    }
    if (committed) return
    committed = true
    deregister()
    params.onCommit()
  }

  function cancel() {
    if (params.serial) {
      deregister()
      params.onCancel()
      return
    }
    if (committed) return
    committed = true
    deregister()
    params.onCancel()
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.isComposing) return // IME composition — Enter selects a candidate
    if (e.key === 'Enter') {
      if (e.ctrlKey || e.metaKey) {
        // With a scope the chord is fully handled here (commit + close).
        // WITHOUT one — always-mounted form fields that deliberately
        // skip registration so they don't block Escape-close — commit
        // but let the event bubble to the container's own window
        // handler, which owns commitAll + close. Swallowing it here
        // made Ctrl+Enter in those fields commit without closing AND
        // blocked the dialog's handler.
        e.preventDefault()
        if (params.scope) {
          e.stopPropagation()
          const close = params.closeOnCtrlEnter !== false ? params.scope.requestClose : null
          commit()
          close?.()
        } else {
          commit()
        }
        return
      }
      if (params.multiline && (e.shiftKey || params.enterInsertsNewline)) return // newline
      e.preventDefault()
      commit()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      cancel()
    }
  }

  function handleBlur(e: FocusEvent) {
    if (params.container) {
      const container = node.closest(params.container)
      const related = e.relatedTarget as HTMLElement | null
      if (container && related && container.contains(related)) return
    }
    if (params.serial) {
      // Keep the draft, just stop counting as an active edit.
      deregister()
      return
    }
    if (params.blurCommits !== false) commit()
    else deregister()
  }

  function handleFocus() {
    // Serial and blurCommits:false fields deregister on blur while
    // staying mounted — refocusing must re-register. register() guards
    // double-registration for everything else.
    committed = false
    register()
  }

  function handleInput() {
    // Typing starts a new edit session. The committed latch exists only
    // to block the Enter→immediate-blur double-fire within ONE session;
    // on always-mounted fields (dialog forms) it would otherwise stay
    // latched forever after the first commit and silently swallow every
    // subsequent save.
    committed = false
    register()
  }

  node.addEventListener('keydown', handleKeydown as EventListener)
  node.addEventListener('blur', handleBlur as EventListener)
  node.addEventListener('focus', handleFocus as EventListener)
  node.addEventListener('input', handleInput as EventListener)

  // Edit-in-place fields mount only while editing → register for the
  // element's lifetime. Serial fields register on focus instead.
  if (!params.serial) register()
  else if (document.activeElement === node) register()

  return {
    update(newParams: InlineEditParams) {
      params = newParams
      committed = false
      if (!unregister && !params.serial) register()
    },
    destroy() {
      node.removeEventListener('keydown', handleKeydown as EventListener)
      node.removeEventListener('blur', handleBlur as EventListener)
      node.removeEventListener('focus', handleFocus as EventListener)
      node.removeEventListener('input', handleInput as EventListener)
      deregister()
    },
  }
}
