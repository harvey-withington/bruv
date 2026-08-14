// --- Hierarchy models (mirror internal/model/model.go) ---

export type Brand = {
  id: string
  name: string
  slug: string
  description?: string
  icon?: string
  logo?: string
  website?: string
  system_prompt?: string
  position: number
  created_at: string
  updated_at: string
}

export type Stream = {
  id: string
  brand_id: string
  name: string
  slug: string
  description?: string
  icon?: string
  position: number
  created_at: string
  updated_at: string
}

export type Project = {
  id: string
  stream_id: string
  brand_id: string
  name: string
  slug: string
  description?: string
  icon?: string
  position: number
  created_at: string
  updated_at: string
}

export type Category = {
  id: string
  project_id: string
  name: string
  slug: string
  description?: string
  icon?: string
  position: number
  accepted_types?: string[]   // nil/empty = all card types accepted
  created_at: string
  updated_at: string
}

// PromotedProject is the result of promoting a card into its own project:
// the new project plus its default category (where the card is pinned).
export type PromotedProject = {
  project: Project
  category: Category
}

// Label is a project-scoped label that can be assigned to cards.
// (Presented as "tags" in some user-facing surfaces.)
export type Label = {
  id: string
  name: string
  color: string
  icon?: string
}

// Pin is a card's raw membership in a Project/Category (model.Pin).
// For the enriched breadcrumb shape see CardPin below.
export type Pin = {
  card_id: string
  project_id: string
  category_id: string
  position: number
  pinned_at: string        // ISO 8601
}

// --- Card domain types ---

export type ChecklistItem = {
  id: string
  text: string
  done: boolean
}

export type ListItem = {
  id: string
  text: string
}

// Runtime list of every block type the model knows. `BlockType` derives
// from it so the union and the list can never drift — import validation
// (cardJson.ts) checks incoming blocks against this.
export const BLOCK_TYPES = ['text', 'checklist', 'list', 'media', 'url', 'divider', 'select', 'number', 'date', 'rating', 'checkbox', 'radio', 'checkbox_group', 'image', 'progress', 'alarm', 'survey', 'slide_deck'] as const

export type BlockType = (typeof BLOCK_TYPES)[number]

export type SurveyQuestionType = 'text' | 'rating' | 'choice'

export type SurveyQuestion = {
  id: string
  prompt: string
  type: SurveyQuestionType
  options?: string[]      // for type='choice'
  multi?: boolean         // for type='choice': allow multi-select
  max?: number            // for type='rating'
  answer?: string | string[] | number
}

export type MediaItem = {
  id: string
  url: string
  caption?: string
  mime?: string
}

// --- Slide Deck block ---
// A deck lives inside a card as block data, so it stays pinnable, taggable,
// searchable, and AI-authorable. The live presentation position is NOT part
// of the persisted value — it's block live state (SetBlockLiveState), held
// in server memory and broadcast as `block:live`; the /present output page
// receives it via the server-side PresentCardJSON overlay.

// The slide model mirrors BRUV's Card Type ↔ Card pattern:
//   - SlideContentType  = the schema (a named set of typed fields)
//   - SlideTemplate      = a data-only renderer that declares which content
//                          types it renders and maps each type's fields to
//                          display roles. Serialisable so templates can be
//                          user-authored / shared via a registry later.
//   - Slide              = an instance: a content type + a template + values
//                          (each literal, or bound live to a card block).

export type SlideFieldType = 'text' | 'longtext' | 'image' | 'video'

export type SlideFieldDef = {
  key: string
  label: string
  type: SlideFieldType
}

export type SlideContentType = {
  id: string
  name: string
  fields: SlideFieldDef[]
}

// Display roles a template assigns a content-type field to. The generic
// renderer maps role → visual treatment; the field's type informs media
// handling (image vs video). New roles extend the format, not the code.
// `avatar` (small round image) and `meta` (muted metadata line) are
// platform-neutral additions for social-post layouts.
export type SlideDisplayRole =
  | 'heading' | 'subheading' | 'body' | 'quote' | 'attribution' | 'media' | 'caption'
  | 'avatar' | 'meta'

export type SlideFieldMapping = {
  field: string          // content-type field key
  role: SlideDisplayRole
}

export type SlideAnimation = 'fadeIn' | 'zoomIn' | 'slideInLeft' | 'slideInRight' | 'slideInUp' | 'none'

export type TemplateStyles = {
  backgroundColor?: string
  textColor?: string
  accentColor?: string
  fontFamily?: string
  // post-card layout tokens — the platform "skin" (Facebook blue, X black…).
  // Renderers map these onto the layout's CSS custom properties; absent
  // tokens keep the layout's built-in defaults.
  cardBackgroundColor?: string
  cardBorderColor?: string
  cardTextColor?: string
  cardMutedColor?: string
  cardAccentColor?: string
  cardFontFamily?: string
  // Extension point for user-authored templates (borders, padding, …).
}

// Layouts are part of the renderer's vocabulary (like display roles):
// 'stack' is the centered column; 'post-card' is a social-post card —
// avatar/name/handle header (first avatar/subheading/meta items), body
// text, media, remaining meta as the footer, platform glyph from
// values.platform. Templates DECLARE a layout; renderers implement it.
export type SlideLayout = 'stack' | 'post-card'

// SlideTemplate is data, not code — a single generic renderer interprets it.
export type SlideTemplate = {
  id: string
  name: string
  supportedContentTypes: string[]                 // content-type ids it can render
  fieldMap: Record<string, SlideFieldMapping[]>   // contentTypeId → field→role mappings
  entrance: SlideAnimation
  durationMs: number
  layout?: SlideLayout                            // default 'stack'
  styles?: TemplateStyles
  // Regex SOURCE tested against a slide's capture URL (values.url) when the
  // slide's template is 'auto'. Powers Auto selection ONLY — never restricts
  // the manual picker. Absent = never auto-matched (generic templates).
  urlHint?: string
}

// Vault-level template preferences (GetTemplatePrefs / SetTemplatePrefs).
export type TemplatePrefs = {
  // Priority for Auto multi-match: template ids first-match-wins in this
  // order; unlisted templates follow in registration order.
  order?: string[]
  // Per-template urlHint replacement (regex source). Invalid regexes fall
  // back to the template's built-in hint.
  urlOverrides?: Record<string, string>
}

export type Slide = {
  id: string
  contentTypeId: string
  templateId?: string                  // AUTO_TEMPLATE_ID = resolve from capture URL (stamped by the
                                       // clipper); explicit id = user pin (always wins); undefined =
                                       // legacy fallback (first template supporting the type)
  cardId?: string                      // optional linked card, for field bindings
  values: Record<string, string>       // field key → literal value
  bindings?: Record<string, string>    // field key → card block id (resolved live)
  title?: string                       // manual display-label override for row lists (never rendered
                                       // on the slide); when absent, labels follow the linked card's
                                       // live title — see shared/slideLabel.ts
  durationSec?: number                 // 0 / undefined = manual advance
  notes?: string                       // presenter notes — control surface only
  thumbnail?: string                   // small data-URI preview for the compact row
  overflow?: 'fit' | 'scroll'          // oversized content: 'fit' (default) shrinks text then
                                       // transform-scales the whole frame — a slide can never clip;
                                       // 'scroll' renders full-size and scrolls (console-paged on /present)
}

export type DeckTheme = {
  backgroundColor?: string
  textColor?: string
  accentColor?: string
  fontFamily?: string
  transparent?: boolean  // OBS overlay mode: transparent output background
}

