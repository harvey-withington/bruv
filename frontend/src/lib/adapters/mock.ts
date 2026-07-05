import type {
  BackendAdapter,
  Brand,
  Card,
  CardTemplate,
  CardTypeInfo,
  Category,
  ChatHistory,
  EventCallback,
  IndexStats,
  Preferences,
  UIPreferences,
  Project,
  Stream,
  UserCardType,
  Workspace,
  WorkspaceIndex,
  WorkspaceState,
  WorkspaceTemplateEntry,
  WorkspaceTemplateInspection,
  WorkspaceTemplatePreview,
} from '@shared/types'

// --- Stub factories ---
// Full model objects so the mocks satisfy the strictly-typed
// BackendAdapter. Override per-test where the values matter.

const mockBrand = (): Brand => ({ id: 'brand-1', name: 'Test Brand', slug: 'test-brand', position: 0, created_at: '', updated_at: '' })
const mockStream = (): Stream => ({ id: 'stream-1', brand_id: 'brand-1', name: 'Test Stream', slug: 'test-stream', position: 0, created_at: '', updated_at: '' })
const mockProject = (): Project => ({ id: 'project-1', stream_id: 'stream-1', brand_id: 'brand-1', name: 'Test Project', slug: 'test-project', position: 0, created_at: '', updated_at: '' })
const mockCategory = (slug = 'test-category', name = 'Test Category'): Category => ({ id: 'category-1', project_id: 'project-1', name, slug, position: 0, created_at: '', updated_at: '' })
const mockCard = (id = 'card-1'): Card => ({ id, title: '', description: '', type: '', tags: [], due_date: null, created_at: '', blocks: [], file_attachments: [] })
const emptyChat = (): ChatHistory => ({ card_id: '', messages: [] })
const mockWorkspace = (): Workspace => ({ id: 'ws-1', project_id: 'project-1', origin: { kind: 'local', url: 'C:/mock' }, adapter: 'plain-folder', created_at: '', updated_at: '' })
const emptyIndexStats = (): IndexStats => ({ CardsIndexed: 0, CardsRemoved: 0, CardsSkipped: 0, PinsIndexed: 0, Duration: 0 })
const mockPreferences = (): Preferences => ({
  default_category_name: 'Ideas',
  trello_api_key: '',
  trello_api_token: '',
  due_date_notify: true,
  due_date_thresholds: ['24h', '1h', '0'],
  due_date_channels: 'in-app,system',
})
const mockUIPreferences = (): UIPreferences => ({
  reopen_last_repo: true,
  theme: 'dark',
  locale: 'en',
  confirm_before_delete: true,
  sidebar_width: 260,
  type_badge_display: 'color',
  inbox_recent_cards_limit: 21,
  inbox_activity_limit: 25,
  sidebar_collapse_default: false,
  llm_nudge_shown: false,
})

/**
 * In-memory mock adapter for testing.
 * All methods return sensible defaults (empty arrays, stub objects).
 * Override individual methods in tests via Object.assign or spread.
 */
