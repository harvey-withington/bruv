import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import { tick } from 'svelte'
import type { GitServeReport, Workspace, WorkspaceCheckoutInfo } from '@shared/types'

// The component's whole job is telling the truth about a two-machine
// lifecycle: the host prepares, then this device clones. Every state has to
// be distinguishable on screen, because the failure mode this feature exists
// to fix was a workspace that looked fine and had no files behind it.

const api = vi.hoisted(() => ({
  GetWorkspaceCheckout: vi.fn(),
  InspectWorkspaceGitServe: vi.fn(),
  EnableWorkspaceGitServe: vi.fn(),
  DisableWorkspaceGitServe: vi.fn(),
  MaterializeWorkspace: vi.fn(),
  PullWorkspaceCheckout: vi.fn(),
  PushWorkspaceCheckout: vi.fn(),
  ForgetWorkspaceCheckout: vi.fn(),
  OpenWorkspacePath: vi.fn(),
}))
vi.mock('@shared/api', () => api)

const confirmed = vi.hoisted(() => ({ answer: true }))
vi.mock('../../lib/confirm.svelte', () => ({
  showConfirm: vi.fn(async () => confirmed.answer),
}))
vi.mock('../../lib/toast.svelte', () => ({ showToast: vi.fn() }))

const { default: WorkspaceLocalCopy } = await import('./WorkspaceLocalCopy.svelte')
const { showConfirm } = await import('../../lib/confirm.svelte')

function workspace(overrides: Partial<Workspace> = {}): Workspace {
  return {
    id: 'ws-1',
    project_id: 'p-1',
    origin: { kind: 'local', url: 'D:\\work\\song-alpha' },
    adapter: 'plain-folder',
    created_at: '',
    updated_at: '',
    ...overrides,
  }
}

function checkout(overrides: Partial<WorkspaceCheckoutInfo> = {}): WorkspaceCheckoutInfo {
  return {
    workspace_id: 'ws-1',
    has_copy: false,
    status: 'idle',
    dirty: false,
    ahead: 0,
    behind: 0,
    git_available: true,
    ...overrides,
  }
}

function report(overrides: Partial<GitServeReport> = {}): GitServeReport {
  return {
    path: 'D:\\work\\song-alpha',
    git_available: true,
    state: '',
    is_repo: false,
    has_commits: false,
    uncommitted_paths: 0,
    has_gitignore: true,
    files: 1200,
    bytes: 3_200_000_000,
    truncated: false,
    ...overrides,
  }
}

function mount(ws: Workspace) {
  return render(WorkspaceLocalCopy, {
    props: {
      ws,
      brandSlug: 'acme',
      streamSlug: 'films',
      projectSlug: 'song-alpha',
      serverName: 'RIPPED',
    },
  })
}

/** Lets the mount effect's fetch settle and the DOM update. */
async function settle() {
  await tick()
  await Promise.resolve()
  await tick()
}

beforeEach(() => {
  vi.clearAllMocks()
  confirmed.answer = true
  api.GetWorkspaceCheckout.mockResolvedValue(checkout())
  api.InspectWorkspaceGitServe.mockResolvedValue(report())
  api.EnableWorkspaceGitServe.mockResolvedValue(workspace({ git_serve: 'initializing' }))
  api.MaterializeWorkspace.mockResolvedValue(undefined)
})

