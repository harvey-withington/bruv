// The generic clip pipeline — 100% platform-blind. Consumes a ClipResult
// (whatever plugin produced it) and drives the BRUV API:
//
//   download media → create card → attachments → blocks → tags → pin →
//   (optionally) append a slide to the sticky deck target.
//
// Media is downloaded to base64 AT CAPTURE TIME (before queueing) so CDN
// URLs can't rot while a job waits offline — attachments are the durable
// home, never remote links.
//
// Every step after CreateCard records itself in a ClipProgress, so a job
// that fails midway (network drop, deck deleted) resumes on retry instead
// of creating a second card.

import {
  MAX_STORABLE_MEDIA_BYTES,
  PIN_WITH_DECK,
  type CaptureChoices,
  type ClipJob,
  type ClipMediaKind,
  type ClipProgress,
  type ClipResult,
  type ClipperSettings,
  type DeckTarget,
  type MediaFallback,
} from './types'
import { repoRPC } from './api'
import { ensureSocialPostType } from './socialPostType'

type Card = {
  id: string
  file_attachments?: Array<{ id: string; name: string }>
}

function newID(prefix: string): string {
  return `${prefix}-${crypto.randomUUID().slice(0, 8)}`
}

function extFromMime(mime: string, kind: ClipMediaKind): string {
  if (mime.includes('png')) return 'png'
  if (mime.includes('gif')) return 'gif'
  if (mime.includes('webp')) return 'webp'
  if (mime.includes('mp4')) return 'mp4'
  if (mime.includes('webm')) return 'webm'
  return kind === 'video' ? 'mp4' : 'jpg'
}

// Encodes in 3-byte-aligned chunks and joins the pieces, so there is no
// whole-file binary string alongside the result (half the peak memory of
// the old approach). Callers cap sizes at MAX_STORABLE_MEDIA_BYTES first.
async function toBase64(blob: Blob): Promise<string> {
  const buf = new Uint8Array(await blob.arrayBuffer())
  const CHUNK = 3 * 0x4000
  const parts: string[] = []
  for (let i = 0; i < buf.length; i += CHUNK) {
    parts.push(btoa(String.fromCharCode(...buf.subarray(i, i + CHUNK))))
  }
  return parts.join('')
}

class TooLargeError extends Error {}

// downloadMedia fetches one file for storage, refusing anything over the
// storable cap BEFORE buffering it when the server states its size.
async function downloadMedia(url: string): Promise<Blob> {
  const res = await fetch(url)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  const declared = Number(res.headers.get('content-length') ?? 0)
  if (declared > MAX_STORABLE_MEDIA_BYTES) {
    void res.body?.cancel()
    throw new TooLargeError()
  }
  const blob = await res.blob()
  if (blob.size > MAX_STORABLE_MEDIA_BYTES) throw new TooLargeError()
  return blob
}

export type BuiltJob = {
  job: ClipJob
  // Media the user asked to STORE that ended up as a platform link — the
  // caller tells the user, never silently.
  fallbacks: MediaFallback[]
}

