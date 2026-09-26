import { describe, it, expect } from 'vitest'
import {
  applyMentionToElement, cardMentionLink, detectMentionTrigger, isMentionPickerOpenFor,
  mentionMarkdown, setMentionPickerOpen, spliceMention,
} from '@shared/mentions'

describe('detectMentionTrigger', () => {
  it('fires for @ at the start or after whitespace', () => {
    expect(detectMentionTrigger('@', 1)).toBe(0)
    expect(detectMentionTrigger('see @', 5)).toBe(4)
    expect(detectMentionTrigger('line\n@', 6)).toBe(5)
  })
  it('ignores @ inside a word (emails) and non-caret positions', () => {
    expect(detectMentionTrigger('harv@example', 5)).toBeNull()
    expect(detectMentionTrigger('see @x', 6)).toBeNull()
    expect(detectMentionTrigger('', 0)).toBeNull()
  })
})

describe('spliceMention', () => {
  it('replaces the @ and any partial query, then adds a space', () => {
    const r = spliceMention('Settle the name, see @nam', 21, 25, '[Naming](bruv:card:1)')
    expect(r.value).toBe('Settle the name, see [Naming](bruv:card:1) ')
    expect(r.caret).toBe(r.value.length)
  })
  it('does not double a space that already follows', () => {
    const r = spliceMention('see @ later', 4, 5, '[X](bruv:card:1)')
    expect(r.value).toBe('see [X](bruv:card:1) later')
  })
})

describe('markdown + element application', () => {
  it('escapes brackets in labels and builds card links', () => {
    expect(mentionMarkdown('Idea [draft]', cardMentionLink('abc'))).toBe('[Idea \\[draft\\]](bruv:card:abc)')
  })
  it('writes into a textarea, fires input, and places the caret', () => {
    const ta = document.createElement('textarea')
    document.body.appendChild(ta)
    ta.value = 'hello @'
    ta.setSelectionRange(7, 7)
    let inputs = 0
    ta.addEventListener('input', () => inputs++)
    applyMentionToElement(ta, 6, '[Card](bruv:card:1)')
    expect(ta.value).toBe('hello [Card](bruv:card:1) ')
    expect(inputs).toBe(1)
    expect(ta.selectionStart).toBe(ta.value.length)
    ta.remove()
  })
  it('tracks which element has a picker open', () => {
    const el = document.createElement('input')
    expect(isMentionPickerOpenFor(el)).toBe(false)
    setMentionPickerOpen(el, true)
    expect(isMentionPickerOpenFor(el)).toBe(true)
    setMentionPickerOpen(el, false)
    expect(isMentionPickerOpenFor(el)).toBe(false)
    expect(isMentionPickerOpenFor(null)).toBe(false)
  })
})

describe('isMentionKeystroke', () => {
  it('is true only for a typed @', async () => {
    const { isMentionKeystroke } = await import('@shared/mentions')
    expect(isMentionKeystroke(new InputEvent('input', { inputType: 'insertText', data: '@' }))).toBe(true)
    expect(isMentionKeystroke(new InputEvent('input', { inputType: 'insertCompositionText', data: 'see @' }))).toBe(true)
    expect(isMentionKeystroke(new InputEvent('input', { inputType: 'deleteContentBackward' }))).toBe(false)
    expect(isMentionKeystroke(new InputEvent('input', { inputType: 'insertText', data: 'a' }))).toBe(false)
    expect(isMentionKeystroke(new Event('input'))).toBe(false)
  })
})
