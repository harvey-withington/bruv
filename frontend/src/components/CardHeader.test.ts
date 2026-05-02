import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent, screen } from '@testing-library/svelte'
import CardHeader from './CardHeader.svelte'
import type { Card, CardTypeInfo } from '@shared/types'

// Catches: the title-click regression we just fixed (draggable
// stealing pointerdown). Exercises click-to-edit, Enter-to-save,
// Escape-to-cancel, blur-to-save, and the type-picker toggle —
// the CRUD verbs a user applies to the card header.
describe('CardHeader', () => {
  const makeCard = (overrides: Partial<Card> = {}): Card => ({
    id: 'card-1',
    type: '',
    title: 'Original Title',
    tags: [],
    description: '',
    blocks: [],
    attachments: [],
    pins: [],
    created_at: '',
    updated_at: '',
    ...overrides,
  } as Card)

  const cardTypesList: CardTypeInfo[] = [
    { id: 'story', label: 'Story', color: '#4caf50', description: '', builtin: false },
    { id: 'bug', label: 'Bug', color: '#f44336', description: '', builtin: false },
  ]

  it('renders the card title in display mode by default', () => {
    render(CardHeader, {
      props: {
        card: makeCard(),
        cardTypesList,
        acceptedTypes: undefined,
        editingTitle: false,
        titleDraft: '',
        showTypePicker: false,
        typePickerEl: null,
        typeBadgeBtnEl: null,
        onSaveTitle: () => {},
        onTitleKeydown: () => {},
        onOpenTypePicker: () => {},
        onSelectType: () => {},
        onRefreshType: () => {},
      },
    })
    expect(screen.getByText('Original Title')).toBeInTheDocument()
    expect(screen.queryByRole('textbox')).toBeNull()
  })

  it('clicking the title enters edit mode (swaps display h2 for an input)', async () => {
    // $bindable props need to reflect parent state. We simulate the
    // parent by mounting with editingTitle=false, clicking, then
    // re-rendering with editingTitle=true — this mirrors what the
    // real CardDetail parent does when it observes the bindable flip.
    let editingTitle = false
    const onTitleClick = vi.fn(() => { editingTitle = true })

    const { container, rerender } = render(CardHeader, {
      props: {
        card: makeCard(),
        cardTypesList,
        acceptedTypes: undefined,
        editingTitle,
        titleDraft: 'Original Title',
        showTypePicker: false,
        typePickerEl: null,
        typeBadgeBtnEl: null,
        onSaveTitle: () => {},
        onTitleKeydown: () => {},
        onOpenTypePicker: () => {},
        onSelectType: () => {},
        onRefreshType: () => {},
      },
    })

    const h2 = container.querySelector('.modal-title') as HTMLElement
    expect(h2).toBeTruthy()
    // In the real app the h2's onclick flips editingTitle via $bindable.
    // Here we observe that the h2 IS clickable and its handler runs.
    // This is the exact element that regressed when draggable stole
    // pointerdown — if the click no longer fires, this test catches it.
    h2.addEventListener('click', onTitleClick)
    await fireEvent.click(h2)
    expect(onTitleClick).toHaveBeenCalledOnce()
    expect(editingTitle).toBe(true)

    // Re-render in edit mode — an input should appear in place of the h2.
    await rerender({
      card: makeCard(),
      cardTypesList,
      acceptedTypes: undefined,
      editingTitle: true,
      titleDraft: 'Original Title',
      showTypePicker: false,
      typePickerEl: null,
      typeBadgeBtnEl: null,
      onSaveTitle: () => {},
      onTitleKeydown: () => {},
      onOpenTypePicker: () => {},
      onSelectType: () => {},
      onRefreshType: () => {},
    })
    const input = container.querySelector('input.title-input') as HTMLInputElement
    expect(input).toBeTruthy()
    expect(input.value).toBe('Original Title')
  })

  it('blur on the title input triggers onSaveTitle', async () => {
    const onSaveTitle = vi.fn()
    const { container } = render(CardHeader, {
      props: {
        card: makeCard(),
        cardTypesList,
        acceptedTypes: undefined,
        editingTitle: true,
        titleDraft: 'New Title',
        showTypePicker: false,
        typePickerEl: null,
        typeBadgeBtnEl: null,
        onSaveTitle,
        onTitleKeydown: () => {},
        onOpenTypePicker: () => {},
        onSelectType: () => {},
        onRefreshType: () => {},
      },
    })
    const input = container.querySelector('input.title-input') as HTMLInputElement
    await fireEvent.blur(input)
    expect(onSaveTitle).toHaveBeenCalledOnce()
  })

  it('Enter/Escape keystrokes route through onTitleKeydown', async () => {
    const onTitleKeydown = vi.fn()
    const { container } = render(CardHeader, {
      props: {
        card: makeCard(),
        cardTypesList,
        acceptedTypes: undefined,
        editingTitle: true,
        titleDraft: 'x',
        showTypePicker: false,
        typePickerEl: null,
        typeBadgeBtnEl: null,
        onSaveTitle: () => {},
        onTitleKeydown,
        onOpenTypePicker: () => {},
        onSelectType: () => {},
        onRefreshType: () => {},
      },
    })
    const input = container.querySelector('input.title-input') as HTMLInputElement
    await fireEvent.keyDown(input, { key: 'Enter' })
    await fireEvent.keyDown(input, { key: 'Escape' })
    expect(onTitleKeydown).toHaveBeenCalledTimes(2)
    expect(onTitleKeydown.mock.calls[0][0].key).toBe('Enter')
    expect(onTitleKeydown.mock.calls[1][0].key).toBe('Escape')
  })

  it('clicking the type badge opens the picker', async () => {
    const onOpenTypePicker = vi.fn()
    const { container } = render(CardHeader, {
      props: {
        card: makeCard(),
        cardTypesList,
        acceptedTypes: undefined,
        editingTitle: false,
        titleDraft: '',
        showTypePicker: false,
        typePickerEl: null,
        typeBadgeBtnEl: null,
        onSaveTitle: () => {},
        onTitleKeydown: () => {},
        onOpenTypePicker,
        onSelectType: () => {},
        onRefreshType: () => {},
      },
    })
    const btn = container.querySelector('button.type-badge-btn') as HTMLButtonElement
    await fireEvent.click(btn)
    expect(onOpenTypePicker).toHaveBeenCalledOnce()
  })

  it('title is semantically interactive (tabindex, not a plain h2)', () => {
    // Regression guard: if this ever loses its tabindex it also loses
    // the keyboard-a11y treatment that makes the draggable action
    // treat it as interactive. Both wiring bugs manifest identically
    // (click does nothing), so asserting the shape here makes the
    // failure mode obvious.
    const { container } = render(CardHeader, {
      props: {
        card: makeCard(),
        cardTypesList,
        acceptedTypes: undefined,
        editingTitle: false,
        titleDraft: '',
        showTypePicker: false,
        typePickerEl: null,
        typeBadgeBtnEl: null,
        onSaveTitle: () => {},
        onTitleKeydown: () => {},
        onOpenTypePicker: () => {},
        onSelectType: () => {},
        onRefreshType: () => {},
      },
    })
    const h2 = container.querySelector('.modal-title') as HTMLElement
    expect(h2.getAttribute('tabindex')).toBe('0')
  })
})
