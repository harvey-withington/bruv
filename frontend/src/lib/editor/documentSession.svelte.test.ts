import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import type { WorkspaceFileStamp } from '@shared/types'
import { DocumentSession, type DocumentSessionHooks } from './documentSession.svelte'
import type { DocumentSource } from './documentSource'

// A fake disk: the session only ever sees it through DocumentSource, so
// external edits are just writes to `disk` that bump the stamp.
function fakeDisk(initial: string) {
  let content = initial
  let version = 1
  const stamp = (): WorkspaceFileStamp => ({ hash: `sha256:v${version}`, size: content.length })
  const source: DocumentSource = {
    path: 'notes/draft.md',
    open: vi.fn(async () => ({ content, stamp: stamp() })),
    stat: vi.fn(async () => stamp()),
    save: vi.fn(async (next: string, expectedHash: string) => {
      if (expectedHash && expectedHash !== stamp().hash) return { diverged: true, stamp: stamp() }
      content = next
      version++
      return { diverged: false, stamp: stamp() }
    }),
  }
  return {
    source,
    read: () => content,
    externalWrite(next: string) { content = next; version++ },
  }
}

function hooks(overrides: Partial<DocumentSessionHooks> = {}): DocumentSessionHooks & { reload: ReturnType<typeof vi.fn>; overwrite: ReturnType<typeof vi.fn>; reloaded: ReturnType<typeof vi.fn> } {
  const reload = vi.fn(async () => true)
  const overwrite = vi.fn(async () => true)
  const reloaded = vi.fn()
  return { confirmReload: reload, confirmOverwrite: overwrite, onReloaded: reloaded, reload, overwrite, reloaded, ...overrides }
}

const flushMicrotasks = () => new Promise<void>(r => setTimeout(r, 0))

