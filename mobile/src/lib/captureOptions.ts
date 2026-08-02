// The user's capture-time choices — the data half of the Capture Options
// dialog (design: plan/2026-08-02 capture options at capture time.md).
//
// Harvey's ruling that produced this file: every capture defect of that
// week was BRUV deciding silently — which video rung, what "too large"
// means, whether a failed video becomes a thumbnail. The decision is the
// user's now, pre-populated from their defaults but never assumed.
//
// The rules live here rather than in the sheet component because the two
// things worth reading on their own are (a) what actually gets sent to
// the server and (b) what "link only" really means — and both should be
// testable without a DOM.

import { t } from './i18n.svelte'
import { captureOptsFrom, type CapturePrefs as ClipTargetPrefs } from './capturePrefs'
import type {
  CaptureMediaPreview,
  CaptureOpts,
  CapturePreview,
  ImageMode,
} from '@shared/types'

/** What to do with the post's video. A variant with an empty id means
 *  "store it, at whatever rung the resolver picked" — the case where the
 *  platform gave us no quality ladder to choose from. */
export type VideoChoice = { kind: 'variant'; id: string } | { kind: 'link' } | { kind: 'skip' }

export type CaptureChoices = {
  title: string
  /** null when the post has no video at all. */
  video: VideoChoice | null
  /** null when the post has no images at all. */
  imageMode: ImageMode | null
}

export function videoItem(preview: CapturePreview): CaptureMediaPreview | null {
  return preview.media?.find((m) => m.kind === 'video') ?? null
}

export function imageItems(preview: CapturePreview): CaptureMediaPreview[] {
  return preview.media?.filter((m) => m.kind === 'image') ?? []
}

/** The vault's defaults applied to THIS post — what the sheet opens on,
 *  and what a no-ask capture uses. One tap accepts it unchanged. */
export function defaultChoices(preview: CapturePreview): CaptureChoices {
  const video = videoItem(preview)
  const images = imageItems(preview)
  return {
    title: preview.title ?? '',
    video: video ? defaultVideoChoice(video, preview) : null,
    imageMode: images.length ? (preview.prefs?.imageMode ?? 'all') : null,
  }
}

function defaultVideoChoice(video: CaptureMediaPreview, preview: CapturePreview): VideoChoice {
  const mode = preview.prefs?.videoMode
  if (mode === 'skip') return { kind: 'skip' }
  if (mode === 'link') return { kind: 'link' }
  if (video.variants?.length) {
    // An empty defaultVariantId is the server saying "nothing on this
    // ladder fits your budget" — that means link-only, NOT "store the
    // biggest one anyway".
    return video.defaultVariantId
      ? { kind: 'variant', id: video.defaultVariantId }
      : { kind: 'link' }
  }
  return { kind: 'variant', id: '' }
}

/**
 * Fold the user's choices into the server's CaptureOpts envelope, on top
 * of the sticky deck/pin targets.
 *
 * Only what the user actually decided goes out: the title travels only
 * when they changed it, and the media modes only for media the post
 * actually has. Everything omitted falls back to the vault's prefs
 * server-side, so this stays honest about what was chosen.
 */
export function captureOptsWith(
  prefs: ClipTargetPrefs,
  choices: CaptureChoices,
  preview: CapturePreview,
): CaptureOpts {
  const opts: CaptureOpts = captureOptsFrom(prefs)
  const title = choices.title.trim()
  if (title && title !== (preview.title ?? '').trim()) opts.title = title

  if (choices.video) {
    if (choices.video.kind === 'variant') {
      // A rung the user picked must not be quietly overridden by a vault
      // default of link/skip, so say "store it" explicitly.
      opts.videoMode = 'fit'
      if (choices.video.id) opts.videoVariantId = choices.video.id
    } else {
      opts.videoMode = choices.video.kind
    }
  }
  if (choices.imageMode) opts.imageMode = choices.imageMode
  return opts
}

/** Sizes the way a person reads them: MB up to a gigabyte, then GB.
 *  Returns '' when the size is unknown — a blank is honest, "0 MB" isn't. */
export function formatEstBytes(bytes?: number): string {
  if (!bytes || bytes <= 0) return ''
  const mb = bytes / (1024 * 1024)
  if (mb < 1024) return t('capture.size_mb', { size: mb < 10 ? mb.toFixed(1) : Math.round(mb) })
  return t('capture.size_gb', { size: (mb / 1024).toFixed(1) })
}

// Trigger id → the line explaining why the dialog appeared. Unknown ids
// (a newer server) are dropped rather than rendered raw.
const REASON_KEYS: Record<string, string> = {
  video_large: 'capture.why_video_large',
  gallery_large: 'capture.why_gallery_large',
  unsupported: 'capture.why_unsupported',
  blocked: 'capture.why_blocked',
  pin_may_reject: 'capture.why_pin',
}

/** "Why you're seeing this", from the triggers the vault actually fired. */
export function askReasonText(preview: CapturePreview): string {
  const keys = (preview.askReasons ?? [])
    .map((r) => REASON_KEYS[r])
    .filter((k): k is string => !!k)
  const unique = [...new Set(keys)]
  if (!unique.length) return t('capture.why_default')
  return unique.map((k) => t(k)).join(' ')
}