export function createMockAdapter(overrides: Partial<BackendAdapter> = {}): BackendAdapter {
  const base: BackendAdapter = {
    getCapabilities: () => ({
      hasLocalFilesystem: false,
      hasAuth: false,
      hasRealtime: false,
    }),

    GetAuthInfo: async () => ({
      id: 'test-user',
      provider: 'local',
      email: 'test@example.com',
      authenticated: true,
      username: 'testuser',
    }),

    GetProfile: async () => ({
      display_name: 'Test User',
      role: '',
      bio: '',
      expertise: [],
      avatar_url: '',
    }),
    SetProfile: async () => {},

    GetLLMConfig: async () => ({
      context: '',
      provider: '',
      model: '',
      api_key: '',
      base_url: '',
      ai_mode: 'edit',
      min_confidence: '',
    }),
    SetLLMConfig: async () => {},

    ListConnections: async () => ({ active: '', connections: [] }),
    AddConnection: async (name: string, url: string, deviceToken: string) => ({
      id: 'mock-' + Math.random().toString(36).slice(2, 10),
      name,
      url,
      device_token: deviceToken,
      added_at: new Date().toISOString(),
    }),
    RemoveConnection: async () => {},
    UpdateConnection: async (id: string, name: string, url: string, deviceToken: string) => ({
      id,
      name,
      url,
      device_token: deviceToken,
      added_at: new Date().toISOString(),
    }),
    SetActiveConnection: async () => {},

    SignAttachmentURL: async (cardID: string, attachmentID: string) => `mock://${cardID}/${attachmentID}`,
    SetActiveRepo: async () => {},

    subscribe: (_cb: EventCallback) => {},
    unsubscribe: (_cb: EventCallback) => {},

    Version: async () => '0.1.0-test',
    GetBuildInfo: async () => ({ version: '0.1.0-test', build_date: 'test', os: 'test', arch: 'test', go_version: 'test' }),
    OpenConfigFolder: async () => {},
    OpenLogsFolder: async () => {},
    OpenBugReportURL: async () => {},
    CheckForUpdates: async () => ({ status: 'up_to_date' as const, current_version: '0.1.0-test' }),
    ExportCardTypesToFile: async () => {},
    ImportCardTypesFromFile: async () => ({ types_added: 0, types_overwritten: 0, types_skipped: 0, templates_added: 0, templates_overwritten: 0, templates_skipped: 0 }),
    ImportCardTypesFromRepo: async () => ({ types_added: 0, types_overwritten: 0, types_skipped: 0, templates_added: 0, templates_overwritten: 0, templates_skipped: 0 }),
    ListMCPServers: async () => [],
    AddMCPServer: async () => {},
    UpdateMCPServer: async () => {},
    DeleteMCPServer: async () => {},
    SetMCPServerSecret: async () => {},
    GetMCPServerSecretStatus: async () => ({}),
    RestartMCPServer: async () => {},
    PickFolder: async () => '/tmp/picked',
    GetCurrentRepo: async () => null,
    GetRepoDescription: async () => '',
    UpdateRepoDescription: async () => {},

    CreateBrand: async () => mockBrand(),
    GetBrand: async () => mockBrand(),
    ListBrands: async () => [],
    RenameBrand: async () => mockBrand(),
    UpdateBrandDescription: async () => mockBrand(),
    UpdateBrandIcon: async () => mockBrand(),
    DeleteBrand: async () => {},

    CreateStream: async () => mockStream(),
    ListStreams: async () => [],
    RenameStream: async () => mockStream(),
    UpdateStreamDescription: async () => mockStream(),
    UpdateStreamIcon: async () => mockStream(),
    DeleteStream: async () => {},

    CreateProject: async () => mockProject(),
    ListProjects: async () => [],
    RenameProject: async () => mockProject(),
    UpdateProjectDescription: async () => mockProject(),
    UpdateProjectIcon: async () => mockProject(),
    DeleteProject: async () => {},
    GetProjectMembers: async () => [],

    CreateCategory: async () => mockCategory(),
    ListCategories: async () => [],
    RenameCategory: async () => mockCategory(),
    DeleteCategory: async () => {},
    MoveCategoryCards: async () => {},
    CopyCategory: async () => mockCategory('copy-category', 'Copy Category'),
    UpdateCategoryAcceptedTypes: async () => mockCategory(),
    UpdateCategoryDescription: async () => mockCategory(),
    UpdateCategoryIcon: async () => mockCategory(),

    CreateCard: async () => mockCard(),
    GetCard: async () => mockCard(),
    ListCards: async () => [],
    DeleteCard: async () => {},
    DuplicateCard: async () => mockCard('card-2'),

    UpdateCardTitle: async () => mockCard(),
    UpdateCardType: async () => mockCard(),
    RefreshTypeBlocks: async () => mockCard(),
    UpdateCardDescription: async () => mockCard(),
    UpdateCardBlocks: async () => mockCard(),
    UpdateCardTags: async () => mockCard(),
    UpdateCardLabels: async () => mockCard(),
    UpdateCardDueDate: async () => mockCard(),

    PinCard: async () => {},
    UnpinCard: async () => {},
    GetCardPins: async () => [],
    GetCardLocation: async () => ({ brandSlug: '', streamSlug: '', projectSlug: '' }),
    GetProjectLocation: async () => ({ brandSlug: '', streamSlug: '', projectSlug: '' }),
    ListAllCategories: async () => [],
    GetCardPinBreadcrumbs: async () => [],

    MoveCardInCategory: async () => {},
    MoveCardToCategory: async () => {},
    ReorderBrands: async () => {},
    ReorderStreams: async () => {},
    ReorderProjects: async () => {},
    ReorderCategories: async () => {},

    MoveProject: async () => {},
    MoveStream: async () => {},
    CopyBrand: async () => mockBrand(),
    CopyStream: async () => mockStream(),
    CopyProject: async () => mockProject(),

    GetTagColors: async () => ({}),
    SetTagColor: async () => ({}),
    AssignTagColor: async () => ({}),

    GetProjectLabels: async () => [],
    AddProjectLabel: async () => [],
    RemoveProjectLabel: async () => [],
    UpdateProjectLabel: async () => [],
    SetProjectLabelIcon: async () => [],

    ListCardTypes: async (): Promise<CardTypeInfo[]> => [],
    ValidateCardFields: async () => [],

    CreateUserCardType: async (): Promise<UserCardType> => ({ id: 'ut-1', label: '', color: '', description: '' }),
    UpdateUserCardType: async (): Promise<UserCardType> => ({ id: 'ut-1', label: '', color: '', description: '' }),
    DeleteUserCardType: async () => {},
    UpdateUserCardTypeIcon: async () => ({ id: '', label: '', color: '', description: '' }),
    UpdateBuiltinCardType: async () => {},
    CreateCardTypeFromCard: async (_cardID: string, name: string, icon: string, color: string, _blockIDs: string[], _keepValueBlockIDs: string[]): Promise<CardTypeInfo> => ({ id: 'ut-new', label: name, color, icon: icon || undefined, description: '', builtin: false }),

    GetWorkspaceState: async (): Promise<WorkspaceState> => ({ attached: false }),
    AttachWorkspace: async (): Promise<Workspace> => mockWorkspace(),
    DetachWorkspace: async () => {},
    RefreshWorkspaceIndex: async (): Promise<WorkspaceIndex> => ({ workspace_id: 'ws-1', generated_at: '', adapter: 'plain-folder', summary: '', tree: [] }),
    SetWorkspaceLaunchCommand: async (): Promise<Workspace> => mockWorkspace(),
    ReadWorkspaceFile: async (): Promise<string> => '',
    WriteWorkspaceFile: async () => {},
    OpenWorkspacePath: async () => {},
    RevealWorkspacePath: async () => {},
    RunWorkspaceLaunchCommand: async () => {},
    ListWorkspaceTemplates: async (): Promise<WorkspaceTemplateEntry[]> => [],
    GetWorkspaceTemplateParams: async () => [],
    PreviewWorkspaceTemplate: async (): Promise<WorkspaceTemplatePreview> => ({ entries: [] }),
    GenerateWorkspaceFromTemplate: async (): Promise<Workspace> => mockWorkspace(),
    InspectWorkspaceTemplateFolder: async (): Promise<WorkspaceTemplateInspection> => ({ is_template: false, name: '', description: '', default_target_path: '', parameters: [], size_bytes: 0, large_warning: false }),
    ImportWorkspaceTemplate: async (): Promise<WorkspaceTemplateEntry> => ({ id: 'templates/mock', name: 'Mock', description: '', scope: 'global', parameters: [], default_target_path: '' }),
    SaveWorkspaceTemplate: async () => {},
    ListProjectTemplates: async (): Promise<WorkspaceTemplateEntry[]> => [],
    GenerateCardFolder: async (): Promise<Card> => mockCard(),
    ClearCardFolder: async (): Promise<Card> => mockCard(),
    LinkCardFolder: async (): Promise<Card> => mockCard(),

    ListCardTemplates: async (): Promise<CardTemplate[]> => [],
    CreateCardTemplate: async (): Promise<CardTemplate> => ({ id: 'tpl-1', name: '', blocks: [] }),
    UpdateCardTemplate: async (): Promise<CardTemplate> => ({ id: 'tpl-1', name: '', blocks: [] }),
    DeleteCardTemplate: async () => {},

    SearchCards: async () => [],
    SearchOrphanedCards: async () => [],
    GetCardProjectContext: async () => '',
    RebuildIndex: async () => emptyIndexStats(),
    RefreshIndex: async () => emptyIndexStats(),
    ListCardIDsInCategory: async () => [],
    ListOrphanedCardIDs: async () => [],
    ListCardIDsByTag: async () => [],

    ListAgentCardStates: async () => ({}),
    GetNotifyConfig: async () => ({ system_enabled: false, smtp_host: '', smtp_port: 587, smtp_username: '', smtp_password: '', smtp_from_addr: '', smtp_to_addr: '', smtp_tls: true, webhook_url: '', webhook_auth_header: '' }),
    SetNotifyConfig: async () => {},
    GetNotifications: async () => [],
    MarkNotificationRead: async () => {},
    MarkAllNotificationsRead: async () => {},
    ClearAllNotifications: async () => {},
    GetCategoryAcceptedTypes: async () => null,
    ValidateSchedulePreview: async () => [],
    GetAgentConfig: async () => ({ card_id: '', config: { enabled: false, goal: '', schedule: '', allowed_tools: [], status: 'disabled' as const, notify_on: [], notify_channel: '', llm_account_id: '', llm_model: '', last_run_at: null, next_run_at: null, max_tokens_budget: 0, run_started_at: null, min_interval_minutes: 0, max_retries: 0, retry_count: 0, retry_backoff_minutes: 0, cost_budget_usd: 0, cost_spent_usd: 0, start_date: null, end_date: null, active_window_start: '', active_window_end: '', one_shot: false, timezone: '' }, runs: [] }),
    SaveAgentConfig: async () => {},
    GetAgentRuns: async () => [],
    TriggerAgent: async () => {},
    CancelAgent: async () => {},
    ClearAgentRuns: async () => {},
    PauseAllAgents: async () => {},
    ResumeAllAgents: async () => {},
    GetAgentSchedulerStatus: async () => ({ active: false, paused: false, runningCount: 0 }),
    GetAllAgents: async () => [],
    GetAllAgentRuns: async () => [],
    GetAgentAnalytics: async () => ({ total_agents: 0, enabled_agents: 0, total_runs: 0, success_runs: 0, failed_runs: 0, total_tokens: 0, total_cost: 0, cost_today: 0, cost_7d: 0, cost_by_model: {} }),
    ForceQuit: async () => {},

    LoadChatHistory: async () => emptyChat(),
    SendChatMessage: async () => emptyChat(),
    LoadProjectChatHistory: async () => emptyChat(),
    SendProjectChatMessage: async () => emptyChat(),
    ClearProjectChatHistory: async () => {},
    ClearCardChatHistory: async () => {},
    GetLLMAccounts: async () => [],
    SaveLLMAccounts: async () => {},
    TestLLMAccountConnection: async () => 'OK',
    TestSystemNotification: async () => {},
    IsLLMConfigured: async () => false,
    TestLLMConnection: async () => 'OK',

    AcceptPinSuggestion: async () => {},
    RejectPinSuggestion: async () => {},

    AcceptPendingEdit: async () => emptyChat(),
    RejectPendingEdit: async () => emptyChat(),
    AcceptAllPendingEdits: async () => emptyChat(),
    RejectAllPendingEdits: async () => emptyChat(),
    ApplyPendingEdits: async () => emptyChat(),
    ApplyProjectPendingEdits: async () => emptyChat(),

    AddCardAttachment: async () => mockCard(),
    RemoveCardAttachment: async () => mockCard(),

    GetTokenPricing: async () => ({}),
    SaveTokenPricing: async () => {},

    GetDueDateSettings: async () => ({ enabled: true, thresholds: ['24h', '1h', '0'], channels: 'in-app,system' }),
    SaveDueDateSettings: async () => {},

    GetVapidPublicKey: async () => '',
    RegisterPushSubscription: async () => {},
    UnregisterPushSubscription: async () => {},

    GetPreferences: async () => mockPreferences(),
    SetPreferences: async () => {},
    GetUIPreferences: async () => mockUIPreferences(),
    SetUIPreferences: async () => {},

    ListActivityLog: async () => [],
    ListRecentlyUpdatedCards: async () => [],

    PickFile: async () => '',
    PickSaveFile: async () => '',

    ListCardComments: async () => [],
    AddCardComment: async () => ({ id: 'mock', author: 'Test', created_at: '', updated_at: '', text: '' }),
    UpdateCardComment: async () => ({ id: 'mock', author: 'Test', created_at: '', updated_at: '', text: '' }),
    DeleteCardComment: async () => {},

    ImportTrelloBoard: async () => ({
      project_slug: 'mock', project_name: 'Mock', categories: 0, cards: 0,
      labels: 0, comments: 0, archived: 0, skipped_closed: 0,
    }),
    ImportTrelloBoardFromJSON: async () => ({
      project_slug: 'mock', project_name: 'Mock', categories: 0, cards: 0,
      labels: 0, comments: 0, archived: 0, skipped_closed: 0,
    }),
    ExportProjectToFile: async () => 0,
  }

  return { ...base, ...overrides }
}
