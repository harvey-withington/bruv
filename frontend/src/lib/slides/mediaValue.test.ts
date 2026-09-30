import { describe, it, expect } from 'vitest'
import { splitMediaValue, joinMediaValue, pastedMediaItems } from './mediaValue'

// Gallery media values are newline-joined lists; the slide editor must
// never merge them into one string or drop items it didn't touch.
describe('slide media values', () => {
  it('splits a gallery into items, dropping blank lines', () => {
    expect(splitMediaValue('https://a/1.png\n\nattachment:c1/att-2\n')).toEqual(['https://a/1.png', 'attachment:c1/att-2'])
    expect(splitMediaValue('')).toEqual([])
  })

  it('round-trips items and drops blank rows on join', () => {
    expect(joinMediaValue(['https://a/1.png', '  ', 'attachment:c1/att-2'])).toBe('https://a/1.png\nattachment:c1/att-2')
  })

  it('removing one item keeps the others intact', () => {
    const items = splitMediaValue('attachment:c1/a\nattachment:c1/b\nhttps://x/y.png')
    expect(joinMediaValue(items.filter(i => i !== 'attachment:c1/b'))).toBe('attachment:c1/a\nhttps://x/y.png')
  })

  it('turns a pasted multi-line gallery (CRLF too) into separate items', () => {
    expect(pastedMediaItems('https://a/1.png\r\nhttps://a/2.png')).toEqual(['https://a/1.png', 'https://a/2.png'])
  })
})