export type SlideDeckValue = {
  slides: Slide[]
  // Legacy live-position field: no longer written (live state owns the
  // position), kept optional so previously-saved decks still parse.
  currentIndex?: number
  theme?: DeckTheme
}

// Transient per-block runtime state — server-held in memory, never
// persisted, lost on restart. Slide decks store their live presentation
// position and video play/pause commands here; other real-time block
// concerns can extend the shape (PresentCardJSON overlays every key
// except the persisted deck's own slides/theme).
export type BlockLiveState = {
  currentIndex?: number
  // Monotonic per-console counter; the present page executes videoAction
  // whenever it sees a new seq. Absent = no command (video stays on its
  // first frame — playback is presenter-fired, never automatic).
  videoSeq?: number
  videoAction?: 'play' | 'pause'
  // Enlarge the current slide's video to fill the output page's viewport
  // (hiding the rest of the slide) — an in-page layout state, deliberately
  // NOT the browser Fullscreen API, so OBS captures stay seamless.
  videoFull?: boolean
  // Monotonic per-console counter for scroll-mode slides: each new seq
  // pages the output one viewport in scrollDir's direction, clamped at
  // the ends (the pre-⬆ behaviour wrapped at the bottom; with both
  // directions available, clamping is the less disorienting contract).
  scrollSeq?: number
  // Monotonic per-console counter for multi-image (carousel) slides: each
  // new seq steps the output one image in carouselDir's direction,
  // wrapping in both directions.
  carouselSeq?: number
  // Direction for the most recent scrollSeq/carouselSeq bump: 1 pages
  // down/next, -1 pages up/previous. Absent = 1, so older payloads keep
  // their one-way forward semantics.
  scrollDir?: 1 | -1
  carouselDir?: 1 | -1
  // Image analogue of videoFull: enlarge the slide's visible image
  // (single or the carousel's active one) to fill the output viewport.
  // Same deliberate non-use of the Fullscreen API — OBS captures as-is.
  imageFull?: boolean
}

// Options for a server-side URL capture. categoryID "" pins nowhere
// (Inbox); the "__deck__" sentinel mirrors the deck-target card's pins.
export type CaptureOpts = {
  includeInDeck?: boolean
  deckCardID?: string
  deckBlockID?: string
  categoryID?: string
  /** The user's capture-time choices. Anything omitted falls back to the
   *  vault's CapturePrefs, so a caller with no dialog still behaves as
   *  configured. */
  title?: string
  videoVariantId?: string
  videoMode?: VideoMode
  imageMode?: ImageMode
}

// Result of CaptureFromURL/RetryCapture. pending=true means the resolver
// was blocked and the clip degraded to the link-only rung: the card and
// (when requested) the deck slide exist now, and completion happens from
// the browser extension or a later retry. pinFailed=true means a
// requested pin destination bounced (accepted-types gate, missing
// category, unpinned deck mirror) and the card is in the Inbox — the UI
// must surface it (pinError carries the reason), never celebrate.
export type CaptureResult = {
  cardId: string
  slideAppended: boolean
  platform: string
  pending: boolean
  pinFailed?: boolean
  pinError?: string
  /** Media that landed in a degraded form — e.g. a video too large to
   *  store, kept as a platform link instead. The card still works; the
   *  user needs to know the video isn't theirs to keep. */
  mediaNotes?: string[]
}

// The clip-pending marker tag — user-visible chip by design (searchable
// on every surface); the extension polls it for badge + pending list.
export const CLIP_PENDING_TAG = 'clip-pending'

// --- Capture options (plan/2026-08-02 capture options at capture time) ---
//
// Every capture defect in the 07-31/08-02 build was BRUV deciding for the
// user — which video rung, what "too large" means, thumbnail-vs-video.
// These types exist so the decision is theirs, pre-populated but never
// assumed. Defaults live per VAULT so phone, extension and desktop agree.

export type VideoMode = 'fit' | 'best' | 'smallest' | 'link' | 'skip'
export type ImageMode = 'all' | 'first' | 'link' | 'skip'
export type AskMode = 'always' | 'triggers' | 'never'

/** The user's own definition of "consequential" — a zero/false entry
 *  turns that trigger off. Asked for explicitly (Harvey, 2026-08-02:
 *  "How do we define consequential? That would need to come from the
 *  user in a setting."). */
export type CaptureTriggers = {
  videoOverMB?: number
  galleryOverCount?: number
  unsupportedUrl?: boolean
  blocked?: boolean
  pinMayReject?: boolean
}

export type CapturePrefs = {
  videoMode?: VideoMode
  /** Bounds 'fit'. Separate from videoOverMB on purpose: how much you'll
   *  STORE and when you want to be ASKED are different questions. */
  videoBudgetMB?: number
  imageMode?: ImageMode
  askMode?: AskMode
  triggers: CaptureTriggers
}

/** One rung of a video quality ladder, with the size worked out from
 *  bitrate × duration so the dialog can show "1280×720 · ~725 MB". */
export type MediaVariant = {
  id: string
  label: string
  url: string
  bitrate?: number
  estBytes?: number
}

export type CaptureMediaPreview = {
  kind: 'image' | 'video'
  url: string
  posterUrl?: string
  estBytes?: number
  variants?: MediaVariant[]
  /** The rung this vault's prefs would take — pre-select it so the
   *  common case stays one tap. Empty means "link, don't store". */
  defaultVariantId?: string
  note?: string
}

/** What capturing this URL WOULD do. Writes nothing. */
export type CapturePreview = {
  url: string
  platform: string
  supported: boolean
  /** A resolver claimed it but the platform refused the server — capture
   *  would land a pending clip for the desktop extension to complete. */
  blocked: boolean
  blockedError?: string
  title: string
  author?: string
  handle?: string
  text?: string
  publishedAt?: string
  media?: CaptureMediaPreview[]
  prefs: CapturePrefs
  /** The vault's own answer to "is this consequential?", computed from
   *  the user's thresholds. askReasons names which trigger fired
   *  ('video_large' | 'gallery_large' | 'unsupported' | 'blocked'). */
  shouldAsk: boolean
  askReasons?: string[]
}

// Result of GetLocalServerStatus — see the BackendAdapter entry.
export type LocalServerStatus = {
  addr: string
  requestedPort: number
  actualPort: number
  fallbackReason?: string
}

export type BlockMeta = {
  options?: string[]
  collapsed?: boolean
  min?: number
  max?: number
  suffix?: string
  multi?: boolean
  orientation?: 'vertical' | 'horizontal'
  // Date block: "date" (default, YYYY-MM-DD) or "date-time" (full ISO 8601 with time)
  format?: string
  // Short description shown as a helper label for template-declared fields
  description?: string
  // Alarm-specific
  alarm_time?: string      // ISO 8601 datetime for the alarm
  alarm_channels?: string  // notification channels: "in-app,system"
  alarm_fired?: boolean    // whether the alarm has already fired
}

export type Block = {
  id: string
  type: BlockType
  label: string
  key: string
  value: string | number | boolean | string[] | ChecklistItem[] | ListItem[] | MediaItem[] | SurveyQuestion[] | SlideDeckValue | { url: string; caption?: string } | null
  meta?: BlockMeta
}

// --- Card comments ---

export type CardComment = {
  id: string
  author: string
  created_at: string       // ISO 8601
  updated_at: string       // ISO 8601
  text: string
}

// --- Chat (mirror internal/model/model.go ChatMessage et al.) ---

// ToolAction records a tool call the AI made and what happened.
export type ToolAction = {
  tool: string
  input: unknown            // arguments the AI passed — shape is tool-specific
  result?: string
}

