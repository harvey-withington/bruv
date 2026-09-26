import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import EditableItemText from './EditableItemText.svelte'

// Mobile item rows edit in a multiline textarea (UI-CONVENTIONS §8,
// ruling 2026-09-20): Enter inserts a newline instead of committing,
// tap-away and ✓ Done commit, Escape cancels (and a never-committed blank
// row is dropped via onEmpty), Ctrl+Enter commits.

function mount(props: Record<string, unknown> = {}) {
  const onSave = vi.fn()
  const onEmpty = vi.fn()
  const utils = render(EditableItemText, { props: { text: 'Buy milk', onSave, onEmpty, ...props } })
  return { onSave, onEmpty, ...utils }
}

async function open(container: HTMLElement) {
  await fireEvent.click(container.querySelector('.display') as HTMLElement)
  await tick()
  await tick()
  return container.querySelector('textarea') as HTMLTextAreaElement
}

describe('EditableItemText (mobile, multiline)', () => {
  it('edits in a textarea and plain Enter does not commit', async () => {
    const { container, onSave } = mount()
    const ta = await open(container)
    expect(ta).not.toBeNull()
    expect(ta.rows).toBe(1)
    await fireEvent.input(ta, { target: { value: 'Buy milk\nand eggs' } })
    await fireEvent.keyDown(ta, { key: 'Enter' })
    expect(onSave).not.toHaveBeenCalled()
    expect(container.querySelector('textarea')).not.toBeNull()
  })

  it('commits on blur with the trimmed multi-line text', async () => {
    const { container, onSave } = mount()
    const ta = await open(container)
    await fireEvent.input(ta, { target: { value: '  Buy milk\nand eggs  ' } })
    await fireEvent.blur(ta)
    expect(onSave).toHaveBeenCalledWith('Buy milk\nand eggs')
    expect(container.querySelector('textarea')).toBeNull()
  })

  it('Ctrl+Enter commits', async () => {
    const { container, onSave } = mount()
    const ta = await open(container)
    await fireEvent.input(ta, { target: { value: 'Changed' } })
    await fireEvent.keyDown(ta, { key: 'Enter', ctrlKey: true })
    expect(onSave).toHaveBeenCalledWith('Changed')
  })

  it('Escape cancels and keeps the old text', async () => {
    const { container, onSave, onEmpty } = mount()
    const ta = await open(container)
    await fireEvent.input(ta, { target: { value: 'Changed' } })
    await fireEvent.keyDown(ta, { key: 'Escape' })
    expect(onSave).not.toHaveBeenCalled()
    expect(onEmpty).not.toHaveBeenCalled()
    expect(container.querySelector('.display')?.textContent).toContain('Buy milk')
  })

  it('a fresh blank row cancelled with Escape is dropped', async () => {
    const { container, onEmpty } = mount({ text: '', autoEdit: true })
    await tick()
    const ta = container.querySelector('textarea') as HTMLTextAreaElement
    await fireEvent.keyDown(ta, { key: 'Escape' })
    expect(onEmpty).toHaveBeenCalled()
  })

  it('keeps both lines of a multi-line item in the rendered display', () => {
    const { container } = mount({ text: 'Line one\nLine two' })
    const display = container.querySelector('.display') as HTMLElement
    expect(display.textContent).toContain('Line one')
    expect(display.textContent).toContain('Line two')
  })
})
