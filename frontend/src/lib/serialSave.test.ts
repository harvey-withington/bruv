import { describe, it, expect } from 'vitest'
import { createSerialSaver } from '@shared/serialSave'

// A persist fn whose calls stay pending until the test releases them, so
// overlap and ordering are deterministic.
function controlledPersist<T>() {
  const calls: { value: T; resolve: () => void; reject: (e: unknown) => void }[] = []
  const persist = (value: T) =>
    new Promise<void>((resolve, reject) => {
      calls.push({ value, resolve, reject })
    })
  return { calls, persist }
}

const flush = () => new Promise((r) => setTimeout(r, 0))

describe('createSerialSaver', () => {
  it('persists immediately when idle', async () => {
    const { calls, persist } = controlledPersist<string>()
    const saver = createSerialSaver(persist)
    const p = saver.save('A')
    expect(calls.map((c) => c.value)).toEqual(['A'])
    expect(saver.busy).toBe(true)
    calls[0].resolve()
    await p
    expect(saver.busy).toBe(false)
  })

  it('never runs two persists at once and coalesces to the latest value', async () => {
    // Regression for "checklist rapid multi-check loses items": ticks
    // A, A+B, A+B+C in quick succession must end with A+B+C on disk and
    // never let an older value land after a newer one.
    const { calls, persist } = controlledPersist<string>()
    const saver = createSerialSaver(persist)
    const p1 = saver.save('A')
    const p2 = saver.save('A+B')
    const p3 = saver.save('A+B+C')
    expect(calls.map((c) => c.value)).toEqual(['A'])

    calls[0].resolve()
    await flush()
    expect(calls.map((c) => c.value)).toEqual(['A', 'A+B+C'])
    expect(saver.busy).toBe(true)

    calls[1].resolve()
    await Promise.all([p1, p2, p3])
    expect(saver.busy).toBe(false)
  })

  it('rejects the waiters of a failed persist and keeps going', async () => {
    const { calls, persist } = controlledPersist<string>()
    const saver = createSerialSaver(persist)
    const p1 = saver.save('A')
    const p2 = saver.save('B')
    calls[0].reject(new Error('offline'))
    await expect(p1).rejects.toThrow('offline')
    await flush()
    expect(calls.map((c) => c.value)).toEqual(['A', 'B'])
    calls[1].resolve()
    await expect(p2).resolves.toBeUndefined()
  })

  it('idle() waits for the queue to drain', async () => {
    const { calls, persist } = controlledPersist<string>()
    const saver = createSerialSaver(persist)
    await saver.idle() // resolves immediately when nothing is pending
    void saver.save('A')
    void saver.save('B')
    let drained = false
    void saver.idle().then(() => { drained = true })
    calls[0].resolve()
    await flush()
    expect(drained).toBe(false)
    calls[1].resolve()
    await flush()
    expect(drained).toBe(true)
  })
})
