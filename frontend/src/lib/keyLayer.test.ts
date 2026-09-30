import { describe, it, expect, vi, afterEach } from 'vitest'
import { pushKeyLayer, hasKeyLayer, globalShortcutsBlocked, modalOpen } from './keyLayer'

// The key-layer stack (UI-CONVENTIONS §8.1): overlays above a card own
// Escape / Ctrl+Enter, topmost first, and the card's bubble-phase window
// handler never sees a consumed key.
describe('keyLayer', () => {
  const layers: { remove(): void }[] = []
  const push = (...args: Parameters<typeof pushKeyLayer>) => {
    const l = pushKeyLayer(...args)
    layers.push(l)
    return l
  }
  afterEach(() => { while (layers.length) layers.pop()!.remove() })

  function press(init: KeyboardEventInit): { event: KeyboardEvent; card: ReturnType<typeof vi.fn> } {
    const card = vi.fn()
    window.addEventListener('keydown', card)
    const event = new KeyboardEvent('keydown', { bubbles: true, cancelable: true, ...init })
    document.body.dispatchEvent(event)
    window.removeEventListener('keydown', card)
    return { event, card }
  }

  it('only the topmost layer handles Escape, and the card never sees it', () => {
    const lower = vi.fn()
    const upper = vi.fn()
    push({ onEscape: lower })
    push({ onEscape: upper })
    const { event, card } = press({ key: 'Escape' })
    expect(upper).toHaveBeenCalledOnce()
    expect(lower).not.toHaveBeenCalled()
    expect(card).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(true)
  })

  it('a declining layer falls through to the one below', () => {
    const lower = vi.fn()
    push({ onEscape: lower })
    push({ onEscape: () => false })
    press({ key: 'Escape' })
    expect(lower).toHaveBeenCalledOnce()
  })

  it('swallows Ctrl+Enter without onEnter but leaves plain Enter alone', () => {
    push({ onEscape: vi.fn() })
    expect(press({ key: 'Enter', ctrlKey: true }).card).not.toHaveBeenCalled()
    const plain = press({ key: 'Enter' })
    expect(plain.card).toHaveBeenCalledOnce()
    expect(plain.event.defaultPrevented).toBe(false)
  })

  it('removing the last layer stops intercepting', () => {
    const l = push({ onEscape: vi.fn() })
    expect(hasKeyLayer()).toBe(true)
    l.remove()
    expect(hasKeyLayer()).toBe(false)
    expect(press({ key: 'Escape' }).card).toHaveBeenCalledOnce()
  })

  it('an open modal or layer blocks global single-key shortcuts', () => {
    expect(globalShortcutsBlocked()).toBe(false)
    const m = modalOpen(document.createElement('div'))
    expect(globalShortcutsBlocked()).toBe(true)
    m.destroy()
    expect(globalShortcutsBlocked()).toBe(false)
  })
})
