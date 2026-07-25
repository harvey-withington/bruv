// Plugin registry — the ONE seam where platforms plug in. A plugin has a
// DOM side (runs in the content script: resolve the capture unit from a
// click target, extract a ClipResult) and an optional background side
// (enrich: API lookups the page context can't do, e.g. resolving video
// URLs). Everything else in the extension is platform-blind.
//
// Expanding beyond Twitter = adding one plugin here (+ one slide template
// on the BRUV side). Nothing else changes.

import type { ClipResult } from '../types'
import { twitterPlugin } from './twitter'

export type ClipperPlugin = {
  id: string
  // Which slide template BRUV should render this platform's posts with.
  // Stamped onto the slide at construction — template resolution on the
  // BRUV side stays platform-blind.
  defaultTemplateId: string
  matchesUrl(url: string): boolean
  // DOM side (content script). Returns the capture unit containing the
  // click/selection target, or null when the target isn't capturable.
  resolveCaptureUnit(target: Element, doc: Document): Element | null
  extract(unit: Element, doc: Document): ClipResult | null
  // Background side. Resolve anything the DOM couldn't (video URLs, …).
  // Must tolerate failure by returning the clip unchanged or degraded —
  // never throw the whole clip away.
  enrich?(clip: ClipResult): Promise<ClipResult>
}

const PLUGINS: ClipperPlugin[] = [twitterPlugin]

export function pluginForUrl(url: string): ClipperPlugin | null {
  return PLUGINS.find((p) => p.matchesUrl(url)) ?? null
}

export function pluginById(id: string): ClipperPlugin | null {
  return PLUGINS.find((p) => p.id === id) ?? null
}
