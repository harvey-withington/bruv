import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import { setBackend } from '@shared/adapters'
import { createMockAdapter } from '../../../lib/adapters/mock'
import type { DocumentSource } from '../../../lib/editor/documentSource'
import DocumentEditor from './DocumentEditor.svelte'

// The editor prefs (layout, outline) come from UI preferences.
beforeEach(() => { setBackend(createMockAdapter()) })

// CodeMirror measures text with Range.getClientRects, which jsdom lacks.
// An empty rect list is enough for it to lay out (in)visibly.
const emptyRects = (): DOMRectList => Object.assign([] as DOMRect[], { item: () => null }) as unknown as DOMRectList
if (typeof Range !== 'undefined' && !Range.prototype.getClientRects) {
  Range.prototype.getClientRects = emptyRects
  Range.prototype.getBoundingClientRect = () => new DOMRect()
}

// jsdom has no layout, so this stays a smoke test of the shell: the
// document loads into the editor, previews as Markdown, and the outline
// lists its headings. CodeMirror's own behaviour is not under test here.

function source(content: string): DocumentSource {
  return {
    path: 'notes/plan.md',
    open: vi.fn(async () => ({ content, stamp: { hash: 'sha256:1', size: content.length } })),
    stat: vi.fn(async () => ({ hash: 'sha256:1', size: content.length })),
    save: vi.fn(async () => ({ diverged: false, stamp: { hash: 'sha256:2', size: 0 } })),
  }
}

describe('DocumentEditor', () => {
  it('loads a markdown file into the editor, preview and outline', async () => {
    const src = source('# Title\n\nSome *prose*.\n\n## Part two\n')
    render(DocumentEditor, { props: { source: src, onClose: () => {} } })

    await waitFor(() => expect(document.querySelector('.cm-content')).not.toBeNull())
    expect(document.querySelector('.cm-content')?.textContent).toContain('Some *prose*.')

    // Preview renders through the shared Markdown engine.
    await waitFor(() => expect(document.querySelector('.doc-preview h1')?.textContent).toBe('Title'))
    expect(document.querySelector('.doc-preview em')?.textContent).toBe('prose')

    // Outline follows the headings after the analysis debounce.
    await waitFor(() => expect(screen.getByRole('button', { name: 'Part two' })).toBeInTheDocument())
    expect(screen.getByRole('button', { name: 'Title' })).toBeInTheDocument()
    expect(screen.getByText('5 words')).toBeInTheDocument()
  })

  it('shows the load error with the Tier 1 fallbacks', async () => {
    const src: DocumentSource = {
      path: 'photo.png',
      open: async () => { throw new Error('photo.png is not a text file') },
      stat: async () => ({ size: 0 }),
      save: async () => ({ diverged: false, stamp: { size: 0 } }),
    }
    const onOpenExternal = vi.fn()
    render(DocumentEditor, { props: { source: src, onClose: () => {}, onOpenExternal } })
    await waitFor(() => expect(screen.getByText('photo.png is not a text file')).toBeInTheDocument())
    await fireEvent.click(screen.getAllByRole('button', { name: 'Open in default app' })[0])
    expect(onOpenExternal).toHaveBeenCalled()
  })

  it('switches layouts and keeps the editor mounted in preview mode', async () => {
    const src = source('hello')
    render(DocumentEditor, { props: { source: src, onClose: () => {} } })
    await waitFor(() => expect(document.querySelector('.cm-content')).not.toBeNull())

    await fireEvent.click(screen.getByRole('button', { name: 'Preview' }))
    const pane = document.querySelector('.cm-content')?.closest('.pane') as HTMLElement | null
    expect(pane?.hidden).toBe(true)
    expect(document.querySelector('.doc-preview')).not.toBeNull()

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    expect(pane?.hidden).toBe(false)
    expect(document.querySelector('.doc-preview')).toBeNull()
  })
})
