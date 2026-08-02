// Capture options — the decision layer that sits IN FRONT of the clip
// pipeline (design: `plan/2026-08-02 capture options at capture time.md`;
// Harvey's ruling the same day: "make all these decisions the users' at
// capture time… let them set reasonable defaults and present them with a
// capture options dialog at capture time with as much pre-populated as
// possible").
//
// Everything here is background-side and pure-ish: it turns
// (clip + server preview + vault prefs + settings) into either a dialog to
// show or the choices the user's defaults imply. It decides nothing on the
// user's behalf that they haven't configured.
//
// Two sources of truth about a video ladder, deliberately:
//   - the PLUGIN's (resolved in the logged-in browser) — the only one that
//     exists for platforms that bot-wall BRUV's server, i.e. X;
//   - the SERVER's PreviewCapture (same facts the phone would show).
// The plugin's wins when it has one, because its URLs are guaranteed
// fetchable from here; the server's fills in everywhere else.

import { repoRPC } from './api'
import { formatBytes } from './format'
import { cardTitle } from './clip'
import {
  PIN_WITH_DECK,
  VIDEO_OPTION_LINK,
  VIDEO_OPTION_SKIP,
  type CaptureChoices,
  type CaptureDialogRequest,
  type CaptureDialogVideoOption,
  type CapturePrefs,
  type CapturePreview,
  type ClipMediaVariant,
  type ClipResult,
  type ClipperSettings,
  type ImageChoice,
} from './types'

const MB = 1024 * 1024

const msg = (key: string): string => chrome.i18n.getMessage(key)

// PreviewCapture resolves the post server-side, which on a platform that
// blocks BRUV means waiting for a refusal. Right-click capture is prized
// for being one click, so the automatic path gives up quickly and falls
// back to what the browser itself knows; an explicit "options…" request
// waits longer, because the user asked to see the facts.
const PREVIEW_TIMEOUT_MS = 4_000
const PREVIEW_TIMEOUT_PATIENT_MS = 15_000

// The vault's defaults, used when the server can't be asked for them but
// the user explicitly wants the dialog. Same values as the server's
// DefaultCapturePrefs — the dialog would otherwise have nothing to
// pre-select.
export function fallbackPrefs(): CapturePrefs {
  return {
    videoMode: 'fit',
    videoBudgetMB: 50,
    imageMode: 'all',
    askMode: 'triggers',
    triggers: { videoOverMB: 50, galleryOverCount: 8, unsupportedUrl: true, blocked: true, pinMayReject: true },
  }
}

// loadCapturePrefs reads the vault's defaults. Failure returns null — the
// caller then behaves exactly as the extension did before options existed.
export async function loadCapturePrefs(s: ClipperSettings): Promise<CapturePrefs | null> {
  try {
    const prefs = await repoRPC<CapturePrefs>(s, 'GetCapturePrefs', [])
    return prefs ? { ...prefs, triggers: prefs.triggers ?? {} } : null
  } catch (err) {
    // An older server has no such method; either way the clip proceeds.
    console.warn('capture prefs unavailable:', err)
    return null
  }
}

// previewCapture asks the server what capturing this URL would do. Bounded
// by a client-side timeout: the preview is an ENRICHMENT (sizes, ladder,
// "is this consequential"), never a precondition — the extension captures
// from the live DOM regardless.
export async function previewCapture(
  s: ClipperSettings,
  url: string,
  patient: boolean,
): Promise<CapturePreview | null> {
  if (!url) return null
  const timeout = patient ? PREVIEW_TIMEOUT_PATIENT_MS : PREVIEW_TIMEOUT_MS
  try {
    return await Promise.race([
      repoRPC<CapturePreview>(s, 'PreviewCapture', [url]),
      new Promise<null>((resolve) => setTimeout(() => resolve(null), timeout)),
    ])
  } catch (err) {
    console.warn('capture preview unavailable:', err)
    return null
  }
}

// --- video ladder ---------------------------------------------------------

function clipVideo(clip: ClipResult): { url: string; variants?: ClipMediaVariant[] } | null {
  const v = clip.media.find((m) => m.kind === 'video' && m.url)
  return v ? { url: v.url, variants: v.variants } : null
}

function previewVideo(preview: CapturePreview | null): { estBytes?: number; variants?: ClipMediaVariant[] } | null {
  const v = preview?.media?.find((m) => m.kind === 'video')
  return v ? { estBytes: v.estBytes, variants: v.variants } : null
}

