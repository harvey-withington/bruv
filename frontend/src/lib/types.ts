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

export type BlockType = 'text' | 'checklist' | 'list' | 'media' | 'url' | 'divider' | 'select' | 'number' | 'date' | 'rating' | 'checkbox' | 'radio' | 'checkbox_group' | 'image' | 'progress' | 'alarm'

export type MediaItem = {
  id: string
  url: string
  caption?: string
  mime?: string
}

export type BlockMeta = {
  options?: string[]
  collapsed?: boolean
  min?: number
  max?: number
  suffix?: string
  multi?: boolean
  orientation?: 'vertical' | 'horizontal'
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
  value: string | number | boolean | string[] | ChecklistItem[] | ListItem[] | MediaItem[] | { url: string; caption?: string } | null
  meta?: BlockMeta
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
  type: string
  tags: string[]
  due_date: string | null
  created_at: string
  fields: Record<string, string>
  blocks: Block[]
  file_attachments: Attachment[]
}

export type CardPin = {
  brandSlug: string
  streamSlug: string
  projectSlug: string
  categorySlug: string
  brandName: string
  streamName: string
  projectName: string
  categoryName: string
  projectId: string
  categoryId: string
  breadcrumb: string
  pinnedProjectId?: string
}

// --- Identity: who the user is (editable, visible to collaborators & LLMs) ---
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
  actor: string            // display name of the user or LLM model
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

