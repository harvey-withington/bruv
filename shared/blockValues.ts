// Canonical narrowing for block values whose on-disk shape is richer
// than the thing being displayed.
//
// URL blocks store `{ url, caption? }` — that's what cardMarkdown
// exports, what mobile's UrlBlock edits, and what the clipper and
// server-side capture ingest write. Desktop's BlockItem used to treat
// the value as a bare string, so any URL block created anywhere else
// rendered as "[object Object]" (found 2026-08-01 on a captured link
// card), and desktop's own edits wrote a string that mobile then showed
// as empty. One helper, tolerant of both shapes, ends the disagreement.

export type UrlBlockValue = { url: string; caption?: string }

/**
 * Narrow a url block's value to `{ url, caption? }`.
 *
 * Accepts the canonical object AND a legacy bare string (what older
 * desktop edits wrote), so no migration is needed — cards heal into the
 * canonical shape the next time they're edited.
 */
export function asUrlValue(v: unknown): UrlBlockValue {
  if (typeof v === 'string') return { url: v }
  if (v && typeof v === 'object' && 'url' in v) {
    const obj = v as { url?: unknown; caption?: unknown }
    return {
      url: typeof obj.url === 'string' ? obj.url : '',
      caption: typeof obj.caption === 'string' ? obj.caption : undefined,
    }
  }
  return { url: '' }
}

/**
 * Build the canonical stored value, preserving an existing caption —
 * editing the URL must never silently drop one.
 */
export function urlBlockValue(url: string, previous?: unknown): UrlBlockValue {
  const caption = asUrlValue(previous).caption
  return caption ? { url, caption } : { url }
}
