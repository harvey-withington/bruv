// A chat turn as both chat surfaces show it: while it runs, how long it
// has taken, its output so far (from chat:progress) and whether it has
// passed a warn budget; once it ends, the wording of any notice it left.
//
// The budget itself lives server-side (core/runtime/chat/budget.go).
// shared/ can't import a surface's i18n module, so callers pass `t`;
// keys live under chat.* in both surfaces' en.json.

import type { TranslateFn } from './relativeTime'
import type { ChatNotice, ChatProgress } from './types'

/** Budget bounds. DEFAULT and MAX mirror DefaultChatBudgetTokens and
 *  MaxChatOutputTokens in internal/config/llm_config.go (which clamps to
 *  the max too); MIN only keeps typos like "16" out of Settings. */
export const CHAT_BUDGET_DEFAULT = 16000
export const CHAT_BUDGET_MIN = 1000
export const CHAT_BUDGET_MAX = 64000

/** A typed-in budget, made valid: whole tokens, within the bounds. */
export function clampChatBudget(n: number): number {
  if (!Number.isFinite(n) || n <= 0) return CHAT_BUDGET_DEFAULT
  return Math.min(CHAT_BUDGET_MAX, Math.max(CHAT_BUDGET_MIN, Math.round(n)))
}

/** Which chat a panel shows, in the terms chat:progress names it by. */
export type ChatTurnScope = { cardId: string } | { projectPath: string }

export class ChatTurn {
  running = $state(false)
  stopping = $state(false)
  elapsedMs = $state(0)
  /** Output tokens so far; 0 until the first report (hidden thinking
   *  only counts once a call finishes). */
  used = $state(0)
  budget = $state(0)
  overBudget = $state(false)

  #timer: ReturnType<typeof setInterval> | null = null
  #scope: () => ChatTurnScope

  /** `scope` is read on every report, so a panel that switches chats
   *  never takes another chat's progress. */
  constructor(scope: () => ChatTurnScope) {
    this.#scope = scope
  }

  start(): void {
    this.finish()
    this.running = true
    const startedAt = Date.now()
    this.#timer = setInterval(() => { this.elapsedMs = Date.now() - startedAt }, 1000)
  }

  finish(): void {
    if (this.#timer) clearInterval(this.#timer)
    this.#timer = null
    this.running = false
    this.stopping = false
    this.elapsedMs = 0
    this.used = 0
    this.budget = 0
    this.overBudget = false
  }

  /** Applies a chat:progress payload if it is this chat's running turn. */
  progress(p: ChatProgress): void {
    if (!this.running || !progressMatches(p, this.#scope())) return
    this.used = p.used
    this.budget = p.budget
    this.overBudget = p.over_budget
  }
}

function progressMatches(p: ChatProgress, scope: ChatTurnScope): boolean {
  return 'cardId' in scope ? p.card_id === scope.cardId : p.project_path === scope.projectPath
}

/** "m:ss" for a running turn's elapsed time. */
export function formatElapsed(ms: number): string {
  const s = Math.floor(ms / 1000)
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

/** The notice in the user's language. */
export function describeNotice(n: ChatNotice, t: TranslateFn): string {
  return t(`chat.notice_${n.code}`, {
    budget: (n.budget ?? 0).toLocaleString(),
    used: (n.used ?? 0).toLocaleString(),
  })
}

/** Notices that mean the reply is missing or incomplete, styled as
 *  warnings; the rest (stopped, over budget) are informational. */
export function isWarningNotice(n: ChatNotice): boolean {
  return n.code !== 'stopped' && n.code !== 'over_budget'
}
