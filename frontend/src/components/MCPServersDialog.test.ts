import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import MCPServersDialog from './MCPServersDialog.svelte'
import { createMockAdapter } from '../lib/adapters/mock'
import { setBackend } from '@shared/adapters'
import { confirmState, resolveConfirm } from '../lib/confirm.svelte'
import type { MCPServerView } from '@shared/types'

// A shared repo's server arrives enabled but unapproved on this device:
// the dialog must say so, and Approve must show the exact command before
// approving the fingerprint the user saw.
const unapproved: MCPServerView = {
  spec: { name: 'shared', command: 'npx', args: ['-y', 'some-server'], env_names: ['API_TOKEN'], enabled: true },
  health: { name: 'shared', status: 'unapproved', tool_count: 0 },
  tools: [],
  fingerprint: 'fp-shown',
}

let adapter: ReturnType<typeof createMockAdapter>

beforeEach(() => {
  adapter = createMockAdapter()
  adapter.ListMCPServers = vi.fn(async () => [unapproved])
  adapter.ApproveMCPServer = vi.fn(async () => {})
  setBackend(adapter)
})

describe('MCPServersDialog unapproved server', () => {
  it('shows the not-approved notice and approves only after confirming the command', async () => {
    render(MCPServersDialog, { props: { onClose: () => {} } })
    await waitFor(() => expect(screen.getByText(/Not approved on this device/)).toBeInTheDocument())

    await fireEvent.click(screen.getByRole('button', { name: 'Approve' }))
    expect(confirmState.visible).toBe(true)
    expect(confirmState.message).toContain('npx -y some-server')
    expect(confirmState.message).toContain('API_TOKEN')
    expect(adapter.ApproveMCPServer).not.toHaveBeenCalled()

    resolveConfirm(true)
    await waitFor(() => expect(adapter.ApproveMCPServer).toHaveBeenCalledWith('shared', 'fp-shown'))
  })

  it('does not approve when the confirmation is cancelled', async () => {
    render(MCPServersDialog, { props: { onClose: () => {} } })
    await waitFor(() => expect(screen.getByRole('button', { name: 'Approve' })).toBeInTheDocument())
    await fireEvent.click(screen.getByRole('button', { name: 'Approve' }))
    resolveConfirm(false)
    await Promise.resolve()
    expect(adapter.ApproveMCPServer).not.toHaveBeenCalled()
  })
})
