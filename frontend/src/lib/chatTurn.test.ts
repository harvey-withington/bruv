import { describe, it, expect, afterEach } from 'vitest'
import { ChatTurn, clampChatBudget, describeNotice, formatElapsed, CHAT_BUDGET_DEFAULT, CHAT_BUDGET_MAX, CHAT_BUDGET_MIN } from '@shared/chatTurn.svelte'
import type { ChatProgress } from '@shared/types'

const t = (key: string, params?: Record<string, string | number>) =>
  params ? `${key} ${JSON.stringify(params)}` : key

const progress = (p: Partial<ChatProgress>): ChatProgress => ({ used: 0, budget: 16000, over_budget: false, ...p })

describe('ChatTurn', () => {
  let turn: ChatTurn
  afterEach(() => turn?.finish())

  it("takes only its own chat's progress, and only while running", () => {
    turn = new ChatTurn(() => ({ cardId: 'c1' }))
    turn.progress(progress({ card_id: 'c1', used: 50 }))
    expect(turn.used).toBe(0) // not running yet

    turn.start()
    turn.progress(progress({ card_id: 'other', used: 99 }))
    turn.progress(progress({ project_path: 'b/s/p', used: 98 }))
    expect(turn.used).toBe(0)

    turn.progress(progress({ card_id: 'c1', used: 17000, over_budget: true }))
    expect([turn.used, turn.budget, turn.overBudget]).toEqual([17000, 16000, true])
  })

  it('matches a project chat by its path', () => {
    turn = new ChatTurn(() => ({ projectPath: 'b/s/p' }))
    turn.start()
    turn.progress(progress({ project_path: 'b/s/p', used: 42 }))
    expect(turn.used).toBe(42)
  })

  it('finish clears everything for the next turn', () => {
    turn = new ChatTurn(() => ({ cardId: 'c1' }))
    turn.start()
    turn.stopping = true
    turn.progress(progress({ card_id: 'c1', used: 500, over_budget: true }))
    turn.finish()
    expect([turn.running, turn.stopping, turn.used, turn.overBudget]).toEqual([false, false, 0, false])
  })
})

describe('chat turn helpers', () => {
  it('formats elapsed time as m:ss', () => {
    expect(formatElapsed(0)).toBe('0:00')
    expect(formatElapsed(9_999)).toBe('0:09')
    expect(formatElapsed(754_000)).toBe('12:34')
  })

  it('clamps a typed budget into range', () => {
    expect(clampChatBudget(NaN)).toBe(CHAT_BUDGET_DEFAULT)
    expect(clampChatBudget(0)).toBe(CHAT_BUDGET_DEFAULT)
    expect(clampChatBudget(16)).toBe(CHAT_BUDGET_MIN)
    expect(clampChatBudget(1e9)).toBe(CHAT_BUDGET_MAX)
    expect(clampChatBudget(20000.4)).toBe(20000)
  })

  it('words a notice from its code with formatted numbers', () => {
    expect(describeNotice({ code: 'over_budget', budget: 16000, used: 23410 }, t))
      .toBe(`chat.notice_over_budget ${JSON.stringify({ budget: (16000).toLocaleString(), used: (23410).toLocaleString() })}`)
    expect(describeNotice({ code: 'stopped' }, t)).toContain('chat.notice_stopped')
  })
})
