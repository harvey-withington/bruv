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
import type { LLMConfig, SearchResult, UncliProvider, UncliRunsOn, UncliStatus } from './types'

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
  /** The CLI and model new sessions open on (runsOnFor); null = UNCLI's default. */
  runsOn?: UncliRunsOn | null
}

/** The CLI and model the settings choose for chat's kind
 *  (LLMConfig.uncli_chats); null = UNCLI's default. */
export function runsOnFor(config: Pick<LLMConfig, 'uncli_chats'> | null, chat: ChatRef): UncliRunsOn | null {
  const choice = config?.uncli_chats?.[chat.kind === 'card' ? 'card_chat' : 'project_chat']
  return choice?.provider || choice?.model ? choice : null
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

/** Starts a new session for the chat, with its meta, on its CLI and
 *  model, and records it as the chat's. A chosen CLI that can't run now
 *  (not installed, signed out) rejects with UNCLI's reason, and no
 *  session is made. */
export async function startNewChat(el: UncliChat, sessions: ChatSessions): Promise<void> {
  const { provider, model } = sessions.runsOn ?? {}
  const created = await el.newSession('chat', '', model ?? '', provider ? { meta: sessions.meta, provider } : { meta: sessions.meta })
  await sessions.set(created)
}

// --- Choosing what chats open on (Settings → AI) ---

/** A <select> value for a choice: '' is UNCLI's default. */
export function runsOnValue(r: UncliRunsOn | null | undefined): string {
  return r?.provider || r?.model ? JSON.stringify([r.provider ?? '', r.model ?? '']) : ''
}

export function parseRunsOn(value: string): UncliRunsOn | null {
  if (!value) return null
  const [provider, model] = JSON.parse(value) as [string, string]
  return { provider, model }
}

export type RunsOnOption = { value: string; label: string; disabled: boolean }
/** One CLI's choices; a CLI that can't run now is listed, disabled, with why. */
export type RunsOnGroup = { label: string; options: RunsOnOption[] }

/** The choices for what chats open on, a group per CLI: its default
 *  model, then each model. t is the surface's (uncli.runs_on_*). */
export function runsOnGroups(providers: UncliProvider[], t: (key: string, params?: Record<string, string>) => string): RunsOnGroup[] {
  return providers.map(p => {
    const disabled = !p.ready
    const defaultLabel = p.models.find(m => m.id === p.default_model)?.label ?? p.default_model
    return {
      label: p.ready ? p.name : t('uncli.runs_on_unready', { cli: p.name, reason: p.reason ?? '' }),
      options: [
        {
          value: runsOnValue({ provider: p.id }),
          label: defaultLabel ? t('uncli.runs_on_cli_default', { cli: p.name, model: defaultLabel }) : t('uncli.runs_on_cli_picks', { cli: p.name }),
          disabled,
        },
        ...p.models.map(m => ({ value: runsOnValue({ provider: p.id, model: m.id }), label: `${p.name} · ${m.label}`, disabled })),
      ],
    }
  })
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

// --- Suggest mode's review, once per turn ---

/** The history message BRUV's server stages a UNCLI chat's changes on
 *  (supervisor.UncliStagedContent: keep the two the same). */
export const UNCLI_STAGED_CONTENT = 'Edits proposed in the UNCLI chat.'

type StagedEdit = { id: string; label: string; detail?: string; status: string; error?: string }
type HistoryMessage = { id: string; content: string; pending_edits?: StagedEdit[] }

/** A chat's history, where Suggest mode stages its changes, and
 *  Apply(Project)PendingEdits, which applies acceptIDs and rejects the
 *  message's other pending changes. */
type History = { messages?: HistoryMessage[] | null } | null

export interface StagedHistory {
  load(): Promise<History>
  apply(messageId: string, acceptIDs: string[]): Promise<History>
}

/** The changes Suggest staged that still wait, by history message. */
export function stagedGroups(messages: HistoryMessage[]): { messageId: string; edits: StagedEdit[] }[] {
  return messages
    .filter(m => m.content === UNCLI_STAGED_CONTENT)
    .map(m => ({ messageId: m.id, edits: (m.pending_edits ?? []).filter(e => e.status === 'pending') }))
    .filter(g => g.edits.length > 0)
}

export interface ReviewStrings {
  /** "Apply 3 suggested changes?" */
  title: (count: number) => string
  /** "From this turn." */
  detail: string
}

/** What the user made of a turn's staged changes. */
export interface ReviewOutcome {
  applied: number
  failed: number
  /** For the model's next turn (nextTurnNote): which were applied, turned
   *  down or failed, and why. Model-facing, so not localized. */
  note: string
}

/** The model-facing account of a review: each staged change by its label,
 *  once its history message has the outcome. */
export function reviewNote(edits: StagedEdit[]): string {
  const labels = (status: string) => edits.filter(e => e.status === status).map(e => `"${e.label}"`)
  const parts = [
    labels('accepted').length ? `applied ${labels('accepted').join(', ')}` : '',
    labels('rejected').length ? `turned down ${labels('rejected').join(', ')}` : '',
    ...edits.filter(e => e.status === 'failed').map(e => `accepted "${e.label}" but it failed: ${e.error || 'no reason given'}`),
  ].filter(Boolean)
  return parts.length ? `Since your last turn, the user reviewed the changes you suggested: ${parts.join('; ')}.` : ''
}

/** A note for the model's next turn only, such as a review's outcome. */
export function nextTurnNote(): { set: (note: string) => void; take: () => string | null } {
  let note = ''
  return {
    set: n => { note = n },
    take: () => {
      const n = note
      note = ''
      return n || null
    },
  }
}

/** Suggest mode's review: when a turn ends (uncli-idle), one UNCLI review
 *  card for the changes it staged; Apply applies the ones left ticked and
 *  rejects the rest, Discard rejects them all, and a withdrawn card
 *  (cleanup, a new turn's review) leaves them waiting for the next one.
 *  done hears the outcome; error, a failure. Returns the cleanup. */
export function reviewStagedChanges(
  el: UncliChat,
  history: StagedHistory,
  strings: ReviewStrings,
  on: { done: (outcome: ReviewOutcome) => void; error: (e: unknown) => void },
): () => void {
  let open: AbortController | null = null
  const review = async () => {
    open?.abort()
    const controller = new AbortController()
    open = controller
    try {
      const groups = stagedGroups((await history.load())?.messages ?? [])
      const edits = groups.flatMap(g => g.edits)
      if (!edits.length || controller.signal.aborted) return
      const answer = await el.review(
        {
          title: strings.title(edits.length),
          detail: strings.detail,
          items: edits.map(e => ({ id: e.id, label: e.label, detail: e.detail || undefined })),
        },
        { signal: controller.signal },
      )
      if (answer.action !== 'apply' && answer.action !== 'discard') return
      const ticked = new Set(answer.action === 'apply' ? answer.items : [])
      const reviewed: StagedEdit[] = []
      for (const g of groups) {
        const after = await history.apply(g.messageId, g.edits.filter(e => ticked.has(e.id)).map(e => e.id))
        const outcome = (after?.messages ?? []).find(m => m.id === g.messageId)?.pending_edits ?? []
        reviewed.push(...outcome.filter(e => g.edits.some(s => s.id === e.id)))
      }
      on.done({
        applied: reviewed.filter(e => e.status === 'accepted').length,
        failed: reviewed.filter(e => e.status === 'failed').length,
        note: reviewNote(reviewed),
      })
    } catch (e) {
      on.error(e)
    } finally {
      if (open === controller) open = null
    }
  }
  const onIdle = () => void review()
  el.addEventListener('uncli-idle', onIdle)
  return () => {
    el.removeEventListener('uncli-idle', onIdle)
    open?.abort()
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
