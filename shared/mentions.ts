// @mentions — the shared half.
//
// A mention is plain CommonMark: `[Label](bruv:card:<id>)` (or
// `bruv:project:<id>`), rendered by shared/markdown.ts as
// `.bruv-link[data-bruv]` and navigated by each surface's click delegate.
// Both surfaces trigger the picker the same way — an `@` typed at the
// start of a field or after whitespace — and splice the chosen markdown
// over the `@…` the user typed. That logic lived in two hand-rolled
// copies on desktop (and one of them rotted into dead code); it lives
// here now, and every editor on both surfaces goes through it.

export const CARD_MENTION_PREFIX = 'bruv:card:'
export const PROJECT_MENTION_PREFIX = 'bruv:project:'

export function cardMentionLink(cardId: string): string {
  return CARD_MENTION_PREFIX + cardId
}

export function projectMentionLink(projectId: string): string {
  return PROJECT_MENTION_PREFIX + projectId
}

/** The canonical stored form. Square brackets in the label are escaped. */
export function mentionMarkdown(label: string, link: string): string {
  return `[${label.replace(/[[\]]/g, '\\$&')}](${link})`
}

/**
 * Did this input event TYPE an `@`? The picker opens on typing the
 * trigger, never on merely arriving next to one — backspacing back to an
 * `@` the user chose to keep re-opened the picker (field report
 * 2026-09-20). Deletions, pastes of longer text, and the synthetic input
 * event `applyMentionToElement` fires all say no.
 */
export function isMentionKeystroke(e: Event): boolean {
  if (!('inputType' in e)) return false
  const ie = e as InputEvent
  if (ie.inputType !== 'insertText' && ie.inputType !== 'insertCompositionText') return false
  return (ie.data ?? '').endsWith('@')
}

/**
 * Is the caret sitting just after a trigger `@`? Returns the index of
 * the `@` or null. The `@` must start the field or follow whitespace so
 * an email address never opens the picker.
 */
export function detectMentionTrigger(value: string, caret: number): number | null {
  if (caret <= 0 || value[caret - 1] !== '@') return null
  if (caret === 1 || /\s/.test(value[caret - 2])) return caret - 1
  return null
}

/**
 * Replace `value[triggerPos, caret)` — the `@` plus whatever was typed
 * after it while the picker was open — with the mention, followed by a
 * space so typing continues naturally. Returns the new value and caret.
 */
export function spliceMention(value: string, triggerPos: number, caret: number, markdown: string): { value: string; caret: number } {
  const before = value.slice(0, Math.max(0, triggerPos))
  const after = value.slice(Math.max(triggerPos, caret))
  const inserted = after.startsWith(' ') ? markdown : markdown + ' '
  return { value: before + inserted + after, caret: before.length + inserted.length }
}

/**
 * Apply a mention to a live input/textarea: rewrite its value, fire an
 * `input` event so a framework binding (Svelte `bind:value`) picks the
 * change up, and put the caret after the mention.
 */
export function applyMentionToElement(el: HTMLInputElement | HTMLTextAreaElement, triggerPos: number, markdown: string): void {
  const caret = el.selectionStart ?? triggerPos + 1
  const next = spliceMention(el.value, triggerPos, caret, markdown)
  el.value = next.value
  el.dispatchEvent(new Event('input', { bubbles: true }))
  el.focus()
  el.setSelectionRange(next.caret, next.caret)
}

// --- Blur suspension ---------------------------------------------------
//
// The picker takes focus, which blurs the editor, which would commit
// (or unmount) it mid-mention. Editors ask here before acting on blur;
// the picker host marks its target element for the picker's lifetime.

const pickerOpenFor = new WeakSet<Element>()

export function setMentionPickerOpen(el: Element, open: boolean): void {
  if (open) pickerOpenFor.add(el)
  else pickerOpenFor.delete(el)
}

export function isMentionPickerOpenFor(el: Element | null | undefined): boolean {
  return !!el && pickerOpenFor.has(el)
}
