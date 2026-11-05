import { describe, it, expect, vi } from 'vitest'
import type { MenuControl, UncliChat } from '@uncli/chat/element'
import { chatHost, chatMeta, cardTurnContext, followChatSessions, modeControl, nextTurnNote, openChatSession, parseRunsOn, projectTurnContext, resolveChatEngine, reviewStagedChanges, runsOnFor, runsOnGroups, runsOnValue, startNewChat, uncliSrc, UNCLI_STAGED_CONTENT } from './uncliChat'

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
  const sessions = (existing = '', had = 0) => ({ get: vi.fn(async () => existing), set: vi.fn(async () => {}), count: vi.fn(async () => had), name: () => 'Trip', meta: 'bruv/r1/card/c1' })

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
    expect(el.newSession).toHaveBeenCalledWith('chat', '', '', { meta: 'bruv/r1/card/c1', title: 'Trip' })
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
    expect(el.newSession).toHaveBeenCalledWith('chat', '', '', { meta: 'bruv/r1/card/c1', title: 'Trip' })

    // A session the element started itself is recorded.
    el.dispatchEvent(new CustomEvent('uncli-new-session', { detail: { session: 's-own', previous: 's-new-3' } }))
    await vi.waitFor(() => expect(s.set).toHaveBeenLastCalledWith('s-own'))

    stop()
    el.dispatchEvent(new CustomEvent('uncli-deleted', { detail: { session: 's-own' } }))
    expect(s.set).toHaveBeenCalledTimes(4)
  })

  it('startNewChat names the session after the chat, numbered from the second, and records it', async () => {
    const first = element()
    await startNewChat(first, sessions())
    expect(first.newSession).toHaveBeenCalledWith('chat', '', '', { meta: 'bruv/r1/card/c1', title: 'Trip' })

    const el = element()
    const s = sessions('s-old', 2)
    await startNewChat(el, s)
    expect(el.newSession).toHaveBeenCalledWith('chat', '', '', { meta: 'bruv/r1/card/c1', title: 'Trip 3' })
    expect(s.set).toHaveBeenCalledWith('s-new-1')
  })

  it('opens new chats on the chosen CLI and model, and records none when the CLI refuses', async () => {
    const el = element()
    await startNewChat(el, { ...sessions(), runsOn: { provider: 'codex', model: 'gpt-5' } })
    expect(el.newSession).toHaveBeenCalledWith('chat', '', 'gpt-5', { meta: 'bruv/r1/card/c1', title: 'Trip', provider: 'codex' })

    const signedOut = element()
    signedOut.newSession.mockRejectedValueOnce(new Error("can't start a session on Codex: Codex isn't signed in"))
    const s = { ...sessions(), runsOn: { provider: 'codex' } }
    await expect(startNewChat(signedOut, s)).rejects.toThrow("Codex isn't signed in")
    expect(s.set).not.toHaveBeenCalled()
  })
})

describe('choosing what chats open on', () => {
  const card = { kind: 'card', cardId: 'c1' } as const
  const project = { kind: 'project', brand: 'b', stream: 's', project: 'p' } as const

  it("reads the choice for the chat's kind", () => {
    const config = { uncli_chats: { card_chat: { provider: 'claude', model: 'opus' }, project_chat: {} } }
    expect(runsOnFor(config, card)).toEqual({ provider: 'claude', model: 'opus' })
    expect(runsOnFor(config, project)).toBeNull()
    expect(runsOnFor(null, card)).toBeNull()
  })

  it('round-trips a choice through a select value; empty is the default', () => {
    expect(runsOnValue(null)).toBe('')
    expect(parseRunsOn('')).toBeNull()
    const v = runsOnValue({ provider: 'claude', model: 'claude/opus' })
    expect(parseRunsOn(v)).toEqual({ provider: 'claude', model: 'claude/opus' })
  })

  it('groups the choices by CLI, an unready one disabled with why', () => {
    const t = (key: string, p?: Record<string, string>) => `${key}${p ? JSON.stringify(p) : ''}`
    const groups = runsOnGroups([
      { id: 'claude', name: 'Claude Code', ready: true, models: [{ id: 'opus', label: 'Opus' }], default_model: 'opus' },
      { id: 'codex', name: 'Codex', ready: false, reason: "Codex isn't signed in", models: [] },
    ], t)
    expect(groups[0]).toEqual({
      label: 'Claude Code',
      options: [
        { value: runsOnValue({ provider: 'claude' }), label: 'uncli.runs_on_cli_default{"cli":"Claude Code","model":"Opus"}', disabled: false },
        { value: runsOnValue({ provider: 'claude', model: 'opus' }), label: 'Claude Code · Opus', disabled: false },
      ],
    })
    expect(groups[1].label).toBe('uncli.runs_on_unready{"cli":"Codex","reason":"Codex isn\'t signed in"}')
    expect(groups[1].options).toEqual([{ value: runsOnValue({ provider: 'codex' }), label: 'uncli.runs_on_cli_picks{"cli":"Codex"}', disabled: true }])
  })
})

