// The generic clip pipeline — 100% platform-blind. Consumes a ClipResult
// (whatever plugin produced it) and drives the BRUV API:
//
//   download media → create card → blocks → tags → attachments → pin →
//   (optionally) append a slide to the sticky deck target.
//
// Media is downloaded to base64 AT CAPTURE TIME (before queueing) so CDN
// URLs can't rot while a job waits offline — attachments are the durable
// home, never remote links.

import { PIN_WITH_DECK, type ClipJob, type ClipMediaKind, type ClipResult, type ClipperSettings } from './types'
import { repoRPC } from './api'

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

async function toBase64(blob: Blob): Promise<string> {
  const buf = new Uint8Array(await blob.arrayBuffer())
  let bin = ''
  const CHUNK = 0x8000
  for (let i = 0; i < buf.length; i += CHUNK) {
    bin += String.fromCharCode(...buf.subarray(i, i + CHUNK))
  }
  return btoa(bin)
}

// downloadMedia fetches every media item + the avatar into the job so it is
// fully self-contained. Individual failures drop that item, never the clip.
export async function buildJob(clip: ClipResult, includeInDeck: boolean): Promise<ClipJob> {
  const media: ClipJob['media'] = []
  let n = 0
  for (const m of clip.media) {
    if (!m.url) continue
    try {
      const res = await fetch(m.url)
      if (!res.ok) continue
      const blob = await res.blob()
      n++
      media.push({
        name: `${clip.platform}-${n}.${extFromMime(blob.type, m.kind)}`,
        base64: await toBase64(blob),
        kind: m.kind,
      })
    } catch {
      // Unreachable media item — skip it; text + link still carry the clip.
    }
  }

  let avatarBase64: string | undefined
  let avatarName: string | undefined
  if (clip.avatarUrl) {
    try {
      const res = await fetch(clip.avatarUrl)
      if (res.ok) {
        const blob = await res.blob()
        avatarBase64 = await toBase64(blob)
        avatarName = `${clip.platform}-avatar.${extFromMime(blob.type, 'image')}`
      }
    } catch { /* avatar is decoration — never blocks a clip */ }
  }

  return {
    id: newID('job'),
    createdAt: new Date().toISOString(),
    clip,
    includeInDeck,
    media,
    avatarBase64,
    avatarName,
    attempts: 0,
  }
}

function cardTitle(clip: ClipResult): string {
  const who = clip.handle || clip.author || clip.platform
  const text = clip.text.replace(/\s+/g, ' ').trim()
  return text ? `${who}: ${text.slice(0, 60)}${text.length > 60 ? '…' : ''}` : who
}

// --- "Social Post" card type -------------------------------------------
// Clipped cards are TYPED: one generic Social Post card type (mirroring the
// generic `post` slide content type — platforms differentiate by template,
// never by schema), provisioned in-context on first clip per repo. This is
// the Create-Type-from-Card adoption lesson: the type exists because real
// data needed it. NOTE: these template blocks are the 4th mirror of the
// post schema (with shared/slideContentTypes.ts + the two Go maps) — keep
// all four in sync.
const SOCIAL_POST_TYPE_LABEL = 'Social Post'

const SOCIAL_POST_TEMPLATE_BLOCKS = [
  { id: 'tpl-author', type: 'text', label: 'Author', key: 'author', value: '' },
  { id: 'tpl-handle', type: 'text', label: 'Handle', key: 'handle', value: '' },
  { id: 'tpl-avatar', type: 'image', label: 'Avatar', key: 'avatar', value: { url: '' } },
  { id: 'tpl-text', type: 'text', label: 'Text', key: 'text', value: '' },
  { id: 'tpl-media', type: 'image', label: 'Media', key: 'media', value: { url: '' } },
  { id: 'tpl-video', type: 'media', label: 'Video', key: 'video', value: [] },
  { id: 'tpl-date', type: 'text', label: 'Date', key: 'date', value: '' },
  { id: 'tpl-url', type: 'url', label: 'Source', key: 'url', value: { url: '' } },
]

type CardTypeInfo = { id: string; label: string; builtin?: boolean }