// PinSuggestion is a pending suggestion to pin the card to a category.
export type PinSuggestion = {
  category_id: string
  category_name: string
  breadcrumb: string
  reason: string
  confidence?: 'high' | 'medium' | 'low' | string
  status: 'pending' | 'accepted' | 'rejected' | string
}

// PendingEdit is a staged LLM-proposed change awaiting user approval
// in Suggest mode.
export type PendingEdit = {
  id: string
  tool: string
  input: Record<string, unknown>
  label: string
  detail: string
  status: 'pending' | 'accepted' | 'rejected'
}

export type ChatMessage = {
  id: string
  role: 'user' | 'assistant' | 'system' | string
  content: string
  timestamp: string         // ISO 8601
  tool_actions?: ToolAction[]
  pin_suggestion?: PinSuggestion
  pending_edits?: PendingEdit[]
  /** Jump-back marker for the chat panel's bookmark navigation. */
  bookmarked?: boolean
}

// ChatHistory mirrors Go's model.ChatFile — the unit returned by the
// chat load/send/pending-edit RPCs.
export type ChatHistory = {
  card_id: string
  messages: ChatMessage[] | null   // Go nil slice encodes as null
}

// --- Search / index (mirror internal/index) ---

// SearchResult has no JSON tags on the Go side, so fields arrive in
// PascalCase (Go field names).
export type SearchResult = {
  CardID: string
  Title: string
  Type: string
  Rank: number
  ProjectContext: string
}

// IndexStats mirrors Go's index.RebuildStats (no JSON tags either).
export type IndexStats = {
  CardsIndexed: number
  CardsRemoved: number
  CardsSkipped: number
  PinsIndexed: number
  Duration: number          // Go time.Duration — nanoseconds
}

// --- User preferences (mirror internal/config/preferences.go +
// ui_preferences.go) ---

// Server-zone preferences: shared by every device talking to this
// backend (config.Preferences, <configDir>/preferences.json).
export type Preferences = {
  default_category_name: string
  trello_api_key: string
  trello_api_token: string
  due_date_notify: boolean
  due_date_thresholds: string[]
  due_date_channels: string
}

// Per-device UI preferences (config.UIPreferences,
// <clientdata>/ui_preferences.json). Served by the local Wails shell
// in all desktop modes — never over RPC — so each device keeps its
// own theme/locale/layout. Browser mode falls back to localStorage.
export type UIPreferences = {
  reopen_last_repo: boolean
  theme: string             // "dark", "light", "system"
  locale: string            // e.g. "en", "es"
  confirm_before_delete: boolean
  sidebar_width: number
  type_badge_display: 'text' | 'color' | 'hidden'
  inbox_recent_cards_limit: number
  inbox_activity_limit: number
  sidebar_collapse_default: boolean
  llm_nudge_shown: boolean
  // Fixed loopback port for this device's embedded HTTP server; 0 =
  // ephemeral (default). Applied on next launch; lets URL-pairing tools
  // (web clipper) survive restarts.
  local_server_port: number
}

// --- Import / Export ---

export type TrelloArchiveMode = 'skip' | 'archive' | 'inline'

export type ProjectMember = {
  id: string
  fullName: string
  username: string
}

export type TrelloImportResult = {
  project_slug: string
  project_name: string
  categories: number
  cards: number
  labels: number
  comments: number
  archived: number
  skipped_closed: number
}

export type Attachment = {
  id: string
  name: string
  path: string
  mime: string
  size: number
  added_at: string
}

export type Card = {
  id: string
  title: string
  // Intrinsic primary description (markdown, mentions). Every card has
  // one — empty string when unset. Replaced the legacy
  // `fields.description` magic-key pattern.
  description: string
  type: string
  tags: string[]
  due_date: string | null
  created_at: string
  updated_at?: string
  context_level?: string    // "isolated" | "project" | "brand" | "global"
  labels?: string[]         // label IDs from the project's tags.json
  folder?: CardFolder       // workspace subfolder binding (Card Folders)
  blocks: Block[]
  file_attachments: Attachment[]
  members?: string[]
}

// CardPin mirrors Go's card.CategoryPath — a category's full
// hierarchy position with display names and breadcrumb. Returned by
// GetCardPinBreadcrumbs and ListAllCategories (the latter is exposed
// under the CategoryPath alias below for call-site readability).
export type CardPin = {
  brandSlug: string
  streamSlug: string
  projectSlug: string
  categorySlug: string
  brandName: string
  streamName: string
  projectName: string
  categoryName: string
  brandDescription?: string
  streamDescription?: string
  projectDescription?: string
  categoryDescription?: string
  projectId: string
  categoryId: string
  breadcrumb: string
  acceptedTypes?: string[]
  pinnedProjectId?: string
}

export type CategoryPath = CardPin

// --- Identity: who the user is (editable, visible to collaborators & LLMs) ---
// Per-device identity (used by the activity log to shard writes) lives
// outside the profile in clientdata — see internal/config/identity.go.
export type UserProfile = {
  display_name: string
  role: string
  bio: string
  expertise: string[]
  avatar_url: string
}

// --- Auth: session/provider state (not user-editable) ---
export type AuthInfo = {
  id: string
  provider: string   // "local", "google", "github", etc.
  email: string
  authenticated: boolean
  username: string
}

// --- Card types ---

export type CardTypeInfo = {
  id: string
  label: string
  color: string
  icon?: string
  description: string
  ai_hint?: string
  template_id?: string
  builtin: boolean
}

export type UserCardType = {
  id: string
  label: string
  color: string
  icon?: string
  description: string
  ai_hint?: string
  template_id?: string
}

export type CardTemplate = {
  id: string
  name: string
  blocks: Block[]
}

// --- Activity log ---

export type ActivityEntry = {
  id: string
  timestamp: string        // ISO 8601
  actor_id?: string        // stable identity (UserProfile.user_id for users, model name for LLMs)
  actor: string            // display name snapshot at write time
  actor_type: 'user' | 'llm'
  action: string           // e.g. "created", "updated_title", "updated_field", "pinned"
  field: string            // human label of the changed field (may be empty)
  card_id: string
  card_title: string
  brand_slug?: string
  stream_slug?: string
  project_slug?: string
  brand_name?: string
  stream_name?: string
  project_name?: string
  category_name?: string
}

// --- Recently updated card (enriched with first-pin path) ---

export type RecentCard = {
  id: string
  title: string
  type: string
  updated_at: string       // ISO 8601
  tags: string[]
  due_date?: string
  brand_slug?: string
  stream_slug?: string
  project_slug?: string
  brand_name?: string
  stream_name?: string
  project_name?: string
  category_name?: string
  breadcrumb?: string
}

// --- Agent ---

export type AgentStatus = 'idle' | 'running' | 'failed' | 'disabled'

export type AgentConfig = {
  enabled: boolean
  goal: string
  schedule: string
  allowed_tools: string[]
  status: AgentStatus
  notify_on: string[]
  notify_channel: string
  llm_account_id: string
  llm_model: string
  last_run_at: string | null
  next_run_at: string | null
  max_tokens_budget: number
  run_started_at: string | null
  min_interval_minutes: number
  max_retries: number
  retry_count: number
  retry_backoff_minutes: number
  cost_budget_usd: number
  cost_spent_usd: number
  start_date: string | null
  end_date: string | null
  active_window_start: string
  active_window_end: string
  one_shot: boolean
  timezone: string
}

export type AgentRun = {
  id: string
  card_id: string
  started_at: string
  finished_at: string | null
  status: string
  summary: string
  tool_calls: { tool: string; input: Record<string, unknown>; result?: string }[]
  error: string
  tokens_used: number
}

