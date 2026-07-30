import type { SlideTemplate, SlideAnimation, SlideFieldMapping, TemplatePrefs } from './types'

// Built-in Slide Templates. A template is DATA (not code): it declares which
// content types it renders and maps each type's fields to display roles, which
// one generic renderer interprets. This keeps templates serialisable so they
// can be user-authored and shared via a registry/marketplace later (and leaves
// room for a Reveal.js-style backend). The built-ins are deliberately simple.
//
// Template selection (plan/2026-07-31 per-platform slide templates…):
//   - explicit templateId = user pin, always wins;
//   - AUTO_TEMPLATE_ID    = resolve from the slide's capture URL via each
//                           template's urlHint (Auto — what the clipper stamps);
//   - undefined           = legacy fallback (first template for the type).
// urlHint powers Auto ONLY — the manual picker filters by content type alone;
// rendering Facebook content on the X template is a choice, not an error.

// Sentinel for Slide.templateId: resolve dynamically from the capture URL.
export const AUTO_TEMPLATE_ID = 'auto'

const CLEAN: SlideTemplate = {
  id: 'clean',
  name: 'Clean',
  supportedContentTypes: ['title', 'statement', 'lower_third'],
  fieldMap: {
    title: [
      { field: 'title', role: 'heading' },
      { field: 'subtitle', role: 'subheading' },
    ],
    statement: [{ field: 'statement', role: 'heading' }],
    lower_third: [
      { field: 'name', role: 'heading' },
      { field: 'subtitle', role: 'subheading' },
    ],
  },
  entrance: 'fadeIn',
  durationMs: 500,
}

const ELEGANT_QUOTE: SlideTemplate = {
  id: 'elegant-quote',
  name: 'Elegant Quote',
  supportedContentTypes: ['quote'],
  fieldMap: {
    quote: [
      { field: 'quote', role: 'quote' },
      { field: 'author', role: 'attribution' },
    ],
  },
  entrance: 'zoomIn',
  durationMs: 600,
}

const SHOWCASE: SlideTemplate = {
  id: 'showcase',
  name: 'Showcase',
  supportedContentTypes: ['image', 'video'],
  fieldMap: {
    image: [
      { field: 'image', role: 'media' },
      { field: 'caption', role: 'caption' },
    ],
    video: [
      { field: 'video', role: 'media' },
      { field: 'caption', role: 'caption' },
    ],
  },
  entrance: 'zoomIn',
  durationMs: 700,
}

// All social templates render the SAME generic `post` content type with the
// same field→role mapping — platforms differ by look (styles) and by urlHint,
// never by schema. The 'post-card' layout interprets the roles: avatar +
// subheading + first meta in the header, text as body, media below, later
// meta items (date) in the footer, platform glyph from values.platform.
// `url` is deliberately unrendered (data-only — it feeds Auto matching).
const POST_FIELDS: SlideFieldMapping[] = [
  { field: 'avatar', role: 'avatar' },
  { field: 'author', role: 'subheading' },
  { field: 'handle', role: 'meta' },
  { field: 'text', role: 'heading' },
  { field: 'media', role: 'media' },
  { field: 'video', role: 'media' },
  { field: 'date', role: 'meta' },
]

// The neutral fallback for `post` content whose capture URL matches no
// platform hint (or that has no URL at all). No urlHint by design.
const SOCIAL_POST: SlideTemplate = {
  id: 'social-post',
  name: 'Social Post',
  supportedContentTypes: ['post'],
  fieldMap: { post: POST_FIELDS },
  entrance: 'slideInUp',
  durationMs: 500,
  layout: 'post-card',
}

const X_POST: SlideTemplate = {
  id: 'x-post',
  name: 'X Post',
  supportedContentTypes: ['post'],
  fieldMap: { post: POST_FIELDS },
  entrance: 'slideInUp',
  durationMs: 500,
  layout: 'post-card',
  urlHint: '^https?://(mobile\\.)?(twitter|x)\\.com/',
  styles: {
    cardBackgroundColor: '#000000',
    cardBorderColor: '#2f3336',
    cardTextColor: '#e7e9ea',
    cardMutedColor: '#71767b',
    // Accent colors the platform glyph — X's mark is white-on-black.
    cardAccentColor: '#e7e9ea',
    cardFontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif",
  },
}

const TRUTH_POST: SlideTemplate = {
  id: 'truth-post',
  name: 'Truth Social Post',
  supportedContentTypes: ['post'],
  fieldMap: { post: POST_FIELDS },
  entrance: 'slideInUp',
  durationMs: 500,
  layout: 'post-card',
  urlHint: '^https?://([a-z0-9-]+\\.)?truthsocial\\.com/',
  styles: {
    cardBackgroundColor: '#10141f',
    cardBorderColor: '#2a3245',
    cardTextColor: '#e8eaf0',
    cardMutedColor: '#7a849b',
    cardAccentColor: '#5448ee',
    cardFontFamily: "'Segoe UI', Roboto, Helvetica, Arial, sans-serif",
  },
}

const REDDIT_POST: SlideTemplate = {
  id: 'reddit-post',
  name: 'Reddit Post',
  supportedContentTypes: ['post'],
  fieldMap: { post: POST_FIELDS },
  entrance: 'slideInUp',
  durationMs: 500,
  layout: 'post-card',
  urlHint: '^https?://([a-z0-9-]+\\.)?(reddit\\.com|redd\\.it)/',
  styles: {
    cardBackgroundColor: '#0f1113',
    cardBorderColor: '#343536',
    cardTextColor: '#d7dadc',
    cardMutedColor: '#818384',
    cardAccentColor: '#ff4500',
    cardFontFamily: "'Segoe UI', Roboto, Helvetica, Arial, sans-serif",
  },
}

