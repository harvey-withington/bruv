import { describe, it, expect, vi, afterEach } from 'vitest'
import { inlineEdit, type InlineEditParams } from '@shared/inlineEdit'
import { EditScope } from '@shared/editScope'

// inlineEdit is a plain Svelte action `(node, params) => { update, destroy }`
// — it owns all the DOM listener wiring itself, so we drive it directly
// against real <input>/<textarea> nodes with real events instead of going
// through a Svelte component. See shared/editScope.test.ts for the
// containment half of the same contract.

type ActionInstance = { update: (p: InlineEditParams) => void; destroy: () => void }

let mounted: ActionInstance[] = []
let cleanupNodes: HTMLElement[] = []

function mountInput(params: InlineEditParams): { node: HTMLInputElement; action: ActionInstance } {
  const node = document.createElement('input')
  document.body.appendChild(node)
  const action = inlineEdit(node, params) as ActionInstance
  mounted.push(action)
  cleanupNodes.push(node)
  return { node, action }
}

function mountTextarea(params: InlineEditParams): { node: HTMLTextAreaElement; action: ActionInstance } {
  const node = document.createElement('textarea')
  document.body.appendChild(node)
  const action = inlineEdit(node, params) as ActionInstance
  mounted.push(action)
  cleanupNodes.push(node)
  return { node, action }
}

function makeParams(overrides: Partial<InlineEditParams> = {}): InlineEditParams {
  return { onCommit: vi.fn(), onCancel: vi.fn(), ...overrides }
}

function keydown(key: string, opts: KeyboardEventInit = {}): KeyboardEvent {
  return new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...opts })
}

function focusEvt(opts: FocusEventInit = {}): FocusEvent {
  return new FocusEvent('focus', { bubbles: true, ...opts })
}

function blurEvt(opts: FocusEventInit = {}): FocusEvent {
  return new FocusEvent('blur', { bubbles: true, ...opts })
}

afterEach(() => {
  mounted.forEach(a => a.destroy())
  cleanupNodes.forEach(n => n.remove())
  mounted = []
  cleanupNodes = []
})

describe('inlineEdit — single-line input', () => {
  it('Enter commits once', () => {
    const params = makeParams()
    const { node } = mountInput(params)
    const event = keydown('Enter')

    node.dispatchEvent(event)

    expect(params.onCommit).toHaveBeenCalledOnce()
    expect(params.onCancel).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(true)
  })

  it('Escape cancels, does not commit, and consumes the event (never bubbles)', () => {
    const params = makeParams()
    const { node } = mountInput(params)
    const outerListener = vi.fn()
    document.addEventListener('keydown', outerListener)
    const event = keydown('Escape')

    node.dispatchEvent(event)

    expect(params.onCancel).toHaveBeenCalledOnce()
    expect(params.onCommit).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(true)
    expect(outerListener).not.toHaveBeenCalled() // stopPropagation()

    document.removeEventListener('keydown', outerListener)
  })

  it('Ctrl+Enter commits, then calls scope.requestClose()', () => {
    const scope = new EditScope()
    const order: string[] = []
    scope.requestClose = vi.fn(() => order.push('close'))
    const params = makeParams({ onCommit: vi.fn(() => order.push('commit')), scope })
    const { node } = mountInput(params)

    node.dispatchEvent(keydown('Enter', { ctrlKey: true }))

    expect(params.onCommit).toHaveBeenCalledOnce()
    expect(scope.requestClose).toHaveBeenCalledOnce()
    expect(order).toEqual(['commit', 'close'])
  })

  it('blur after Enter does not double-commit (latched)', () => {
    const params = makeParams()
    const { node } = mountInput(params)

    node.dispatchEvent(keydown('Enter'))
    node.dispatchEvent(blurEvt())

    expect(params.onCommit).toHaveBeenCalledOnce()
  })

  it('Shift+Enter on a single-line field still commits (multiline is off)', () => {
    const params = makeParams()
    const { node } = mountInput(params)

    node.dispatchEvent(keydown('Enter', { shiftKey: true }))

    expect(params.onCommit).toHaveBeenCalledOnce()
  })
})