export type AgentFile = {
  card_id: string
  config: AgentConfig
  runs: AgentRun[]
}

export type AgentSummary = {
  card_id: string
  card_title: string
  enabled: boolean
  status: string
  schedule: string
  goal: string
  is_running: boolean
  last_run_at: string | null
  next_run_at: string | null
  last_run_status: string | null
  last_run_summary: string | null
  last_run_tokens: number | null
  last_run_error: string | null
  one_shot: boolean
  start_date: string | null
  end_date: string | null
}

export type AgentRunEntry = {
  id: string
  card_id: string
  card_title: string
  started_at: string
  finished_at: string | null
  duration_secs: number | null
  status: string
  summary: string | null
  error: string | null
  tokens_used: number
  tool_count: number
  model_used: string
  estimated_cost: number
}

export type AgentAnalytics = {
  total_agents: number
  enabled_agents: number
  total_runs: number
  success_runs: number
  failed_runs: number
  total_tokens: number
  total_cost: number
  cost_today: number
  cost_7d: number
  cost_by_model: Record<string, number>
}

// --- Notifications ---

export type AppNotification = {
  id: string
  title: string
  body: string
  source: string
  card_id?: string
  card_title?: string
  created_at: string
  read: boolean
}

export type NotifyConfig = {
  system_enabled: boolean
  smtp_host: string
  smtp_port: number
  smtp_username: string
  smtp_password: string
  smtp_from_addr: string
  smtp_to_addr: string
  smtp_tls: boolean
  webhook_url: string
  webhook_auth_header: string
}

// --- LLM accounts ---
export type LLMAccount = {
  id: string
  label: string
  provider: string
  model: string
  api_key: string
  base_url: string
  is_default: boolean
}

// --- LLM: AI-specific configuration (grows independently) ---
export type LLMConfig = {
  context: string
  provider: string
  model: string
  api_key: string
  base_url: string
  ai_mode: string
  min_confidence: string
}

// --- Backend capabilities ---
export type BackendCapabilities = {
  hasLocalFilesystem: boolean
  hasAuth: boolean
  hasRealtime: boolean
}

// --- Real-time events ---
//
// `type` is the topic name (see shared/adapters/topics.ts for the
// full list the backend publishes). Topic-specific payload fields are
// spread alongside `type`; their shapes are defined by the Go
// publishers and vary per topic, so handlers narrow them as needed
// (a per-topic discriminated union is a follow-up).
export type BackendEvent = {
  type: string
  [field: string]: unknown
}

export type EventCallback = (event: BackendEvent) => void

// --- Backend adapter interface ---
export interface BuildInfo {
  version: string
  build_date: string
  os: string
  arch: string
  go_version: string
}

export interface UpdateCheckResult {
  status: 'up_to_date' | 'update_available' | 'error'
  current_version: string
  latest_version?: string
  release_url?: string
  release_notes?: string
  published_at?: string
  error?: string
}

export type CardTypesImportMode = 'replace' | 'merge' | 'merge_overwrite'

export interface CardTypesImportResult {
  types_added: number
  types_overwritten: number
  types_skipped: number
  templates_added: number
  templates_overwritten: number
  templates_skipped: number
}

// --- MCP (Model Context Protocol) external server support ---
//
// Users can configure external MCP servers per-repo to expose
// additional tools to agents in that repo. See docs/mcp-servers.md
// for the end-user story and internal/mcp/ for the Go implementation.
// These types mirror the Go structs one-for-one.

export type MCPHealthStatus =
  | 'disabled'
  | 'starting'
  | 'ready'
  | 'failed'
  | 'restarting'

export interface MCPServerSpec {
  name: string
  description?: string
  command: string
  args?: string[]
  /**
   * Names of environment variables this server requires. Values
   * live in the OS keychain keyed by repo + server + name so they
   * never travel when a repo is shared. Use SetMCPServerSecret to
   * set values and GetMCPServerSecretStatus to check which are
   * populated without exposing the values themselves.
   */
  env_names?: string[]
  enabled: boolean
}

export interface MCPServerHealth {
  name: string
  status: MCPHealthStatus
  last_error?: string
  tool_count: number
  protocol_version?: string
  server_name?: string
  server_version?: string
  started_at?: string
}

export interface MCPServerViewTool {
  /** The plain tool name as the server returns it (e.g. "read_file"). */
  name: string
  /**
   * The namespaced ID (e.g. "filesystem__read_file") — this is what
   * goes into per-card allowed_tools lists and what the agent tool
   * dispatch matches on.
   */
  namespace_id: string
  description: string
}

export interface MCPServerView {
  spec: MCPServerSpec
  health: MCPServerHealth
  tools: MCPServerViewTool[]
}

// --- Connections (per-machine known remote BRUV servers) ---
//
// The "Local" connection (this device's loopback backend) is implicit
// — never returned by ListConnections, never assignable as active by
// ID. active === "" means "use Local".

export type Connection = {
  id: string
  name: string
  url: string
  device_token: string
  added_at: string  // ISO 8601
}

export type ConnectionStore = {
  active: string         // "" = Local, else Connection.id
  connections: Connection[]
}

// --- Wails desktop shell globals ---
//
// When the app runs inside the Wails desktop shell, Wails injects
// `window.go` (generated Go bindings) and `window.runtime` (runtime
// helpers). Shell methods are looked up dynamically by name, so the
// binding surface is modelled as an index of optional functions.
// Cast `window` to WailsWindow instead of `any` at access sites.
export interface WailsShellAPI {
  [method: string]: ((...args: unknown[]) => Promise<unknown>) | undefined
}

export interface WailsWindow extends Window {
  go?: { main?: { ShellAPI?: WailsShellAPI } }
  runtime?: { BrowserOpenURL?: (url: string) => void }
}

export interface BackendAdapter {
  // Capabilities
  getCapabilities(): BackendCapabilities

  // Auth / identity
  GetAuthInfo(): Promise<AuthInfo>

  // User profile
  GetProfile(): Promise<UserProfile>
  SetProfile(p: UserProfile): Promise<void>

  // LLM config
  GetLLMConfig(): Promise<LLMConfig>
  SetLLMConfig(c: LLMConfig): Promise<void>

  // Connections (per-machine known remote BRUV servers)
  ListConnections(): Promise<ConnectionStore>
  AddConnection(name: string, url: string, deviceToken: string): Promise<Connection>
  RemoveConnection(id: string): Promise<void>
  UpdateConnection(id: string, name: string, url: string, deviceToken: string): Promise<Connection>
  SetActiveConnection(id: string): Promise<void>

  // Attachments — returns a short-lived signed URL for downloading
  // (or embedding in <img src>) the attachment's bytes.
  SignAttachmentURL(cardID: string, attachmentID: string): Promise<string>

  // Present — returns a long-lived signed URL for the card's read-only
  // slide-deck output page (OBS Browser Source / fullscreen display).
  SignPresentURL(cardID: string): Promise<string>

  // Live block state — transient per-block runtime state (slide decks
  // store {currentIndex}). Server-held in memory, never persisted; every
  // set broadcasts a `block:live` event. Get returns null when nothing
  // has been set. Seeding state before minting a present URL is how
  // "start presenting at slide N" works — no dedicated API needed.
  SetBlockLiveState(cardID: string, blockID: string, state: BlockLiveState): Promise<void>
  GetBlockLiveState(cardID: string, blockID: string): Promise<BlockLiveState | null>

