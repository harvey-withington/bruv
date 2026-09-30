// Serialized, coalescing saves for whole-value writes (a card's blocks
// array, a document's text, …).
//
// The lost-update bug class this exists for: two overlapping whole-value
// saves can complete out of order on the server (save #1 = A, save #2 =
// A+B, disk ends at A), and a refetch that lands while a save is still in
// flight replaces local state with a snapshot that predates it. The rules:
//
//   - at most ONE persist call is in flight per saver;
//   - saves requested while one is in flight coalesce into a single
//     follow-up carrying the LATEST value (intermediate values are
//     superseded — they're whole-value writes, the latest one contains
//     them);
//   - `busy` is true from the first request until the queue drains, so a
//     caller can skip server refetches that would clobber local edits.
//
// Each save() promise settles with the persist call that carried its value
// (or the later value that superseded it): resolves on success, rejects
// with that call's error. A failed persist does not block later saves.

export interface SerialSaver<T> {
  /** Persist `value`, after any in-flight save; coalesces with other waiters. */
  save(value: T): Promise<void>
  /** Resolves once nothing is in flight or queued (never rejects). */
  idle(): Promise<void>
  /** True while a persist call is in flight or a follow-up is queued. */
  readonly busy: boolean
}

interface Waiter {
  resolve: () => void
  reject: (err: unknown) => void
}

export function createSerialSaver<T>(persist: (value: T) => Promise<void>): SerialSaver<T> {
  let inFlight = false
  let queued: { value: T; waiters: Waiter[] } | null = null
  let idleWaiters: (() => void)[] = []

  async function run(value: T, waiters: Waiter[]): Promise<void> {
    inFlight = true
    try {
      await persist(value)
      for (const w of waiters) w.resolve()
    } catch (err) {
      for (const w of waiters) w.reject(err)
    } finally {
      inFlight = false
      const next = queued
      queued = null
      if (next) {
        void run(next.value, next.waiters)
      } else {
        const done = idleWaiters
        idleWaiters = []
        for (const r of done) r()
      }
    }
  }

  return {
    save(value: T): Promise<void> {
      return new Promise<void>((resolve, reject) => {
        const waiter = { resolve, reject }
        if (!inFlight) {
          void run(value, [waiter])
          return
        }
        if (queued) {
          queued.value = value
          queued.waiters.push(waiter)
        } else {
          queued = { value, waiters: [waiter] }
        }
      })
    },
    idle(): Promise<void> {
      if (!inFlight && !queued) return Promise.resolve()
      return new Promise<void>((resolve) => idleWaiters.push(resolve))
    },
    get busy() {
      return inFlight || queued !== null
    },
  }
}
