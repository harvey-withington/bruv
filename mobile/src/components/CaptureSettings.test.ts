import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import type { CapturePrefs } from '@shared/types'

// Settings → Capture edits the VAULT's defaults (Harvey, 2026-08-02:
// the phone and the desktop clipper capture into the same vault, so they
// must agree). What matters here is that what the user sees is what the
// server holds, and that a failed save says so instead of pretending.

const repoRPC = vi.fn()
const showToast = vi.fn()

vi.mock('../lib/auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/auth')>()
  return { ...actual, repoRPC: (...args: unknown[]) => repoRPC(...args) }
})
vi.mock('../lib/toast.svelte', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/toast.svelte')>()
  return { ...actual, showToast: (...args: unknown[]) => showToast(...args) }
})

const CaptureSettings = (await import('./CaptureSettings.svelte')).default

const stored: CapturePrefs = {
  videoMode: 'best',
  videoBudgetMB: 120,
  imageMode: 'first',
  askMode: 'always',
  triggers: { videoOverMB: 64, galleryOverCount: 4, unsupportedUrl: true, blocked: false },
}

function stub(over: Record<string, (params: unknown[]) => Promise<unknown>> = {}) {
  repoRPC.mockImplementation((method: string, params: unknown[] = []) => {
    const h = over[method]
    if (h) return h(params)
    if (method === 'GetCapturePrefs') return Promise.resolve(stored)
    return Promise.resolve(undefined)
  })
}

function savedPrefs(): CapturePrefs {
  const call = repoRPC.mock.calls.find((c) => c[0] === 'SetCapturePrefs')
  return (call![1] as CapturePrefs[])[0]
}

beforeEach(() => {
  repoRPC.mockReset()
  showToast.mockReset()
})

describe('CaptureSettings', () => {
  it("shows the vault's stored defaults, not local guesses", async () => {
    stub()
    render(CaptureSettings)

    expect(await screen.findByRole('radio', { name: /^Always the best quality/ })).toBeChecked()
    expect(screen.getByRole('radio', { name: /^Store the first image only/ })).toBeChecked()
    expect(screen.getByLabelText('Size budget (MB)')).toHaveValue(120)
    expect(screen.getByLabelText('A video is bigger than (MB)')).toHaveValue(64)
    expect(screen.getByLabelText('The platform blocks the server')).not.toBeChecked()
    expect(screen.getByText(/belong to this vault/)).toBeInTheDocument()
  })

  it('saves the edited prefs back to the vault', async () => {
    stub()
    render(CaptureSettings)
    await screen.findByRole('radio', { name: /^Always the best quality/ })

    await fireEvent.click(screen.getByRole('radio', { name: /^Best that fits the budget/ }))
    await fireEvent.input(screen.getByLabelText('A video is bigger than (MB)'), {
      target: { value: '200' },
    })
    // A zero threshold is a real choice: never ask about galleries.
    await fireEvent.input(screen.getByLabelText('A post has more images than'), {
      target: { value: '0' },
    })
    await fireEvent.click(screen.getByLabelText('The platform blocks the server'))
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(savedPrefs()).toBeTruthy())
    expect(savedPrefs()).toMatchObject({
      videoMode: 'fit',
      videoBudgetMB: 120,
      askMode: 'always',
      triggers: { videoOverMB: 200, galleryOverCount: 0, unsupportedUrl: true, blocked: true },
    })
    expect(showToast).toHaveBeenCalledWith('Capture settings saved.', 'success')
  })

  it('surfaces a failed save instead of pretending it stuck', async () => {
    stub({ SetCapturePrefs: () => Promise.reject(new Error('disk full')) })
    render(CaptureSettings)
    await screen.findByRole('button', { name: 'Save' })

    await fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('disk full')
    expect(showToast).not.toHaveBeenCalled()
  })

  it('surfaces a failed load and still offers the defaults', async () => {
    stub({ GetCapturePrefs: () => Promise.reject(new Error('server down')) })
    render(CaptureSettings)

    expect(await screen.findByRole('alert')).toHaveTextContent('server down')
    expect(screen.getByRole('radio', { name: /^Best that fits the budget/ })).toBeChecked()
  })
})