  // Deck mutations — atomic server-side append so remote producers (the web
  // clipper foremost) never read-modify-write a whole deck over the wire.
  // The slide runs through the standard coercion (id stamping, content-type
  // field filtering) before saving.
  AppendDeckSlide(cardID: string, blockID: string, slide: Partial<Slide>): Promise<Card>

  // Server-side URL capture (the mobile share/clip pipeline). Match is the
  // cheap preflight (platform id or ""); CaptureFromURL resolves + ingests,
  // degrading to a pending clip (clip-pending tag, link-only card,
  // pre-bound slide) instead of failing when a platform blocks the server;
  // RetryCapture re-resolves an existing pending card in place — block IDs
  // survive so live slide bindings keep pointing at the same blocks.
  MatchCaptureURL(url: string): Promise<string>
  CaptureFromURL(url: string, opts: CaptureOpts): Promise<CaptureResult>
  RetryCapture(cardID: string): Promise<CaptureResult>
  /** Resolve a URL and report what capturing it WOULD do — media
   *  inventory, video ladder with sizes, and whether the user's own
   *  triggers say to show the dialog. Writes nothing. */
  PreviewCapture(url: string): Promise<CapturePreview>
  GetCapturePrefs(): Promise<CapturePrefs>
  SetCapturePrefs(prefs: CapturePrefs): Promise<void>

  // Vault-level slide-template preferences (Auto priority order +
  // per-template urlHint regex overrides). Stored blindly server-side;
  // regex validity is a renderer concern.
  GetTemplatePrefs(): Promise<TemplatePrefs>
  SetTemplatePrefs(prefs: TemplatePrefs): Promise<void>

  // Presentation gate — presenting is explicit start/stop, not inferred.
  // While closed, /present-data serves a waiting payload (the signed URL
  // stays valid; output pages resume when reopened). Transitions publish
  // present:active / present:idle; ListPresentingCards serves initial
  // state. In-memory: a server restart closes every gate.
  SetPresenting(cardID: string, active: boolean): Promise<void>
  ListPresentingCards(): Promise<string[]>

  // SetActiveRepo persists the user's repo choice for the active
  // connection. The frontend reloads after calling it so the cloud
  // adapter re-resolves the URL prefix to /repos/<id>/.
  SetActiveRepo(repoID: string): Promise<void>

  // Real-time events (no-op for local)
  subscribe(cb: EventCallback): void
  unsubscribe(cb: EventCallback): void

  // Repository / workspace management
  Version(): Promise<string>
  GetBuildInfo(): Promise<BuildInfo>
  OpenConfigFolder(): Promise<void>
  OpenLogsFolder(): Promise<void>

  // How this desktop process bound its loopback HTTP transport. When
  // actualPort differs from requestedPort the port was taken and the app
  // moved — URL-keyed pairings (clipper, phone) will be pointing at the
  // old one, so the UI announces it. Shell-only: a remote connection has
  // no local server to report on.
  GetLocalServerStatus(): Promise<LocalServerStatus>
  OpenBugReportURL(): Promise<void>
  CheckForUpdates(): Promise<UpdateCheckResult>
  ExportCardTypesToFile(filePath: string): Promise<void>
  ImportCardTypesFromFile(filePath: string, mode: CardTypesImportMode): Promise<CardTypesImportResult>
  ImportCardTypesFromRepo(otherRepoPath: string, mode: CardTypesImportMode): Promise<CardTypesImportResult>

  // MCP servers — per-repo external tool providers
  ListMCPServers(): Promise<MCPServerView[]>
  AddMCPServer(spec: MCPServerSpec): Promise<void>
  UpdateMCPServer(spec: MCPServerSpec): Promise<void>
  DeleteMCPServer(name: string): Promise<void>
  SetMCPServerSecret(serverName: string, envVarName: string, value: string): Promise<void>
  GetMCPServerSecretStatus(serverName: string): Promise<Record<string, boolean>>
  RestartMCPServer(name: string): Promise<void>
  // Native dialog methods kept on the cloud-adapter contract because
  // the cloud adapter routes them through the Wails Shell binding
  // (see SHELL_METHODS in cloud.ts). In browser mode they reject
  // with a clear "only available in Wails desktop shell" error.
  PickFolder(title: string): Promise<string>
  PickFile(title: string, filterName: string, filterPattern: string): Promise<string>
  PickSaveFile(title: string, defaultName: string, filterName: string, filterPattern: string): Promise<string>
  // Repo registry CRUD lives on transport HTTP routes (POST /repos,
  // PATCH /repos/<id>, DELETE /repos/<id>) reached via the helpers
  // in lib/repos.svelte.ts — not on this RPC surface. Per-repo data
  // RPCs (GetRepoDescription, etc.) stay here.
  GetRepoDescription(): Promise<string>
  UpdateRepoDescription(description: string): Promise<void>
  // GetCurrentRepo asks the backend whether it currently has a repo
  // open. Returns null when the backend is the desktop and no repo
  // has been opened yet. Returns repo info when the backend is a
  // remote server (which always has its install-time repo open).
  GetCurrentRepo(): Promise<{ id: string; name: string; path: string; description: string } | null>

  // Brand CRUD
  CreateBrand(name: string): Promise<Brand>
  GetBrand(slug: string): Promise<Brand>
  ListBrands(): Promise<Brand[]>
  RenameBrand(slug: string, newName: string): Promise<Brand>
  UpdateBrandDescription(slug: string, description: string): Promise<Brand>
  UpdateBrandIcon(slug: string, icon: string): Promise<Brand>
  DeleteBrand(slug: string): Promise<void>

  // Stream CRUD
  CreateStream(brandSlug: string, name: string): Promise<Stream>
  ListStreams(brandSlug: string): Promise<Stream[]>
  RenameStream(brandSlug: string, streamSlug: string, newName: string): Promise<Stream>
  UpdateStreamDescription(brandSlug: string, streamSlug: string, description: string): Promise<Stream>
  UpdateStreamIcon(brandSlug: string, streamSlug: string, icon: string): Promise<Stream>
  DeleteStream(brandSlug: string, streamSlug: string): Promise<void>

  // Project CRUD
  CreateProject(brandSlug: string, streamSlug: string, name: string): Promise<Project>
  ListProjects(brandSlug: string, streamSlug: string): Promise<Project[]>
  RenameProject(brandSlug: string, streamSlug: string, projectSlug: string, newName: string): Promise<Project>
  UpdateProjectDescription(brandSlug: string, streamSlug: string, projectSlug: string, description: string): Promise<Project>
  UpdateProjectIcon(brandSlug: string, streamSlug: string, projectSlug: string, icon: string): Promise<Project>
  DeleteProject(brandSlug: string, streamSlug: string, projectSlug: string): Promise<void>
  GetProjectMembers(brandSlug: string, streamSlug: string, projectSlug: string): Promise<ProjectMember[]>
  // Promote a card into its own project: creates the project (with its
  // default category), pins the card there (existing pins untouched; tags
  // sync into the new project's palette), and optionally copies the card's
  // description onto the project.
  PromoteCardToProject(cardID: string, brandSlug: string, streamSlug: string, name: string, copyDescription: boolean): Promise<PromotedProject>

