// YouTube extractor — the ONLY file on the extension side that knows what
// YouTube is. The capture unit here is a VIDEO, not a "post": YouTube has no
// downloadable video URL (playback is ciphered MSE/DASH, not a plain mp4),
// so this plugin never attempts to resolve one. Instead it records an
// `embedVideo` reference and lets the platform's own official iframe player
// render it at Present time — the thumbnail image is the only "media" this
// plugin ever produces.

import type { ClipperPlugin } from './registry'
import type { ClipMedia, ClipResult } from '../types'

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

    // Thumbnail: i.ytimg.com URLs are stable and unauthenticated (no signed
    // token, no rot), but maxresdefault (1280x720) 404s for older or
    // low-resolution uploads that never got a high-res thumbnail generated.
    // enrich() verifies it and downgrades to hqdefault when it's missing.
    const media: ClipMedia[] = [{ url: `https://i.ytimg.com/vi/${id}/maxresdefault.jpg`, kind: 'image' }]

    return {
      platform: 'youtube',
      canonicalUrl,
      author,
      handle,
      avatarUrl,
      text,
      media,
      publishedAt,
      // No downloadable stream — the deck resolves this to "embed://youtube/<id>"
      // and renders YouTube's own iframe player at Present time.
      embedVideo: { provider: 'youtube', id },
      extras: { videoId: id },
      needsEnrichment: true,
    }
  },

  // Two independent enrichment passes, each failure-isolated (a clip must
  // never die here):
  //  1. oEmbed — youtube.com/oembed is public (no key, no auth) and returns
  //     title / channel name / channel URL for any video id. It is
  //     AUTHORITATIVE over the DOM extraction: thumbnail captures have no
  //     channel metadata at all, and watch-page captures can be SPA-stale
  //     (og:title / document.title lag client-side navigation). The channel
  //     avatar is the one field oEmbed lacks — thumbnail captures go
  //     without it rather than guessing.
  //  2. Thumbnail check — verify maxresdefault exists; downgrade to
  //     hqdefault (360p, generated for every upload since the site's early
  //     days — effectively guaranteed) when it doesn't.
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

    const pending = next.media.find((m) => m.kind === 'image' && m.url.includes('/maxresdefault.jpg'))
    if (pending) {
      const hqUrl = `https://i.ytimg.com/vi/${id}/hqdefault.jpg`
      try {
        // HEAD is enough to check existence without pulling the full image
        // into the service worker just to throw it away. res.ok is treated
        // as sufficient, with a known caveat: YouTube doesn't always 404 a
        // missing maxres — for some videos it serves a tiny (~1KB, 120x90)
        // grey placeholder with a 200 status instead. We accept the rare
        // placeholder slipping through rather than add dimension checks,
        // since hqdefault is only ever a downgrade path.
        const res = await fetch(pending.url, { method: 'HEAD' })
        if (!res.ok) {
          next = { ...next, media: next.media.map((m) => (m === pending ? { ...m, url: hqUrl } : m)) }
        }
      } catch {
        next = { ...next, media: next.media.map((m) => (m === pending ? { ...m, url: hqUrl } : m)) }
      }
    }

    return { ...next, needsEnrichment: false }
  },
}
