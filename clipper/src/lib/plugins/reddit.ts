// Reddit extractor — the ONLY file on the extension side that knows what
// Reddit is. Two live frontends to support: new Reddit (www/sh.reddit.com),
// which renders posts as <shreddit-post> custom elements carrying the post's
// data as ATTRIBUTES (the most stable surface on new Reddit — its inner DOM
// is a churny web-component tree), and old.reddit.com, which still renders
// posts as classic div.thing nodes with data-* attributes. DOM side never
// extracts media — Reddit's public JSON API is authoritative and far simpler
// than chasing lazy-loaded previews/galleries, so media resolution always
// happens in enrich().

import type { ClipperPlugin } from './registry'
import type { ClipMedia, ClipResult } from '../types'

// Resolve a subreddit handle from whatever we've got. New Reddit's
// subreddit-prefixed-name attribute already arrives as "r/foo"; old Reddit's
// data-subreddit attribute is bare ("foo"), so only prefix when needed. Falls
// back to pulling /r/<name>/ out of the permalink when neither attribute is
// present (belt-and-braces for markup drift).
function subredditHandle(rawPrefixed: string | null, permalink: string): string {
  if (rawPrefixed) return rawPrefixed.startsWith('r/') ? rawPrefixed : `r/${rawPrefixed}`
  const m = permalink.match(/\/r\/([A-Za-z0-9_]+)\//)
  return m ? `r/${m[1]}` : ''
}

// -- Reddit's public JSON API shape (only the fields we read) --------------
// GET https://www.reddit.com/<permalink>.json?raw_json=1 returns a 2-element
// Listing array: [0] is the post, [1] is the comment tree. raw_json=1 stops
// Reddit HTML-entity-escaping URLs (so &amp; doesn't show up in media links).
type RedditPostData = {
  title?: string
  selftext?: string
  author?: string
  subreddit_name_prefixed?: string
  created_utc?: number
  permalink?: string
  is_gallery?: boolean
  preview?: { images?: Array<{ source?: { url?: string } }> }
  // Gallery images keyed by media ID; each entry's `s.u` is the full-size
  // image URL once raw_json=1 has de-escaped it.
  media_metadata?: Record<string, { s?: { u?: string } }>
  secure_media?: { reddit_video?: { fallback_url?: string } }
}

type RedditListing = {
  data?: { children?: Array<{ data?: RedditPostData }> }
}

export const redditPlugin: ClipperPlugin = {
  id: 'reddit',

  matchesUrl(url: string): boolean {
    return /^https?:\/\/([a-z0-9-]+\.)?(reddit\.com|redd\.it)\//.test(url)
  },

  // New Reddit: the post is the nearest <shreddit-post> ancestor. Old
  // Reddit: the post/comment is the nearest div.thing. On a comments page,
  // the user may right-click somewhere that's neither (e.g. the sidebar, or
  // between shreddit-post's shadow-DOM slots) — in that case fall back to
  // the page's own post, since right-clicking anywhere on a detail page
  // should reasonably capture the post being viewed.
  resolveCaptureUnit(target: Element, doc: Document): Element | null {
    return target.closest('shreddit-post') ?? target.closest('.thing') ?? doc.querySelector('shreddit-post')
  },

  extract(unit: Element): ClipResult | null {
    const isShreddit = unit.tagName.toLowerCase() === 'shreddit-post'

    // Pull the same logical fields off whichever frontend we're on. New
    // Reddit exposes everything as attributes on the custom element itself;
    // old Reddit exposes them as data-* attributes plus a couple of child
    // nodes (title link, <time>).
    const permalink = isShreddit
      ? (unit.getAttribute('permalink') ?? '')
      : (unit.getAttribute('data-permalink') ?? '')
    const rawAuthor = isShreddit ? unit.getAttribute('author') : unit.getAttribute('data-author')
    const rawSubredditPrefixed = isShreddit
      ? unit.getAttribute('subreddit-prefixed-name')
      : unit.getAttribute('data-subreddit')
    const title = isShreddit
      ? (unit.getAttribute('post-title') ?? '')
      : (unit.querySelector('a.title')?.textContent?.trim() ?? '')
    const publishedAt = isShreddit
      ? (unit.getAttribute('created-timestamp') ?? undefined)
      : (unit.querySelector('time')?.getAttribute('datetime') ?? undefined)

    if (!permalink && !title) return null

    const canonicalUrl = permalink ? new URL(permalink, location.origin).toString() : location.href
    const handle = subredditHandle(rawSubredditPrefixed, permalink)

    return {
      platform: 'reddit',
      canonicalUrl,
      author: rawAuthor ? `u/${rawAuthor}` : '',
      handle,
      // Reddit's post UI doesn't surface a reliable author avatar next to
      // the post itself (it's a separate hover-card fetch) — leave unset
      // rather than guess at a stale/wrong image.
      avatarUrl: undefined,
      text: title,
      // Media is intentionally left empty here; enrich() resolves it from
      // the JSON API, which — unlike the DOM — isn't lazy-loaded and gives
      // clean gallery/video URLs in one shot.
      media: [],
      publishedAt,
      extras: permalink ? { permalink } : {},
      needsEnrichment: true,
    }
  },

  // Resolve title/selftext/media authoritatively via Reddit's public JSON
  // API. Degrades to a poster image on failure, same pattern as twitter.ts —
  // a clip must never die here.
  async enrich(clip: ClipResult): Promise<ClipResult> {
    const permalink = clip.extras.permalink
    if (!permalink) return { ...clip, needsEnrichment: false }
    try {
      const res = await fetch(`https://www.reddit.com${permalink}.json?raw_json=1`)
      if (!res.ok) throw new Error(`reddit api ${res.status}`)
      const listing = (await res.json()) as RedditListing[]
      const post = listing[0]?.data?.children?.[0]?.data
      if (!post) throw new Error('no post data')

      // Slides are for the headline; the card keeps the link to the full
      // post, so a long selftext only needs a preview, not the whole thing.
      const title = post.title ?? clip.text
      let text = title
      if (post.selftext) {
        const preview = post.selftext.length > 500 ? `${post.selftext.slice(0, 500)}…` : post.selftext
        text = `${title}\n\n${preview}`
      }

      const media: ClipMedia[] = []
      const videoUrl = post.secure_media?.reddit_video?.fallback_url
      if (videoUrl) {
        // Reddit serves video and audio as separate DASH tracks; the
        // fallback_url is video-only (no sound). We accept that trade-off —
        // muted video beats no video — and still carry the poster as a
        // fallback for players/thumbnails that can't play it.
        media.push({ url: videoUrl, kind: 'video', posterUrl: post.preview?.images?.[0]?.source?.url })
      } else if (post.is_gallery && post.media_metadata) {
        // Whole gallery (capped well above typical posts — each image is
        // downloaded to base64 at capture, so the cap bounds job size).
        // Slides render multi-image media as a carousel.
        for (const item of Object.values(post.media_metadata)) {
          const u = item.s?.u
          if (!u) continue
          media.push({ url: u, kind: 'image' })
          if (media.length >= 12) break
        }
      } else {
        const previewUrl = post.preview?.images?.[0]?.source?.url
        if (previewUrl) media.push({ url: previewUrl, kind: 'image' })
      }

      return {
        ...clip,
        canonicalUrl: post.permalink ? new URL(post.permalink, 'https://www.reddit.com').toString() : clip.canonicalUrl,
        author: post.author ? `u/${post.author}` : clip.author,
        handle: post.subreddit_name_prefixed ?? clip.handle,
        text,
        media,
        publishedAt: post.created_utc ? new Date(post.created_utc * 1000).toISOString() : clip.publishedAt,
        extras: { ...clip.extras, permalink: post.permalink ?? permalink },
        needsEnrichment: false,
      }
    } catch {
      // Degrade any url-less video entry to its poster image (mirrors
      // twitter.ts's catch exactly) — in practice this is a no-op today
      // since the DOM side never populates media, but keeps the plugin safe
      // if that ever changes.
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