// videoOptions lists the storable qualities for the clip's video, cheapest
// first. Empty when there's no downloadable video (text-only post, or a
// platform BRUV embeds rather than stores, like YouTube).
export function videoOptions(clip: ClipResult, preview: CapturePreview | null): CaptureDialogVideoOption[] {
  const video = clipVideo(clip)
  if (!video) return []
  const server = previewVideo(preview)
  const ladder = video.variants?.length ? video.variants : (server?.variants ?? [])
  if (ladder.length > 0) {
    return ladder.map((v) => ({ id: v.id, label: v.label, url: v.url, estBytes: v.estBytes }))
  }
  // No ladder anywhere: one rendition, take it or leave it.
  return [{ id: 'original', label: msg('dialog_video_original'), url: video.url, estBytes: server?.estBytes }]
}

// defaultVideoOptionId applies the vault's video mode to the ladder. An
// unknown size counts as fitting (same rule as the server's
// defaultVariantID) — guessing high would re-introduce the silent
// downgrade this whole feature exists to remove.
export function defaultVideoOptionId(options: CaptureDialogVideoOption[], prefs: CapturePrefs): string {
  const mode = prefs.videoMode ?? 'fit'
  if (mode === 'link') return VIDEO_OPTION_LINK
  if (mode === 'skip') return VIDEO_OPTION_SKIP
  if (options.length === 0) return VIDEO_OPTION_SKIP
  if (mode === 'best') return options[options.length - 1].id
  if (mode === 'smallest') return options[0].id
  const budget = (prefs.videoBudgetMB ?? 50) * MB
  for (let i = options.length - 1; i >= 0; i--) {
    const est = options[i].estBytes ?? 0
    if (est === 0 || est <= budget) return options[i].id
  }
  return VIDEO_OPTION_LINK // nothing fits the budget → link, don't store
}

// --- choices --------------------------------------------------------------

function imageCount(clip: ClipResult): number {
  return clip.media.filter((m) => m.kind === 'image' && m.url).length
}

// choicesFor turns a picked video row + image mode into the pipeline's
// choices. Shared by the defaults path and the dialog's answer.
export function choicesFor(
  options: CaptureDialogVideoOption[],
  videoOptionId: string,
  images: ImageChoice,
  title: string,
  includeInDeck: boolean,
): CaptureChoices {
  if (videoOptionId === VIDEO_OPTION_SKIP) {
    return { title, includeInDeck, video: 'skip', videoUrl: '', images }
  }
  const picked = options.find((o) => o.id === videoOptionId)
  if (videoOptionId === VIDEO_OPTION_LINK || !picked) {
    return { title, includeInDeck, video: 'link', videoUrl: '', images }
  }
  return { title, includeInDeck, video: 'store', videoUrl: picked.url, images }
}

// defaultChoices is what the vault's prefs imply with nothing asked — the
// silent path, and the dialog's pre-selection.
export function defaultChoices(
  clip: ClipResult,
  preview: CapturePreview | null,
  prefs: CapturePrefs,
  includeInDeck: boolean,
): CaptureChoices {
  const options = videoOptions(clip, preview)
  return choicesFor(options, defaultVideoOptionId(options, prefs), prefs.imageMode ?? 'all', '', includeInDeck)
}

// --- "is this consequential?" --------------------------------------------

// shouldAsk answers whether the dialog appears. The server's own answer
// leads (it owns the thresholds), but it's re-checked locally because the
// server frequently can't see the post at all — a 3.5 GB X video is
// invisible to a preview that got a 403, and that's exactly the case the
// user wants to be asked about.
export function shouldAsk(clip: ClipResult, preview: CapturePreview | null, prefs: CapturePrefs): boolean {
  const mode = prefs.askMode ?? 'triggers'
  if (mode === 'never') return false
  if (mode === 'always') return true
  if (preview?.shouldAsk) return true

  const t = prefs.triggers ?? {}
  const galleryOver = t.galleryOverCount ?? 0
  if (galleryOver > 0 && imageCount(clip) > galleryOver) return true

  const videoOver = t.videoOverMB ?? 0
  if (videoOver > 0 && videoBytesAtStake(clip, preview, prefs) > videoOver * MB) return true
  return false
}

// plannedVideoBytes is the estimated size of the rung the defaults would
// STORE; 0 when the defaults store nothing (link/skip) or nothing is known.
function plannedVideoBytes(options: CaptureDialogVideoOption[], prefs: CapturePrefs): number {
  const id = defaultVideoOptionId(options, prefs)
  return options.find((o) => o.id === id)?.estBytes ?? 0
}

