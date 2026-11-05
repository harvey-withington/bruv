import { describe, it, expect, afterEach } from 'vitest'
import { deepActiveElement, isTyping } from './focus'

describe('isTyping', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('sees a field focused inside a shadow root (an embedded chat), not just its host', () => {
    const host = document.createElement('div')
    document.body.append(host)
    const textarea = document.createElement('textarea')
    host.attachShadow({ mode: 'open' }).append(textarea)
    textarea.focus()

    expect(document.activeElement).toBe(host) // what the old check saw
    expect(deepActiveElement()).toBe(textarea)
    expect(isTyping()).toBe(true)
  })

  it('is true in light-DOM fields and false elsewhere', () => {
    const input = document.createElement('input')
    const button = document.createElement('button')
    document.body.append(input, button)
    input.focus()
    expect(isTyping()).toBe(true)
    button.focus()
    expect(isTyping()).toBe(false)
  })
})
