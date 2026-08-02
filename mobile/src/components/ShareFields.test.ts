import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import ShareFields from './ShareFields.svelte'

// The chips are the share page's only "what will Save actually do?"
// signal. Plain mode used to be silent, so an unsupported platform
// looked identical to a supported one until the card came out bare
// (2026-08-02). These tests pin down which chip shows when, and that
// neither shows while we still don't know.

type Props = {
  platformLabel: string
  unsupportedHost?: string
  title: string
  text: string
  url: string
  disabled?: boolean
  onPasteError: (message: string | null) => void
}

function mount(over: Partial<Props> = {}) {
  const props: Props = {
    platformLabel: '',
    unsupportedHost: '',
    title: '',
    text: '',
    url: 'https://example.com/a',
    onPasteError: vi.fn(),
    ...over,
  }
  return render(ShareFields, { props })
}

describe('ShareFields chips', () => {
  it('shows the platform chip and clip hint when the URL is clippable', () => {
    mount({ platformLabel: 'Youtube' })
    expect(screen.getByText('Youtube')).toBeInTheDocument()
    expect(screen.getByText('BRUV can capture this post for you.')).toBeInTheDocument()
    expect(screen.queryByText('Link')).toBeNull()
  })

  it('shows the Link chip and the host hint when no plugin claims the URL', () => {
    mount({ unsupportedHost: 'instagram.com' })
    expect(screen.getByText('Link')).toBeInTheDocument()
    expect(
      screen.getByText('No capture plugin for instagram.com yet — saving as a link card.'),
    ).toBeInTheDocument()
    expect(screen.queryByText('BRUV can capture this post for you.')).toBeNull()
  })

  it('shows NEITHER chip while the verdict is unknown (both props empty)', () => {
    // Guards the real bug's other half: an unchecked or failed preflight
    // must not be dressed up as "unsupported".
    mount({ platformLabel: '', unsupportedHost: '' })
    expect(screen.queryByText('Link')).toBeNull()
    expect(screen.queryByText('BRUV can capture this post for you.')).toBeNull()
    expect(
      screen.queryByText(/No capture plugin for .* yet — saving as a link card\./),
    ).toBeNull()
  })

  it('prefers the platform chip when both props somehow arrive set', () => {
    mount({ platformLabel: 'Reddit', unsupportedHost: 'reddit.com' })
    expect(screen.getByText('Reddit')).toBeInTheDocument()
    expect(screen.queryByText('Link')).toBeNull()
  })
})

describe('ShareFields inputs', () => {
  it('offers only the URL field in clip mode (the server writes title and body)', () => {
    mount({ platformLabel: 'Youtube', title: 'seeded title', text: 'seeded text' })
    expect(screen.getByLabelText('URL')).toBeInTheDocument()
    expect(screen.queryByText('Title')).toBeNull()
    expect(screen.queryByText('Text')).toBeNull()
    expect(screen.queryByDisplayValue('seeded title')).toBeNull()
  })

  it('offers title and text alongside the URL in plain mode', () => {
    mount({ platformLabel: '', title: 'seeded title', text: 'seeded text' })
    expect(screen.getByLabelText('URL')).toBeInTheDocument()
    expect(screen.getByText('Title')).toBeInTheDocument()
    expect(screen.getByText('Text')).toBeInTheDocument()
    expect(screen.getByDisplayValue('seeded title')).toBeInTheDocument()
    expect(screen.getByDisplayValue('seeded text')).toBeInTheDocument()
  })

  it('keeps title and text hidden in the unsupported-host case only if clipping', () => {
    // An unsupported host still saves as a plain card, so the plain
    // fields must stay available — the Link chip is information, not a
    // mode switch.
    mount({ unsupportedHost: 'instagram.com', title: 'seeded title', text: 'seeded text' })
    expect(screen.getByText('Title')).toBeInTheDocument()
    expect(screen.getByText('Text')).toBeInTheDocument()
  })

  it('renders the shared URL in the URL field', () => {
    mount({ url: 'https://youtu.be/abc' })
    expect(screen.getByDisplayValue('https://youtu.be/abc')).toBeInTheDocument()
  })

  it('disables every control while a save is in flight', () => {
    mount({ disabled: true })
    expect(screen.getByLabelText('URL')).toBeDisabled()
    expect(screen.getByRole('button', { name: /Paste/ })).toBeDisabled()
  })
})
