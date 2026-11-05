import { describe, it, expect, vi } from 'vitest'
import type { MenuControl, UncliChat } from '@uncli/chat/element'
import { chatHost, chatMeta, cardTurnContext, followChatSessions, modeControl, openChatSession, projectTurnContext, resolveChatEngine, startNewChat, uncliSrc } from './uncliChat'

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
      controls: [modeControl('chat', k => k, () => {})],
    })

  it("sends BRUV's token, the surface's Enter key and the mode control", () => {
    const h = host()
    expect(h.headers?.()).toEqual({ Authorization: 'Bearer tok' })
    expect(h.enter).toBe('newline')
    expect(h.composerControls?.map(c => c.id)).toEqual(['mode'])
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

describe('modeControl', () => {
  it('is a drop-down of the three modes showing the current one, in UNCLI tones', () => {
    const onChange = vi.fn()
    const c = modeControl('suggest', k => k, onChange) as MenuControl
    expect(c.value).toBe('suggest')
    expect(c.options.map(o => [o.id, o.tone, o.label, o.hint])).toEqual([
      ['chat', 'success', 'chat.mode_chat', 'uncli.mode_chat_hint'],
      ['suggest', 'warning', 'chat.mode_suggest', 'uncli.mode_suggest_hint'],
      ['edit', 'danger', 'chat.mode_edit', 'uncli.mode_edit_hint'],
    ])
    c.onChange('edit')
    expect(onChange).toHaveBeenCalledWith('edit')
  })
})

describe('chatMeta', () => {
  it("names the chat's repo first, as the server reads it", () => {
    expect(chatMeta('r1', { kind: 'card', cardId: 'c1' })).toBe('bruv/r1/card/c1')
    expect(chatMeta('r1', { kind: 'project', brand: 'b', stream: 's', project: 'p' })).toBe('bruv/r1/project/b/s/p')
  })
})

describe('chat sessions', () => {
  // A stand-in element: newSession hands out s-new-1, s-new-2, …
  function element() {
    let n = 0
    const el = Object.assign(new EventTarget(), {
      session: '',
      newSession: vi.fn(async () => `s-new-${++n}`),
    })
    return el as unknown as UncliChat & { newSession: ReturnType<typeof vi.fn> }
  }
  const sessions = (existing = '') => ({ get: vi.fn(async () => existing), set: vi.fn(async () => {}), meta: 'bruv/r1/card/c1' })

  it("reopens the chat's session", async () => {
    const el = element()
    const s = sessions('s-old')
    await openChatSession(el, s)
    expect(el.session).toBe('s-old')
    expect(s.set).not.toHaveBeenCalled()
  })

  it('starts a chat without one with its meta, and records it', async () => {
    const el = element()
    const s = sessions()
    await openChatSession(el, s)
    expect(el.newSession).toHaveBeenCalledWith('chat', '', '', { meta: 'bruv/r1/card/c1' })
    expect(s.set).toHaveBeenCalledWith('s-new-1')
  })

  it('makes New chat itself, and replaces deleted or missing sessions', async () => {
    const el = element()
    const s = sessions('s-old')
    const stop = followChatSessions(el, s, () => {})

    const newChat = new CustomEvent('uncli-new-chat', { cancelable: true, detail: { session: 's-old' } })
    el.dispatchEvent(newChat)
    expect(newChat.defaultPrevented).toBe(true)
    el.dispatchEvent(new CustomEvent('uncli-deleted', { detail: { session: 's-new-1' } }))
    el.dispatchEvent(new CustomEvent('uncli-missing', { detail: { session: 's-new-2' } }))
    await vi.waitFor(() => expect(s.set).toHaveBeenCalledTimes(3))
    expect(el.newSession).toHaveBeenCalledWith('chat', '', '', { meta: 'bruv/r1/card/c1' })

    // A session the element started itself is recorded.
    el.dispatchEvent(new CustomEvent('uncli-new-session', { detail: { session: 's-own', previous: 's-new-3' } }))
    await vi.waitFor(() => expect(s.set).toHaveBeenLastCalledWith('s-own'))

    stop()
    el.dispatchEvent(new CustomEvent('uncli-deleted', { detail: { session: 's-own' } }))
    expect(s.set).toHaveBeenCalledTimes(4)
  })

  it('startNewChat records the new session', async () => {
    const el = element()
    const s = sessions('s-old')
    await startNewChat(el, s)
    expect(s.set).toHaveBeenCalledWith('s-new-1')
  })
})

describe('uncliSrc', () => {
  it("is the server's /uncli mount", () => {
    expect(uncliSrc('https://box.example/')).toBe('https://box.example/uncli')
  })
})
