// Truth Social extractor — the ONLY file on the extension side that knows
// what Truth Social is. Truth Social's web client is a Soapbox-derived
// (Mastodon-flavoured) React frontend: unlike Twitter's `data-testid`
// contract, its DOM is closed-source, minified, and has changed class/attr
// naming across builds. So the DOM side here is deliberately best-effort —
// it just needs to find SOME text/media and, critically, the status id.
// Everything that matters for fidelity (author, text, media URLs,
// timestamp) is re-fetched from Truth Social's public Mastodon-compatible
// statuses API in enrich(), which is authoritative and overwrites whatever
// the DOM guessed.

import type { ClipperPlugin } from './registry'
import type { ClipMedia, ClipResult } from '../types'

// Truth Social permalinks look like /@<user>/posts/<id> (Mastodon status
// ids are numeric snowflakes). Pull the id out of either a permalink href
// or location.href when the page itself is the detail view.
function statusIdFromUrl(url: string): string {
  const m = url.match(/\/@[^/]+\/posts\/(\d+)/)
  return m ? m[1] : ''
}

// Strip a Mastodon-schema `content` HTML string down to plain text. This
// runs in the background service worker, which has no DOM/DOMParser
// sandboxed access to page content, so it's regex/string-replacement only.
// <br> and </p> become newlines (paragraph breaks matter for readability);
// everything else is stripped. Entity decoding covers the handful of
// entities Mastodon-style servers actually emit in status content.
function decodeEntities(str: string): string {
  return str
    .replace(/&nbsp;/g, ' ')
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&#(\d+);/g, (_, dec: string) => String.fromCharCode(Number(dec)))
}

function htmlContentToText(html: string): string {
  return decodeEntities(
    html
      .replace(/<br\s*\/?>/gi, '\n')
      .replace(/<\/p>/gi, '\n\n')
      .replace(/<[^>]+>/g, ''),
  ).trim()
}

// Shape of the public (unauthenticated) Mastodon-compatible statuses
// endpoint's response, trimmed to the fields we use. Unofficial in the
// sense that Truth Social doesn't document it, but it's the same public
// contract every Mastodon-fork frontend relies on to render a status — and
// it's isolated to this file, so a schema change is a one-file fix.
type MastodonStatus = {
  account?: {
    display_name?: string
    username?: string
    avatar?: string
  }
  content?: string
  created_at?: string
  url?: string
  media_attachments?: Array<{
    type?: 'image' | 'video' | 'gifv' | string
    url?: string
    preview_url?: string
  }>
}

