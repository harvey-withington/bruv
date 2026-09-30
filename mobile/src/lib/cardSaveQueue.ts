import { createSerialSaver } from '@shared/serialSave'

// Save pipeline for a card's whole-value block array on CardPage.
//
// Three lost-update shapes this closes (pre-release sweep 2.8 / 2.9):
//   - leaving the card inside the typing debounce dropped the edit —
//     `leave()` flushes a pending debounce on teardown;
//   - overlapping UpdateCardBlocks calls could land out of order and a
//     boolean "saving" flag was cleared by whichever finished first —
//     every save goes through ONE shared/serialSave saver (one in
//     flight, later saves coalesce to the latest value);
//   - a live-update refetch that started before a local edit replaced
//     the card with a snapshot predating it — `edits` counts local
//     mutations, and `canApplyRefetch(since)` refuses a fetched card
//     if anything changed (or is still saving) since the fetch began.

export interface CardSaveQueueOptions<T> {
  /** Current local value, or null when there is nothing to save. */
  read: () => T | null
  /** Persist one whole value. Never called concurrently. */
  persist: (value: T) => Promise<void>
  /** A save failed. `retry` re-saves the then-current value. */
  onError: (err: unknown, retry: () => Promise<void>) => void
  /** Saving indicator — true while a persist is in flight or queued. */
  onSavingChange?: (saving: boolean) => void
  delayMs?: number
}

export interface CardSaveQueue<T> {
  /** Record a local edit and persist it after the debounce. */
  schedule(): void
  /** Record a local edit and persist now (cancels a pending debounce). */
  saveNow(): Promise<void>
  /** Record a local mutation that doesn't go through this queue (e.g.
   *  a single-field save) so in-flight refetches are discarded. */
  markEdit(): void
  /** Edit generation — capture before a refetch, pass to canApplyRefetch. */
  readonly edits: number
  /** True while a debounce is pending or a save is in flight/queued. */
  readonly busy: boolean
  /** A refetch that began at generation `since` may replace local state. */
  canApplyRefetch(since: number): boolean
  /** Resolves once nothing is pending, in flight or queued. */
  idle(): Promise<void>
  /**
   * Teardown: cancel the debounce and, if an edit was pending or `clean`
   * changed the value, persist the cleaned value. Returns that save's
   * promise (rejects on failure), or null when nothing needed saving.
   */
  leave(clean?: (value: T) => { value: T; changed: boolean }): Promise<void> | null
}

export function createCardSaveQueue<T>(opts: CardSaveQueueOptions<T>): CardSaveQueue<T> {
  const delayMs = opts.delayMs ?? 200
  const saver = createSerialSaver<T>(opts.persist)
  let timer: ReturnType<typeof setTimeout> | null = null
  let edits = 0
  let debounceWaiters: (() => void)[] = []

  function clearTimer(): boolean {
    if (!timer) return false
    clearTimeout(timer)
    timer = null
    return true
  }

  function releaseDebounceWaiters() {
    const done = debounceWaiters
    debounceWaiters = []
    for (const r of done) r()
  }

  async function persistCurrent(): Promise<void> {
    const value = opts.read()
    releaseDebounceWaiters()
    if (value === null) return
    opts.onSavingChange?.(true)
    try {
      await saver.save(value)
    } catch (err) {
      opts.onError(err, persistCurrent)
    } finally {
      opts.onSavingChange?.(saver.busy)
    }
  }

  return {
    schedule() {
      edits++
      clearTimer()
      timer = setTimeout(() => {
        timer = null
        void persistCurrent()
      }, delayMs)
    },
    saveNow() {
      edits++
      clearTimer()
      return persistCurrent()
    },
    markEdit() {
      edits++
    },
    get edits() {
      return edits
    },
    get busy() {
      return timer !== null || saver.busy
    },
    canApplyRefetch(since: number) {
      return since === edits && timer === null && !saver.busy
    },
    async idle() {
      while (timer !== null || saver.busy) {
        if (timer !== null) await new Promise<void>((r) => debounceWaiters.push(r))
        await saver.idle()
      }
    },
    leave(clean) {
      const pending = clearTimer()
      releaseDebounceWaiters()
      const current = opts.read()
      if (current === null) return null
      const { value, changed } = clean ? clean(current) : { value: current, changed: false }
      if (!pending && !changed) return null
      return saver.save(value)
    },
  }
}