describe('inlineEdit — multiline textarea', () => {
  it('plain Enter commits', () => {
    const params = makeParams({ multiline: true })
    const { node } = mountTextarea(params)
    const event = keydown('Enter')

    node.dispatchEvent(event)

    expect(params.onCommit).toHaveBeenCalledOnce()
    expect(event.defaultPrevented).toBe(true)
  })

  it('Shift+Enter does not commit — the browser inserts a newline instead', () => {
    const params = makeParams({ multiline: true })
    const { node } = mountTextarea(params)
    const event = keydown('Enter', { shiftKey: true })

    node.dispatchEvent(event)

    expect(params.onCommit).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)
  })

  it('Escape cancels', () => {
    const params = makeParams({ multiline: true })
    const { node } = mountTextarea(params)

    node.dispatchEvent(keydown('Escape'))

    expect(params.onCancel).toHaveBeenCalledOnce()
    expect(params.onCommit).not.toHaveBeenCalled()
  })
})

describe('inlineEdit — enterInsertsNewline (mobile multiline variant)', () => {
  it('plain Enter is not intercepted — no commit, browser inserts a newline', () => {
    const params = makeParams({ multiline: true, enterInsertsNewline: true })
    const { node } = mountTextarea(params)
    const event = keydown('Enter')

    node.dispatchEvent(event)

    expect(params.onCommit).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)
  })

  it('Shift+Enter is not intercepted either — same newline behaviour', () => {
    const params = makeParams({ multiline: true, enterInsertsNewline: true })
    const { node } = mountTextarea(params)
    const event = keydown('Enter', { shiftKey: true })

    node.dispatchEvent(event)

    expect(params.onCommit).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)
  })

  it('Ctrl+Enter still commits and closes the scope', () => {
    const scope = new EditScope()
    scope.requestClose = vi.fn()
    const params = makeParams({ multiline: true, enterInsertsNewline: true, scope })
    const { node } = mountTextarea(params)

    node.dispatchEvent(keydown('Enter', { ctrlKey: true }))

    expect(params.onCommit).toHaveBeenCalledOnce()
    expect(scope.requestClose).toHaveBeenCalledOnce()
  })

  it('Escape still cancels', () => {
    const params = makeParams({ multiline: true, enterInsertsNewline: true })
    const { node } = mountTextarea(params)

    node.dispatchEvent(keydown('Escape'))

    expect(params.onCancel).toHaveBeenCalledOnce()
    expect(params.onCommit).not.toHaveBeenCalled()
  })

  it('without multiline the option is inert — Enter still commits', () => {
    const params = makeParams({ enterInsertsNewline: true })
    const { node } = mountInput(params)

    node.dispatchEvent(keydown('Enter'))

    expect(params.onCommit).toHaveBeenCalledOnce()
  })
})

describe('inlineEdit — serial mode ("add another" rows)', () => {
  it('Enter fires onCommit and re-arms — a second Enter fires again', () => {
    const params = makeParams({ serial: true })
    const { node } = mountInput(params)

    node.dispatchEvent(keydown('Enter'))
    node.dispatchEvent(keydown('Enter'))

    expect(params.onCommit).toHaveBeenCalledTimes(2)
  })

  it('blur does NOT commit — the draft is preserved', () => {
    const params = makeParams({ serial: true })
    const { node } = mountInput(params)

    node.dispatchEvent(focusEvt())
    node.dispatchEvent(blurEvt())

    expect(params.onCommit).not.toHaveBeenCalled()
  })

  it('Escape fires onCancel', () => {
    const params = makeParams({ serial: true })
    const { node } = mountInput(params)

    node.dispatchEvent(keydown('Escape'))

    expect(params.onCancel).toHaveBeenCalledOnce()
  })

  it('registers with the scope only while focused: hasActive() true while focused, false after blur', () => {
    const scope = new EditScope()
    const params = makeParams({ serial: true, scope })
    const { node } = mountInput(params)

    // Mounted but not focused — an idle "add another" row must not
    // block a container's Escape-close.
    expect(scope.hasActive()).toBe(false)

    node.dispatchEvent(focusEvt())
    expect(scope.hasActive()).toBe(true)

    node.dispatchEvent(blurEvt())
    expect(scope.hasActive()).toBe(false)
  })
})

