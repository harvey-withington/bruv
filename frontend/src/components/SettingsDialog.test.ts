import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import SettingsDialog from './SettingsDialog.svelte'
import { createMockAdapter } from '../lib/adapters/mock'
import { setBackend } from '@shared/adapters'
import type { LLMAccount, LLMRouting } from '@shared/types'

const accounts: LLMAccount[] = [
  { id: 'a1', label: 'Claude', provider: 'anthropic', api_key: '', base_url: '', model: 'claude-sonnet-5', is_default: true },
]
// No `tasks` key — what an older server (or a nil Go map) sends.
const routing = {
  models: [{ id: 'm1', account_id: 'a1', name: 'claude-sonnet-5', tier: 'balanced', supports_tools: true, enabled: true }],
  routers: [],
  default: 'model:m1',
} as unknown as LLMRouting

let adapter: ReturnType<typeof createMockAdapter>

beforeEach(() => {
  adapter = createMockAdapter()
  adapter.GetLLMAccounts = vi.fn(async () => accounts)
  adapter.GetLLMRouting = vi.fn(async () => routing)
  setBackend(adapter)
})

describe('SettingsDialog AI tab', () => {
  // Regression (2026-09-26): the backend omitted an empty `tasks` map, so
  // the Use-for table indexed undefined and the dialog broke on open.
  it('renders when the routing response has no tasks map', async () => {
    render(SettingsDialog, { props: { onClose: () => {}, initialTab: 'ai' } })
    await waitFor(() => expect(screen.getByText('Claude')).toBeInTheDocument())
    expect(screen.getByText('Card chat')).toBeInTheDocument()
  })
})

// A section that failed to load is never saved (lib/settingsSections).
describe('SettingsDialog load failures', () => {
  it('shows the error, keeps other sections working, and never saves the failed one', async () => {
    adapter.GetLLMRouting = vi.fn(async () => { throw new Error('boom') })
    adapter.SaveLLMAccounts = vi.fn(async () => {})
    adapter.SaveLLMRouting = vi.fn(async () => {})
    adapter.SetLLMConfig = vi.fn(async () => {})
    const onClose = vi.fn()
    render(SettingsDialog, { props: { onClose, initialTab: 'ai' } })

    expect(await screen.findByText('boom')).toBeInTheDocument()
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(adapter.SetLLMConfig).toHaveBeenCalled()
    expect(adapter.SaveLLMAccounts).not.toHaveBeenCalled()
    expect(adapter.SaveLLMRouting).not.toHaveBeenCalled()
  })

  it('Try again reloads the failed section', async () => {
    let fail = true
    adapter.GetLLMRouting = vi.fn(async () => { if (fail) throw new Error('boom'); return routing })
    render(SettingsDialog, { props: { onClose: () => {}, initialTab: 'ai' } })
    await screen.findByText('boom')
    fail = false
    await fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByText('Claude')).toBeInTheDocument()
  })

  it('a failed save keeps the dialog open', async () => {
    adapter.SetLLMConfig = vi.fn(async () => { throw new Error('nope') })
    const onClose = vi.fn()
    render(SettingsDialog, { props: { onClose, initialTab: 'ai' } })
    await screen.findByText('Claude')
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(adapter.SetLLMConfig).toHaveBeenCalled())
    expect(onClose).not.toHaveBeenCalled()
  })
})