describe('DocumentSession', () => {
  beforeEach(() => { vi.useFakeTimers({ shouldAdvanceTime: true }) })
  afterEach(() => { vi.useRealTimers() })

  it('loads content + stamp and starts clean', async () => {
    const disk = fakeDisk('hello')
    const s = new DocumentSession(disk.source, hooks(), 50)
    expect(s.status).toBe('loading')
    await s.load()
    expect(s.status).toBe('ready')
    expect(s.text).toBe('hello')
    expect(s.dirty).toBe(false)
    expect(s.stamp?.hash).toBe('sha256:v1')
  })

  it('reports a load failure instead of throwing', async () => {
    const source: DocumentSource = {
      path: 'x.bin',
      open: async () => { throw new Error('not a text file') },
      stat: async () => ({ size: 0 }),
      save: async () => ({ diverged: false, stamp: { size: 0 } }),
    }
    const s = new DocumentSession(source, hooks(), 50)
    await s.load()
    expect(s.status).toBe('error')
    expect(s.loadError).toBe('not a text file')
  })

  it('autosaves after a quiet interval, presenting the loaded stamp', async () => {
    const disk = fakeDisk('v1')
    const s = new DocumentSession(disk.source, hooks(), 50)
    await s.load()
    s.edit('v1 typed')
    expect(s.dirty).toBe(true)
    expect(disk.source.save).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(60)
    expect(disk.source.save).toHaveBeenCalledWith('v1 typed', 'sha256:v1')
    expect(disk.read()).toBe('v1 typed')
    expect(s.dirty).toBe(false)
    expect(s.stamp?.hash).toBe('sha256:v2')
  })

  it('keeps typing during a save dirty and saves again', async () => {
    const disk = fakeDisk('a')
    let release: () => void = () => {}
    const slowSave = disk.source.save as ReturnType<typeof vi.fn>
    const realSave = slowSave.getMockImplementation()!
    slowSave.mockImplementationOnce(async (next: string, hash: string) => {
      await new Promise<void>(r => { release = r })
      return realSave(next, hash)
    })
    const s = new DocumentSession(disk.source, hooks(), 50)
    await s.load()
    s.edit('ab')
    await vi.advanceTimersByTimeAsync(60)
    expect(s.saving).toBe(true)
    s.edit('abc')
    release()
    await flushMicrotasks()
    expect(s.saving).toBe(false)
    expect(s.dirty).toBe(true)
    await vi.advanceTimersByTimeAsync(60)
    expect(disk.read()).toBe('abc')
    expect(s.dirty).toBe(false)
  })

  it('flush saves immediately and resolves true when clean', async () => {
    const disk = fakeDisk('a')
    const s = new DocumentSession(disk.source, hooks(), 50)
    await s.load()
    expect(await s.flush()).toBe(true)
    s.edit('b')
    expect(await s.flush()).toBe(true)
    expect(disk.read()).toBe('b')
  })

  it('surfaces a failed save inline and retries on the next edit', async () => {
    const disk = fakeDisk('a')
    const save = disk.source.save as ReturnType<typeof vi.fn>
    save.mockRejectedValueOnce(new Error('disk full'))
    const s = new DocumentSession(disk.source, hooks(), 50)
    await s.load()
    s.edit('b')
    expect(await s.flush()).toBe(false)
    expect(s.saveError).toBe('disk full')
    expect(s.dirty).toBe(true)
    s.edit('bc')
    expect(s.saveError).toBe('')
    await vi.advanceTimersByTimeAsync(60)
    expect(disk.read()).toBe('bc')
  })

  describe('external change on focus', () => {
    it('reloads silently when the draft is clean', async () => {
      const disk = fakeDisk('a')
      const h = hooks()
      const s = new DocumentSession(disk.source, h, 50)
      await s.load()
      disk.externalWrite('from obsidian')
      await s.checkExternal()
      expect(s.text).toBe('from obsidian')
      expect(s.dirty).toBe(false)
      expect(h.reloaded).toHaveBeenCalledOnce()
      expect(h.reload).not.toHaveBeenCalled()
    })

    it('does nothing when the stamp still matches', async () => {
      const disk = fakeDisk('a')
      const h = hooks()
      const s = new DocumentSession(disk.source, h, 50)
      await s.load()
      s.edit('ab')
      await s.checkExternal()
      expect(s.text).toBe('ab')
      expect(h.reload).not.toHaveBeenCalled()
      expect(h.reloaded).not.toHaveBeenCalled()
    })

    it('asks before dropping a dirty draft, and reloads on yes', async () => {
      const disk = fakeDisk('a')
      const h = hooks()
      const s = new DocumentSession(disk.source, h, 50)
      await s.load()
      s.edit('mine')
      disk.externalWrite('theirs')
      await s.checkExternal()
      expect(h.reload).toHaveBeenCalledOnce()
      expect(s.text).toBe('theirs')
      expect(s.dirty).toBe(false)
      expect(s.paused).toBe(false)
    })

    it('keeps the draft and pauses autosave on no; the next explicit save asks to overwrite', async () => {
      const disk = fakeDisk('a')
      const h = hooks()
      h.reload.mockResolvedValue(false)
      const s = new DocumentSession(disk.source, h, 50)
      await s.load()
      s.edit('mine')
      disk.externalWrite('theirs')
      await s.checkExternal()
      expect(s.text).toBe('mine')
      expect(s.paused).toBe(true)
      // Autosave must not fire while paused.
      s.edit('mine more')
      await vi.advanceTimersByTimeAsync(100)
      expect(disk.read()).toBe('theirs')
      // Explicit save: the backend refuses the stale stamp → overwrite prompt.
      expect(await s.flush()).toBe(true)
      expect(h.overwrite).toHaveBeenCalledOnce()
      expect(disk.read()).toBe('mine more')
      expect(s.paused).toBe(false)
      expect(s.dirty).toBe(false)
    })

    it('swallows a stat failure', async () => {
      const disk = fakeDisk('a')
      ;(disk.source.stat as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('gone'))
      const s = new DocumentSession(disk.source, hooks(), 50)
      await s.load()
      await expect(s.checkExternal()).resolves.toBeUndefined()
      expect(s.status).toBe('ready')
    })
  })

  describe('diverged save', () => {
    it('overwrites when the user says so', async () => {
      const disk = fakeDisk('a')
      const h = hooks()
      const s = new DocumentSession(disk.source, h, 50)
      await s.load()
      disk.externalWrite('theirs')
      s.edit('mine')
      await vi.advanceTimersByTimeAsync(60)
      expect(h.overwrite).toHaveBeenCalledOnce()
      expect(disk.read()).toBe('mine')
      expect(s.dirty).toBe(false)
      expect(s.stamp?.hash).toBe('sha256:v3')
    })

    it('keeps the draft unsaved and pauses when the user declines', async () => {
      const disk = fakeDisk('a')
      const h = hooks()
      h.overwrite.mockResolvedValue(false)
      const s = new DocumentSession(disk.source, h, 50)
      await s.load()
      disk.externalWrite('theirs')
      s.edit('mine')
      await vi.advanceTimersByTimeAsync(60)
      expect(disk.read()).toBe('theirs')
      expect(s.dirty).toBe(true)
      expect(s.paused).toBe(true)
      // flush reports "not clean" so the shell can confirm a discard on close.
      expect(await s.flush()).toBe(false)
      expect(h.overwrite).toHaveBeenCalledTimes(2)
    })
  })

  it('drops in-flight results after dispose', async () => {
    const disk = fakeDisk('a')
    const s = new DocumentSession(disk.source, hooks(), 50)
    await s.load()
    s.edit('b')
    s.dispose()
    await vi.advanceTimersByTimeAsync(100)
    expect(disk.source.save).not.toHaveBeenCalled()
  })
})
