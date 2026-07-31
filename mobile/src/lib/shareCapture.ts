// Persistence for the share page's PLAIN mode (the clip mode path is a
// single CaptureFromURL call and lives in the page).
//
// Kept out of the component so SharePage stays a view: the ordering
// rules here — description vs bound URL block, card-first-then-slide —
// are data concerns, and they're the part worth reading on its own.

import { repoRPC } from './auth'
import { t } from './i18n.svelte'
import type { Block, Card, Slide } from '@shared/types'
import type { CapturePrefs, DeckTarget } from './capturePrefs'

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
