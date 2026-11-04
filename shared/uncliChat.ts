// The UNCLI chat engine's page side, shared by both surfaces: which
// engine a card's chat uses, and what BRUV does for the <uncli-chat>
// element (UNCLI's EMBEDDING.md, "host"): its sign-in, @mentions of
// cards, bruv: links, and the open card as each turn's context. The
// surfaces mount the element (UncliCardChat) and supply their own
// navigation, search and strings.

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
}

export interface UncliChatElement extends HTMLElement {
  host: UncliChatHost | null
  session: string
  newSession(profile?: string, workdir?: string, model?: string): Promise<string>
}

/** The element's events (detail: { session }). */
export const UNCLI_NEEDS_APPROVAL = 'uncli-needs-approval'

// --- Which engine a card's chat uses ---

export type CardChatEngine =
  | { engine: 'builtin'; fallback: UncliUnavailable | null }
  | { engine: 'uncli' }

/** The engine for card chats: UNCLI only when the user opted in and the
 *  server can run it. Otherwise the built-in chat, with fallback saying
 *  why when the user did opt in. */
export function cardChatEngine(config: Pick<LLMConfig, 'chat_engine'> | null, status: UncliStatus | null, statusError: string): CardChatEngine {
  if (config?.chat_engine !== 'uncli') return { engine: 'builtin', fallback: null }
  const unavailable = uncliUnavailable(status, statusError)
  return unavailable ? { engine: 'builtin', fallback: unavailable } : { engine: 'uncli' }
}

/** Reads the choice: the setting, and the server's UNCLI status only when
 *  the user opted in. A status that can't be read keeps the built-in
 *  chat, saying why. */
export async function readCardChatEngine(
  getConfig: () => Promise<Pick<LLMConfig, 'chat_engine'> | null>,
  getStatus: () => Promise<UncliStatus>,
): Promise<CardChatEngine> {
  const config = await getConfig()
  if (config?.chat_engine !== 'uncli') return cardChatEngine(config, null, '')
  try {
    return cardChatEngine(config, await getStatus(), '')
  } catch (e) {
    return cardChatEngine(config, null, errorDetail(e))
  }
}

// --- A card's session ---

export interface CardSessions {
  get(cardId: string): Promise<string>
  set(cardId: string, session: string): Promise<void>
}

/** Shows the card's chat in el: the session it had, or a new chat
 *  recorded as the card's (the server gives it that repo's board tools,
 *  config.UncliSessionCard). */
export async function openCardSession(el: UncliChatElement, cardId: string, sessions: CardSessions): Promise<void> {
  const existing = await sessions.get(cardId)
  if (existing) {
    el.session = existing
    return
  }
  const created = await el.newSession()
  await sessions.set(cardId, created)
}

// --- The host hooks for a card's chat ---

/** What the chat is told about the open card with each turn. The model
 *  reads the rest through the bruv board tools. Model-facing, so not
 *  localized, like BRUV's own chat prompts. */
export function cardTurnContext(card: { id: string; title: string; type: string }): string {
  const type = card.type ? `, a ${card.type} card` : ''
  return `The user has the BRUV card "${card.title}" (${cardMentionLink(card.id)}${type}) open beside this chat. ` +
    `Read and change it with the bruv board tools (get_card and the others). ` +
    `Link cards in answers as [Title](${CARD_MENTION_PREFIX}<id>).`
}

export interface CardChatHostOptions {
  /** The device token BRUV's sign-in in front of /uncli/ wants. */
  token: () => string
  /** Follow a bruv: link, given its data-bruv form ("card:<id>"). */
  follow: (target: string) => void
  searchCards: (query: string) => Promise<SearchResult[]>
  /** The card whose chat this is, while it's loaded. */
  card: () => { id: string; title: string; type: string } | null
  /** The mention menu's detail for a card (localized). */
  cardLabel: string
}

const MENTION_LIMIT = 8

export function cardChatHost(o: CardChatHostOptions): UncliChatHost {
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
    turnContext: () => {
      const card = o.card()
      return card ? cardTurnContext(card) : null
    },
  }
}