export const truthsocialPlugin: ClipperPlugin = {
  id: 'truthsocial',

  matchesUrl(url: string): boolean {
    return /^https?:\/\/([a-z0-9-]+\.)?truthsocial\.com\//.test(url)
  },

  // Layered fallbacks because Truth Social's status wrapper has no stable,
  // documented contract the way Twitter's `article[data-testid="tweet"]`
  // does:
  resolveCaptureUnit(target: Element): Element | null {
    return (
      // Soapbox/Mastodon-derived UIs frequently mirror Twitter-style
      // testids for their status cards — try it first since it's the most
      // specific.
      target.closest('[data-testid="status"]') ??
      // Older/alternate Soapbox builds render a plain `.status` wrapper
      // class instead of a testid.
      target.closest('div.status') ??
      // Last resort: Mastodon's own status components stamp the status's
      // numeric id onto a `data-id` attribute on their wrapping element —
      // less semantically precise but still scoped to one post.
      target.closest('[data-id]')
    )
  },

  // Completion captures (a pending clip's URL opened in a transient tab)
  // have no click target. Only a post permalink (/@user/posts/<id>) has an
  // unambiguous primary status, and on one the focused post renders first;
  // the same fallback ladder as resolveCaptureUnit applies, since none of
  // these selectors is a documented contract.
  resolvePageUnit(doc: Document): Element | null {
    if (!statusIdFromUrl(doc.location?.href ?? '')) return null
    return (
      doc.querySelector('[data-testid="status"]') ??
      doc.querySelector('div.status') ??
      doc.querySelector('[data-id]')
    )
  },

  extract(unit: Element): ClipResult | null {
    // Permalink: prefer an in-unit anchor pointing at /@user/posts/<id>
    // (the only reliable per-post URL — a reply's own link, not the page's).
    // Fall back to location.href for detail-view pages where the post IS
    // the page and no such anchor exists inside the unit.
    const permalinkAnchor = Array.from(unit.querySelectorAll<HTMLAnchorElement>('a[href]')).find((a) =>
      /\/@[^/]+\/posts\/\d+/.test(a.getAttribute('href') ?? ''),
    )
    const permalink = permalinkAnchor?.getAttribute('href') ?? ''
    const canonicalUrl = permalink
      ? new URL(permalink, 'https://truthsocial.com').toString()
      : location.href
    const statusId = statusIdFromUrl(canonicalUrl) || statusIdFromUrl(location.href)

    // Author: the profile anchor is the most stable thing here — its href
    // is always /@<username>. Display name is whatever text sits alongside
    // it; if we can't find a dedicated name element, the handle itself is
    // an acceptable placeholder (enrich() will overwrite it).
    const handleAnchor = unit.querySelector<HTMLAnchorElement>('a[href^="/@"]')
    const handleFromHref = handleAnchor?.getAttribute('href')?.match(/^\/@([^/]+)/)?.[1]
    const handle = handleFromHref ? `@${handleFromHref}` : ''
    const author =
      unit.querySelector('[data-testid="account-name"]')?.textContent?.trim() ||
      unit.querySelector('.display-name__html')?.textContent?.trim() ||
      handleAnchor?.textContent?.trim() ||
      handle

    const avatarUrl =
      unit.querySelector<HTMLImageElement>('[data-testid="account-avatar"] img')?.src ??
      unit.querySelector<HTMLImageElement>('.account__avatar img')?.src ??
      undefined

    // Rendered content: try the likely testid/class first, and fall back to
    // scanning for the largest text block isn't worth it here — enrich()
    // is the fidelity backstop, so a miss just means an empty DOM guess.
    const contentEl =
      unit.querySelector('[data-testid="status-content"]') ?? unit.querySelector('.status__content')
    const text = contentEl?.textContent?.trim() ?? ''

    const media: ClipMedia[] = []
    const galleryEl = unit.querySelector('[data-testid="media-gallery"]') ?? unit.querySelector('.media-gallery')
    if (galleryEl) {
      for (const img of Array.from(galleryEl.querySelectorAll<HTMLImageElement>('img'))) {
        if (img.src) media.push({ url: img.src, kind: 'image' })
      }
    }

    // Video playback, like Twitter's, isn't reliably a fetchable URL from
    // the DOM (blob: URLs / signed CDN URLs that only the API resolves
    // cleanly) — record the poster and defer to enrich().
    const videoEl = unit.querySelector<HTMLVideoElement>('video')
    if (videoEl) {
      media.push({ url: '', kind: 'video', posterUrl: videoEl.poster || undefined })
    }

    if (!text && media.length === 0) return null

    const timeEl = unit.querySelector('time')

    return {
      platform: 'truthsocial',
      canonicalUrl,
      author,
      handle,
      avatarUrl,
      text,
      media,
      publishedAt: timeEl?.getAttribute('datetime') ?? undefined,
      extras: statusId ? { statusId } : {},
      // Always re-resolve via the API when we have an id — the DOM guess
      // above is best-effort even for plain text posts (unstable class
      // names mean author/text can easily be wrong or missing).
      needsEnrichment: Boolean(statusId),
    }
  },

  // Resolve authoritative status data via Truth Social's public statuses
  // API (Mastodon-compatible; no auth required for public posts). Degrades
  // to the DOM-extracted clip, with any unresolved video downgraded to its
  // poster image, on any failure — a clip must never die here.
  async enrich(clip: ClipResult): Promise<ClipResult> {
    const id = clip.extras.statusId
    if (!id) return { ...clip, needsEnrichment: false }
    try {
      const res = await fetch(`https://truthsocial.com/api/v1/statuses/${encodeURIComponent(id)}`)
      if (!res.ok) throw new Error(`statuses ${res.status}`)
      const data = (await res.json()) as MastodonStatus

      // API media, when the field is present, fully replaces the DOM's
      // best-effort media list (API is authoritative). An absent field
      // (schema drift) keeps whatever the DOM found rather than losing it.
      const apiMedia = data.media_attachments
        ?.map((m): ClipMedia | null => {
          if (!m.url) return null
          if (m.type === 'video' || m.type === 'gifv') {
            return { url: m.url, kind: 'video', posterUrl: m.preview_url }
          }
          if (m.type === 'image') {
            return { url: m.url, kind: 'image' }
          }
          return null
        })
        .filter((m): m is ClipMedia => m !== null)

      return {
        ...clip,
        author: data.account?.display_name || clip.author,
        handle: data.account?.username ? `@${data.account.username}` : clip.handle,
        avatarUrl: data.account?.avatar || clip.avatarUrl,
        text: data.content ? htmlContentToText(data.content) : clip.text,
        canonicalUrl: data.url || clip.canonicalUrl,
        publishedAt: data.created_at || clip.publishedAt,
        media: apiMedia ?? clip.media,
        needsEnrichment: false,
      }
    } catch {
      // API unreachable, non-200, or shape changed — never drop the clip.
      // Same degrade pattern as twitter.ts: an unresolved video (no url)
      // falls back to its poster as a plain image; anything still urlless
      // after that is dropped.
      return {
        ...clip,
        media: clip.media
          .map((m) => (m.kind === 'video' && !m.url && m.posterUrl ? { url: m.posterUrl, kind: 'image' as const } : m))
          .filter((m) => m.url),
        needsEnrichment: false,
      }
    }
  },
}
