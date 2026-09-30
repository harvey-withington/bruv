// One serialized save queue for an open card's blocks array, plus the gate
// that keeps silent reloads from clobbering local edits.
//
// The lost-update bug (TODO "checklist rapid multi-check loses items"):
// every block edit writes the WHOLE blocks array, and the card dialog
// reloaded itself on every `card:updated` — including the watcher's echo of
// its own save ~200 ms later. A GetCard started after save #1 could land
// after local change #2 and replace the local state with a snapshot missing
// #2; the next save then wrote that snapshot back. Rules here:
//
//   - every whole-blocks write for the card goes through ONE serial saver
//     (shared/serialSave.ts): one persist in flight, later writes coalesce
//     to the latest snapshot;
//   - a silent reload requested while the saver is busy is deferred and run
//     ONCE after it drains;
//   - a silent reload whose response lands after a save was requested
//     (generation moved) or while one is in flight is discarded and retried.

import { createSerialSaver } from '@shared/serialSave'
import type { Block } from '@shared/types'

export interface CardBlockSaves {
  /** Persist a snapshot of `blocks` (after any in-flight save). */
  save(blocks: Block[]): Promise<void>
  /** True while a save is in flight or queued. */
  readonly busy: boolean
  /** Resolves once the queue drains (never rejects). */
  idle(): Promise<void>
  /** Bumped on every save request — lets a reload detect a save it raced. */
  readonly generation: number
  /**
   * Run `reload` now, or — while saves are pending — once after they
   * drain (repeat requests meanwhile collapse into that one run).
   */
  requestReload(reload: () => void): void
}

export function createCardBlockSaves(persist: (blocks: Block[]) => Promise<unknown>): CardBlockSaves {
  const saver = createSerialSaver<Block[]>(async (blocks) => { await persist(blocks) })
  let generation = 0
  let deferred: (() => void) | null = null

  return {
    save(blocks) {
      generation++
      // Snapshot now: the live $state proxy keeps changing while this
      // waits in the queue, and a coalesced follow-up carries the latest.
      return saver.save($state.snapshot(blocks) as Block[])
    },
    get busy() { return saver.busy },
    idle: () => saver.idle(),
    get generation() { return generation },
    requestReload(reload) {
      if (!saver.busy) { reload(); return }
      const alreadyWaiting = deferred !== null
      deferred = reload
      if (alreadyWaiting) return
      void saver.idle().then(() => {
        const run = deferred
        deferred = null
        run?.()
      })
    },
  }
}
