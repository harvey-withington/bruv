// The UNCLI chat engine's page side, shared by both surfaces: which
// engine BRUV's chats use, and what BRUV does for the <uncli-chat>
// element (UNCLI's EMBEDDING.md, "host"): its sign-in, @mentions of
// cards, bruv: links, and what the chat is about as each turn's context.
// A chat is a card's or a project's. The surfaces mount the element
// (UncliChat, UncliChatSheet) and supply their own navigation, search,
// RPCs and strings.

import { CARD_MENTION_PREFIX, cardMentionLink, mentionMarkdown } from './mentions'
import { errorDetail } from './errors'
import { uncliUnavailable, type UncliUnavailable } from './uncliStatus'
import type { LLMConfig, SearchResult, UncliStatus } from './types'

/** Link schemes the chat keeps in answers, besides the web's. */
export const UNCLI_LINK_SCHEMES = 'bruv'

/** The element's src: the server's UNCLI mount (transport/http/uncli.go). */
export function uncliSrc(serverBase: string): string {
  return serverBase.replace(/\/+$/, '') + '/uncli'
}

// The element's API as far as BRUV uses it. The package ships no types
// for its built element, so these mirror its ChatHost and Mention.
export type UncliMention = { label: string; detail?: string; insert: string }

export interface UncliChatHost {
  openLink?(href: string): boolean
  complete?(query: string): Promise<UncliMention[]> | UncliMention[]
  turnContext?(): string | null
  headers?(): Record<string, string> | null
  /** What Enter does in the message box: "send" (Shift+Enter for a new
   *  line) or "newline" (Ctrl+Enter or Send sends). */
  enter?: UncliEnterKey
}

export type UncliEnterKey = 'send' | 'newline'

export interface UncliChatElement extends HTMLElement {
  host: UncliChatHost | null
  session: string
  newSession(profile?: string, workdir?: string, model?: string): Promise<string>
}

/** The element's event when a session finishes answering (detail:
 *  { session }). */
export const UNCLI_IDLE = 'uncli-idle'

// --- Which engine BRUV's chats use ---

export type ChatEngineChoice =
  | { engine: 'builtin'; fallback: UncliUnavailable | null }
  | { engine: 'uncli' }

/** UNCLI only when the user opted in and the server can run it.
 *  Otherwise the built-in chat, with fallback saying why when the user
 *  did opt in. */
export function resolveChatEngine(config: Pick<LLMConfig, 'chat_engine'> | null, status: UncliStatus | null, statusError: string): ChatEngineChoice {
  if (config?.chat_engine !== 'uncli') return { engine: 'builtin', fallback: null }
  const unavailable = uncliUnavailable(status, statusError)
  return unavailable ? { engine: 'builtin', fallback: unavailable } : { engine: 'uncli' }
}

/** Reads the choice: the setting, and the server's UNCLI status only when
 *  the user opted in. A status that can't be read keeps the built-in
 *  chat, saying why. */
export async function readChatEngine(
  getConfig: () => Promise<Pick<LLMConfig, 'chat_engine'> | null>,
  getStatus: () => Promise<UncliStatus>,
): Promise<ChatEngineChoice> {
  const config = await getConfig()
  if (config?.chat_engine !== 'uncli') return resolveChatEngine(config, null, '')
  try {
    return resolveChatEngine(config, await getStatus(), '')
  } catch (e) {
    return resolveChatEngine(config, null, errorDetail(e))
  }
}

// --- A chat's session ---

/** Where a chat's session is recorded on the server (Get/Set
 *  Card|ProjectUncliSession), which is how the server gives the session
 *  that repo's board tools (config.UncliSessionChat). */
export interface ChatSessions {
  get(): Promise<string>
  set(session: string): Promise<void>
}

/** Shows the chat in el: the session it had, or a new one recorded as
 *  the chat's. */
export async function openChatSession(el: UncliChatElement, sessions: ChatSessions): Promise<void> {
  const existing = await sessions.get()
  if (existing) {
    el.session = existing
    return
  }
  await startNewChat(el, sessions)
}

/** Starts a new session for the chat and records it as the chat's. The
 *  old one stays in UNCLI, but this chat no longer opens it. */
export async function startNewChat(el: UncliChatElement, sessions: ChatSessions): Promise<void> {
  const created = await el.newSession()
  await sessions.set(created)
}

// --- What each turn is told (model-facing, so not localized, like
// BRUV's own chat prompts) ---

const BOARD_TOOLS_HINT =
  `Read and change the board with the bruv board tools (get_card, list_cards and the others). ` +
  `Link cards in answers as [Title](${CARD_MENTION_PREFIX}<id>).`

export function cardTurnContext(card: { id: string; title: string; type: string }): string {
  const type = card.type ? `, a ${card.type} card` : ''
  return `The user has the BRUV card "${card.title}" (${cardMentionLink(card.id)}${type}) open beside this chat. ` + BOARD_TOOLS_HINT
}

export function projectTurnContext(project: { brand: string; stream: string; project: string }): string {
  return `The user has the BRUV project "${project.project}" (in ${project.brand} › ${project.stream}) open beside this chat; ` +
    `its cards are in its categories. ` + BOARD_TOOLS_HINT
}

// --- The host hooks ---

export interface ChatHostOptions {
  /** The device token BRUV's sign-in in front of /uncli/ wants. */
  token: () => string
  /** Follow a bruv: link, given its data-bruv form ("card:<id>"). */
  follow: (target: string) => void
  searchCards: (query: string) => Promise<SearchResult[]>
  /** What the chat is about, told to the model with each turn. */
  context: () => string | null
  /** The mention menu's detail for a card (localized). */
  cardLabel: string
  /** Enter as in the surface's own chat: desktop's ChatSection sends,
   *  mobile's ChatSheet starts a new line. */
  enter: UncliEnterKey
}

const MENTION_LIMIT = 8

export function chatHost(o: ChatHostOptions): UncliChatHost {
  return {
    headers: () => ({ Authorization: `Bearer ${o.token()}` }),
    openLink: href => {
      if (!href.startsWith('bruv:')) return false
      o.follow(href.slice('bruv:'.length))
      return true
    },
    complete: async query => {
      const hits = query.trim() ? await o.searchCards(query.trim()) : []
      return (hits ?? []).slice(0, MENTION_LIMIT).map(h => ({
        label: h.Title,
        detail: h.ProjectContext || o.cardLabel,
        insert: mentionMarkdown(h.Title, cardMentionLink(h.CardID)),
      }))
    },
    turnContext: o.context,
    enter: o.enter,
  }
}