// buildJob fetches the media the user asked for (plus the avatar) into the
// job so it is fully self-contained. Individual failures drop that item
// (or keep a video as its link), never the clip — and every such downgrade
// is returned in `fallbacks` for the caller to report.
//
// `choices` carries the user's capture-time decisions (or the vault
// defaults when the dialog wasn't shown). Absent = store everything, which
// is what the extension did before capture options existed — and what a
// job queued by an older build still expects. Downloading stays HERE, in
// the logged-in browser: that authenticated context is the extension's
// whole reason to exist, so "store the 3.5 GB rung" means store it.
export async function buildJob(
  clip: ClipResult,
  includeInDeck: boolean,
  choices?: CaptureChoices,
): Promise<BuiltJob> {
  const imageChoice = choices?.images ?? 'all'
  const videoChoice = choices?.video ?? 'store'
  const media: ClipJob['media'] = []
  const linkMedia: NonNullable<ClipJob['linkMedia']> = []
  const fallbacks: MediaFallback[] = []
  let n = 0
  let images = 0
  let videos = 0

  for (const m of clip.media) {
    const isVideo = m.kind === 'video'
    // A chosen rung replaces the plugin's own pick — but only for the FIRST
    // video, since a ladder describes one video and nothing we capture
    // serves two. Counters advance only for items that HAVE a URL, so an
    // unresolved entry can't make "first image only" skip a real one.
    const url = isVideo && videos === 0 && choices?.videoUrl ? choices.videoUrl : m.url
    if (!url) continue
    if (isVideo) videos++
    else images++

    if (isVideo) {
      if (videoChoice === 'skip') continue
      if (videoChoice === 'link') {
        linkMedia.push({ url, kind: 'video' })
        continue
      }
    } else {
      if (imageChoice === 'skip') continue
      if (imageChoice === 'first' && images > 1) continue
      if (imageChoice === 'link') {
        linkMedia.push({ url, kind: 'image' })
        continue
      }
    }

    try {
      const blob = await downloadMedia(url)
      n++
      media.push({
        name: `${clip.platform}-${n}.${extFromMime(blob.type, m.kind)}`,
        base64: await toBase64(blob),
        kind: m.kind,
      })
    } catch (err) {
      // A video that won't download (or is too big to store) is kept as a
      // platform link rather than vanishing (the same rule the server
      // applies) — the slide still plays, it just depends on the
      // platform's CDN. An unreachable image is dropped; text + link still
      // carry the clip. Either way the user is told.
      const reason = err instanceof TooLargeError || err instanceof RangeError ? 'too_large' : 'download_failed'
      fallbacks.push({ kind: m.kind, reason })
      if (isVideo) linkMedia.push({ url, kind: 'video' })
    }
  }

  let avatarBase64: string | undefined
  let avatarName: string | undefined
  if (clip.avatarUrl) {
    try {
      const blob = await downloadMedia(clip.avatarUrl)
      avatarBase64 = await toBase64(blob)
      avatarName = `${clip.platform}-avatar.${extFromMime(blob.type, 'image')}`
    } catch { /* avatar is decoration — never blocks a clip */ }
  }

  const job: ClipJob = {
    id: newID('job'),
    createdAt: new Date().toISOString(),
    clip,
    includeInDeck,
    media,
    linkMedia: linkMedia.length > 0 ? linkMedia : undefined,
    title: choices?.title?.trim() || undefined,
    avatarBase64,
    avatarName,
  }
  return { job, fallbacks }
}

// jobTitle is the title the job's card gets: the user's own, else derived.
export function jobTitle(job: ClipJob): string {
  return job.title?.trim() || cardTitle(job.clip)
}

// cardTitle is the derived title — the capture dialog pre-fills its title
// field with it (and the user's edit wins, via ClipJob.title).
export function cardTitle(clip: ClipResult): string {
  const who = clip.handle || clip.author || clip.platform
  const text = clip.text.replace(/\s+/g, ' ').trim()
  return text ? `${who}: ${text.slice(0, 60)}${text.length > 60 ? '…' : ''}` : who
}

