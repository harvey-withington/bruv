// Twitter/X extractor — the ONLY file on the extension side that knows what
// Twitter is. DOM side reads the tweet <article>; background side resolves
// video through X's syndication API (the react-tweet approach), because the
// DOM only exposes blob: URLs for video.

import type { ClipperPlugin } from './registry'
import type { ClipMedia, ClipMediaVariant, ClipResult } from '../types'

function statusIdFromUrl(url: string): string {
  const m = url.match(/\/status\/(\d+)/)
  return m ? m[1] : ''
}

// X serves its mp4s from paths like /vid/avc1/1280x720/…, which is the only
// place the rendition's dimensions appear. Bitrate is the fallback label.
function variantLabel(url: string, bitrate: number | undefined): string {
  const dims = url.match(/\/(\d+)x(\d+)\//)
  if (dims) return `${dims[1]}×${dims[2]}`
  return bitrate ? `${Math.round(bitrate / 1000)} kbps` : url.split('/').pop() ?? url
}

// Size is arithmetic, not a guess: bitrate × duration. Without a duration
// there is no honest estimate, so the ladder says nothing rather than
// inventing a number.
function estimateBytes(bitrate: number | undefined, durationMs: number): number | undefined {
  if (!bitrate || durationMs <= 0) return undefined
  return Math.round((bitrate / 8) * (durationMs / 1000))
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

  // Completion captures (a pending clip's URL opened in a transient tab)
  // have no click target. Only a permalink page has an unambiguous primary
  // tweet — and on one, the focused tweet is the FIRST article rendered
  // (replies and the "more posts" rail follow it). A timeline URL would
  // otherwise silently capture whatever happened to be top of the feed.
  resolvePageUnit(doc: Document): Element | null {
    if (!statusIdFromUrl(doc.location?.href ?? '')) return null
    return doc.querySelector('article[data-testid="tweet"]')
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
  //
  // The WHOLE ladder is recorded, not just the pick: X bot-walls BRUV's
  // server, so a server-side PreviewCapture of the same tweet comes back
  // blocked — this is the only place the capture dialog can learn that the
  // 1080p rung is 3.5 GB. `url` still holds the best rendition, so nothing
  // downstream changes when no choice is made.
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
          video_info?: {
            duration_millis?: number
            variants?: Array<{ content_type?: string; bitrate?: number; url?: string }>
          }
        }>
      }
      const info = data.mediaDetails?.find((m) => m.type === 'video' || m.type === 'animated_gif')?.video_info
      const mp4s = (info?.variants ?? []).filter(
        (v): v is { content_type: string; bitrate?: number; url: string } =>
          v.content_type === 'video/mp4' && typeof v.url === 'string' && v.url.length > 0,
      )
      // Ascending (cheapest first) — the order every ladder consumer assumes.
      mp4s.sort((a, b) => (a.bitrate ?? 0) - (b.bitrate ?? 0))
      const ladder: ClipMediaVariant[] = mp4s.map((v, i) => ({
        id: `tw-${v.bitrate ?? i}`,
        label: variantLabel(v.url, v.bitrate),
        url: v.url,
        bitrate: v.bitrate,
        estBytes: estimateBytes(v.bitrate, info?.duration_millis ?? 0),
      }))
      // Highest-bitrate mp4 remains the default rendition.
      const best = ladder[ladder.length - 1]?.url
      const media = clip.media.map((m) =>
        m === pendingVideo && best ? { ...m, url: best, variants: ladder } : m,
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