  // Category CRUD
  CreateCategory(brandSlug: string, streamSlug: string, projectSlug: string, name: string, position: number): Promise<Category>
  ListCategories(brandSlug: string, streamSlug: string, projectSlug: string): Promise<Category[]>
  RenameCategory(brandSlug: string, streamSlug: string, projectSlug: string, categorySlug: string, newName: string): Promise<Category>
  DeleteCategory(brandSlug: string, streamSlug: string, projectSlug: string, categorySlug: string): Promise<void>
  MoveCategoryCards(brandSlug: string, streamSlug: string, projectSlug: string, fromCategoryID: string, toCategoryID: string): Promise<void>
  CopyCategory(brandSlug: string, streamSlug: string, projectSlug: string, categorySlug: string): Promise<Category>
  UpdateCategoryAcceptedTypes(brandSlug: string, streamSlug: string, projectSlug: string, categorySlug: string, acceptedTypes: string[]): Promise<Category>
  UpdateCategoryDescription(brandSlug: string, streamSlug: string, projectSlug: string, categorySlug: string, description: string): Promise<Category>
  UpdateCategoryIcon(brandSlug: string, streamSlug: string, projectSlug: string, categorySlug: string, icon: string): Promise<Category>

  // Card CRUD
  CreateCard(cardType: string, title: string): Promise<Card>
  GetCard(id: string): Promise<Card>
  ListCards(): Promise<Card[]>
  DeleteCard(id: string): Promise<void>
  DuplicateCard(cardID: string, categoryID: string): Promise<Card>

  // Card updates
  UpdateCardTitle(id: string, title: string): Promise<Card>
  UpdateCardType(id: string, cardType: string): Promise<Card>
  RefreshTypeBlocks(cardID: string): Promise<Card>
  UpdateCardDescription(id: string, description: string): Promise<Card>
  UpdateCardBlocks(id: string, blocks: Block[]): Promise<Card>
  UpdateCardTags(id: string, tags: string[]): Promise<Card>
  UpdateCardLabels(id: string, labelIDs: string[]): Promise<Card>
  UpdateCardDueDate(id: string, dueDate: string): Promise<Card>

  // Pins
  PinCard(cardID: string, categoryID: string): Promise<void>
  UnpinCard(cardID: string, categoryID: string): Promise<void>
  GetCardPins(cardID: string): Promise<Pin[]>
  GetCardLocation(cardID: string): Promise<{ brandSlug: string; streamSlug: string; projectSlug: string }>
  GetProjectLocation(projectID: string): Promise<{ brandSlug: string; streamSlug: string; projectSlug: string }>
  ListAllCategories(): Promise<CategoryPath[]>
  GetCardPinBreadcrumbs(cardID: string): Promise<CardPin[]>

  // Move & reorder
  MoveCardInCategory(cardID: string, categoryID: string, newPosition: number): Promise<void>
  MoveCardToCategory(cardID: string, fromCategoryID: string, toCategoryID: string, newPosition: number): Promise<void>
  ReorderBrands(orderedSlugs: string[]): Promise<void>
  ReorderStreams(brandSlug: string, orderedSlugs: string[]): Promise<void>
  ReorderProjects(brandSlug: string, streamSlug: string, orderedSlugs: string[]): Promise<void>
  ReorderCategories(brandSlug: string, streamSlug: string, projectSlug: string, orderedSlugs: string[]): Promise<void>

  // Move & copy (cross-hierarchy)
  MoveProject(fromBrand: string, fromStream: string, projectSlug: string, toBrand: string, toStream: string): Promise<void>
  MoveStream(fromBrand: string, streamSlug: string, toBrand: string): Promise<void>
  CopyBrand(brandSlug: string): Promise<Brand>
  CopyStream(fromBrand: string, streamSlug: string, toBrand: string): Promise<Stream>
  CopyProject(fromBrand: string, fromStream: string, projectSlug: string, toBrand: string, toStream: string, position: number): Promise<Project>

  // Tag colors
  GetTagColors(): Promise<Record<string, string>>
  SetTagColor(tag: string, color: string): Promise<Record<string, string>>
  AssignTagColor(tag: string): Promise<Record<string, string>>

  // Labels (per-project)
  GetProjectLabels(brandSlug: string, streamSlug: string, projectSlug: string): Promise<Label[]>
  AddProjectLabel(brandSlug: string, streamSlug: string, projectSlug: string, name: string, color: string): Promise<Label[]>
  RemoveProjectLabel(brandSlug: string, streamSlug: string, projectSlug: string, labelID: string): Promise<Label[]>
  UpdateProjectLabel(brandSlug: string, streamSlug: string, projectSlug: string, labelID: string, name: string, color: string): Promise<Label[]>
  SetProjectLabelIcon(brandSlug: string, streamSlug: string, projectSlug: string, labelID: string, icon: string): Promise<Label[]>

  // Schema
  ListCardTypes(): Promise<CardTypeInfo[]>
  ValidateCardFields(cardType: string, fields: Record<string, unknown>): Promise<string[]>

  // User card types
  CreateUserCardType(label: string, color: string, description: string, aiHint: string, templateId: string): Promise<UserCardType>
  UpdateUserCardType(id: string, label: string, color: string, description: string, aiHint: string, templateId: string): Promise<UserCardType>
  DeleteUserCardType(id: string): Promise<void>
  UpdateUserCardTypeIcon(id: string, icon: string): Promise<UserCardType>
  UpdateBuiltinCardType(id: string, color: string, templateId: string): Promise<void>
  // Create a new user card type from an existing card's blocks: the chosen
  // blocks become the type's template (values stripped, keys/meta kept), and
  // the originating card is switched to the new type. blockIDs in keepValueBlockIDs
  // keep their current value as a predefined template structure (e.g. a
  // checklist's items). Returns the new type.
  CreateCardTypeFromCard(cardID: string, name: string, icon: string, color: string, blockIDs: string[], keepValueBlockIDs: string[]): Promise<CardTypeInfo>

  // Card templates
  ListCardTemplates(): Promise<CardTemplate[]>
  CreateCardTemplate(name: string, blocks: Block[]): Promise<CardTemplate>
  UpdateCardTemplate(id: string, name: string, blocks: Block[]): Promise<CardTemplate>
  DeleteCardTemplate(id: string): Promise<void>

  // Workspaces (M1: local origins). Vault-side state lives with the
  // project; params are positional in Go declaration order.
  GetWorkspaceState(brandSlug: string, streamSlug: string, projectSlug: string): Promise<WorkspaceState>
  AttachWorkspace(brandSlug: string, streamSlug: string, projectSlug: string, dirPath: string): Promise<Workspace>
  DetachWorkspace(brandSlug: string, streamSlug: string, projectSlug: string): Promise<void>
  RefreshWorkspaceIndex(brandSlug: string, streamSlug: string, projectSlug: string): Promise<WorkspaceIndex>
  SetWorkspaceLaunchCommand(brandSlug: string, streamSlug: string, projectSlug: string, command: string): Promise<Workspace>
  // Immediate children of ONE workspace directory (rel '' = root). The
  // file tree calls this per folder on expand, so browsing costs only the
  // folder you opened — a workspace holding node_modules, a nested .git,
  // or 80k photos never pays for them until asked. Reads the real
  // directory, so it's never stale (unlike the cached whole-tree index,
  // which exists for the adapter summary + AI).
  ListWorkspaceDir(brandSlug: string, streamSlug: string, projectSlug: string, rel: string): Promise<WorkspaceEntry[]>
  ReadWorkspaceFile(brandSlug: string, streamSlug: string, projectSlug: string, rel: string): Promise<string>
  WriteWorkspaceFile(brandSlug: string, streamSlug: string, projectSlug: string, rel: string, content: string): Promise<void>

