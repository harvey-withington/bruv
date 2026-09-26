import { applyMentionToElement, detectMentionTrigger, isMentionKeystroke, setMentionPickerOpen } from '@shared/mentions'

// The desktop @mention host: ONE picker instance (mounted in App.svelte)
// serves every editor in the app. Editors opt in with `use:mentionable`;
// typing `@` at a word boundary opens the picker anchored under the
// field, choosing an entry splices `[Label](bruv:card:id)` over the `@`
// and refocuses the field. While the picker is open the field's blur is
// suspended (shared/mentions.ts), so inlineEdit and the hand-rolled
// description/text-block handlers neither commit nor unmount mid-mention.

type MentionField = HTMLInputElement | HTMLTextAreaElement

export interface MentionRequest {
  node: MentionField
  triggerPos: number
  anchor: { top: number; left: number }
}

export const mentionHost = $state<{ request: MentionRequest | null }>({ request: null })

export function isMentionPickerOpen(): boolean {
  return mentionHost.request !== null
}

function open(node: MentionField, triggerPos: number) {
  const rect = node.getBoundingClientRect()
  setMentionPickerOpen(node, true)
  mentionHost.request = { node, triggerPos, anchor: { top: rect.bottom + 4, left: rect.left } }
}

/** Picker callback: insert the chosen markdown into the requesting field. */
export function selectMention(markdown: string): void {
  const r = mentionHost.request
  if (!r) return
  mentionHost.request = null
  applyMentionToElement(r.node, r.triggerPos, markdown)
  setMentionPickerOpen(r.node, false)
}

/** Picker callback: dismissed without choosing — hand focus back. */
export function closeMentionPicker(): void {
  const r = mentionHost.request
  if (!r) return
  mentionHost.request = null
  r.node.focus()
  setMentionPickerOpen(r.node, false)
}

/**
 * Svelte action: makes an input/textarea @mentionable. Place it BEFORE
 * `use:inlineEdit` in the markup so its listeners register first.
 */
export function mentionable(node: MentionField) {
  function onInput(e: Event) {
    if (mentionHost.request || !isMentionKeystroke(e)) return
    const pos = detectMentionTrigger(node.value, node.selectionStart ?? 0)
    if (pos !== null) open(node, pos)
  }
  node.addEventListener('input', onInput)
  return {
    destroy() {
      node.removeEventListener('input', onInput)
      if (mentionHost.request?.node === node) {
        mentionHost.request = null
        setMentionPickerOpen(node, false)
      }
    },
  }
}
