import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent, screen, waitFor } from '@testing-library/svelte'
import EditableText from './EditableText.svelte'

// EditableText is the shared/inlineEdit.ts action's most direct consumer —
// see UI-CONVENTIONS "Keyboard entry". These tests exercise the contract
// through the real component: click-to-edit, Enter-to-commit (with
// Shift+Enter as a real newline in multiline mode), and Escape-to-revert.
describe('EditableText', () => {
  it('renders the value in display mode by default (no input mounted)', () => {
    render(EditableText, { props: { value: 'Hello world' } })
    expect(screen.getByText('Hello world')).toBeInTheDocument()
    expect(screen.queryByRole('textbox')).toBeNull()
  })

  it('clicking the display span enters edit mode', async () => {
    const { container } = render(EditableText, { props: { value: 'Hello world' } })
    await fireEvent.click(container.querySelector('.editable-display') as HTMLElement)

    const input = await waitFor(() => {
      const el = container.querySelector('input.inline-edit-input') as HTMLInputElement
      expect(el).toBeTruthy()
      return el
    })
    expect(input.value).toBe('Hello world')
  })

  describe('single-line', () => {
    it('Enter commits the draft via onSave and exits edit mode', async () => {
      const onSave = vi.fn()
      const { container } = render(EditableText, { props: { value: 'Original', onSave } })
      await fireEvent.click(container.querySelector('.editable-display') as HTMLElement)
      const input = container.querySelector('input.inline-edit-input') as HTMLInputElement

      await fireEvent.input(input, { target: { value: 'Updated' } })
      await fireEvent.keyDown(input, { key: 'Enter' })

      expect(onSave).toHaveBeenCalledWith('Updated')
      await waitFor(() => {
        expect(container.querySelector('input.inline-edit-input')).toBeNull()
      })
    })

    it('Escape reverts the draft, calls onCancel, and does not save', async () => {
      const onSave = vi.fn()
      const onCancel = vi.fn()
      const { container } = render(EditableText, { props: { value: 'Original', onSave, onCancel } })
      await fireEvent.click(container.querySelector('.editable-display') as HTMLElement)
      const input = container.querySelector('input.inline-edit-input') as HTMLInputElement

      await fireEvent.input(input, { target: { value: 'Discarded' } })
      await fireEvent.keyDown(input, { key: 'Escape' })

      expect(onCancel).toHaveBeenCalledOnce()
      expect(onSave).not.toHaveBeenCalled()
      await waitFor(() => {
        expect(container.querySelector('input.inline-edit-input')).toBeNull()
      })
      expect(screen.getByText('Original')).toBeInTheDocument()
    })
  })

  describe('multiline', () => {
    it('plain Enter commits the draft via onSave', async () => {
      const onSave = vi.fn()
      const { container } = render(EditableText, { props: { value: 'Original', multiline: true, onSave } })
      await fireEvent.click(container.querySelector('.editable-display') as HTMLElement)
      const textarea = container.querySelector('textarea.inline-edit-input') as HTMLTextAreaElement

      await fireEvent.input(textarea, { target: { value: 'Line one' } })
      await fireEvent.keyDown(textarea, { key: 'Enter' })

      expect(onSave).toHaveBeenCalledWith('Line one')
      await waitFor(() => {
        expect(container.querySelector('textarea.inline-edit-input')).toBeNull()
      })
    })

    it('Shift+Enter inserts a newline instead of committing', async () => {
      const onSave = vi.fn()
      const { container } = render(EditableText, { props: { value: 'Original', multiline: true, onSave } })
      await fireEvent.click(container.querySelector('.editable-display') as HTMLElement)
      const textarea = container.querySelector('textarea.inline-edit-input') as HTMLTextAreaElement

      await fireEvent.input(textarea, { target: { value: 'Line one' } })
      const event = new KeyboardEvent('keydown', { key: 'Enter', shiftKey: true, bubbles: true, cancelable: true })
      await fireEvent(textarea, event)

      expect(onSave).not.toHaveBeenCalled()
      expect(event.defaultPrevented).toBe(false) // browser handles the newline
      // Still editing — the field was not committed or closed.
      expect(container.querySelector('textarea.inline-edit-input')).toBe(textarea)
    })

    it('Escape reverts the draft, calls onCancel, and does not save', async () => {
      const onSave = vi.fn()
      const onCancel = vi.fn()
      const { container } = render(EditableText, { props: { value: 'Original', multiline: true, onSave, onCancel } })
      await fireEvent.click(container.querySelector('.editable-display') as HTMLElement)
      const textarea = container.querySelector('textarea.inline-edit-input') as HTMLTextAreaElement

      await fireEvent.input(textarea, { target: { value: 'Discarded multiline edit' } })
      await fireEvent.keyDown(textarea, { key: 'Escape' })

      expect(onCancel).toHaveBeenCalledOnce()
      expect(onSave).not.toHaveBeenCalled()
      await waitFor(() => {
        expect(container.querySelector('textarea.inline-edit-input')).toBeNull()
      })
      expect(screen.getByText('Original')).toBeInTheDocument()
    })
  })

  it('blur commits the draft (edit-in-place)', async () => {
    const onSave = vi.fn()
    const { container } = render(EditableText, { props: { value: 'Original', onSave } })
    await fireEvent.click(container.querySelector('.editable-display') as HTMLElement)
    const input = container.querySelector('input.inline-edit-input') as HTMLInputElement

    await fireEvent.input(input, { target: { value: 'Updated via blur' } })
    await fireEvent.blur(input)

    expect(onSave).toHaveBeenCalledWith('Updated via blur')
  })

  it('saving an unchanged (trimmed) value is a no-op — onSave is not called', async () => {
    const onSave = vi.fn()
    const { container } = render(EditableText, { props: { value: 'Original', onSave } })
    await fireEvent.click(container.querySelector('.editable-display') as HTMLElement)
    const input = container.querySelector('input.inline-edit-input') as HTMLInputElement

    await fireEvent.keyDown(input, { key: 'Enter' })

    expect(onSave).not.toHaveBeenCalled()
  })
})
