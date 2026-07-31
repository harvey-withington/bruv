// Sticky capture preferences for the mobile clip flow.
//
// Clipping is a repeat action — the second clip of a session should be
// one tap. So the deck target, the pin destination, and the
// include-in-deck toggle persist between visits to the share page,
// mirroring the browser extension's sticky deck target.
//
// Scoped per vault: a deck card ID from one repo is meaningless in
// another, so the storage key carries the active repo ID.

import { readActiveRepoID } from './auth'
import type { CaptureOpts } from '@shared/types'

/** A slide deck to append clipped slides to: the host card plus the
 *  specific `slide_deck` block inside it. */
export type DeckTarget = {
  cardID: string
  blockID: string
  name: string
}

export type CapturePrefs = {
  deckTarget: DeckTarget | null
  /** '' = Inbox (no pin), DECK_PIN_SENTINEL = mirror the deck card's
   *  pins, anything else = a category ID. */
  categoryID: string
  categoryName: string
  includeInDeck: boolean
}

/** categoryID sentinel: pin the clip wherever the deck card is pinned. */
export const DECK_PIN_SENTINEL = '__deck__'

const KEY_PREFIX = 'bruv:capture_prefs:'

export function defaultCapturePrefs(): CapturePrefs {
  return { deckTarget: null, categoryID: '', categoryName: '', includeInDeck: false }
}

function storageKey(): string | null {
  const repoID = readActiveRepoID()
  return repoID ? `${KEY_PREFIX}${repoID}` : null
}

function parseDeckTarget(raw: unknown): DeckTarget | null {
  if (!raw || typeof raw !== 'object') return null
  const t = raw as Record<string, unknown>
  if (typeof t.cardID !== 'string' || typeof t.blockID !== 'string') return null
  if (!t.cardID || !t.blockID) return null
  return { cardID: t.cardID, blockID: t.blockID, name: typeof t.name === 'string' ? t.name : '' }
}

/** Read this vault's saved prefs. Anything missing, malformed, or
 *  unreadable falls back to the defaults — prefs are convenience, never
 *  a reason to fail a capture. */
export function loadCapturePrefs(): CapturePrefs {
  const key = storageKey()
  if (!key) return defaultCapturePrefs()
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return defaultCapturePrefs()
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') return defaultCapturePrefs()
    const p = parsed as Record<string, unknown>
    return {
      deckTarget: parseDeckTarget(p.deckTarget),
      categoryID: typeof p.categoryID === 'string' ? p.categoryID : '',
      categoryName: typeof p.categoryName === 'string' ? p.categoryName : '',
      includeInDeck: p.includeInDeck === true,
    }
  } catch {
    return defaultCapturePrefs()
  }
}

export function saveCapturePrefs(prefs: CapturePrefs): void {
  const key = storageKey()
  if (!key) return
  try {
    localStorage.setItem(key, JSON.stringify(prefs))
  } catch {
    /* private mode / quota — the prefs just don't stick */
  }
}

/**
 * Translate prefs into the server's CaptureOpts envelope.
 *
 * The deck IDs go out whenever a target is set, even with the toggle
 * off: the "pin with deck" sentinel is resolved server-side from
 * deckCardID, so dropping it would silently turn that choice into
 * "Inbox". A sentinel with no deck target degrades to Inbox here.
 */
export function captureOptsFrom(prefs: CapturePrefs): CaptureOpts {
  const deck = prefs.deckTarget
  return {
    includeInDeck: prefs.includeInDeck && !!deck,
    deckCardID: deck?.cardID ?? '',
    deckBlockID: deck?.blockID ?? '',
    categoryID: prefs.categoryID === DECK_PIN_SENTINEL && !deck ? '' : prefs.categoryID,
  }
}