  // Publishing a workspace as git so other devices can clone it. Runs on
  // the machine holding the files; EnableWorkspaceGitServe returns as soon
  // as the work starts and reports progress through Workspace.git_serve.
  InspectWorkspaceGitServe(brandSlug: string, streamSlug: string, projectSlug: string): Promise<GitServeReport>
  EnableWorkspaceGitServe(brandSlug: string, streamSlug: string, projectSlug: string): Promise<Workspace>
  DisableWorkspaceGitServe(brandSlug: string, streamSlug: string, projectSlug: string): Promise<Workspace>

  // Workspace Tier 1 actions — SHELL_METHODS, device-local only.
  // root = the workspace's on-disk root (origin.url for local origins).
  OpenWorkspacePath(root: string, rel: string): Promise<void>
  RevealWorkspacePath(root: string, rel: string): Promise<void>
  RunWorkspaceLaunchCommand(root: string, command: string): Promise<void>

  // Workspace checkouts — SHELL_METHODS. Cloning puts files on THIS
  // device, so the whole lifecycle runs in the desktop shell.
  GetWorkspaceCheckout(workspaceID: string): Promise<WorkspaceCheckoutInfo>
  MaterializeWorkspace(workspaceID: string, brandSlug: string, streamSlug: string, projectSlug: string, destOverride: string): Promise<void>
  PullWorkspaceCheckout(workspaceID: string): Promise<WorkspaceSyncResult>
  PushWorkspaceCheckout(workspaceID: string): Promise<WorkspaceSyncResult>
  /** Combines the two sides after they've diverged. Aborts on conflict. */
  MergeWorkspaceCheckout(workspaceID: string): Promise<WorkspaceSyncResult>
  /** Drops BRUV's record of the copy; never touches the files. */
  ForgetWorkspaceCheckout(workspaceID: string): Promise<void>

  // Workspace folder templates (vault content; ref = vault-relative id or absolute path)
  ListWorkspaceTemplates(): Promise<WorkspaceTemplateEntry[]>
  GetWorkspaceTemplateParams(ref: string): Promise<WorkspaceTemplateParameter[]>
  PreviewWorkspaceTemplate(ref: string, values: Record<string, string>): Promise<WorkspaceTemplatePreview>
  GenerateWorkspaceFromTemplate(brandSlug: string, streamSlug: string, projectSlug: string, ref: string, targetParent: string, values: Record<string, string>): Promise<Workspace>
  InspectWorkspaceTemplateFolder(dir: string): Promise<WorkspaceTemplateInspection>
  ImportWorkspaceTemplate(srcDir: string, brandSlug: string): Promise<WorkspaceTemplateEntry>
  SaveWorkspaceTemplate(ref: string, tpl: WorkspaceTemplateDescriptor): Promise<void>
  // Delete a vault-resident template folder (vault-relative id only).
  DeleteWorkspaceTemplate(ref: string): Promise<void>

  // Card Folders: bind a card to a subfolder of its project's workspace,
  // generated from a template. Workspace-resident templates list first.
  ListProjectTemplates(brandSlug: string, streamSlug: string, projectSlug: string): Promise<WorkspaceTemplateEntry[]>
  GenerateCardFolder(brandSlug: string, streamSlug: string, projectSlug: string, cardID: string, ref: string, targetRel: string, values: Record<string, string>): Promise<Card>
  ClearCardFolder(cardID: string): Promise<Card>
  LinkCardFolder(brandSlug: string, streamSlug: string, projectSlug: string, cardID: string, rel: string): Promise<Card>

  // Index / search
  SearchCards(query: string, limit: number): Promise<SearchResult[]>
  SearchOrphanedCards(query: string, limit: number): Promise<SearchResult[]>
  // Most recently updated cards, newest first — pre-populates pickers
  // before the user types a query. Same shape as SearchCards.
  RecentCards(limit: number): Promise<SearchResult[]>
  GetCardProjectContext(cardID: string): Promise<string>
  RebuildIndex(): Promise<IndexStats>
  RefreshIndex(): Promise<IndexStats>
  ListCardIDsInCategory(categoryID: string): Promise<string[]>
  ListOrphanedCardIDs(): Promise<string[]>
  ListCardIDsByTag(tag: string): Promise<string[]>

  // Agent card states — cardID → enabled, present for every
  // card with an agent configuration on disk (enabled or disabled).
  ListAgentCardStates(): Promise<Record<string, boolean>>

  // Notifications
  GetNotifyConfig(): Promise<NotifyConfig>
  SetNotifyConfig(c: NotifyConfig): Promise<void>
  GetNotifications(): Promise<AppNotification[]>
  MarkNotificationRead(id: string): Promise<void>
  MarkAllNotificationsRead(): Promise<void>
  ClearAllNotifications(): Promise<void>
  DeleteNotification(id: string): Promise<void>

  // Category details
  GetCategoryAcceptedTypes(categoryID: string): Promise<string[] | null>

  // Agent
  ValidateSchedulePreview(schedule: string, startDate: string, endDate: string, timezone: string, count: number): Promise<string[]>
  GetAgentConfig(cardID: string): Promise<AgentFile>
  SaveAgentConfig(cardID: string, config: AgentConfig): Promise<void>
  GetAgentRuns(cardID: string): Promise<AgentRun[]>
  TriggerAgent(cardID: string): Promise<void>
  CancelAgent(cardID: string): Promise<void>
  ClearAgentRuns(cardID: string): Promise<void>
  // Remove a card's agent entirely: config, run history, and index
  // state. Fails while the agent is executing — cancel it first.
  DeleteAgent(cardID: string): Promise<void>
  PauseAllAgents(): Promise<void>
  ResumeAllAgents(): Promise<void>
  GetAgentSchedulerStatus(): Promise<{ active: boolean; paused: boolean; runningCount: number }>
  GetAllAgents(): Promise<AgentSummary[]>
  GetAllAgentRuns(limit: number): Promise<AgentRunEntry[]>
  GetAgentAnalytics(): Promise<AgentAnalytics>
  ForceQuit(): Promise<void>

  // Chat
  LoadChatHistory(cardID: string): Promise<ChatHistory>
  SendChatMessage(cardID: string, userMessage: string): Promise<ChatHistory>

  // Project chat
  LoadProjectChatHistory(brandSlug: string, streamSlug: string, projectSlug: string): Promise<ChatHistory>
  SendProjectChatMessage(brandSlug: string, streamSlug: string, projectSlug: string, userMessage: string, contextLevel: string): Promise<ChatHistory>
  ClearProjectChatHistory(brandSlug: string, streamSlug: string, projectSlug: string): Promise<void>
  ClearCardChatHistory(cardID: string): Promise<void>
  ToggleChatBookmark(cardID: string, messageID: string): Promise<ChatHistory>
  ToggleProjectChatBookmark(brandSlug: string, streamSlug: string, projectSlug: string, messageID: string): Promise<ChatHistory>

  // LLM accounts
  GetLLMAccounts(): Promise<LLMAccount[]>
  SaveLLMAccounts(accounts: LLMAccount[]): Promise<void>
  TestLLMAccountConnection(accountID: string): Promise<string>

  // LLM utilities
  IsLLMConfigured(): Promise<boolean>
  TestLLMConnection(): Promise<string>
  TestSystemNotification(): Promise<void>

  // Pin suggestions (from AI)
  AcceptPinSuggestion(cardID: string, messageID: string): Promise<void>
  RejectPinSuggestion(cardID: string, messageID: string): Promise<void>

