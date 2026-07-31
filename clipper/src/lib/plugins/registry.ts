// Plugin registry — the ONE seam where platforms plug in. A plugin has a
// DOM side (runs in the content script: resolve the capture unit from a
// click target, extract a ClipResult) and an optional background side
// (enrich: API lookups the page context can't do, e.g. resolving video
// URLs). Everything else in the extension is platform-blind.
//
// Adding a platform = one plugin here. Plugins know NOTHING about slide
// templates: slides are stamped 'auto' and BRUV resolves the template from
// the capture URL (shared/slideTemplates.ts) — so a platform captured today
// picks up its dedicated template whenever one ships, retroactively.

import type { ClipResult } from '../types'
import { twitterPlugin } from './twitter'
import { truthsocialPlugin } from './truthsocial'
import { redditPlugin } from './reddit'
import { youtubePlugin } from './youtube'

export type ClipperPlugin = {
  id: string
  matchesUrl(url: string): boolean
  // DOM side (content script). Returns the capture unit containing the
  // click/selection target, or null when the target isn't capturable.
  resolveCaptureUnit(target: Element, doc: Document): Element | null
  // DOM side, no click target: resolve the PAGE's primary capture unit.
  // Used by the completion flow, which opens a pending clip's URL in a
  // transient tab and captures it unattended (no user gesture to anchor
  // on). Returns null when the page isn't a single-post permalink.
  resolvePageUnit?(doc: Document): Element | null
  extract(unit: Element, doc: Document): ClipResult | null
  // Background side. Resolve anything the DOM couldn't (video URLs, …).
  // Must tolerate failure by returning the clip unchanged or degraded —
  // never throw the whole clip away.
  enrich?(clip: ClipResult): Promise<ClipResult>
}

const PLUGINS: ClipperPlugin[] = [twitterPlugin, truthsocialPlugin, redditPlugin, youtubePlugin]

export function pluginForUrl(url: string): ClipperPlugin | null {
  return PLUGINS.find((p) => p.matchesUrl(url)) ?? null
}

export function pluginById(id: string): ClipperPlugin | null {
  return PLUGINS.find((p) => p.id === id) ?? null
}
