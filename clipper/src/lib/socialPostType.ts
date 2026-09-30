// "Social Post" card type provisioning for clipped cards (see below).

import type { ClipperSettings } from './types'
import { repoRPC } from './api'

// --- "Social Post" card type -------------------------------------------
// Clipped cards are TYPED: one generic Social Post card type (mirroring the
// generic `post` slide content type — platforms differentiate by template,
// never by schema), provisioned in-context on first clip per repo. This is
// the Create-Type-from-Card adoption lesson: the type exists because real
// data needed it. NOTE: these template blocks are one of FIVE mirrors of
// the post schema (shared/slideContentTypes.ts, the two Go maps in
// present.go, core/supervisor/capture.go's socialPostTemplateBlocks, and
// this) — keep all five in sync. This copy retires when the extension's
// normal clips move onto the server's ingest via CompleteCapture (planned
// unification).
const SOCIAL_POST_TYPE_LABEL = 'Social Post'

const SOCIAL_POST_TEMPLATE_BLOCKS = [
  { id: 'tpl-author', type: 'text', label: 'Author', key: 'author', value: '' },
  { id: 'tpl-handle', type: 'text', label: 'Handle', key: 'handle', value: '' },
  { id: 'tpl-avatar', type: 'image', label: 'Avatar', key: 'avatar', value: { url: '' } },
  { id: 'tpl-text', type: 'text', label: 'Text', key: 'text', value: '' },
  { id: 'tpl-media', type: 'image', label: 'Media', key: 'media', value: { url: '' } },
  { id: 'tpl-video', type: 'media', label: 'Video', key: 'video', value: [] },
  { id: 'tpl-date', type: 'text', label: 'Date', key: 'date', value: '' },
  { id: 'tpl-url', type: 'url', label: 'Source', key: 'url', value: { url: '' } },
]

type CardTypeInfo = { id: string; label: string; builtin?: boolean }

// Types carry no stable marker, so the label is the identity — matched
// trimmed and case-insensitively, so a user's "social post" (or stray
// whitespace) is reused instead of duplicated.
async function findSocialPostType(s: ClipperSettings): Promise<string> {
  const types = (await repoRPC<CardTypeInfo[]>(s, 'ListCardTypes', [])) ?? []
  const want = SOCIAL_POST_TYPE_LABEL.toLowerCase()
  return types.find((t) => t.label.trim().toLowerCase() === want)?.id ?? ''
}

async function provisionSocialPostType(s: ClipperSettings): Promise<string> {
  try {
    const existing = await findSocialPostType(s)
    if (existing) return existing
    const template = await repoRPC<{ id: string }>(s, 'CreateCardTemplate', [
      SOCIAL_POST_TYPE_LABEL,
      SOCIAL_POST_TEMPLATE_BLOCKS,
    ])
    // Re-check right before creating: another client (the phone's share
    // capture, another browser) may have provisioned it meanwhile.
    const raced = await findSocialPostType(s)
    if (raced) return raced
    const created = await repoRPC<{ id: string }>(s, 'CreateUserCardType', [
      SOCIAL_POST_TYPE_LABEL,
      '#1d9bf0',
      'A captured social post (web clipper)',
      '',
      template.id,
    ])
    return created.id
  } catch (err) {
    console.warn('ensure Social Post card type failed (clipping untyped):', err)
    return ''
  }
}

// Single-flight per repo: concurrent first clips in this worker (a burst of
// right-clicks, a queue drain beside a live clip) share ONE provisioning
// run instead of each creating its own type. Failures aren't cached.
const socialPostTypeInFlight = new Map<string, Promise<string>>()

// ensureSocialPostType returns the type ID, creating template + type on
// first use. Failure degrades to an untyped card — never blocks a clip.
export function ensureSocialPostType(s: ClipperSettings): Promise<string> {
  const key = `${s.serverURL}|${s.repoID}`
  const inFlight = socialPostTypeInFlight.get(key)
  if (inFlight) return inFlight
  const run = provisionSocialPostType(s).finally(() => socialPostTypeInFlight.delete(key))
  socialPostTypeInFlight.set(key, run)
  return run
}
