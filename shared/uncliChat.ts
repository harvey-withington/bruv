// The UNCLI chat engine's page side, shared by both surfaces: which
// engine BRUV's chats use, and what BRUV does for the <uncli-chat>
// element (UNCLI's EMBEDDING.md, "host"): its sign-in, @mentions of
// cards, bruv: links, what the chat is about as each turn's context, and
// the Chat / Suggest / Edit control in the message box. A chat is a
// card's or a project's. The surfaces mount the element (UncliChat,
// UncliChatSheet) and supply their own navigation, search, RPCs and
// strings.

import type { ChatHost, ComposerControl, Tone, UncliChat } from '@uncli/chat/element'
import type { AIMode } from './aiMode'
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

/** Which chat: a card's, or a project's. */
export type ChatRef = { kind: 'card'; cardId: string } | { kind: 'project'; brand: string; stream: string; project: string }

/** What a session is for, given to UNCLI when it's made (newSession's
 *  meta), so the server gives it its repo's board tools from its first
 *  start (internal/uncliengine.RepoOfMeta reads it back). */
export function chatMeta(repoId: string, chat: ChatRef): string {
  const what = chat.kind === 'card' ? `card/${chat.cardId}` : `project/${chat.brand}/${chat.stream}/${chat.project}`
  return `bruv/${repoId}/${what}`
}

/** Where a chat's session is recorded on the server (Get/Set
 *  Card|ProjectUncliSession), and the meta new ones are made with. */
export interface ChatSessions {
  get(): Promise<string>
  set(session: string): Promise<void>
  meta: string
}

/** Shows the chat in el: the session it had, or a new one recorded as
 *  the chat's. */
export async function openChatSession(el: UncliChat, sessions: ChatSessions): Promise<void> {
  const existing = await sessions.get()
  if (existing) {
    el.session = existing
    return
  }
  await startNewChat(el, sessions)
}

/** Starts a new session for the chat, with its meta, and records it as
 *  the chat's. */
export async function startNewChat(el: UncliChat, sessions: ChatSessions): Promise<void> {
  const created = await el.newSession('chat', '', '', { meta: sessions.meta })
  await sessions.set(created)
}

/** Keeps the chat's recorded session right as the element changes it:
 *  its New chat is made here, with the meta (uncli-new-chat, cancelled);
 *  a chat deleted from its header, or one UNCLI no longer has, is
 *  replaced with a new one; a session it started itself is recorded.
 *  onError hears about a failure. Returns the cleanup. */
export function followChatSessions(el: UncliChat, sessions: ChatSessions, onError: (e: unknown) => void): () => void {
  const replace = () => void startNewChat(el, sessions).catch(onError)
  const onNewChat = (e: Event) => {
    e.preventDefault()
    replace()
  }
  const onNewSession = (e: CustomEvent<{ session: string }>) => void sessions.set(e.detail.session).catch(onError)
  el.addEventListener('uncli-new-chat', onNewChat)
  el.addEventListener('uncli-deleted', replace)
  el.addEventListener('uncli-missing', replace)
  el.addEventListener('uncli-new-session', onNewSession)
  return () => {
    el.removeEventListener('uncli-new-chat', onNewChat)
    el.removeEventListener('uncli-deleted', replace)
    el.removeEventListener('uncli-missing', replace)
    el.removeEventListener('uncli-new-session', onNewSession)
  }
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

// --- The Chat / Suggest / Edit control ---

const MODE_LOOK: Record<AIMode, { icon: string; tone: Tone }> = {
  chat: { icon: 'message-circle', tone: 'success' },
  suggest: { icon: 'list-checks', tone: 'warning' },
  edit: { icon: 'pencil-line', tone: 'danger' },
}

/** The chat mode as a drop-down in the message box, drawn by UNCLI
 *  (host.composerControls). t is the surface's: chat.mode_<mode> labels
 *  and uncli.mode_<mode>_hint hints. */
export function modeControl(mode: AIMode, t: (key: string) => string, onChange: (mode: AIMode) => void): ComposerControl {
  return {
    id: 'mode',
    label: t('uncli.mode_label'),
    value: mode,
    options: (['chat', 'suggest', 'edit'] as const).map(m => ({ id: m, label: t(`chat.mode_${m}`), hint: t(`uncli.mode_${m}_hint`), ...MODE_LOOK[m] })),
    onChange: id => onChange(id as AIMode),
  }
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
  enter: NonNullable<ChatHost['enter']>
  /** The message box's controls (modeControl). */
  controls: ComposerControl[]
}

const MENTION_LIMIT = 8

export function chatHost(o: ChatHostOptions): ChatHost {
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
    composerControls: o.controls,
  }
}
