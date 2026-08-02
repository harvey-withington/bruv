// Persistence for the share page — both modes.
//
// Kept out of the component so SharePage stays a view: the ordering
// rules here — description vs bound URL block, card-first-then-slide —
// are data concerns, and they're the part worth reading on its own. The
// clip path lives here too because the same question applies to it: what
// did the server actually do, and what must the user be told about it?

import { repoRPC } from './auth'
import { t } from './i18n.svelte'
import type { Block, Card, CaptureOpts, CaptureResult, Slide } from '@shared/types'
import type { CapturePrefs, DeckTarget } from './capturePrefs'

/** First http(s) token in a blob of shared text. */
const urlInTextRe = /https?:\/\/\S+/

/**
 * Normalize incoming share params: several Android apps put the link in
 * the shared TEXT rather than a URL slot (YouTube's "Check this out
 * https://…"), and Chrome only promotes text to the `url` param when it
 * is PURELY a link — so a clip-able URL can arrive buried in `text`,
 * stranding the share in plain mode. When `url` is empty, lift the first
 * http(s) URL out of `text` (trailing prose punctuation stripped) so the
 * clip-mode preflight sees it.
 *
 * Simulate in devtools with:
 *   /m/share?text=Check%20this%20out%20https%3A%2F%2Fyoutu.be%2FdQw4w9WgXcQ
 */
export function seedShareParams(
  title: string,
  text: string,
  url: string,
): { title: string; text: string; url: string } {
  if (url.trim() || !text) return { title, text, url }
  const match = text.match(urlInTextRe)
  if (!match) return { title, text, url }
  const lifted = match[0].replace(/[)\].,!?'"]+$/, '')
  const remaining = text.replace(match[0], '').replace(/\s{2,}/g, ' ').trim()
  return { title, text: remaining, url: lifted }
}

/** A thing the user must be told after a capture, already localized.
 *  `ms` overrides the toast's default dwell for the long ones. */
export type CaptureWarning = { text: string; ms?: number }

export type ClipReport = {
  result: CaptureResult
  warnings: CaptureWarning[]
  /** The clip is half-done (the platform blocked the server): show the
   *  pending panel, never navigate away like a success. */
  pending: boolean
}

/**
 * Capture a URL server-side and work out what the user has to hear about
 * it. Every warning here is a case where BRUV did something OTHER than
 * what was asked — a bounced pin, a video kept as a link, a deck slide
 * that didn't land. Silence on any of them is the bug this whole feature
 * exists to kill.
 */
export async function saveClipShare(url: string, opts: CaptureOpts): Promise<ClipReport> {
  const result = await repoRPC<CaptureResult>('CaptureFromURL', [url, opts])
  const warnings: CaptureWarning[] = []
  // A bounced pin (accepted-types gate, stale category, unpinned deck
  // mirror) lands the card in the Inbox — say so, with the server's
  // reason, instead of celebrating a destination that was ignored.
  if (result.pinFailed) {
    warnings.push({ text: t('share.pin_failed', { error: result.pinError ?? '' }), ms: 7000 })
  }
  // Media kept in a degraded form (a video linked rather than stored):
  // the card works, but it depends on the platform now.
  for (const note of result.mediaNotes ?? []) warnings.push({ text: note, ms: 9000 })
  if (!result.pending && opts.includeInDeck && !result.slideAppended) {
    warnings.push({ text: t('share.deck_append_failed') })
  }
  return { result, warnings, pending: result.pending }
}

export type PlainShareInput = {
  title: string
  text: string
  url: string
  prefs: CapturePrefs
}

export type PlainShareResult = {
  cardID: string
  /** The card saved but the deck slide didn't — the caller warns. */
  deckFailed: boolean
}

/** The shared text and URL, verbatim. Only dedup case: the two are
 *  byte-identical, which is what Brave's "share this page" produces. */
function buildBody(url: string, text: string): string {
  if (url && text && url !== text) return `${url}\n\n${text}`
  return text || url
}

/** Park the URL in its own block so the slide can bind to it live, then
 *  append the slide. The card is already saved, so both calls are
 *  best-effort from the user's point of view. */
async function appendToDeck(cardID: string, url: string, deck: DeckTarget): Promise<boolean> {
  const blockID = `blk-${crypto.randomUUID().slice(0, 8)}`
  const block: Block = {
    id: blockID,
    type: 'url',
    label: t('share.source_block_label'),
    key: 'url',
    value: { url },
  }
  // templateId 'auto': BRUV resolves the template from the bound URL at
  // render time, so the slide upgrades itself when a matching template
  // ships.
  const slide: Partial<Slide> = {
    contentTypeId: 'post',
    templateId: 'auto',
    cardId: cardID,
    values: {},
    bindings: { url: blockID },
  }
  try {
    await repoRPC('UpdateCardBlocks', [cardID, [block]])
    await repoRPC('AppendDeckSlide', [deck.cardID, deck.blockID, slide])
    return true
  } catch {
    return false
  }
}

/**
 * Create a plain card from a share, optionally appending a deck slide.
 * Throws only if the card itself couldn't be created — a failed
 * description or deck append is reported, not fatal.
 */
export async function savePlainShare(input: PlainShareInput): Promise<PlainShareResult> {
  const url = input.url.trim()
  const text = input.text.trim()
  const deck = input.prefs.includeInDeck ? input.prefs.deckTarget : null
  const withDeck = !!deck && !!url

  const card = await repoRPC<Card>('CreateCard', ['', input.title.trim()])
  // With a deck slide the URL lives in its own bound block, so repeating
  // it in the description would just duplicate it.
  const body = withDeck ? text : buildBody(url, text)
  if (body) {
    try {
      await repoRPC('UpdateCardDescription', [card.id, body])
    } catch {
      /* the card exists either way — don't block the navigate */
    }
  }

  const appended = deck && withDeck ? await appendToDeck(card.id, url, deck) : true
  return { cardID: card.id, deckFailed: !appended }
}
