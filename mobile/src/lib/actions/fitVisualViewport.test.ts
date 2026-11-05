import { describe, it, expect, afterEach } from 'vitest'
import { fitVisualViewport, keyboardInset } from './fitVisualViewport'

describe('keyboardInset', () => {
  it('is what the keyboard hides at the bottom, however the browser shows it', () => {
    expect(keyboardInset({ height: 800, offsetTop: 0 }, 800)).toBe(0) // no keyboard
    expect(keyboardInset({ height: 500, offsetTop: 0 }, 800)).toBe(300) // the visible area shrank
    expect(keyboardInset({ height: 500, offsetTop: 120 }, 800)).toBe(180) // …and the page slid up
    expect(keyboardInset({ height: 900, offsetTop: 0 }, 800)).toBe(0)
  })
})

describe('fitVisualViewport', () => {
  const original = window.visualViewport
  afterEach(() => Object.defineProperty(window, 'visualViewport', { value: original, writable: true }))

  it('follows the visual viewport as the keyboard opens, and stops when destroyed', () => {
    const vv = Object.assign(new EventTarget(), { height: window.innerHeight, offsetTop: 0 })
    Object.defineProperty(window, 'visualViewport', { value: vv, writable: true })
    const node = document.createElement('div')
    const action = fitVisualViewport(node)
    expect(node.style.getPropertyValue('--keyboard-inset')).toBe('0px')

    vv.height = window.innerHeight - 300
    vv.dispatchEvent(new Event('resize'))
    expect(node.style.getPropertyValue('--keyboard-inset')).toBe('300px')
    expect(node.style.getPropertyValue('--visible-height')).toBe(`${window.innerHeight - 300}px`)

    action.destroy?.()
    vv.height = window.innerHeight
    vv.dispatchEvent(new Event('resize'))
    expect(node.style.getPropertyValue('--keyboard-inset')).toBe('300px')
  })
})