  // Pending edits (Suggest mode)
  AcceptPendingEdit(cardID: string, msgID: string, editID: string): Promise<ChatHistory>
  RejectPendingEdit(cardID: string, msgID: string, editID: string): Promise<ChatHistory>
  AcceptAllPendingEdits(cardID: string, msgID: string): Promise<ChatHistory>
  RejectAllPendingEdits(cardID: string, msgID: string): Promise<ChatHistory>
  ApplyPendingEdits(cardID: string, msgID: string, acceptIDs: string[]): Promise<ChatHistory>
  ApplyProjectPendingEdits(brandSlug: string, streamSlug: string, projectSlug: string, msgID: string, acceptIDs: string[]): Promise<ChatHistory>

  // Attachments
  AddCardAttachment(cardID: string, name: string, data: string): Promise<Card>
  RemoveCardAttachment(cardID: string, attachmentID: string): Promise<Card>

  // Comments
  ListCardComments(cardID: string): Promise<CardComment[]>
  AddCardComment(cardID: string, author: string, text: string): Promise<CardComment>
  UpdateCardComment(cardID: string, commentID: string, text: string): Promise<CardComment>
  DeleteCardComment(cardID: string, commentID: string): Promise<void>

  // Import / Export
  ImportTrelloBoard(brandSlug: string, streamSlug: string, filePath: string, archiveMode: TrelloArchiveMode, apiKey?: string, apiToken?: string): Promise<TrelloImportResult>
  ImportTrelloBoardFromJSON(brandSlug: string, streamSlug: string, jsonContent: string, archiveMode: TrelloArchiveMode, apiKey?: string, apiToken?: string): Promise<TrelloImportResult>
  ExportProjectToFile(brandSlug: string, streamSlug: string, projectSlug: string, filePath: string): Promise<number>

  // Due-date notifications
  GetDueDateSettings(): Promise<{ enabled: boolean; thresholds: string[]; channels: string }>
  SaveDueDateSettings(enabled: boolean, thresholds: string[], channels: string): Promise<void>

  // Web Push (Phase 3 prep). The mobile service worker subscribes
  // against the public VAPID key and forwards the resulting endpoint
  // + keys via RegisterPushSubscription. Returns errPushNotConfigured
  // on a host without push wired up — clients should treat that as
  // "push is disabled here" rather than a hard failure.
  GetVapidPublicKey(): Promise<string>
  RegisterPushSubscription(deviceID: string, endpoint: string, p256dh: string, auth: string): Promise<void>
  UnregisterPushSubscription(deviceID: string): Promise<void>

  // Server-zone preferences. SetPreferences accepts a partial object —
  // the backend merges the given keys into the stored preferences
  // (config.UpdatePreferencesPartial).
  GetPreferences(): Promise<Preferences>
  SetPreferences(p: Partial<Preferences>): Promise<void>

  // Per-device UI preferences — routed to the local Wails shell (all
  // desktop modes) or localStorage (browser); never over RPC. Same
  // partial-merge semantics as SetPreferences.
  GetUIPreferences(): Promise<UIPreferences>
  SetUIPreferences(p: Partial<UIPreferences>): Promise<void>

  // Activity & recently updated
  ListActivityLog(limit: number): Promise<ActivityEntry[]>
  ListRecentlyUpdatedCards(limit: number): Promise<RecentCard[]>
}

// --- Workspaces --------------------------------------------------------------

export interface WorkspaceOrigin {
  kind: 'local' | 'git' | 'rclone'
  url?: string
  subpath?: string
  rclone_remote?: string
}

export interface WorkspaceClaim {
  device: string
  instance_id?: string
  materialized_at: string
  state?: 'clean' | 'dirty' | ''
  last_seen: string
}

export interface Workspace {
  id: string
  project_id: string
  origin: WorkspaceOrigin
  adapter: string
  launch_command?: string
  claim?: WorkspaceClaim
  /**
   * Publishing state: '' (not published), 'initializing', 'ready', 'error'.
   * When ready, devices that can't see origin.url themselves clone a
   * working copy from the host over the BRUV connection.
   */
  git_serve?: GitServeState
  git_serve_error?: string
  default_branch?: string
  created_at: string
  updated_at: string
}

export type GitServeState = '' | 'initializing' | 'ready' | 'error'

/** What publishing a workspace as git would involve — read-only report. */
export interface GitServeReport {
  path: string
  git_available: boolean
  git_version?: string
  state: GitServeState
  error?: string
  is_repo: boolean
  has_commits: boolean
  branch?: string
  /** Work in an existing repo that isn't committed — clones won't see it. */
  uncommitted_paths: number
  has_gitignore: boolean
  /** Estimated size of the initial commit, honouring .gitignore. */
  files: number
  bytes: number
  truncated: boolean
}

/** This device's working copy of a workspace (SHELL_METHOD result). */
export interface WorkspaceCheckoutInfo {
  workspace_id: string
  has_copy: boolean
  local_path?: string
  branch?: string
  status: 'idle' | 'cloning' | 'error'
  /** git's own progress line while cloning. */
  progress?: string
  error?: string
  dirty: boolean
  ahead: number
  behind: number
  /**
   * Both machines have commits the other doesn't, so neither getting nor
   * sending can proceed until they're merged. Counts are as of the last
   * fetch; the sync operations fetch first and see live numbers.
   */
  diverged: boolean
  git_available: boolean
}

/**
 * What a pull/push/merge actually did. A status rather than prose: the two
 * machines can be in several perfectly normal relationships, and each needs
 * naming in the user's language with the server's name in it.
 */
export interface WorkspaceSyncResult {
  status: 'ok' | 'up_to_date' | 'diverged' | 'server_dirty' | 'conflict'
  /** Supporting text — git's own words, or the files a merge couldn't reconcile. */
  detail?: string
}

export interface WorkspaceEntry {
  path: string
  is_dir?: boolean
  size?: number
  symlink?: boolean
}

export interface WorkspaceIndex {
  workspace_id: string
  generated_at: string
  adapter: string
  summary: string
  details?: Record<string, string>
  warnings?: string[]
  tree: WorkspaceEntry[]
}

export interface WorkspaceState {
  attached: boolean
  workspace?: Workspace
  index?: WorkspaceIndex
}

// CardFolder binds a card to a subfolder of a project Workspace (intrinsic,
// 0-or-1 per card — see plan/2026-07-05 card folders design.md).
export interface CardFolder {
  workspace_id: string
  path: string
}

// .ft/template.json parameter — camelCase keys, matching the on-disk
// Folder Templates format (not BRUV's snake_case vault convention).
export interface WorkspaceTemplateParameter {
  name: string
  type: string
  prompt: string | null
  placeholder: string | null
  defaultValue: string | null
  match: string | null
  replaceInFileNames: boolean
  replaceInFiles: boolean
}

export interface WorkspaceTemplateDescriptor {
  name: string
  description: string
  defaultTargetPath: string
  parameters: WorkspaceTemplateParameter[]
}

export interface WorkspaceTemplateEntry {
  id: string
  name: string
  description: string
  scope: string // "global", "workspace", or a brand slug
  parameters: WorkspaceTemplateParameter[]
  // The template's own output location (relative → resolved against the
  // template folder's parent; ../ allowed). Blank target field = use this.
  default_target_path: string
}

export interface WorkspaceTemplatePreviewEntry {
  sourceRel: string
  outputRel: string
  isDir: boolean
  processed: boolean
}

export interface WorkspaceTemplatePreview {
  entries: WorkspaceTemplatePreviewEntry[]
  warnings?: string[]
}

export interface WorkspaceTemplateInspection {
  is_template: boolean
  name: string
  description: string
  default_target_path: string
  parameters: WorkspaceTemplateParameter[]
  size_bytes: number
  large_warning: boolean
  issues?: string[]
}