describe('inlineEdit — blurCommits: false', () => {
  it('blur does not commit', () => {
    const params = makeParams({ blurCommits: false })
    const { node } = mountInput(params)

    node.dispatchEvent(blurEvt())

    expect(params.onCommit).not.toHaveBeenCalled()
  })

  it('refocus re-registers with the scope', () => {
    const scope = new EditScope()
    const params = makeParams({ blurCommits: false, scope })
    const { node } = mountInput(params)

    expect(scope.hasActive()).toBe(true) // registered at mount (non-serial)

    node.dispatchEvent(blurEvt())
    expect(scope.hasActive()).toBe(false) // deregistered, draft kept

    node.dispatchEvent(focusEvt())
    expect(scope.hasActive()).toBe(true) // re-registered
  })
})

describe('inlineEdit — scope registration lifecycle (edit-in-place)', () => {
  it('registers with the scope on mount', () => {
    const scope = new EditScope()
    mountInput(makeParams({ scope }))
    expect(scope.hasActive()).toBe(true)
  })

  it('deregisters after commit', () => {
    const scope = new EditScope()
    const { node } = mountInput(makeParams({ scope }))
    expect(scope.hasActive()).toBe(true)

    node.dispatchEvent(keydown('Enter'))

    expect(scope.hasActive()).toBe(false)
  })

  it('deregisters after cancel', () => {
    const scope = new EditScope()
    const { node } = mountInput(makeParams({ scope }))

    node.dispatchEvent(keydown('Escape'))

    expect(scope.hasActive()).toBe(false)
  })
})

describe('inlineEdit — update() and destroy()', () => {
  it('update() with new params resets the commit latch', () => {
    const onCommit1 = vi.fn()
    const onCommit2 = vi.fn()
    const { node, action } = mountInput(makeParams({ onCommit: onCommit1 }))

    node.dispatchEvent(keydown('Enter'))
    expect(onCommit1).toHaveBeenCalledOnce()

    // Still latched — a second Enter before update() must not re-fire.
    node.dispatchEvent(keydown('Enter'))
    expect(onCommit1).toHaveBeenCalledOnce()

    action.update(makeParams({ onCommit: onCommit2 }))
    node.dispatchEvent(keydown('Enter'))

    expect(onCommit2).toHaveBeenCalledOnce()
    expect(onCommit1).toHaveBeenCalledOnce() // unaffected by the reset
  })

  it('update() re-registers with the scope after a prior commit deregistered it', () => {
    const scope = new EditScope()
    const { node, action } = mountInput(makeParams({ scope }))

    node.dispatchEvent(keydown('Enter'))
    expect(scope.hasActive()).toBe(false)

    action.update(makeParams({ scope }))

    expect(scope.hasActive()).toBe(true)
  })

  it('destroy() deregisters from the scope', () => {
    const scope = new EditScope()
    const { action } = mountInput(makeParams({ scope }))
    expect(scope.hasActive()).toBe(true)

    action.destroy()

    expect(scope.hasActive()).toBe(false)
  })

  it('destroy() removes listeners — a later Enter no longer commits', () => {
    const params = makeParams()
    const { node, action } = mountInput(params)

    action.destroy()
    node.dispatchEvent(keydown('Enter'))

    expect(params.onCommit).not.toHaveBeenCalled()
  })
})

