import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { createCardSaveQueue } from './cardSaveQueue'

interface Deferred {
  promise: Promise<void>
  resolve: () => void
  reject: (err: unknown) => void
}

function deferred(): Deferred {
  let resolve!: () => void
  let reject!: (err: unknown) => void
  const promise = new Promise<void>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

/** A queue over a mutable string[] whose persist calls are held open. */
function setup() {
  let value: string[] | null = []
  const calls: { value: string[]; d: Deferred }[] = []
  const onError = vi.fn()
  const saving: boolean[] = []
  const queue = createCardSaveQueue<string[]>({
    read: () => value,
    persist: (v) => {
      const d = deferred()
      calls.push({ value: v, d })
      return d.promise
    },
    onError,
    onSavingChange: (s) => saving.push(s),
  })
  return {
    queue,
    calls,
    onError,
    saving,
    set: (v: string[] | null) => (value = v),
  }
}

const flush = () => new Promise<void>((r) => setTimeout(r, 0))

describe('createCardSaveQueue', () => {
  beforeEach(() => vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] }))
  afterEach(() => vi.useRealTimers())

  it('debounces scheduled edits into one save of the latest value', () => {
    const s = setup()
    s.set(['a'])
    s.queue.schedule()
    s.set(['a', 'b'])
    s.queue.schedule()
    expect(s.calls).toHaveLength(0)
    expect(s.queue.busy).toBe(true)
    vi.advanceTimersByTime(200)
    expect(s.calls.map((c) => c.value)).toEqual([['a', 'b']])
  })

  it('keeps one save in flight and coalesces the rest to the latest value', async () => {
    const s = setup()
    s.set(['1'])
    void s.queue.saveNow()
    s.set(['1', '2'])
    void s.queue.saveNow()
    s.set(['1', '2', '3'])
    void s.queue.saveNow()
    expect(s.calls).toHaveLength(1)
    s.calls[0].d.resolve()
    vi.useRealTimers()
    await flush()
    expect(s.calls.map((c) => c.value)).toEqual([['1'], ['1', '2', '3']])
    s.calls[1].d.resolve()
    await s.queue.idle()
    expect(s.queue.busy).toBe(false)
    expect(s.saving.at(-1)).toBe(false)
  })

  it('refuses a refetch that overlapped a local edit or an in-flight save', async () => {
    const s = setup()
    const since = s.queue.edits
    expect(s.queue.canApplyRefetch(since)).toBe(true)
    s.set(['tick'])
    s.queue.schedule()
    expect(s.queue.canApplyRefetch(since)).toBe(false)
    vi.advanceTimersByTime(200)
    // Save in flight: even a refetch started now can't apply.
    expect(s.queue.canApplyRefetch(s.queue.edits)).toBe(false)
    s.calls[0].d.resolve()
    vi.useRealTimers()
    await s.queue.idle()
    expect(s.queue.canApplyRefetch(since)).toBe(false)
    expect(s.queue.canApplyRefetch(s.queue.edits)).toBe(true)
    s.queue.markEdit()
    expect(s.queue.canApplyRefetch(since + 1)).toBe(false)
  })

  it('routes failures to onError with a retry of the current value', async () => {
    const s = setup()
    s.set(['x'])
    void s.queue.saveNow()
    const err = new Error('nope')
    s.calls[0].d.reject(err)
    vi.useRealTimers()
    await flush()
    expect(s.onError).toHaveBeenCalledTimes(1)
    expect(s.onError.mock.calls[0][0]).toBe(err)
    s.set(['x', 'y'])
    void s.onError.mock.calls[0][1]()
    expect(s.calls.at(-1)?.value).toEqual(['x', 'y'])
  })

  describe('leave', () => {
    it('flushes a pending debounce immediately', () => {
      const s = setup()
      s.set(['typed'])
      s.queue.schedule()
      const p = s.queue.leave()
      expect(p).not.toBeNull()
      expect(s.calls.map((c) => c.value)).toEqual([['typed']])
      vi.advanceTimersByTime(500)
      expect(s.calls).toHaveLength(1)
    })

    it('saves the cleaned value when an edit was pending', () => {
      const s = setup()
      s.set(['keep', ''])
      s.queue.schedule()
      s.queue.leave((v) => ({ value: v.filter(Boolean), changed: false }))
      expect(s.calls[0].value).toEqual(['keep'])
    })

    it('saves when only cleaning changed something', () => {
      const s = setup()
      s.set(['keep', ''])
      const p = s.queue.leave((v) => ({ value: v.filter(Boolean), changed: true }))
      expect(p).not.toBeNull()
      expect(s.calls[0].value).toEqual(['keep'])
    })

    it('does nothing when nothing is pending or changed', () => {
      const s = setup()
      s.set(['a'])
      expect(s.queue.leave((v) => ({ value: v, changed: false }))).toBeNull()
      expect(s.calls).toHaveLength(0)
    })

    it('queues behind an in-flight save instead of overlapping it', () => {
      const s = setup()
      s.set(['a'])
      void s.queue.saveNow()
      s.set(['a', 'b'])
      s.queue.schedule()
      void s.queue.leave()
      expect(s.calls).toHaveLength(1)
    })
  })
})