// videoBytesAtStake is the size that makes this capture consequential. When
// a rung would be stored, that's its size; when the defaults would store
// NOTHING it's the biggest known rung — because "too big to store" is
// precisely the decision the user asked to be consulted about, and treating
// it as zero would hide the 3.5 GB video behind a silent link. Mirrors the
// server's shouldAskFor, which falls back to the item's own estimate the
// same way.
function videoBytesAtStake(clip: ClipResult, preview: CapturePreview | null, prefs: CapturePrefs): number {
  const options = videoOptions(clip, preview)
  if (options.length === 0) return 0
  const planned = plannedVideoBytes(options, prefs)
  if (planned > 0) return planned
  return Math.max(0, ...options.map((o) => o.estBytes ?? 0))
}

// --- the dialog payload ---------------------------------------------------

function byline(clip: ClipResult): string {
  return [clip.author, clip.handle].filter((s) => !!s).join(' · ')
}

function pinName(s: ClipperSettings): string {
  if (s.categoryID === PIN_WITH_DECK) return msg('options_category_deck')
  if (!s.categoryID) return msg('options_category_inbox')
  return s.categoryName || s.categoryID
}

// notesFor states what will ACTUALLY happen, in plain language. The
// extension's honest answer differs from the phone's: a platform that
// blocks BRUV's server doesn't block this browser, so a "blocked" preview
// means the sizes are unknown — not that the capture degrades.
function notesFor(
  clip: ClipResult,
  preview: CapturePreview | null,
  prefs: CapturePrefs,
  options: CaptureDialogVideoOption[],
): string[] {
  const notes: string[] = []
  if (!preview) {
    notes.push(msg('dialog_note_no_preview'))
  } else if (!preview.supported) {
    notes.push(msg('dialog_note_unsupported'))
  } else if (preview.blocked) {
    notes.push(msg('dialog_note_blocked').replace('{platform}', clip.platform))
  }
  if (clip.embedVideo) notes.push(msg('dialog_note_embed'))

  const planned = plannedVideoBytes(options, prefs)
  const videoOver = prefs.triggers?.videoOverMB ?? 0
  if (planned > 0 && videoOver > 0 && planned > videoOver * MB) {
    notes.push(msg('dialog_note_video_large').replace('{size}', formatBytes(planned)))
  } else if (planned === 0 && options.length > 0 && (prefs.videoMode ?? 'fit') === 'fit') {
    // 'fit' found nothing inside the budget. Say so with the size of the
    // CHEAPEST rung — that number is the reason — and leave the choice open:
    // the rung radios above are still live, and this browser can download a
    // 3.5 GB file if that's what the user wants.
    const smallest = options[0].estBytes ?? 0
    if (smallest > 0) {
      notes.push(
        msg('dialog_note_video_budget')
          .replace('{size}', formatBytes(smallest))
          .replace('{budget}', String(prefs.videoBudgetMB ?? 50)),
      )
    }
  }
  const images = imageCount(clip)
  const galleryOver = prefs.triggers?.galleryOverCount ?? 0
  if (galleryOver > 0 && images > galleryOver) {
    notes.push(msg('dialog_note_gallery').replace('{n}', String(images)))
  }
  if (options.length === 0 && clip.media.some((m) => m.kind === 'video') && !clip.embedVideo) {
    notes.push(msg('dialog_note_no_video'))
  }
  return notes
}

export function buildDialogRequest(
  clip: ClipResult,
  preview: CapturePreview | null,
  prefs: CapturePrefs,
  settings: ClipperSettings,
  includeInDeck: boolean,
): CaptureDialogRequest {
  const options = videoOptions(clip, preview)
  return {
    type: 'BRUV_OPTIONS',
    platform: clip.platform,
    byline: byline(clip),
    imageCount: imageCount(clip),
    videoOptions: options,
    notes: notesFor(clip, preview, prefs, options),
    // Destinations are STICKY settings, shown here read-only: they're set
    // once (deck in the popup, pin in Options) and reused for every clip.
    deckName: settings.deckTarget?.name ?? '',
    pinName: pinName(settings),
    canDeck: settings.deckTarget !== null,
    defaults: {
      // The server's title when it could actually read the post (the same
      // string the phone would show), else the pipeline's own — a blocked
      // preview reports the bare URL as its title.
      title: preview && preview.supported && !preview.blocked ? preview.title || cardTitle(clip) : cardTitle(clip),
      includeInDeck,
      videoOptionId: defaultVideoOptionId(options, prefs),
      images: prefs.imageMode ?? 'all',
    },
  }
}
