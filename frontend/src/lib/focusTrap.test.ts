import { describe, it, expect, beforeEach } from 'vitest'
import { focusTrap } from './actions'

const flush = () => new Promise<void>(resolve => queueMicrotask(resolve))

function dialog(html: string, attrs: Record<string, string> = {}): HTMLElement {
  const el = document.createElement('div')
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v)
  el.innerHTML = html
  document.body.appendChild(el)
  return el
}

describe('focusTrap focus handling', () => {
  let opener: HTMLButtonElement

  beforeEach(() => {
    document.body.innerHTML = ''
    opener = document.createElement('button')
    opener.textContent = 'open'
    document.body.appendChild(opener)
    opener.focus()
  })

  it('focuses [data-autofocus] on open', async () => {
    const el = dialog('<button>a</button><button data-autofocus>b</button>')
    focusTrap(el)
    await flush()
    expect(document.activeElement?.textContent).toBe('b')
  })

  it('focuses a focusable dialog itself rather than its first button', async () => {
    const el = dialog('<button>delete</button>', { tabindex: '-1' })
    focusTrap(el)
    await flush()
    expect(document.activeElement).toBe(el)
  })

  it('leaves focus alone when a child already took it', async () => {
    const el = dialog('<input /><button data-autofocus>b</button>', { tabindex: '-1' })
    const trap = focusTrap(el)
    el.querySelector('input')!.focus() // a child's focusOnMount, run before the microtask
    await flush()
    expect(document.activeElement?.tagName).toBe('INPUT')
    trap.destroy()
  })

  it('returns focus to the opener on close', async () => {
    const el = dialog('<button data-autofocus>b</button>')
    const trap = focusTrap(el)
    await flush()
    trap.destroy()
    el.remove()
    expect(document.activeElement).toBe(opener)
  })

  it('keeps focus elsewhere if the user moved it before close', async () => {
    const el = dialog('<button data-autofocus>b</button>')
    const other = document.createElement('input')
    document.body.appendChild(other)
    const trap = focusTrap(el)
    await flush()
    other.focus()
    trap.destroy()
    expect(document.activeElement).toBe(other)
  })
})
