import { describe, it, expect } from 'vitest'
import { countWords, documentFormatForPath, documentName } from '@shared/documentFormats'
import { markdownOutline } from '@shared/documentFormats/markdown'
import { stripInlineMarkdown } from '@shared/documentFormats/text'

describe('documentFormatForPath', () => {
  it('maps markdown extensions case-insensitively and falls back to plain text', () => {
    expect(documentFormatForPath('notes/README.md').id).toBe('markdown')
    expect(documentFormatForPath('a/b/Chapter.MARKDOWN').id).toBe('markdown')
    expect(documentFormatForPath('script.txt').id).toBe('plaintext')
    expect(documentFormatForPath('Makefile').id).toBe('plaintext')
    expect(documentFormatForPath('.gitignore').id).toBe('plaintext')
    // A dotted directory must not be mistaken for the file's extension.
    expect(documentFormatForPath('vault.md/notes').id).toBe('plaintext')
  })

  it('takes the file name from either separator', () => {
    expect(documentName('a/b/c.md')).toBe('c.md')
    expect(documentName('a\\b\\c.md')).toBe('c.md')
    expect(documentName('c.md')).toBe('c.md')
  })
})

describe('markdownOutline', () => {
  it('collects ATX headings with their level and 1-based line', () => {
    const nodes = markdownOutline('# Title\n\ntext\n\n## Part **one**\n### Sub [link](x) `code`\n####### not a heading\n#no space')
    expect(nodes).toEqual([
      { id: '1', level: 1, title: 'Title', line: 1 },
      { id: '5', level: 2, title: 'Part one', line: 5 },
      { id: '6', level: 3, title: 'Sub link code', line: 6 },
    ])
  })

  it('strips closing hashes and ignores empty headings', () => {
    expect(markdownOutline('## Closed ##\n#\n##   ').map(n => n.title)).toEqual(['Closed'])
  })

  it('recognises setext headings but not a rule under a list item or blank line', () => {
    const nodes = markdownOutline('Title\n=====\n\nSecond\n---\n\n- item\n---\n\n---\n')
    expect(nodes).toEqual([
      { id: '1', level: 1, title: 'Title', line: 1 },
      { id: '4', level: 2, title: 'Second', line: 4 },
    ])
  })

  it('does not read headings inside fenced code', () => {
    const text = '# Real\n```md\n# Not a heading\n```\n~~~\n## Also not\n~~~\n# Real again'
    expect(markdownOutline(text).map(n => n.title)).toEqual(['Real', 'Real again'])
  })

  it('handles CRLF line endings', () => {
    expect(markdownOutline('# A\r\n\r\n## B').map(n => [n.title, n.line])).toEqual([['A', 1], ['B', 3]])
  })
})

describe('countWords', () => {
  it('counts tokens with letters or digits only', () => {
    expect(countWords('')).toBe(0)
    expect(countWords('one two  three')).toBe(3)
    expect(countWords('* * *\n---\n— … ***')).toBe(0)
    expect(countWords("It's 2 words — no, three\n")).toBe(5)
    expect(countWords('café naïve 日本語')).toBe(3)
  })
})

describe('stripInlineMarkdown', () => {
  it('reduces emphasis, code, links and images to their text', () => {
    expect(stripInlineMarkdown('**Bold** _it_ `x` [link](u) ![alt](i) ~~gone~~')).toBe('Bold it x link alt gone')
  })
})
