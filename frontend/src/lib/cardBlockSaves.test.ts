import { describe, it, expect, vi } from 'vitest'
import { createCardBlockSaves } from './cardBlockSaves.svelte'
import type { Block } from '@shared/types'

// Root cause of "checklist rapid multi-check loses items": overlapping
// whole-blocks saves + a silent reload landing between them. One serial
// queue per card, and reloads deferred until it drains.
describe('createCardBlockSaves', () => {
  const block = (id: string, value: boolean): Block => ({ id, type: 'checkbox', key: '', label: id, value } as Block)

  function deferredPersist() {
    const calls: { blocks: Block[]; resolve: () => void }[] = []
    const persist = vi.fn((blocks: Block[]) => new Promise<void>((resolve) => { calls.push({ blocks, resolve }) }))
    return { persist, calls }
  }

  it('serializes saves and coalesces to the latest snapshot', async () => {
    const { persist, calls } = deferredPersist()
    const saves = createCardBlockSaves(persist)
    const live = [block('a', false), block('b', false)]
    live[0].value = true
    const first = saves.save(live)
    live[1].value = true
    const second = saves.save(live)
    const third = saves.save(live)
    expect(persist).toHaveBeenCalledTimes(1)
    expect(saves.busy).toBe(true)
    calls[0].resolve()
    await first
    expect(persist).toHaveBeenCalledTimes(2)
    // The follow-up carries both toggles — nothing lost.
    expect(calls[1].blocks.map(b => b.value)).toEqual([true, true])
    calls[1].resolve()
    await Promise.all([second, third])
    expect(saves.busy).toBe(false)
  })

  it('snapshots the value at request time', async () => {
    const { persist, calls } = deferredPersist()
    const saves = createCardBlockSaves(persist)
    const live = [block('a', false)]
    const p = saves.save(live)
    live[0].value = true
    expect(calls[0].blocks[0].value).toBe(false)
    calls[0].resolve()
    await p
  })

  it('defers a reload requested while busy and runs it once after the drain', async () => {
    const { persist, calls } = deferredPersist()
    const saves = createCardBlockSaves(persist)
    const reload = vi.fn()
    const p = saves.save([block('a', true)])
    saves.requestReload(reload)
    saves.requestReload(reload)
    expect(reload).not.toHaveBeenCalled()
    calls[0].resolve()
    await p
    await saves.idle()
    await Promise.resolve()
    expect(reload).toHaveBeenCalledOnce()
  })

  it('reloads immediately when idle; generation counts save requests', async () => {
    const saves = createCardBlockSaves(async () => {})
    const reload = vi.fn()
    saves.requestReload(reload)
    expect(reload).toHaveBeenCalledOnce()
    expect(saves.generation).toBe(0)
    await saves.save([])
    expect(saves.generation).toBe(1)
  })
})