export type ModelPricing = {
  InputPerMTok: number
  OutputPerMTok: number
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
  auto_pin: string
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
export type BackendEvent =
  | { type: 'card:updated'; cardId: string }
  | { type: 'category:updated'; categoryId: string }
  | { type: 'board:changed' }

export type EventCallback = (event: BackendEvent) => void

// --- Backend adapter interface ---
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

  // Real-time events (no-op for local)
  subscribe(cb: EventCallback): void
  unsubscribe(cb: EventCallback): void

  // Repository / workspace management
  Version(): Promise<string>
  HasRepository(): Promise<boolean>
  InitRepository(basePath: string, name: string): Promise<string>
  OpenRepository(id: string): Promise<void>
  CloseRepository(): Promise<void>
  PickFolder(title: string): Promise<string>
  ListRecentRepos(): Promise<Array<{ path: string; name: string; last_opened: string }>>
  RemoveRecentRepo(path: string): Promise<void>
  GetRepoDescription(): Promise<string>
  UpdateRepoDescription(description: string): Promise<void>

  // Brand CRUD
  CreateBrand(name: string): Promise<any>
  GetBrand(slug: string): Promise<any>
  ListBrands(): Promise<any[]>
  RenameBrand(slug: string, newName: string): Promise<any>
  UpdateBrandDescription(slug: string, description: string): Promise<any>
  UpdateBrandIcon(slug: string, icon: string): Promise<any>
  DeleteBrand(slug: string): Promise<void>

  // Stream CRUD
  CreateStream(brandSlug: string, name: string): Promise<any>
  ListStreams(brandSlug: string): Promise<any[]>
  RenameStream(brandSlug: string, streamSlug: string, newName: string): Promise<any>
  UpdateStreamDescription(brandSlug: string, streamSlug: string, description: string): Promise<any>
  UpdateStreamIcon(brandSlug: string, streamSlug: string, icon: string): Promise<any>
  DeleteStream(brandSlug: string, streamSlug: string): Promise<void>

  // Project CRUD
  CreateProject(brandSlug: string, streamSlug: string, name: string): Promise<any>
  ListProjects(brandSlug: string, streamSlug: string): Promise<any[]>
  RenameProject(brandSlug: string, streamSlug: string, projectSlug: string, newName: string): Promise<any>
  UpdateProjectDescription(brandSlug: string, streamSlug: string, projectSlug: string, description: string): Promise<any>
  UpdateProjectIcon(brandSlug: string, streamSlug: string, projectSlug: string, icon: string): Promise<any>
  DeleteProject(brandSlug: string, streamSlug: string, projectSlug: string): Promise<void>

  // Category CRUD
  CreateCategory(brandSlug: string, streamSlug: string, projectSlug: string, name: string, position: number): Promise<any>
  ListCategories(brandSlug: string, streamSlug: string, projectSlug: string): Promise<any[]>
  RenameCategory(brandSlug: string, streamSlug: string, projectSlug: string, categorySlug: string, newName: string): Promise<any>
  DeleteCategory(brandSlug: string, streamSlug: string, projectSlug: string, categorySlug: string): Promise<void>
  MoveCategoryCards(brandSlug: string, streamSlug: string, projectSlug: string, fromCategoryID: string, toCategoryID: string): Promise<void>
  CopyCategory(brandSlug: string, streamSlug: string, projectSlug: string, categorySlug: string): Promise<any>
  UpdateCategoryAcceptedTypes(brandSlug: string, streamSlug: string, projectSlug: string, categorySlug: string, acceptedTypes: string[]): Promise<any>
  UpdateCategoryDescription(brandSlug: string, streamSlug: string, projectSlug: string, categorySlug: string, description: string): Promise<any>
  UpdateCategoryIcon(brandSlug: string, streamSlug: string, projectSlug: string, categorySlug: string, icon: string): Promise<any>

  // Card CRUD
  CreateCard(cardType: string, title: string): Promise<any>
  GetCard(id: string): Promise<any>
  ListCards(): Promise<any[]>
  DeleteCard(id: string): Promise<void>
  DuplicateCard(cardID: string, categoryID: string): Promise<any>

  // Card updates
  UpdateCardTitle(id: string, title: string): Promise<any>
  UpdateCardType(id: string, cardType: string): Promise<any>
  RefreshTypeBlocks(cardID: string): Promise<any>
  UpdateCardFields(id: string, fields: Record<string, any>): Promise<any>
  UpdateCardBlocks(id: string, blocks: any[]): Promise<any>
  UpdateCardTags(id: string, tags: string[]): Promise<any>
  UpdateCardLabels(id: string, labelIDs: string[]): Promise<any>
  UpdateCardDueDate(id: string, dueDate: string): Promise<any>

  // Checklist
  AddChecklistItem(cardID: string, text: string): Promise<any>
  ToggleChecklistItem(cardID: string, itemID: string): Promise<any>
  RemoveChecklistItem(cardID: string, itemID: string): Promise<any>

  // Pins
  PinCard(cardID: string, projectID: string, categoryID: string): Promise<void>
  UnpinCard(cardID: string, projectID: string, categoryID: string): Promise<void>
  GetCardPins(cardID: string): Promise<any[]>
  GetCardLocation(cardID: string): Promise<{ brandSlug: string; streamSlug: string; projectSlug: string }>
  GetProjectLocation(projectID: string): Promise<{ brandSlug: string; streamSlug: string; projectSlug: string }>
  ListAllCategories(): Promise<any[]>
  GetCardPinBreadcrumbs(cardID: string): Promise<any[]>

  // Move & reorder
  MoveCardInCategory(cardID: string, projectID: string, categoryID: string, newPosition: number): Promise<void>
  MoveCardToCategory(cardID: string, projectID: string, fromCategoryID: string, toCategoryID: string, newPosition: number): Promise<void>
  ReorderBrands(orderedSlugs: string[]): Promise<void>
  ReorderStreams(brandSlug: string, orderedSlugs: string[]): Promise<void>
  ReorderProjects(brandSlug: string, streamSlug: string, orderedSlugs: string[]): Promise<void>
  ReorderCategories(brandSlug: string, streamSlug: string, projectSlug: string, orderedSlugs: string[]): Promise<void>

  // Move & copy (cross-hierarchy)
  MoveProject(fromBrand: string, fromStream: string, projectSlug: string, toBrand: string, toStream: string): Promise<void>
  MoveStream(fromBrand: string, streamSlug: string, toBrand: string): Promise<void>
  CopyBrand(brandSlug: string): Promise<any>
  CopyStream(fromBrand: string, streamSlug: string, toBrand: string): Promise<any>
  CopyProject(fromBrand: string, fromStream: string, projectSlug: string, toBrand: string, toStream: string): Promise<any>

  // Tag colors
  GetTagColors(): Promise<Record<string, string>>
  SetTagColor(tag: string, color: string): Promise<Record<string, string>>
  AssignTagColor(tag: string): Promise<Record<string, string>>

  // Labels (per-project)
  GetProjectLabels(brandSlug: string, streamSlug: string, projectSlug: string): Promise<any[]>
  AddProjectLabel(brandSlug: string, streamSlug: string, projectSlug: string, name: string, color: string): Promise<any[]>
  RemoveProjectLabel(brandSlug: string, streamSlug: string, projectSlug: string, labelID: string): Promise<any[]>
  UpdateProjectLabel(brandSlug: string, streamSlug: string, projectSlug: string, labelID: string, name: string, color: string): Promise<any[]>
  SetProjectLabelIcon(brandSlug: string, streamSlug: string, projectSlug: string, labelID: string, icon: string): Promise<any[]>

  // Schema
  ListCardTypes(): Promise<CardTypeInfo[]>
  ValidateCardFields(cardType: string, fields: Record<string, any>): Promise<string[]>

  // User card types
  CreateUserCardType(label: string, color: string, description: string, aiHint: string, templateId: string): Promise<UserCardType>
  UpdateUserCardType(id: string, label: string, color: string, description: string, aiHint: string, templateId: string): Promise<UserCardType>
  DeleteUserCardType(id: string): Promise<void>
  UpdateUserCardTypeIcon(id: string, icon: string): Promise<UserCardType>
  UpdateBuiltinCardType(id: string, color: string, templateId: string): Promise<void>

  // Card templates
  ListCardTemplates(): Promise<CardTemplate[]>
  CreateCardTemplate(name: string, blocks: Block[]): Promise<CardTemplate>
  UpdateCardTemplate(id: string, name: string, blocks: Block[]): Promise<CardTemplate>
  DeleteCardTemplate(id: string): Promise<void>

  // Index / search
  SearchCards(query: string, limit: number): Promise<any[]>
  SearchOrphanedCards(query: string, limit: number): Promise<any[]>
  GetCardProjectContext(cardID: string): Promise<string>
  RebuildIndex(): Promise<any>
  RefreshIndex(): Promise<any>
  ListCardIDsInCategory(projectID: string, categoryID: string): Promise<string[]>
  ListOrphanedCardIDs(): Promise<string[]>
  ListCardIDsByTag(tag: string): Promise<string[]>

  // Agent card IDs
  ListAgentCardIDs(): Promise<string[]>

  // Notifications
  GetNotifyConfig(): Promise<NotifyConfig>
  SetNotifyConfig(c: NotifyConfig): Promise<void>
  GetNotifications(): Promise<AppNotification[]>
  MarkNotificationRead(id: string): Promise<void>
  MarkAllNotificationsRead(): Promise<void>
  ClearAllNotifications(): Promise<void>

  // Category details
  GetCategoryAcceptedTypes(categoryID: string): Promise<string[] | null>

  // Token pricing
  GetTokenPricing(): Promise<Record<string, ModelPricing>>
  SaveTokenPricing(pricing: Record<string, ModelPricing>): Promise<void>

  // Agent
  ValidateSchedulePreview(schedule: string, startDate: string, endDate: string, timezone: string, count: number): Promise<string[]>
  GetAgentConfig(cardID: string): Promise<AgentFile>
  SaveAgentConfig(cardID: string, config: AgentConfig): Promise<void>
  GetAgentRuns(cardID: string): Promise<AgentRun[]>
  TriggerAgent(cardID: string): Promise<void>
  CancelAgent(cardID: string): Promise<void>
  ClearAgentRuns(cardID: string): Promise<void>
  PauseAllAgents(): Promise<void>
  ResumeAllAgents(): Promise<void>
  GetAgentSchedulerStatus(): Promise<{ active: boolean; paused: boolean; runningCount: number }>
  GetAllAgents(): Promise<AgentSummary[]>
  GetAllAgentRuns(limit: number): Promise<AgentRunEntry[]>
  GetAgentAnalytics(): Promise<AgentAnalytics>
  ForceQuit(): Promise<void>

  // Chat
  LoadChatHistory(cardID: string): Promise<any>
  SendChatMessage(cardID: string, userMessage: string): Promise<any>

  // Project chat
  LoadProjectChatHistory(brandSlug: string, streamSlug: string, projectSlug: string): Promise<any>
  SendProjectChatMessage(brandSlug: string, streamSlug: string, projectSlug: string, userMessage: string, contextLevel: string): Promise<any>
  ClearProjectChatHistory(brandSlug: string, streamSlug: string, projectSlug: string): Promise<void>
  ClearCardChatHistory(cardID: string): Promise<void>

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
  AcceptPendingEdit(cardID: string, msgID: string, editID: string): Promise<any>
  RejectPendingEdit(cardID: string, msgID: string, editID: string): Promise<any>
  AcceptAllPendingEdits(cardID: string, msgID: string): Promise<any>
  RejectAllPendingEdits(cardID: string, msgID: string): Promise<any>
  ApplyPendingEdits(cardID: string, msgID: string, acceptIDs: string[]): Promise<any>
  ApplyProjectPendingEdits(brandSlug: string, streamSlug: string, projectSlug: string, msgID: string, acceptIDs: string[]): Promise<any>

  // Attachments
  AddCardAttachment(cardID: string, name: string, data: string): Promise<Card>
  RemoveCardAttachment(cardID: string, attachmentID: string): Promise<Card>

  // Due-date notifications
  GetDueDateSettings(): Promise<{ enabled: boolean; thresholds: string[]; channels: string }>
  SaveDueDateSettings(enabled: boolean, thresholds: string[], channels: string): Promise<void>

  // User preferences
  GetPreferences(): Promise<any>
  SetPreferences(p: any): Promise<void>

  // Activity & recently updated
  ListActivityLog(limit: number): Promise<ActivityEntry[]>
  ListRecentlyUpdatedCards(limit: number): Promise<RecentCard[]>
}
