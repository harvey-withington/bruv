// attachment:<cardID>/<attachmentID> — the durable reference the capture
// pipeline writes into media/image block values (core/supervisor/capture.go
// ingestClip). Attachments are the durable home for clipped media; CDN URLs
// rot in days. The PERSISTED value keeps the ref forever — every renderer
// resolves it to a short-lived signed URL at view time, the same rule the
// slide presenter applies server-side (core/supervisor/present.go).
//
// Renderers that receive block values must handle this form. History note
// (2026-08-16): the pipeline shipped 2026-08-02 but only the presenter and
// SlideEditorDialog resolved refs — captured videos/images rendered as
// broken links in the media/image blocks on both surfaces.

export const ATTACHMENT_REF_PREFIX = 'attachment:'

export type AttachmentRef = { cardID: string; attachmentID: string }

/** Parse an attachment ref; null for anything else (plain URLs, bare
 *  attachment IDs, empty values). */
export function parseAttachmentRef(v: string | null | undefined): AttachmentRef | null {
  if (!v || !v.startsWith(ATTACHMENT_REF_PREFIX)) return null
  const rest = v.slice(ATTACHMENT_REF_PREFIX.length)
  const slash = rest.indexOf('/')
  if (slash <= 0 || slash === rest.length - 1) return null
  return { cardID: rest.slice(0, slash), attachmentID: rest.slice(slash + 1) }
}

export function formatAttachmentRef(ref: AttachmentRef): string {
  return `${ATTACHMENT_REF_PREFIX}${ref.cardID}/${ref.attachmentID}`
}

// --- Re-homing refs when a card's attachments are copied ---------------
//
// Card JSON import/merge re-uploads a card's attachments, and the server
// mints NEW attachment ids on the new home card. Block values (image /
// media items, `{url}` shapes, gallery values joined with '\n') and slide
// decks (slide values, the slide's linked `cardId`) still name the SOURCE
// card's attachments; left alone they break once the source is deleted or
// the file is imported into another vault.

export type AttachmentRefRemap = {
  /** Source attachment id → the copy's attachment id. */
  attachments: ReadonlyMap<string, string>
  /** Source card id → new card id (the card part of a remapped ref, and a
   *  slide's linked `cardId`). */
  cards: ReadonlyMap<string, string>
  /** Card that now owns every remapped attachment. */
  targetCardID: string
}

function remapRefString(v: string, remap: AttachmentRefRemap): string {
  if (!v.includes(ATTACHMENT_REF_PREFIX)) return v
  // A multi-image slide value joins every item URL with '\n' — re-home
  // each line on its own.
  return v
    .split('\n')
    .map((line) => {
      const ref = parseAttachmentRef(line)
      if (!ref) return line
      const attachmentID = remap.attachments.get(ref.attachmentID)
      return attachmentID ? formatAttachmentRef({ cardID: remap.targetCardID, attachmentID }) : line
    })
    .join('\n')
}

/**
 * Deep-copies `value`, re-homing every attachment ref whose attachment was
 * copied, and every slide `cardId` naming a remapped card. Refs to
 * attachments that weren't copied (another card's, or a failed upload)
 * are left exactly as they were.
 */
export function remapAttachmentRefs<T>(value: T, remap: AttachmentRefRemap): T {
  return remapValue(value, remap) as T
}

function remapValue(v: unknown, remap: AttachmentRefRemap): unknown {
  if (typeof v === 'string') return remapRefString(v, remap)
  if (Array.isArray(v)) return v.map((it) => remapValue(it, remap))
  if (v && typeof v === 'object') {
    const out: Record<string, unknown> = {}
    for (const [k, inner] of Object.entries(v as Record<string, unknown>)) {
      out[k] = k === 'cardId' && typeof inner === 'string'
        ? (remap.cards.get(inner) ?? inner)
        : remapValue(inner, remap)
    }
    return out
  }
  return v
}

/** Every attachment ref found anywhere inside `value` (deep, including
 *  '\n'-joined gallery strings). */
export function collectAttachmentRefs(value: unknown): AttachmentRef[] {
  const out: AttachmentRef[] = []
  const walk = (v: unknown): void => {
    if (typeof v === 'string') {
      if (!v.includes(ATTACHMENT_REF_PREFIX)) return
      for (const line of v.split('\n')) {
        const ref = parseAttachmentRef(line)
        if (ref) out.push(ref)
      }
    } else if (Array.isArray(v)) {
      v.forEach(walk)
    } else if (v && typeof v === 'object') {
      Object.values(v as Record<string, unknown>).forEach(walk)
    }
  }
  walk(value)
  return out
}
