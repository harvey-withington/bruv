import { describe, it, expect, vi } from 'vitest'
import { cardChatEngine, cardChatHost, cardTurnContext, openCardSession, uncliSrc, type UncliChatElement } from './uncliChat'

describe('cardChatEngine', () => {
  it('stays built-in unless the user opted in', () => {
    expect(cardChatEngine(null, { ready: true }, '')).toEqual({ engine: 'builtin', fallback: null })
    expect(cardChatEngine({ chat_engine: 'builtin' }, { ready: true }, '')).toEqual({ engine: 'builtin', fallback: null })
  })

  it('uses UNCLI when opted in and ready', () => {
    expect(cardChatEngine({ chat_engine: 'uncli' }, { ready: true }, '')).toEqual({ engine: 'uncli' })
  })

  it('falls back to built-in, saying why, when UNCLI cannot run', () => {
    expect(cardChatEngine({ chat_engine: 'uncli' }, { ready: false, reason: 'service_account', account: 'LocalSystem' }, '')).toEqual({
      engine: 'builtin',
      fallback: { key: 'uncli.unavailable_service_account', params: { account: 'LocalSystem' } },
    })
  })
})

describe('cardChatHost', () => {
  const card = { id: 'c1', title: 'Lisbon trip', type: 'feature' }
  const host = (follow = vi.fn()) =>
    cardChatHost({
      token: () => 'tok',
      follow,
      searchCards: async () => [{ CardID: 'c2', Title: 'Budget [draft]', Type: 'task', Rank: 1, ProjectContext: 'Home › Travel' }],
      card: () => card,
      cardLabel: 'Card',
    })

  it("sends BRUV's token", () => {
    expect(host().headers?.()).toEqual({ Authorization: 'Bearer tok' })
  })

  it('takes bruv: links and leaves the rest', () => {
    const follow = vi.fn()
    const h = host(follow)
    expect(h.openLink?.('bruv:card:c2')).toBe(true)
    expect(follow).toHaveBeenCalledWith('card:c2')
    expect(h.openLink?.('https://example.com')).toBe(false)
  })

  it('completes @mentions as mention markdown', async () => {
    expect(await host().complete?.('budg')).toEqual([{ label: 'Budget [draft]', detail: 'Home › Travel', insert: '[Budget \\[draft\\]](bruv:card:c2)' }])
    expect(await host().complete?.('  ')).toEqual([])
  })

  it('tells each turn which card is open', () => {
    expect(host().turnContext?.()).toBe(cardTurnContext(card))
    expect(cardTurnContext(card)).toContain('bruv:card:c1')
  })
})

describe('openCardSession', () => {
  const element = () => ({ session: '', newSession: vi.fn(async () => 's-new') }) as unknown as UncliChatElement

  it("reopens the card's session", async () => {
    const el = element()
    const set = vi.fn()
    await openCardSession(el, 'c1', { get: async () => 's-old', set })
    expect(el.session).toBe('s-old')
    expect(set).not.toHaveBeenCalled()
  })

  it('starts and records a session for a card without one', async () => {
    const el = element()
    const set = vi.fn(async () => {})
    await openCardSession(el, 'c1', { get: async () => '', set })
    expect(el.newSession).toHaveBeenCalled()
    expect(set).toHaveBeenCalledWith('c1', 's-new')
  })
})

describe('uncliSrc', () => {
  it("is the server's /uncli mount", () => {
    expect(uncliSrc('https://box.example/')).toBe('https://box.example/uncli')
  })
})
