import { applyMentionToElement, detectMentionTrigger, isMentionKeystroke, setMentionPickerOpen } from '@shared/mentions'
import { repoRPC } from './auth'
import { navigate, cardURL, projectURL } from './router.svelte'

// The mobile @mention host: ONE MentionSheet (mounted in App.svelte)
// serves every editor. Editors opt in with `use:mentionable`; typing `@`
// at a word boundary opens the sheet, picking an entry splices
// `[Label](bruv:card:id)` over the `@` and refocuses the field. While the
// sheet is open the field's blur is suspended (shared/mentions.ts), so
// inlineEdit/draftEdit neither commit nor unmount mid-mention.

type MentionField = HTMLInputElement | HTMLTextAreaElement

export interface MentionRequest {
  node: MentionField
  triggerPos: number
}

export const mentionHost = $state<{ request: MentionRequest | null; closing: boolean }>({ request: null, closing: false })

/**
 * True while the sheet is open OR in the instant after it closes. The
 * sheet owns a history entry (BottomSheet), so closing it pops history,
 * and CardPage's Back-cancels-the-edit handler must not read that pop as
 * the user backing out of the edit they were mid-way through — that
 * threw away the typed text along with the @ (field report 2026-09-20).
 */
export function isMentionSheetActive(): boolean {
  return mentionHost.request !== null || mentionHost.closing
}

function beginClose() {
  mentionHost.closing = true
  // Cleared on the pop the sheet's cleanup produces (see the popstate
  // listener in installMentionNavigation); the timeout is the fallback
  // for a close that produced no pop (the user's own Back already did).
  setTimeout(() => { mentionHost.closing = false }, 500)
}

export function selectMention(markdown: string): void {
  const r = mentionHost.request
  if (!r) return
  beginClose()
  mentionHost.request = null
  applyMentionToElement(r.node, r.triggerPos, markdown)
  setMentionPickerOpen(r.node, false)
}

export function closeMentionPicker(): void {
  const r = mentionHost.request
  if (!r) return
  beginClose()
  mentionHost.request = null
  r.node.focus()
  setMentionPickerOpen(r.node, false)
}

/** Svelte action: place BEFORE `use:inlineEdit` / `use:draftEdit`. */
export function mentionable(node: MentionField) {
  function onInput(e: Event) {
    if (mentionHost.request || !isMentionKeystroke(e)) return
    const pos = detectMentionTrigger(node.value, node.selectionStart ?? 0)
    if (pos === null) return
    setMentionPickerOpen(node, true)
    mentionHost.request = { node, triggerPos: pos }
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

// --- Following a mention -------------------------------------------------
//
// shared/markdown.ts renders a mention as `<a class="bruv-link"
// data-bruv="card:<id>">` with no href, so navigation is a delegate —
// the same shape as the desktop's main.ts interceptor. Before this the
// phone rendered mentions as inert text that also swallowed the
// tap-to-edit gesture.

/**
 * Follow the mention under an event target, if there is one. Returns true
 * when it navigated (the caller then skips its own tap handling, e.g.
 * entering edit mode). Editors call this directly from their display's
 * click handler rather than relying on document-level delegation order.
 */
export function followMention(target: EventTarget | null): boolean {
  const anchor = (target as HTMLElement | null)?.closest?.('a[data-bruv]')
  if (!anchor) return false
  const bruv = anchor.getAttribute('data-bruv') ?? ''
  if (bruv.startsWith('card:')) {
    navigate(cardURL(bruv.slice(5)))
    return true
  }
  if (bruv.startsWith('project:')) {
    const id = bruv.slice(8)
    void repoRPC<{ brandSlug: string; streamSlug: string; projectSlug: string }>('GetProjectLocation', [id])
      .then((loc) => { if (loc) navigate(projectURL(loc.brandSlug, loc.streamSlug, loc.projectSlug)) })
      .catch(() => { /* a dead project link just stays put */ })
    return true
  }
  return false
}

export function installMentionNavigation(): void {
  // The pop from the sheet's cleanup: keep `closing` true for the rest of
  // this dispatch (CardPage's handler runs after this one) and clear it
  // once the event has finished.
  window.addEventListener('popstate', () => {
    if (mentionHost.closing) setTimeout(() => { mentionHost.closing = false }, 0)
  })
  // Fallback for rendered text that has no click handler of its own
  // (comment bodies, previews); editors that also toggle edit mode call
  // followMention first so the link wins over entering edit.
  document.addEventListener('click', (e) => {
    if (e.defaultPrevented) return
    if (followMention(e.target)) {
      e.preventDefault()
      e.stopPropagation()
    }
  })
}

// --- Live labels ---------------------------------------------------------
//
// The stored label freezes at insert time; what the user SEES follows the
// card. Same approach as desktop's lib/mentionLabels.ts, over repoRPC.

const titles = new Map<string, Promise<string | null>>()
const CARD_LINK_SEL = '.bruv-link[data-bruv^="card:"]'

function liveTitle(cardId: string): Promise<string | null> {
  let p = titles.get(cardId)
  if (!p) {
    p = repoRPC<{ title?: string }>('GetCard', [cardId]).then(c => c?.title ?? null).catch(() => null)
    titles.set(cardId, p)
  }
  return p
}

function refreshLink(link: HTMLAnchorElement) {
  const cardId = link.getAttribute('data-bruv')!.slice(5)
  void liveTitle(cardId).then(title => { if (title && link.textContent !== title) link.textContent = title })
}

export function installMentionLabelRefresh(onCardUpdated: (handler: (cardID: string) => void) => void): void {
  const refreshIn = (root: Element | Document) => {
    if (root instanceof Element && root.matches(CARD_LINK_SEL)) refreshLink(root as HTMLAnchorElement)
    root.querySelectorAll<HTMLAnchorElement>(CARD_LINK_SEL).forEach(refreshLink)
  }
  refreshIn(document)
  new MutationObserver((mutations) => {
    for (const m of mutations) for (const n of m.addedNodes) if (n instanceof Element) refreshIn(n)
  }).observe(document.body, { childList: true, subtree: true })
  onCardUpdated((cardID) => {
    titles.delete(cardID)
    document.querySelectorAll<HTMLAnchorElement>(`.bruv-link[data-bruv="card:${cardID}"]`).forEach(refreshLink)
  })
}
