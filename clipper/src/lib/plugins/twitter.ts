// Twitter/X extractor — the ONLY file on the extension side that knows what
// Twitter is. DOM side reads the tweet <article>; background side resolves
// video through X's syndication API (the react-tweet approach), because the
// DOM only exposes blob: URLs for video.

import type { ClipperPlugin } from './registry'
import type { ClipMedia, ClipResult } from '../types'

function statusIdFromUrl(url: string): string {
  const m = url.match(/\/status\/(\d+)/)
  return m ? m[1] : ''
}

// Token derivation used by X's own embed endpoint (public knowledge via
// react-tweet). Unofficial; isolated here so a breakage is a one-file fix.
function syndicationToken(id: string): string {
  return ((Number(id) / 1e15) * Math.PI).toString(36).replace(/(0+|\.)/g, '')
}

export const twitterPlugin: ClipperPlugin = {
  id: 'twitter',

  matchesUrl(url: string): boolean {
    return /^https?:\/\/(mobile\.)?(twitter|x)\.com\//.test(url)
  },

  // Every tweet — timeline item, detail page, or reply — is rendered as an
  // <article data-testid="tweet">. The capture unit is the article that
  // contains whatever the user right-clicked.
  resolveCaptureUnit(target: Element): Element | null {
    return target.closest('article[data-testid="tweet"]')
  },

  extract(unit: Element): ClipResult | null {
    // Canonical permalink: the <time> element sits inside an anchor that IS
    // the tweet's own /status/ link — the only reliable per-tweet URL on
    // timeline and thread views.
    const timeEl = unit.querySelector('time')
    const permalink = timeEl?.closest('a')?.getAttribute('href') ?? ''
    const canonicalUrl = permalink ? new URL(permalink, 'https://x.com').toString() : ''
    const statusId = statusIdFromUrl(canonicalUrl)

    // Author block: display name + @handle live under User-Name; the handle
    // is the span whose text starts with "@".
    const userName = unit.querySelector('[data-testid="User-Name"]')
    const spans = Array.from(userName?.querySelectorAll('span') ?? [])
    const handle = spans.map((s) => s.textContent?.trim() ?? '').find((t) => t.startsWith('@')) ?? ''
    const author =
      spans.map((s) => s.textContent?.trim() ?? '').find((t) => t && !t.startsWith('@') && t !== '·') ?? handle

    const avatarUrl =
      unit.querySelector<HTMLImageElement>('[data-testid^="UserAvatar-Container"] img')?.src ?? undefined

    const text = unit.querySelector('[data-testid="tweetText"]')?.textContent?.trim() ?? ''

    // Images: photo media only (card previews and avatars live elsewhere in
    // the article). Upgrade to the large variant for slide use.
    const media: ClipMedia[] = []
    for (const img of Array.from(unit.querySelectorAll<HTMLImageElement>('[data-testid="tweetPhoto"] img'))) {
      if (!img.src.includes('pbs.twimg.com/media')) continue
      const u = new URL(img.src)
      u.searchParams.set('name', 'large')
      media.push({ url: u.toString(), kind: 'image' })
    }

    // Video: only the poster is reachable from the DOM (playback is a blob:
    // URL). Record the poster and flag for background enrichment via the
    // syndication API.
    let needsEnrichment = false
    const videoEl = unit.querySelector<HTMLVideoElement>('[data-testid="videoPlayer"] video, video')
    if (videoEl) {
      needsEnrichment = true
      media.push({ url: '', kind: 'video', posterUrl: videoEl.poster || undefined })
    }

    if (!text && media.length === 0) return null

    return {
      platform: 'twitter',
      canonicalUrl: canonicalUrl || location.href,
      author,
      handle,
      avatarUrl,
      text,
      media,
      publishedAt: timeEl?.getAttribute('datetime') ?? undefined,
      extras: statusId ? { statusId } : {},
      needsEnrichment,
    }
  },

  // Resolve video mp4 URLs via cdn.syndication.twimg.com. Degrades to the
  // poster image on any failure — a clip must never die here.
  async enrich(clip: ClipResult): Promise<ClipResult> {
    const id = clip.extras.statusId
    const pendingVideo = clip.media.find((m) => m.kind === 'video' && !m.url)
    if (!id || !pendingVideo) return { ...clip, needsEnrichment: false }
    try {
      const res = await fetch(
        `https://cdn.syndication.twimg.com/tweet-result?id=${encodeURIComponent(id)}&token=${syndicationToken(id)}`,
      )
      if (!res.ok) throw new Error(`syndication ${res.status}`)
      const data = (await res.json()) as {
        mediaDetails?: Array<{
          type?: string
          video_info?: { variants?: Array<{ content_type?: string; bitrate?: number; url?: string }> }
        }>
      }
      const variants =
        data.mediaDetails
          ?.find((m) => m.type === 'video' || m.type === 'animated_gif')
          ?.video_info?.variants?.filter((v) => v.content_type === 'video/mp4' && v.url) ?? []
      // Highest-bitrate mp4 wins.
      variants.sort((a, b) => (b.bitrate ?? 0) - (a.bitrate ?? 0))
      const best = variants[0]?.url
      const media = clip.media.map((m) =>
        m === pendingVideo && best ? { ...m, url: best } : m,
      )
      // No mp4 found → degrade the video entry to its poster image.
      return {
        ...clip,
        media: best
          ? media
          : clip.media.map((m) =>
              m === pendingVideo && m.posterUrl ? { url: m.posterUrl, kind: 'image' as const } : m,
            ).filter((m) => m.url),
        needsEnrichment: false,
      }
    } catch {
      return {
        ...clip,
        media: clip.media
          .map((m) => (m === pendingVideo && m.posterUrl ? { url: m.posterUrl, kind: 'image' as const } : m))
          .filter((m) => m.url),
        needsEnrichment: false,
      }
    }
  },
}
