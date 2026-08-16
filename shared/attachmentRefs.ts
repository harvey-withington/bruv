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
