// YouTube extractor — the ONLY file on the extension side that knows what
// YouTube is. The capture unit here is a VIDEO, not a "post": YouTube has no
// downloadable video URL (playback is ciphered MSE/DASH, not a plain mp4),
// so this plugin never attempts to resolve one. Instead it records an
// `embedVideo` reference and lets the platform's own official iframe player
// render it at Present time — and produces NO media at all (ruled
// 2026-07-31): the embed IS the content, a thumbnail in the media field
// broke cross-template value purity, and a failed embed should look like
// the error it is (YouTube's own "video unavailable" frame), not hide
// behind a stand-in image.

import type { ClipperPlugin } from './registry'
import type { ClipResult } from '../types'

// Pull a video id out of any of YouTube's three URL shapes:
//   https://www.youtube.com/watch?v=<id>
//   https://www.youtube.com/shorts/<id>
//   https://youtu.be/<id>
// `href` may be relative (anchor hrefs on listing pages are), hence `base`.
function videoIdFromHref(href: string, base: string): string {
  if (!href) return ''
  try {
    const u = new URL(href, base)
    const v = u.searchParams.get('v')
    if (v) return v
    const shortsMatch = u.pathname.match(/\/shorts\/([a-zA-Z0-9_-]+)/)
    if (shortsMatch) return shortsMatch[1]
    if (u.hostname === 'youtu.be') {
      const id = u.pathname.replace(/^\//, '')
      if (id) return id
    }
    return ''
  } catch {
    return ''
  }
}

export const youtubePlugin: ClipperPlugin = {
  id: 'youtube',

  matchesUrl(url: string): boolean {
    return /^https?:\/\/([a-z0-9-]+\.)?(youtube\.com|youtu\.be)\//.test(url)
  },

  // Two capture paths:
  //
  // 1. A right-click on/near a video thumbnail link anywhere (home, search,
  //    sidebar). `closest()` walks up from the click target to whichever of
  //    the anchor or its containing renderer element wraps it — a click on
  //    the title, avatar, or thumbnail image all resolve to the same video.
  //
  // 2. Anywhere else, but ONLY on a watch (or shorts) page: there's no
  //    smaller "video card" to bound an arbitrary click to, so the whole
  //    page IS the capture unit — extract() falls back to location.href and
  //    page-level metadata (title, channel, thumbnail) instead of DOM
  //    scoped to a unit.
  resolveCaptureUnit(target: Element, doc: Document): Element | null {
    const thumb = target.closest(
      'a[href*="/watch?v="], a[href^="/shorts/"], ytd-rich-item-renderer, ytd-video-renderer, ytd-compact-video-renderer',
    )
    if (thumb) return thumb

    const pageHref = doc.location?.href ?? ''
    if (videoIdFromHref(pageHref, pageHref)) return doc.body

    return null
  },

  // Completion captures (a pending clip's URL opened in a transient tab)
  // have no click target — which is exactly path 2 above: any URL carrying
  // a video id makes the whole page the capture unit.
  resolvePageUnit(doc: Document): Element | null {
    const pageHref = doc.location?.href ?? ''
    return videoIdFromHref(pageHref, pageHref) ? doc.body : null
  },

  extract(unit: Element, doc: Document): ClipResult | null {
    const isPageCapture = unit === doc.body

    let id = ''
    if (isPageCapture) {
      const href = doc.location?.href ?? ''
      id = videoIdFromHref(href, href)
    } else {
      // The unit itself may already be the matched anchor (id="video-title"
      // links commonly ARE the anchor `closest()` stopped on), or a renderer
      // wrapping one — check both.
      const anchor = unit.matches('a[href*="/watch?v="], a[href^="/shorts/"]')
        ? (unit as HTMLAnchorElement)
        : unit.querySelector<HTMLAnchorElement>('a[href*="/watch?v="], a[href^="/shorts/"]')
      id = videoIdFromHref(anchor?.getAttribute('href') ?? '', 'https://www.youtube.com')
    }
    if (!id) return null

    const canonicalUrl = `https://www.youtube.com/watch?v=${id}`

    // Field mapping: the slide schema is a social-post shape (author/handle/
    // avatarUrl/text), so a video is mapped onto it as text = video title,
    // author = channel name, handle = channel @handle, avatarUrl = channel
    // avatar.
    let text = ''
    let author = ''
    let handle = ''
    let avatarUrl: string | undefined
    let publishedAt: string | undefined

    if (isPageCapture) {
      // Watch-page metadata. YouTube's watch-page DOM churns constantly (A/B
      // tested layouts, class names that change release to release), so
      // every selector here falls back toward the stable floor:
      // og:title / document.title never disappear even when the primary
      // selector does.
      text =
        doc.querySelector('h1.ytd-watch-metadata yt-formatted-string')?.textContent?.trim() ||
        doc.querySelector('meta[property="og:title"]')?.getAttribute('content')?.trim() ||
        doc.title.replace(/\s*-\s*YouTube\s*$/, '').trim()

      const channelAnchor = doc.querySelector<HTMLAnchorElement>('#owner #channel-name a, ytd-channel-name a')
      author = channelAnchor?.textContent?.trim() ?? ''
      const handleMatch = (channelAnchor?.getAttribute('href') ?? '').match(/\/(@[a-zA-Z0-9_.-]+)/)
      handle = handleMatch ? handleMatch[1] : ''

      avatarUrl = doc.querySelector<HTMLImageElement>('#owner img')?.src || undefined

      publishedAt = doc.querySelector('meta[itemprop="datePublished"]')?.getAttribute('content') ?? undefined
    } else {
      // Thumbnail capture on a listing page (home, search, sidebar, etc.).
      // Only the title is reliably co-located with the clicked element.
      // Channel name/handle/avatar belong to whichever renderer instance we
      // happened to land in and are easy to misattribute — skip them rather
      // than guess, since showing the WRONG channel is worse than showing
      // none.
      const titleEl = unit.matches('#video-title') ? (unit as HTMLElement) : unit.querySelector<HTMLElement>('#video-title')
      text = titleEl?.getAttribute('title')?.trim() || titleEl?.textContent?.trim() || ''
    }

    return {
      platform: 'youtube',
      canonicalUrl,
      author,
      handle,
      avatarUrl,
      text,
      media: [],
      publishedAt,
      // No downloadable stream — the deck resolves this to "embed://youtube/<id>"
      // and renders YouTube's own iframe player at Present time.
      embedVideo: { provider: 'youtube', id },
      extras: { videoId: id },
      needsEnrichment: true,
    }
  },

  // One enrichment pass, failure-isolated (a clip must never die here):
  // oEmbed — youtube.com/oembed is public (no key, no auth) and returns
  // title / channel name / channel URL for any video id. It is
  // AUTHORITATIVE over the DOM extraction: thumbnail-link captures have no
  // channel metadata at all, and watch-page captures can be SPA-stale
  // (og:title / document.title lag client-side navigation). The channel
  // avatar is the one field oEmbed lacks — listing-page captures go
  // without it rather than guessing.
  async enrich(clip: ClipResult): Promise<ClipResult> {
    const id = clip.extras.videoId
    if (!id) return { ...clip, needsEnrichment: false }
    let next = { ...clip }

    try {
      const res = await fetch(
        `https://www.youtube.com/oembed?url=${encodeURIComponent(`https://www.youtube.com/watch?v=${id}`)}&format=json`,
      )
      if (res.ok) {
        const o = (await res.json()) as { title?: string; author_name?: string; author_url?: string }
        const handleMatch = (o.author_url ?? '').match(/\/(@[A-Za-z0-9_.-]+)/)
        next = {
          ...next,
          text: o.title || next.text,
          author: o.author_name || next.author,
          handle: handleMatch ? handleMatch[1] : next.handle,
        }
      }
    } catch {
      // oEmbed unreachable — keep whatever the DOM found.
    }

    return { ...next, needsEnrichment: false }
  },
}
