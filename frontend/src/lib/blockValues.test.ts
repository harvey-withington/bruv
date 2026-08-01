import { describe, it, expect } from 'vitest'
import { asUrlValue, urlBlockValue } from '@shared/blockValues'

// URL blocks are written by four producers (desktop edits, mobile's
// UrlBlock, the web clipper, server-side capture ingest). They disagreed
// on shape until 2026-08-01: desktop stored/read a bare string, everyone
// else `{url, caption?}` — so captured link cards rendered
// "[object Object]" on desktop and desktop's own edits rendered empty on
// mobile. Both shapes must stay readable, and writes must be canonical.
describe('asUrlValue', () => {
  it('reads the canonical object shape', () => {
    expect(asUrlValue({ url: 'https://x.com/a' })).toEqual({ url: 'https://x.com/a', caption: undefined })
  })

  it('reads a legacy bare string (older desktop edits) rather than dropping it', () => {
    expect(asUrlValue('https://example.com')).toEqual({ url: 'https://example.com' })
  })

  it('keeps captions', () => {
    expect(asUrlValue({ url: 'https://x.com/a', caption: 'The post' })).toEqual({
      url: 'https://x.com/a',
      caption: 'The post',
    })
  })

  it('degrades to empty for junk instead of stringifying an object', () => {
    expect(asUrlValue(null).url).toBe('')
    expect(asUrlValue(undefined).url).toBe('')
    expect(asUrlValue({ nope: 1 }).url).toBe('')
    expect(asUrlValue(42).url).toBe('')
  })
})

describe('urlBlockValue', () => {
  it('writes the canonical shape', () => {
    expect(urlBlockValue('https://a.test')).toEqual({ url: 'https://a.test' })
  })

  it('preserves an existing caption when only the URL is edited', () => {
    expect(urlBlockValue('https://b.test', { url: 'https://a.test', caption: 'Keep me' })).toEqual({
      url: 'https://b.test',
      caption: 'Keep me',
    })
  })

  it('upgrades a legacy string value without inventing a caption', () => {
    expect(urlBlockValue('https://b.test', 'https://a.test')).toEqual({ url: 'https://b.test' })
  })
})
