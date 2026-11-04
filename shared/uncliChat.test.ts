import { describe, it, expect, vi } from 'vitest'
import { chatHost, cardTurnContext, openChatSession, projectTurnContext, resolveChatEngine, startNewChat, uncliSrc, type UncliChatElement } from './uncliChat'

describe('resolveChatEngine', () => {
  it('stays built-in unless the user opted in', () => {
    expect(resolveChatEngine(null, { ready: true }, '')).toEqual({ engine: 'builtin', fallback: null })
    expect(resolveChatEngine({ chat_engine: 'builtin' }, { ready: true }, '')).toEqual({ engine: 'builtin', fallback: null })
  })

  it('uses UNCLI when opted in and ready', () => {
    expect(resolveChatEngine({ chat_engine: 'uncli' }, { ready: true }, '')).toEqual({ engine: 'uncli' })
  })

  it('falls back to built-in, saying why, when UNCLI cannot run', () => {
    expect(resolveChatEngine({ chat_engine: 'uncli' }, { ready: false, reason: 'service_account', account: 'LocalSystem' }, '')).toEqual({
      engine: 'builtin',
      fallback: { key: 'uncli.unavailable_service_account', params: { account: 'LocalSystem' } },
    })
  })
})

describe('chatHost', () => {
  const card = { id: 'c1', title: 'Lisbon trip', type: 'feature' }
  const host = (follow = vi.fn()) =>
    chatHost({
      token: () => 'tok',
      follow,
      searchCards: async () => [{ CardID: 'c2', Title: 'Budget [draft]', Type: 'task', Rank: 1, ProjectContext: 'Home › Travel' }],
      context: () => cardTurnContext(card),
      cardLabel: 'Card',
      enter: 'newline',
    })

  it("passes the surface's Enter key", () => {
    expect(host().enter).toBe('newline')
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

  it('tells each turn what the chat is about', () => {
    expect(host().turnContext?.()).toBe(cardTurnContext(card))
    expect(cardTurnContext(card)).toContain('bruv:card:c1')
    expect(projectTurnContext({ brand: 'Home', stream: 'Travel', project: 'Lisbon' })).toContain('"Lisbon" (in Home › Travel)')
  })
})

describe('chat sessions', () => {
  const element = () => ({ session: '', newSession: vi.fn(async () => 's-new') }) as unknown as UncliChatElement

  it("reopens the chat's session", async () => {
    const el = element()
    const set = vi.fn()
    await openChatSession(el, { get: async () => 's-old', set })
    expect(el.session).toBe('s-old')
    expect(set).not.toHaveBeenCalled()
  })

  it('starts and records a session for a chat without one', async () => {
    const el = element()
    const set = vi.fn(async () => {})
    await openChatSession(el, { get: async () => '', set })
    expect(el.newSession).toHaveBeenCalled()
    expect(set).toHaveBeenCalledWith('s-new')
  })

  it('a new chat replaces the recorded one', async () => {
    const el = element()
    const set = vi.fn(async () => {})
    await startNewChat(el, { get: async () => 's-old', set })
    expect(set).toHaveBeenCalledWith('s-new')
  })
})

describe('uncliSrc', () => {
  it("is the server's /uncli mount", () => {
    expect(uncliSrc('https://box.example/')).toBe('https://box.example/uncli')
  })
})
