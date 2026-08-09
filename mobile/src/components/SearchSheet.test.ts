import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, fireEvent, waitFor } from '@testing-library/svelte'

// REGRESSION (2026-08-09): tapping a search result used to onClose()
// then navigate() — but navigate()'s pushState runs inside a view
// transition (async on Chrome/Android) while the sheet's unmount
// cleanup runs earlier, still saw its own {search:true} history entry,
// and queued a history.back() that popped the JUST-pushed card entry.
// The user landed on the sheet's synthetic entry, whose URL is the
// underlying page — the browse tree. On non-VT browsers the ordering
// flipped and the entry leaked instead (one dead Back press per
// search). The fix is PromoteCardSheet's navigatedAway pattern +
// replace(): the sheet's entry is consumed in place by the card URL and
// the cleanup never fires a back for a navigation-close. These tests
// pin that contract.

const repoRPC = vi.fn()
const replace = vi.fn()

vi.mock('../lib/auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/auth')>()
  return { ...actual, repoRPC: (...args: unknown[]) => repoRPC(...args) }
})

vi.mock('../lib/router.svelte', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/router.svelte')>()
  return { ...actual, replace: (...args: unknown[]) => replace(...args) }
})

const SearchSheet = (await import('./SearchSheet.svelte')).default

const HIT = { CardID: 'card-7', Title: 'Found me', Type: '', Rank: 1, ProjectContext: 'Brand / Stream / Project' }

async function mountWithResult() {
  const onClose = vi.fn()
  repoRPC.mockResolvedValue([HIT])
  const utils = render(SearchSheet, { props: { onClose } })
  const input = utils.container.querySelector('input')!
  await fireEvent.input(input, { target: { value: 'found' } })
  // 200ms debounce → SearchCards → result row renders.
  await waitFor(() => expect(utils.container.querySelector('button.result')).not.toBeNull())
  return { onClose, ...utils }
}

beforeEach(() => {
  vi.clearAllMocks()
  window.history.replaceState({}, '', '/m/')
})

describe('SearchSheet result navigation', () => {
  it('tapping a result closes the sheet and REPLACES the entry with the card URL', async () => {
    const { onClose, container } = await mountWithResult()
    await fireEvent.click(container.querySelector('button.result')!)
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(replace).toHaveBeenCalledTimes(1)
    expect(replace).toHaveBeenCalledWith('/c/card-7')
  })

  it('unmount after a result-tap does NOT fire history.back (navigatedAway)', async () => {
    const back = vi.spyOn(window.history, 'back').mockImplementation(() => {})
    const { container, unmount } = await mountWithResult()
    await fireEvent.click(container.querySelector('button.result')!)
    unmount()
    expect(back).not.toHaveBeenCalled()
    back.mockRestore()
  })

  it('unmount WITHOUT navigating still pops the sheet entry (normal close)', async () => {
    const back = vi.spyOn(window.history, 'back').mockImplementation(() => {})
    const onClose = vi.fn()
    const { unmount } = render(SearchSheet, { props: { onClose } })
    expect(window.history.state?.search).toBe(true)
    unmount()
    expect(back).toHaveBeenCalledTimes(1)
    back.mockRestore()
  })
})
