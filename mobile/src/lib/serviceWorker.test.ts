import { describe, it, expect, vi, beforeEach } from 'vitest'
import { handleServiceWorkerMessage } from './serviceWorker'
import { route } from './router.svelte'

// A notification tap while the PWA is open routes IN PLACE (a document
// navigation drops CardPage's pending saves and drafts) and acks the SW,
// which only falls back to a full navigation when no page answers.

function message(data: unknown) {
  const reply = { postMessage: vi.fn() }
  const ev = { data, ports: [reply] } as unknown as MessageEvent
  return { ev, reply }
}

beforeEach(() => {
  window.history.replaceState({}, '', '/m/inbox')
})

describe('handleServiceWorkerMessage', () => {
  it('routes a /m/ deep link in place and acknowledges', () => {
    const before = window.history.length
    const { ev, reply } = message({ type: 'bruv:navigate', url: '/m/c/card-1' })

    handleServiceWorkerMessage(ev)

    expect(window.location.pathname).toBe('/m/c/card-1')
    expect(route.current).toMatchObject({ name: 'card', id: 'card-1' })
    // Pushed, so Back returns to where the user was.
    expect(window.history.length).toBe(before + 1)
    expect(reply.postMessage).toHaveBeenCalledWith({ type: 'bruv:navigated' })
  })

  it('keeps the query and hash of a same-origin absolute URL', () => {
    const url = `${window.location.origin}/m/p/b/s/p#cat=ideas`
    const { ev, reply } = message({ type: 'bruv:navigate', url })

    handleServiceWorkerMessage(ev)

    expect(window.location.pathname + window.location.hash).toBe('/m/p/b/s/p#cat=ideas')
    expect(reply.postMessage).toHaveBeenCalled()
  })

  it('does not ack (so the SW falls back) for URLs outside the app', () => {
    for (const url of ['https://evil.example/m/c/x', '/api/whatever']) {
      const { ev, reply } = message({ type: 'bruv:navigate', url })
      handleServiceWorkerMessage(ev)
      expect(reply.postMessage).not.toHaveBeenCalled()
    }
    expect(window.location.pathname).toBe('/m/inbox')
  })

  it('ignores unrelated messages', () => {
    const { ev, reply } = message({ type: 'something-else' })
    handleServiceWorkerMessage(ev)
    expect(reply.postMessage).not.toHaveBeenCalled()
  })
})
