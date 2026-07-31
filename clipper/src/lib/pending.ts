// Pending clips — the extension half of BRUV's completion flow.
//
// When the server captures a URL shared from mobile and the platform
// bot-walls it, the server can't read the post: it creates a PENDING card
// (tagged 'clip-pending') holding just the source URL, with its deck slide
// already appended and live-bound. This extension — a real, logged-in
// browser — is what finishes the job: open the URL, capture it with the
// same DOM plugins, and post the result back via CompleteCapture, which
// fills the card's blocks in place so the slide upgrades live.
//
// Shared by the background worker (toolbar badge count) and the popup
// (the list of clips waiting to be completed).

import type { ClipperSettings } from './types'
import { loadSettings, repoRPC } from './api'

export const PENDING_TAG = 'clip-pending'

// Badge amber: "waiting on you", not an error. Matches the pending accent
// used for capture states elsewhere in BRUV.
const BADGE_COLOR = '#d97706'

type PendingBlock = { id: string; type: string; key?: string; value?: unknown }
type PendingCardRecord = { id: string; title?: string; blocks?: PendingBlock[] }

// A pending clip as the popup renders it. `url` is empty when the card has
// no source URL block — such a card can't be completed and the row says so
// rather than disappearing (a silently-hidden stuck card is worse).
export type PendingCard = { id: string; title: string; url: string }

export async function listPendingCardIDs(s: ClipperSettings): Promise<string[]> {
  return (await repoRPC<string[]>(s, 'ListCardIDsByTag', [PENDING_TAG])) ?? []
}

// The capture pipeline writes the source link as a `url`-keyed block whose
// value is `{ url }` — the same block CompleteCapture fills in place.
function sourceUrl(card: PendingCardRecord): string {
  const block = (card.blocks ?? []).find((b) => b.key === 'url')
  const value = block?.value as { url?: string } | undefined
  return value?.url ?? ''
}

export async function loadPendingCards(s: ClipperSettings, limit: number): Promise<PendingCard[]> {
  const ids = (await listPendingCardIDs(s)).slice(0, limit)
  const cards: PendingCard[] = []
  for (const id of ids) {
    try {
      const card = await repoRPC<PendingCardRecord>(s, 'GetCard', [id])
      cards.push({ id, title: card.title ?? '', url: sourceUrl(card) })
    } catch (err) {
      // One unreadable card (deleted mid-list, permissions) must not hide
      // the rest — skip it and keep the list useful.
      console.warn('pending clip lookup failed:', err)
    }
  }
  return cards
}

// refreshPendingBadge stamps the count of waiting clips onto the toolbar
// icon. Best-effort by design: offline or unpaired leaves the last known
// badge alone rather than flashing a misleading zero — this runs on alarms
// and startup, where the user isn't watching and can't act on an error.
export async function refreshPendingBadge(): Promise<void> {
  try {
    const settings = await loadSettings()
    if (!settings || !settings.repoID) return
    const n = (await listPendingCardIDs(settings)).length
    await chrome.action.setBadgeText({ text: n > 0 ? String(n) : '' })
    await chrome.action.setBadgeBackgroundColor({ color: BADGE_COLOR })
  } catch (err) {
    console.warn('pending badge refresh failed:', err)
  }
}