describe('nextTurnNote', () => {
  it('is told to the next turn only', () => {
    const note = nextTurnNote()
    expect(note.take()).toBeNull()
    note.set('reviewed')
    expect(note.take()).toBe('reviewed')
    expect(note.take()).toBeNull()
    note.set('') // a review with nothing to tell
    expect(note.take()).toBeNull()
  })
})

describe('reviewStagedChanges', () => {
  type Edit = { id: string; label: string; detail: string; status: string; error?: string }
  type Message = { id: string; content: string; pending_edits?: Edit[] }
  const staged = (id: string, edits: { id: string; status?: string }[]): Message => ({
    id,
    content: UNCLI_STAGED_CONTENT,
    pending_edits: edits.map(e => ({ id: e.id, label: `Change ${e.id}`, detail: `about ${e.id}`, status: e.status ?? 'pending' })),
  })
  function setup(answer: { action: string; items: string[] }, messages: Message[] = [staged('m1', [{ id: 'a' }, { id: 'b' }]), { id: 'm0', content: 'BRUV built-in reply' }, staged('m2', [{ id: 'c' }, { id: 'old', status: 'accepted' }])]) {
    const el = Object.assign(new EventTarget(), { review: vi.fn(async () => answer) }) as unknown as UncliChat & { review: ReturnType<typeof vi.fn> }
    const history = {
      load: vi.fn(async () => ({ messages })),
      // As the server: the ticked ones applied (c fails), the rest rejected.
      apply: vi.fn(async (messageId: string, accept: string[]) => ({
        messages: [{
          id: messageId,
          content: UNCLI_STAGED_CONTENT,
          pending_edits: (messages.find(m => m.id === messageId)?.pending_edits ?? []).map(e =>
            e.status !== 'pending' ? e
            : !accept.includes(e.id) ? { ...e, status: 'rejected' }
            : e.id === 'c' ? { ...e, status: 'failed', error: 'no checklist "Packing"' }
            : { ...e, status: 'accepted' }),
        }],
      })),
    }
    const done = vi.fn()
    const stop = reviewStagedChanges(el, history, { title: n => `Apply ${n}?`, detail: 'From this turn.' }, { done, error: e => { throw e } })
    return { el, history, done, stop }
  }

  it("asks once about the turn's staged changes when it ends, and applies the ticked ones", async () => {
    const { el, history, done } = setup({ action: 'apply', items: ['a', 'c'] })
    el.dispatchEvent(new CustomEvent('uncli-idle', { detail: { session: 's1' } }))
    await vi.waitFor(() => expect(done).toHaveBeenCalled())
    expect(el.review).toHaveBeenCalledTimes(1)
    expect(el.review.mock.calls[0][0]).toEqual({
      title: 'Apply 3?',
      detail: 'From this turn.',
      items: [
        { id: 'a', label: 'Change a', detail: 'about a' },
        { id: 'b', label: 'Change b', detail: 'about b' },
        { id: 'c', label: 'Change c', detail: 'about c' },
      ],
    })
    expect(history.apply).toHaveBeenCalledWith('m1', ['a'])
    expect(history.apply).toHaveBeenCalledWith('m2', ['c'])
    expect(done).toHaveBeenCalledWith({
      applied: 1,
      failed: 1,
      note: 'Since your last turn, the user reviewed the changes you suggested: applied "Change a"; turned down "Change b"; ' +
        'accepted "Change c" but it failed: no checklist "Packing".',
    })
  })

  it('Discard rejects them all; a dismissed card leaves them waiting', async () => {
    const discard = setup({ action: 'discard', items: ['a'] })
    discard.el.dispatchEvent(new CustomEvent('uncli-idle', { detail: { session: 's1' } }))
    await vi.waitFor(() => expect(discard.done).toHaveBeenCalledWith({
      applied: 0,
      failed: 0,
      note: 'Since your last turn, the user reviewed the changes you suggested: turned down "Change a", "Change b", "Change c".',
    }))
    expect(discard.history.apply).toHaveBeenCalledWith('m1', [])

    const dismissed = setup({ action: 'dismissed', items: [] })
    dismissed.el.dispatchEvent(new CustomEvent('uncli-idle', { detail: { session: 's1' } }))
    await vi.waitFor(() => expect(dismissed.el.review).toHaveBeenCalled())
    expect(dismissed.history.apply).not.toHaveBeenCalled()
  })

  it('stays quiet when nothing is staged, and stops listening on cleanup', async () => {
    const { el, history, stop } = setup({ action: 'apply', items: [] }, [{ id: 'm0', content: 'reply' }])
    el.dispatchEvent(new CustomEvent('uncli-idle', { detail: { session: 's1' } }))
    await vi.waitFor(() => expect(history.load).toHaveBeenCalledTimes(1))
    expect(el.review).not.toHaveBeenCalled()
    stop()
    el.dispatchEvent(new CustomEvent('uncli-idle', { detail: { session: 's1' } }))
    expect(history.load).toHaveBeenCalledTimes(1)
  })
})

describe('uncliSrc', () => {
  it("is the server's /uncli mount", () => {
    expect(uncliSrc('https://box.example/')).toBe('https://box.example/uncli')
  })
})