function displayDate(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return isNaN(d.getTime()) ? '' : d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

// addAttachment uploads one file and returns its attachment ID by diffing
// the card's attachment list (names may be de-duplicated server-side, so
// name-matching alone isn't reliable).
async function addAttachment(
  s: ClipperSettings,
  cardID: string,
  known: Set<string>,
  name: string,
  base64: string,
): Promise<string | null> {
  const card = await repoRPC<Card>(s, 'AddCardAttachment', [cardID, name, base64])
  for (const att of card.file_attachments ?? []) {
    if (!known.has(att.id)) {
      known.add(att.id)
      return att.id
    }
  }
  return null
}

type Upload = { name: string; base64: string }

// uploadAll stores every file not yet recorded in `progress.attachments`,
// persisting after each one. On a RESUMED job it first adopts attachments
// already on the card under the same name — an upload whose response was
// lost to a network drop landed server-side, and must not land twice.
async function uploadAll(
  s: ClipperSettings,
  cardID: string,
  uploads: Upload[],
  progress: ClipProgress,
  resumed: boolean,
  save: () => Promise<void>,
): Promise<Record<string, string>> {
  const done = (progress.attachments ??= {})
  const pending = uploads.filter((u) => !(u.name in done))
  const known = new Set(Object.values(done).filter(Boolean))
  let adoptable: Array<{ id: string; name: string }> = []
  if (resumed && pending.length > 0) {
    const card = await repoRPC<Card>(s, 'GetCard', [cardID])
    adoptable = (card.file_attachments ?? []).filter((a) => !known.has(a.id))
    for (const a of card.file_attachments ?? []) known.add(a.id)
  }
  for (const u of pending) {
    const i = adoptable.findIndex((a) => a.name === u.name)
    if (i >= 0) {
      done[u.name] = adoptable.splice(i, 1)[0].id
    } else {
      done[u.name] = (await addAttachment(s, cardID, known, u.name, u.base64)) ?? ''
    }
    await save()
  }
  return done
}

export type ClipOutcome = { cardID: string; slideAppended: boolean; pinFailed: boolean }

// Persists a job's progress after each completed step. The live path has
// nothing to persist to (its progress rides into the queue on failure).
export type ProgressSink = (progress: ClipProgress) => Promise<void>

// executeJob runs the pipeline for one job, resuming from `progress` (which
// it updates in place). Throws on failure — `progress.cardID` then tells
// the caller whether the card already exists (a retry finishes it, never
// re-creates it).
export async function executeJob(
  s: ClipperSettings,
  job: ClipJob,
  progress: ClipProgress = {},
  onProgress?: ProgressSink,
): Promise<ClipOutcome> {
  const clip = job.clip
  const save = async (): Promise<void> => {
    if (onProgress) await onProgress(progress)
  }

  // The card is the FULL structured record: every captured field lands as a
  // typed block (Social Post card type), and the slide BINDS to those blocks
  // rather than baking values in. "Add to BRUV" alone loses nothing, and a
  // card-only clip can be linked into a slide later via the Slide Editor.
  const resumed = !!progress.cardID
  if (!progress.cardID) {
    const typeID = await ensureSocialPostType(s)
    const card = await repoRPC<Card>(s, 'CreateCard', [typeID, jobTitle(job)])
    progress.cardID = card.id
    await save()
  }
  const cardID = progress.cardID

  // Attachments first — the blocks reference them.
  const uploads: Upload[] = job.media.map((m) => ({ name: m.name, base64: m.base64 }))
  if (job.avatarBase64 && job.avatarName) uploads.push({ name: job.avatarName, base64: job.avatarBase64 })
  const attachments = progress.bindings ? {} : await uploadAll(s, cardID, uploads, progress, resumed, save)

  if (!progress.bindings) {
    progress.bindings = await writeBlocks(s, cardID, job, attachments)
    await save()
  }
  if (!progress.tagged) {
    await repoRPC(s, 'UpdateCardTags', [cardID, [clip.platform]])
    progress.tagged = true
    await save()
  }
  if (!progress.pin) {
    progress.pin = { failed: await pinCard(s, cardID) }
    await save()
  }

  if (job.includeInDeck && s.deckTarget && !progress.slideAppended) {
    await appendSlide(s, s.deckTarget, cardID, clip, progress.bindings)
    progress.slideAppended = true
    await save()
  }

  return { cardID, slideAppended: !!progress.slideAppended, pinFailed: progress.pin.failed }
}

// writeBlocks writes the card's blocks from the uploaded attachments and
// returns the slide bindings (schema key → block id). EVERY image ref is
// kept: galleries become a multi-item media block (rendered as a carousel
// on slides), not first-image-wins.
async function writeBlocks(
  s: ClipperSettings,
  cardID: string,
  job: ClipJob,
  attachments: Record<string, string>,
): Promise<Record<string, string>> {
  const clip = job.clip
  const refFor = (name: string): string => (attachments[name] ? `attachment:${cardID}/${attachments[name]}` : '')
  const imageRefs: string[] = []
  let firstVideoRef = ''
  for (const m of job.media) {
    const ref = refFor(m.name)
    if (!ref) continue
    if (m.kind === 'video' && !firstVideoRef) firstVideoRef = ref
    if (m.kind === 'image') imageRefs.push(ref)
  }
  // Media the user chose to keep as a link (or a video that wouldn't
  // download) goes into the block as its URL. A choice applies to a whole
  // kind at once, so appending after the attachment refs can never
  // interleave a gallery out of order.
  for (const lm of job.linkMedia ?? []) {
    if (lm.kind === 'video') {
      if (!firstVideoRef) firstVideoRef = lm.url
    } else {
      imageRefs.push(lm.url)
    }
  }

  const avatarRef = job.avatarName ? refFor(job.avatarName) : ''

  // One block per captured field, keyed with the schema keys (matching the
  // Social Post template, so type-refresh recognises them). Replaces any
  // template-applied blocks wholesale — ours carry the data.
  const blocks: Array<Record<string, unknown>> = []
  const bindings: Record<string, string> = {}
  const addBlock = (key: string, type: string, label: string, value: unknown): void => {
    const id = newID('blk')
    blocks.push({ id, type, label, key, value })
    bindings[key] = id
  }
  if (clip.author) addBlock('author', 'text', 'Author', clip.author)
  if (clip.handle) addBlock('handle', 'text', 'Handle', clip.handle)
  if (avatarRef) addBlock('avatar', 'image', 'Avatar', { url: avatarRef })
  if (clip.text) addBlock('text', 'text', 'Text', clip.text)
  if (imageRefs.length > 1) {
    // Gallery: one multi-item media block. Slide binding resolution joins
    // every item URL (newline-delimited); renderers show a carousel.
    addBlock('media', 'media', 'Media', imageRefs.map((r) => ({ id: newID('m'), url: r })))
  } else if (imageRefs.length === 1) {
    addBlock('media', 'image', 'Media', { url: imageRefs[0] })
  }
  if (firstVideoRef) addBlock('video', 'media', 'Video', [{ id: newID('m'), url: firstVideoRef, mime: 'video/mp4' }])
  const dateText = displayDate(clip.publishedAt)
  if (dateText) addBlock('date', 'text', 'Date', dateText)
  addBlock('url', 'url', 'Source', { url: clip.canonicalUrl })

  await repoRPC(s, 'UpdateCardBlocks', [cardID, blocks])
  return bindings
}

// pinCard applies the pin destination and returns whether a REQUESTED pin
// failed to land. Pinning is best-effort: a failed pin leaves the card in
// the Inbox (visible, recoverable) — never worth failing or requeueing a
// clip whose content already landed. But a bounced pin is REPORTED via the
// outcome (accepted-types gate, a category from another pairing/vault, an
// unpinned deck mirror): the user chose a destination, and silence here
// shipped the 2026-07-31 everything-lands-in-Inbox bug.
async function pinCard(s: ClipperSettings, cardID: string): Promise<boolean> {
  let pinRequested = false
  let pinLanded = false
  if (s.categoryID === PIN_WITH_DECK) {
    if (s.deckTarget) {
      pinRequested = true
      try {
        const pins = (await repoRPC<Array<{ category_id: string }>>(s, 'GetCardPins', [s.deckTarget.cardID])) ?? []
        for (const p of pins) {
          if (!p.category_id) continue
          try {
            await repoRPC(s, 'PinCard', [cardID, p.category_id])
            pinLanded = true
          } catch (err) {
            console.warn('pin to deck location failed:', err)
          }
        }
      } catch (err) {
        console.warn('resolve deck pins failed:', err)
      }
    }
  } else if (s.categoryID) {
    pinRequested = true
    try {
      await repoRPC(s, 'PinCard', [cardID, s.categoryID])
      pinLanded = true
    } catch (err) {
      console.warn('pin failed:', err)
    }
  }
  return pinRequested && !pinLanded
}

// appendSlide adds the clip's slide to the deck target. Every captured
// field binds LIVE to the card's blocks — the card is the source of truth
// and edits propagate to the slide. `platform` is the only literal (it has
// no block; it's routing data, not content). No `title`: the deck row label
// follows the linked card's live title; stamping the clip-time title here
// would freeze it against renames. templateId 'auto': BRUV resolves the
// template from the capture URL (bound `url` field) at render time — so
// platforms without a dedicated template render on the generic fallback
// and upgrade retroactively the moment a matching template ships.
async function appendSlide(
  s: ClipperSettings,
  deck: DeckTarget,
  cardID: string,
  clip: ClipResult,
  bindings: Record<string, string>,
): Promise<void> {
  const values: Record<string, string> = { platform: clip.platform }
  if (clip.embedVideo) values.video = `embed://${clip.embedVideo.provider}/${clip.embedVideo.id}`
  const slide: Record<string, unknown> = {
    contentTypeId: 'post',
    templateId: 'auto',
    cardId: cardID,
    values,
    bindings,
  }
  await repoRPC(s, 'AppendDeckSlide', [deck.cardID, deck.blockID, slide])
}