describe('WorkspaceLocalCopy — lifecycle states', () => {
  it('offers set-up when the host has not published', async () => {
    mount(workspace())
    await settle()
    expect(screen.getByText('Set up local copy')).toBeTruthy()
  })

  it('says the host is preparing rather than looking idle', async () => {
    mount(workspace({ git_serve: 'initializing' }))
    await settle()
    expect(screen.getByText(/Preparing the workspace on RIPPED/)).toBeTruthy()
    expect(screen.queryByText('Set up local copy')).toBeNull()
  })

  it('offers the clone once the host is ready', async () => {
    mount(workspace({ git_serve: 'ready' }))
    await settle()
    expect(screen.getByText('Clone to this device')).toBeTruthy()
  })

  it('surfaces the host failure instead of an empty panel', async () => {
    mount(workspace({ git_serve: 'error', git_serve_error: 'git init: permission denied' }))
    await settle()
    expect(screen.getByText('git init: permission denied')).toBeTruthy()
  })

  it('shows git progress while cloning', async () => {
    api.GetWorkspaceCheckout.mockResolvedValue(
      checkout({ status: 'cloning', progress: 'Receiving objects: 45% (450/1000)' }),
    )
    mount(workspace({ git_serve: 'ready' }))
    await settle()
    expect(screen.getByText('Receiving objects: 45% (450/1000)')).toBeTruthy()
  })

  it('reports the copy and its divergence once it exists', async () => {
    api.GetWorkspaceCheckout.mockResolvedValue(
      checkout({ has_copy: true, local_path: 'C:\\bruv-workspaces\\song-alpha', branch: 'main', dirty: true, ahead: 2 }),
    )
    mount(workspace({ git_serve: 'ready' }))
    await settle()
    expect(screen.getByText('C:\\bruv-workspaces\\song-alpha')).toBeTruthy()
    expect(screen.getByText('uncommitted changes')).toBeTruthy()
    expect(screen.getByText('2 to send')).toBeTruthy()
  })

  it('says so plainly when this device has no git', async () => {
    api.GetWorkspaceCheckout.mockResolvedValue(checkout({ git_available: false }))
    mount(workspace())
    await settle()
    expect(screen.getByText(/git isn't installed on this device/)).toBeTruthy()
    expect(screen.queryByText('Set up local copy')).toBeNull()
  })
})

describe('WorkspaceLocalCopy — set-up asks before it commits anything', () => {
  it('names the size of the initial commit in the confirmation', async () => {
    mount(workspace())
    await settle()
    screen.getByText('Set up local copy').click()
    await settle()

    // The number is the point: a folder with no .gitignore can be far
    // larger than the user expects, and finding out afterwards is too late.
    const asked = vi.mocked(showConfirm).mock.calls[0][0]
    expect(asked).toContain('1,200 files')
    expect(asked).toContain('3.0 GB')
    expect(api.EnableWorkspaceGitServe).toHaveBeenCalled()
  })

  it('warns that uncommitted work on the host will not come across', async () => {
    api.InspectWorkspaceGitServe.mockResolvedValue(report({ is_repo: true, has_commits: true, uncommitted_paths: 4 }))
    mount(workspace())
    await settle()
    screen.getByText('Set up local copy').click()
    await settle()
    expect(vi.mocked(showConfirm).mock.calls[0][0]).toContain('4 uncommitted paths')
  })

  it('publishes nothing when the user declines', async () => {
    confirmed.answer = false
    mount(workspace())
    await settle()
    screen.getByText('Set up local copy').click()
    await settle()
    expect(api.EnableWorkspaceGitServe).not.toHaveBeenCalled()
  })

  it('clones by itself once the host reports ready — one button, not two', async () => {
    const view = mount(workspace())
    await settle()
    screen.getByText('Set up local copy').click()
    await settle()
    expect(api.MaterializeWorkspace).not.toHaveBeenCalled()

    await view.rerender({ ws: workspace({ git_serve: 'ready' }) })
    await settle()
    expect(api.MaterializeWorkspace).toHaveBeenCalledWith('ws-1', 'acme', 'films', 'song-alpha', '')
  })

  it('does not start a clone the user never asked for', async () => {
    // A workspace already published by another device must not pull itself
    // onto this one just because the panel was opened.
    mount(workspace({ git_serve: 'ready' }))
    await settle()
    expect(api.MaterializeWorkspace).not.toHaveBeenCalled()
  })
})

describe('WorkspaceLocalCopy — forgetting never deletes', () => {
  it('confirms with the path and only drops the record', async () => {
    api.GetWorkspaceCheckout.mockResolvedValue(
      checkout({ has_copy: true, local_path: 'C:\\bruv-workspaces\\song-alpha', branch: 'main' }),
    )
    mount(workspace({ git_serve: 'ready' }))
    await settle()
    screen.getByText('Forget').click()
    await settle()
    expect(vi.mocked(showConfirm).mock.calls[0][0]).toContain('C:\\bruv-workspaces\\song-alpha')
    expect(api.ForgetWorkspaceCheckout).toHaveBeenCalledWith('ws-1')
  })
})
