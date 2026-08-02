// The genericity contract lives here. Everything downstream of a plugin —
// card mapping, media download, deck append, queueing — consumes ONLY these
// shapes and must stay platform-blind. Exactly ONE artifact knows a
// platform: the extractor plugin (emits ClipResult). Slides are stamped
// 'auto'; BRUV resolves the template from the capture URL.

export type ClipMediaKind = 'image' | 'video'

// One rung of a video quality ladder. Field-for-field the server's
// MediaVariant (shared/types.ts), so a ladder resolved by a plugin and one
// resolved by the server are interchangeable in the capture dialog.
export type ClipMediaVariant = {
  id: string
  label: string
  url: string
  bitrate?: number
  estBytes?: number
}

export type ClipMedia = {
  url: string
  kind: ClipMediaKind
  // Poster/preview for videos — used as the fallback when video resolution
  // fails, and as the attachment thumbnail source.
  posterUrl?: string
  // Every quality the plugin found, ASCENDING (cheapest first). Optional:
  // most platforms serve one rendition. `url` stays the plugin's own pick
  // so nothing downstream has to understand ladders.
  variants?: ClipMediaVariant[]
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
  // Media the user chose to KEEP AS A LINK (or a video that wouldn't
  // download): the URL goes into the block instead of an attachment. Absent
  // on jobs queued before capture options existed.
  linkMedia?: Array<{ url: string; kind: ClipMediaKind }>
  // Title the user typed in the capture dialog; empty/absent = derive it
  // from the clip as usual.
  title?: string
  avatarBase64?: string
  avatarName?: string
  attempts: number
  lastError?: string
}

// --- capture options ------------------------------------------------------
//
// Capture decisions are the USER'S, made at capture time and pre-populated
// from their (per-vault, server-side) defaults — Harvey's 2026-08-02 ruling;
// design in `plan/2026-08-02 capture options at capture time.md`. The types
// below mirror the server's, so the extension shows the same facts the phone
// would show for the same URL.

export type VideoMode = 'fit' | 'best' | 'smallest' | 'link' | 'skip'
export type ImageMode = 'all' | 'first' | 'link' | 'skip'
export type AskMode = 'always' | 'triggers' | 'never'

// The user's own definition of "consequential" — a zero/false entry turns
// that trigger off.
export type CaptureTriggers = {
  videoOverMB?: number
  galleryOverCount?: number
  unsupportedUrl?: boolean
  blocked?: boolean
  pinMayReject?: boolean
}

// Per-VAULT capture defaults (GetCapturePrefs/SetCapturePrefs) — read from
// the server, never from chrome.storage: the phone and the clipper must
// agree about what happens to a 3.5 GB video.
export type CapturePrefs = {
  videoMode?: VideoMode
  videoBudgetMB?: number
  imageMode?: ImageMode
  askMode?: AskMode
  triggers: CaptureTriggers
}

export type CaptureMediaPreview = {
  kind: ClipMediaKind
  url: string
  posterUrl?: string
  estBytes?: number
  variants?: ClipMediaVariant[]
  defaultVariantId?: string
  note?: string
}

// PreviewCapture(url) — what capturing this URL WOULD do, server-side.
// Writes nothing. `blocked`/`supported` describe the SERVER's reach, not
// the browser's: the extension captures from the live DOM either way.
export type CapturePreview = {
  url: string
  platform: string
  supported: boolean
  blocked: boolean
  blockedError?: string
  title: string
  author?: string
  handle?: string
  text?: string
  publishedAt?: string
  media?: CaptureMediaPreview[]
  prefs: CapturePrefs
  shouldAsk: boolean
  askReasons?: string[]
}

export type VideoChoice = 'store' | 'link' | 'skip'
export type ImageChoice = ImageMode

// What the user actually chose (dialog) or what their defaults imply (no
// dialog). Consumed by buildJob/executeJob.
export type CaptureChoices = {
  // Card title. Empty = derive it from the clip as usual.
  title: string
  includeInDeck: boolean
  video: VideoChoice
  // The chosen rung's URL, replacing whatever the plugin resolved for the
  // clip's first video. Empty = keep the plugin's pick.
  videoUrl: string
  images: ImageChoice
}

// Sentinel option ids for the two non-storing video rows. Real rungs use
// their variant id.
export const VIDEO_OPTION_LINK = '__link__'
export const VIDEO_OPTION_SKIP = '__skip__'

// One row of the dialog's video radio list. `estBytes` is formatted by the
// dialog (presentation lives with the renderer).
export type CaptureDialogVideoOption = { id: string; label: string; url: string; estBytes?: number }

// Everything the in-page dialog renders. Built background-side so the
// content script needs no settings, no RPC and no prefs logic.
export type CaptureDialogRequest = {
  type: 'BRUV_OPTIONS'
  platform: string
  // "Author · @handle", already composed; empty when the clip has neither.
  byline: string
  imageCount: number
  // Empty when there's nothing downloadable to decide about (text-only
  // post, or a platform BRUV embeds rather than stores).
  videoOptions: CaptureDialogVideoOption[]
  // Plain-language facts about what WILL happen, already localized.
  notes: string[]
  deckName: string
  pinName: string
  canDeck: boolean
  defaults: {
    title: string
    includeInDeck: boolean
    videoOptionId: string
    images: ImageChoice
  }
}

export type CaptureDialogResponse = { ok: boolean; choices?: CaptureChoices }

// Messages between content script and background.
export type ClipRequestMessage = { type: 'BRUV_CLIP'; includeInDeck: boolean; withOptions: boolean }
export type ClipExtractedMessage = {
  type: 'BRUV_EXTRACTED'
  clip: ClipResult
  includeInDeck: boolean
  // The user picked "Add to BRUV (options…)" — show the dialog whatever the
  // vault's ask-mode says.
  withOptions: boolean
}
export type ToastMessage = { type: 'BRUV_TOAST'; text: string; ok: boolean }
// Keeps the service worker's idle timer from expiring while a dialog waits
// on a human (inbound messages reset it). No payload, no reply.
export type DialogAliveMessage = { type: 'BRUV_DIALOG_ALIVE' }
// The dialog's "Change in Options" link — only the worker can open the
// options page.
export type OpenOptionsMessage = { type: 'BRUV_OPEN_OPTIONS' }

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
