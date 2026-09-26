import { GetCard, GetCardProjectContext } from '@shared/api'
import { onEvent } from './events'

// Mentions freeze the label they were inserted with —
// `[Old Title](bruv:card:id)` — and nothing on disk rewrites them when
// the card is renamed. Rather than scanning every card on every rename,
// links are refreshed where they render: whenever a `.bruv-link` for a
// card lands in the DOM its text is set to the card's live title and its
// tooltip to the breadcrumb, from a per-session cache that a
// card:updated event invalidates. Stored text stays as written; what the
// user sees follows the card.

const CARD_LINK_SEL = '.bruv-link[data-bruv^="card:"]'
const titles = new Map<string, Promise<string | null>>()
const crumbs = new Map<string, Promise<string | null>>()

function liveTitle(cardId: string): Promise<string | null> {
  let p = titles.get(cardId)
  if (!p) {
    p = GetCard(cardId).then(c => c?.title ?? null).catch(() => null)
    titles.set(cardId, p)
  }
  return p
}

function breadcrumb(cardId: string): Promise<string | null> {
  let p = crumbs.get(cardId)
  if (!p) {
    p = GetCardProjectContext(cardId).then(ctx => ctx || null).catch(() => null)
    crumbs.set(cardId, p)
  }
  return p
}

function refreshLink(link: HTMLAnchorElement) {
  const cardId = link.getAttribute('data-bruv')!.slice(5)
  void liveTitle(cardId).then(title => {
    if (title && link.textContent !== title) link.textContent = title
  })
  if (!link.hasAttribute('title')) {
    void breadcrumb(cardId).then(ctx => { if (ctx) link.title = ctx })
  }
}

export function refreshMentionLinks(root: Element | Document): void {
  if (root instanceof Element && root.matches(CARD_LINK_SEL)) refreshLink(root as HTMLAnchorElement)
  root.querySelectorAll<HTMLAnchorElement>(CARD_LINK_SEL).forEach(refreshLink)
}

/** Watch the whole document; call once at boot. */
export function installMentionLabelRefresh(): void {
  refreshMentionLinks(document)
  const observer = new MutationObserver((mutations) => {
    for (const m of mutations) {
      for (const node of m.addedNodes) {
        if (node instanceof Element) refreshMentionLinks(node)
      }
    }
  })
  observer.observe(document.body, { childList: true, subtree: true })
  // A rename invalidates that card's cached title; links already on
  // screen re-resolve immediately.
  onEvent<{ cardID?: string }>('card:updated', (ev) => {
    if (!ev.cardID) return
    titles.delete(ev.cardID)
    crumbs.delete(ev.cardID)
    document.querySelectorAll<HTMLAnchorElement>(`.bruv-link[data-bruv="card:${ev.cardID}"]`).forEach(refreshLink)
  })
}