// yt-video maps NO `media` field: a YouTube slide carries both the durable
// thumbnail (card/vault copy) and the embed player, and rendering both is
// two near-identical images stacked (Harvey, 2026-07-31). Video-first
// template; the thumbnail still lives on the card and in fallback templates.
const YT_VIDEO: SlideTemplate = {
  id: 'yt-video',
  name: 'YouTube Video',
  supportedContentTypes: ['post'],
  fieldMap: { post: POST_FIELDS.filter((m) => m.field !== 'media') },
  entrance: 'slideInUp',
  durationMs: 500,
  layout: 'post-card',
  urlHint: '^https?://([a-z0-9-]+\\.)?(youtube\\.com|youtu\\.be)/',
  styles: {
    cardBackgroundColor: '#0f0f0f',
    cardBorderColor: '#303030',
    cardTextColor: '#f1f1f1',
    cardMutedColor: '#aaaaaa',
    cardAccentColor: '#ff0000',
    cardFontFamily: "Roboto, 'Segoe UI', Helvetica, Arial, sans-serif",
  },
}

// Registration order = Auto tie-break order when prefs don't say otherwise,
// and picker order. social-post has no hint, so it never competes in Auto.
export const SLIDE_TEMPLATES: SlideTemplate[] = [
  CLEAN,
  ELEGANT_QUOTE,
  SHOWCASE,
  SOCIAL_POST,
  X_POST,
  TRUTH_POST,
  REDDIT_POST,
  YT_VIDEO,
]

// Designated no-match fallback per content type; types not listed fall back
// to their first supporting template (then Clean).
const FALLBACK_TEMPLATE_ID: Record<string, string> = { post: SOCIAL_POST.id }

// templatesForContentType lists the templates that can render a content type
// (the editor's template picker, filtered by the slide's content type — hints
// deliberately play no part here).
export function templatesForContentType(contentTypeId: string): SlideTemplate[] {
  return SLIDE_TEMPLATES.filter((t) => t.supportedContentTypes.includes(contentTypeId))
}

// Compiled-hint cache: regex sources are compiled once and bad sources are
// remembered as null so an invalid user override can never break rendering.
const hintCache = new Map<string, RegExp | null>()
function compileHint(source: string): RegExp | null {
  let re = hintCache.get(source)
  if (re === undefined) {
    try {
      re = new RegExp(source, 'i')
    } catch {
      re = null
    }
    hintCache.set(source, re)
  }
  return re
}

// hintMatches tests a template's effective hint (user override when it
// compiles, else the built-in) against a capture URL.
function hintMatches(t: SlideTemplate, url: string, prefs?: TemplatePrefs): boolean {
  const override = prefs?.urlOverrides?.[t.id]
  const source = (override && compileHint(override) ? override : t.urlHint) ?? ''
  if (!source) return false
  return compileHint(source)?.test(url) ?? false
}

// byPrefOrder sorts candidates by the prefs priority list; unlisted templates
// keep registration order after the listed ones.
function byPrefOrder(candidates: SlideTemplate[], prefs?: TemplatePrefs): SlideTemplate[] {
  const order = prefs?.order
  if (!order?.length) return candidates
  const rank = new Map(order.map((id, i) => [id, i]))
  return [...candidates].sort(
    (a, b) => (rank.get(a.id) ?? order.length) - (rank.get(b.id) ?? order.length),
  )
}

// autoResolveTemplate picks the template for an Auto slide: supporting
// templates whose effective hint matches the capture URL, in priority order.
// Null when nothing matches (caller falls back).
export function autoResolveTemplate(
  contentTypeId: string,
  captureUrl: string | undefined,
  prefs?: TemplatePrefs,
): SlideTemplate | null {
  if (!captureUrl) return null
  const matches = templatesForContentType(contentTypeId).filter((t) => hintMatches(t, captureUrl, prefs))
  return byPrefOrder(matches, prefs)[0] ?? null
}

function fallbackTemplate(contentTypeId: string): SlideTemplate {
  const designated = SLIDE_TEMPLATES.find((t) => t.id === FALLBACK_TEMPLATE_ID[contentTypeId])
  if (designated) return designated
  return templatesForContentType(contentTypeId)[0] ?? CLEAN
}

// resolveSlideTemplate always returns a concrete template that renders the
// given content type:
//   1. an explicit templateId (user pin) if valid + compatible — always wins;
//   2. AUTO_TEMPLATE_ID → hint match against the capture URL (values.url,
//      post-binding-resolution — callers pass the resolved value);
//   3. the content type's designated fallback, else the first supporting
//      template, else Clean.
export function resolveSlideTemplate(
  templateId: string | undefined,
  contentTypeId: string,
  captureUrl?: string,
  prefs?: TemplatePrefs,
): SlideTemplate {
  if (templateId && templateId !== AUTO_TEMPLATE_ID) {
    const found = SLIDE_TEMPLATES.find((t) => t.id === templateId)
    if (found && found.supportedContentTypes.includes(contentTypeId)) return found
  }
  if (templateId === AUTO_TEMPLATE_ID) {
    const matched = autoResolveTemplate(contentTypeId, captureUrl, prefs)
    if (matched) return matched
  }
  return fallbackTemplate(contentTypeId)
}

// entranceClass names the CSS class the renderer applies to (re-)fire a
// slide's entrance animation. Pair with a {#key slide.id} wrapper.
export function entranceClass(anim: SlideAnimation): string {
  return anim === 'none' ? '' : `slide-anim-${anim}`
}
