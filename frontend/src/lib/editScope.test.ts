import { describe, it, expect, vi } from 'vitest'
import { EditScope } from '@shared/editScope'

// EditScope is the containment half of the keyboard-entry contract
// (shared/editScope.ts): active inline edits register themselves, and
// the container's window-level Escape/Ctrl+Enter handling asks the
// scope before acting. See shared/inlineEdit.test.ts for the field
// half of the same contract.
describe('EditScope', () => {
  it('hasActive() is false with nothing registered', () => {
    const scope = new EditScope()
    expect(scope.hasActive()).toBe(false)
  })

  it('register() adds an edit; hasActive() becomes true', () => {
    const scope = new EditScope()
    scope.register({ commit: vi.fn(), cancel: vi.fn() })
    expect(scope.hasActive()).toBe(true)
  })

  it('the function returned by register() unregisters that edit', () => {
    const scope = new EditScope()
    const unregister = scope.register({ commit: vi.fn(), cancel: vi.fn() })
    unregister()
    expect(scope.hasActive()).toBe(false)
  })

  it('unregister is idempotent — a second call is a harmless no-op', () => {
    const scope = new EditScope()
    const edit1 = { commit: vi.fn(), cancel: vi.fn() }
    const edit2 = { commit: vi.fn(), cancel: vi.fn() }
    const unregister1 = scope.register(edit1)
    scope.register(edit2)

    unregister1()
    expect(() => unregister1()).not.toThrow()
    // edit2 must be unaffected by the repeated unregister of edit1.
    expect(scope.hasActive()).toBe(true)
  })

  it('commitAll() calls commit() on every registered edit', () => {
    const scope = new EditScope()
    const edit1 = { commit: vi.fn(), cancel: vi.fn() }
    const edit2 = { commit: vi.fn(), cancel: vi.fn() }
    scope.register(edit1)
    scope.register(edit2)

    scope.commitAll()

    expect(edit1.commit).toHaveBeenCalledOnce()
    expect(edit2.commit).toHaveBeenCalledOnce()
  })

  it('commitAll() with nothing registered does not throw', () => {
    const scope = new EditScope()
    expect(() => scope.commitAll()).not.toThrow()
  })

  it('cancelAll() calls cancel() on every registered edit — commit never fires', () => {
    const scope = new EditScope()
    const edit1 = { commit: vi.fn(), cancel: vi.fn() }
    // cancel() typically unregisters the edit mid-iteration (the actions
    // deregister on cancel) — cancelAll must iterate a copy safely.
    const edit2 = {
      commit: vi.fn(),
      cancel: vi.fn(() => unregister2()),
    }
    scope.register(edit1)
    const unregister2 = scope.register(edit2)

    scope.cancelAll()

    expect(edit1.cancel).toHaveBeenCalledOnce()
    expect(edit2.cancel).toHaveBeenCalledOnce()
    expect(edit1.commit).not.toHaveBeenCalled()
    expect(edit2.commit).not.toHaveBeenCalled()
  })

  it('cancelAll() with nothing registered does not throw', () => {
    const scope = new EditScope()
    expect(() => scope.cancelAll()).not.toThrow()
  })

  describe('handleWindowKeydown', () => {
    it('Escape with no active edits calls requestClose', () => {
      const scope = new EditScope()
      scope.requestClose = vi.fn()
      scope.handleWindowKeydown(new KeyboardEvent('keydown', { key: 'Escape' }))
      expect(scope.requestClose).toHaveBeenCalledOnce()
    })

    it('Escape with an active edit does NOT call requestClose', () => {
      const scope = new EditScope()
      scope.requestClose = vi.fn()
      scope.register({ commit: vi.fn(), cancel: vi.fn() })

      scope.handleWindowKeydown(new KeyboardEvent('keydown', { key: 'Escape' }))

      expect(scope.requestClose).not.toHaveBeenCalled()
    })

    it('Escape when requestClose is unset does not throw', () => {
      const scope = new EditScope()
      expect(() => scope.handleWindowKeydown(new KeyboardEvent('keydown', { key: 'Escape' }))).not.toThrow()
    })

    it('Ctrl+Enter commits every active edit, then calls requestClose (in that order)', () => {
      const scope = new EditScope()
      const order: string[] = []
      const edit = { commit: vi.fn(() => order.push('commit')), cancel: vi.fn() }
      scope.register(edit)
      scope.requestClose = vi.fn(() => order.push('close'))

      scope.handleWindowKeydown(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true }))

      expect(edit.commit).toHaveBeenCalledOnce()
      expect(scope.requestClose).toHaveBeenCalledOnce()
      expect(order).toEqual(['commit', 'close'])
    })

    it('Meta+Enter (Cmd, macOS) also triggers commitAll + requestClose', () => {
      const scope = new EditScope()
      scope.requestClose = vi.fn()
      scope.handleWindowKeydown(new KeyboardEvent('keydown', { key: 'Enter', metaKey: true }))
      expect(scope.requestClose).toHaveBeenCalledOnce()
    })

    it('plain Enter (no modifier) does nothing', () => {
      const scope = new EditScope()
      scope.requestClose = vi.fn()
      scope.handleWindowKeydown(new KeyboardEvent('keydown', { key: 'Enter' }))
      expect(scope.requestClose).not.toHaveBeenCalled()
    })

    it('a key other than Escape/Enter does nothing', () => {
      const scope = new EditScope()
      scope.requestClose = vi.fn()
      scope.handleWindowKeydown(new KeyboardEvent('keydown', { key: 'a' }))
      expect(scope.requestClose).not.toHaveBeenCalled()
    })

    it('Ctrl+Enter calls preventDefault so the browser does not act on it', () => {
      const scope = new EditScope()
      const event = new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, cancelable: true })
      const spy = vi.spyOn(event, 'preventDefault')
      scope.handleWindowKeydown(event)
      expect(spy).toHaveBeenCalledOnce()
    })
  })
})
