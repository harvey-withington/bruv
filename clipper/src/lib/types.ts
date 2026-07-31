// The genericity contract lives here. Everything downstream of a plugin —
// card mapping, media download, deck append, queueing — consumes ONLY these
// shapes and must stay platform-blind. Exactly ONE artifact knows a
// platform: the extractor plugin (emits ClipResult). Slides are stamped
// 'auto'; BRUV resolves the template from the capture URL.

export type ClipMediaKind = 'image' | 'video'

export type ClipMedia = {
  url: string
  kind: ClipMediaKind
  // Poster/preview for videos — used as the fallback when video resolution
  // fails, and as the attachment thumbnail source.
  posterUrl?: string
}

export type ClipResult = {
  // Plugin id, e.g. "twitter". Becomes the card tag; never branches logic.
  platform: string
  // Permalink of the captured unit (a reply's own status URL, not the page's).
  canonicalUrl: string
  author: string
  handle?: string
  avatarUrl?: string
  text: string
  media: ClipMedia[]
  publishedAt?: string
  // Embedded playback via the platform's official player (YouTube etc.) —
  // for sources whose video streams can't be downloaded in-browser. Becomes
  // the slide's `video` value as "embed://<provider>/<id>"; renderers show
  // an iframe. Never downloaded, never an attachment.
  embedVideo?: { provider: string; id: string }
  // Platform-specific extras the generic pipeline stores but never reads.
  extras: Record<string, string>
  // Set by the DOM extractor when media needs background-side resolution
  // (e.g. X serves video through blob: URLs — only an API lookup can get a
  // fetchable URL). The plugin's enrich() consumes this.
  needsEnrichment?: boolean
}

// A slide-deck target the user picked once; every subsequent clip appends to
// it ("repeat for each slide" = one click).
export type DeckTarget = {
  cardID: string
  blockID: string
  name: string
}

// Sentinel for ClipperSettings.categoryID: pin each clipped card to the same
// location(s) as the deck-target CARD (all of its pins), resolved at clip
// time via GetCardPins. Falls back to Inbox when no deck target is set.
export const PIN_WITH_DECK = '__deck__'

export type ClipperSettings = {
  serverURL: string
  deviceToken: string
  deviceID: string
  repoID: string
  repoName: string
  deckTarget: DeckTarget | null
  // Category to pin clipped cards into; empty = Inbox (unpinned);
  // PIN_WITH_DECK = mirror the deck target card's pins.
  categoryID: string
  categoryName: string
}

// A queued clip job: fully self-contained (media already downloaded to
// base64 at capture time, so CDN links can't rot while the job waits).
export type ClipJob = {
  id: string
  createdAt: string
  clip: ClipResult
  includeInDeck: boolean
  media: Array<{ name: string; base64: string; kind: ClipMediaKind }>
  avatarBase64?: string
  avatarName?: string
  attempts: number
  lastError?: string
}

// Messages between content script and background.
export type ClipRequestMessage = { type: 'BRUV_CLIP'; includeInDeck: boolean }
export type ClipExtractedMessage = { type: 'BRUV_EXTRACTED'; clip: ClipResult; includeInDeck: boolean }
export type ToastMessage = { type: 'BRUV_TOAST'; text: string; ok: boolean }

// Completion flow (background → content): capture the page's PRIMARY unit
// with no click target. The tab is transient and unattended, so the reply
// travels back over sendResponse instead of a follow-up message, and the
// content script shows no toast.
export type ClipPageRequestMessage = { type: 'BRUV_CLIP_PAGE' }
export type ClipPageResponse = { clip: ClipResult | null }

// Completion flow (popup → background): finish one pending clip card by
// opening its source URL in a real, logged-in browser tab.
export type CompleteRequestMessage = { type: 'BRUV_COMPLETE'; cardID: string; url: string }
export type CompleteResponse = { ok: boolean; error?: string }