describe('inlineEdit — additional contract coverage', () => {
  it('Ctrl+Enter with closeOnCtrlEnter:false commits but does not close the scope', () => {
    const scope = new EditScope()
    scope.requestClose = vi.fn()
    const params = makeParams({ scope, closeOnCtrlEnter: false })
    const { node } = mountInput(params)

    node.dispatchEvent(keydown('Enter', { ctrlKey: true }))

    expect(params.onCommit).toHaveBeenCalledOnce()
    expect(scope.requestClose).not.toHaveBeenCalled()
  })

  it('blur that moves focus inside the configured container does not commit', () => {
    const wrapper = document.createElement('div')
    wrapper.className = 'rename-group'
    document.body.appendChild(wrapper)
    const node = document.createElement('input')
    const relatedButton = document.createElement('button')
    wrapper.appendChild(node)
    wrapper.appendChild(relatedButton)
    const params = makeParams({ container: '.rename-group' })
    const action = inlineEdit(node, params) as ActionInstance
    mounted.push(action)
    cleanupNodes.push(wrapper)

    node.dispatchEvent(blurEvt({ relatedTarget: relatedButton }))

    expect(params.onCommit).not.toHaveBeenCalled()
  })

  it('blur that moves focus outside the configured container still commits', () => {
    const wrapper = document.createElement('div')
    wrapper.className = 'rename-group'
    document.body.appendChild(wrapper)
    const node = document.createElement('input')
    const outsideButton = document.createElement('button')
    wrapper.appendChild(node)
    document.body.appendChild(outsideButton)
    const params = makeParams({ container: '.rename-group' })
    const action = inlineEdit(node, params) as ActionInstance
    mounted.push(action)
    cleanupNodes.push(wrapper)
    cleanupNodes.push(outsideButton)

    node.dispatchEvent(blurEvt({ relatedTarget: outsideButton }))

    expect(params.onCommit).toHaveBeenCalledOnce()
  })

  it('IME composition Enter is ignored (does not commit)', () => {
    const params = makeParams()
    const { node } = mountInput(params)

    node.dispatchEvent(keydown('Enter', { isComposing: true }))

    expect(params.onCommit).not.toHaveBeenCalled()
  })
})

describe('inlineEdit — always-mounted field sessions (latch reset)', () => {
  it('typing after a commit starts a new session — the next Enter commits again', () => {
    const params = makeParams()
    const { node } = mountInput(params)

    node.dispatchEvent(keydown('Enter'))
    expect(params.onCommit).toHaveBeenCalledOnce()

    // Field stays mounted (dialog form). Typing must reset the latch.
    node.dispatchEvent(new InputEvent('input', { bubbles: true }))
    node.dispatchEvent(keydown('Enter'))
    expect(params.onCommit).toHaveBeenCalledTimes(2)
  })

  it('refocus after commit+blur starts a new session', () => {
    const params = makeParams()
    const { node } = mountInput(params)

    node.dispatchEvent(keydown('Enter'))
    node.dispatchEvent(blurEvt()) // latched — no double-commit
    expect(params.onCommit).toHaveBeenCalledOnce()

    node.dispatchEvent(focusEvt())
    node.dispatchEvent(blurEvt())
    expect(params.onCommit).toHaveBeenCalledTimes(2)
  })

  it('Enter → immediate blur still commits only once (double-fire latch intact)', () => {
    const params = makeParams()
    const { node } = mountInput(params)

    node.dispatchEvent(keydown('Enter'))
    node.dispatchEvent(blurEvt())
    expect(params.onCommit).toHaveBeenCalledOnce()
  })

  it('Ctrl+Enter WITHOUT a scope commits but lets the event bubble to the container handler', () => {
    const params = makeParams()
    const { node } = mountInput(params)
    const windowListener = vi.fn()
    document.addEventListener('keydown', windowListener)
    const event = keydown('Enter', { ctrlKey: true })

    node.dispatchEvent(event)

    expect(params.onCommit).toHaveBeenCalledOnce()
    expect(windowListener).toHaveBeenCalledOnce() // NOT swallowed
    document.removeEventListener('keydown', windowListener)
  })

  it('Ctrl+Enter WITH a scope is fully handled (commit + requestClose, consumed)', () => {
    const scope = new EditScope()
    scope.requestClose = vi.fn()
    const params = makeParams({ scope })
    const { node } = mountInput(params)
    const windowListener = vi.fn()
    document.addEventListener('keydown', windowListener)

    node.dispatchEvent(keydown('Enter', { ctrlKey: true }))

    expect(params.onCommit).toHaveBeenCalledOnce()
    expect(scope.requestClose).toHaveBeenCalledOnce()
    expect(windowListener).not.toHaveBeenCalled()
    document.removeEventListener('keydown', windowListener)
  })
})