// ensureSocialPostType returns the type ID, creating template + type on
// first use. Failure degrades to an untyped card — never blocks a clip.
async function ensureSocialPostType(s: ClipperSettings): Promise<string> {
  try {
    const types = (await repoRPC<CardTypeInfo[]>(s, 'ListCardTypes', [])) ?? []
    const existing = types.find((t) => t.label === SOCIAL_POST_TYPE_LABEL)
    if (existing) return existing.id
    const template = await repoRPC<{ id: string }>(s, 'CreateCardTemplate', [
      SOCIAL_POST_TYPE_LABEL,
      SOCIAL_POST_TEMPLATE_BLOCKS,
    ])
    const created = await repoRPC<{ id: string }>(s, 'CreateUserCardType', [
      SOCIAL_POST_TYPE_LABEL,
      '#1d9bf0',
      'A captured social post (web clipper)',
      '',
      template.id,
    ])
    return created.id
  } catch (err) {
    console.warn('ensure Social Post card type failed (clipping untyped):', err)
    return ''
  }
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

export type ClipOutcome = { cardID: string; slideAppended: boolean }

// executeJob runs the whole pipeline for one job. Throws on failure — the
// caller decides whether it's queueable (network) or terminal (business).
export async function executeJob(s: ClipperSettings, job: ClipJob): Promise<ClipOutcome> {
  const clip = job.clip

  // The card is the FULL structured record: every captured field lands as a
  // typed block (Social Post card type), and the slide BINDS to those blocks
  // rather than baking values in. "Add to BRUV" alone loses nothing, and a
  // card-only clip can be linked into a slide later via the Slide Editor.
  const typeID = await ensureSocialPostType(s)
  const card = await repoRPC<Card>(s, 'CreateCard', [typeID, cardTitle(clip)])
  const cardID = card.id

  // Attachments first — the blocks reference them. EVERY image ref is
  // kept: galleries become a multi-item media block (rendered as a
  // carousel on slides), not first-image-wins.
  const known = new Set<string>()
  const imageRefs: string[] = []
  let firstVideoRef = ''
  for (const m of job.media) {
    const attID = await addAttachment(s, cardID, known, m.name, m.base64)
    if (!attID) continue
    const ref = `attachment:${cardID}/${attID}`
    if (m.kind === 'video' && !firstVideoRef) firstVideoRef = ref
    if (m.kind === 'image') imageRefs.push(ref)
  }
  let avatarRef = ''
  if (job.avatarBase64 && job.avatarName) {
    const attID = await addAttachment(s, cardID, known, job.avatarName, job.avatarBase64)
    if (attID) avatarRef = `attachment:${cardID}/${attID}`
  }

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
  await repoRPC(s, 'UpdateCardTags', [cardID, [clip.platform]])

  // Pinning is best-effort: a failed pin leaves the card in the Inbox
  // (visible, recoverable) — never worth failing or requeueing a clip whose
  // content already landed.
  if (s.categoryID === PIN_WITH_DECK) {
    if (s.deckTarget) {
      try {
        const pins = (await repoRPC<Array<{ category_id: string }>>(s, 'GetCardPins', [s.deckTarget.cardID])) ?? []
        for (const p of pins) {
          if (!p.category_id) continue
          try {
            await repoRPC(s, 'PinCard', [cardID, p.category_id])
          } catch (err) {
            console.warn('pin to deck location failed:', err)
          }
        }
      } catch (err) {
        console.warn('resolve deck pins failed:', err)
      }
    }
  } else if (s.categoryID) {
    try {
      await repoRPC(s, 'PinCard', [cardID, s.categoryID])
    } catch (err) {
      console.warn('pin failed:', err)
    }
  }

  let slideAppended = false
  if (job.includeInDeck && s.deckTarget) {
    // Every captured field binds LIVE to the card's blocks — the card is
    // the source of truth and edits propagate to the slide. `platform` is
    // the only literal (it has no block; it's routing data, not content).
    // No `title`: the deck row label follows the linked card's live title;
    // stamping the clip-time title here would freeze it against renames.
    // templateId 'auto': BRUV resolves the template from the capture URL
    // (bound `url` field) at render time — so platforms without a dedicated
    // template render on the generic fallback and upgrade retroactively the
    // moment a matching template ships.
    const values: Record<string, string> = { platform: clip.platform }
    if (clip.embedVideo) values.video = `embed://${clip.embedVideo.provider}/${clip.embedVideo.id}`
    const slide: Record<string, unknown> = {
      contentTypeId: 'post',
      templateId: 'auto',
      cardId: cardID,
      values,
      bindings,
    }
    await repoRPC(s, 'AppendDeckSlide', [s.deckTarget.cardID, s.deckTarget.blockID, slide])
    slideAppended = true
  }

  return { cardID, slideAppended }
}
