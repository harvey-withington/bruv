import type { BackendAdapter, EventCallback, CardTypeInfo, CardTemplate, UserCardType } from '@shared/types'

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
    MarkLLMNudgeShown: async () => {},
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

    CreateBrand: async () => ({ slug: 'test-brand', name: 'Test Brand' }),
    GetBrand: async () => ({ slug: 'test-brand', name: 'Test Brand' }),
    ListBrands: async () => [],
    RenameBrand: async () => ({ slug: 'test-brand', name: 'Test Brand' }),
    UpdateBrandDescription: async () => ({ slug: 'test-brand', name: 'Test Brand' }),
    UpdateBrandIcon: async () => ({ slug: 'test-brand', name: 'Test Brand' }),
    DeleteBrand: async () => {},

    CreateStream: async () => ({ slug: 'test-stream', name: 'Test Stream' }),
    ListStreams: async () => [],
    RenameStream: async () => ({ slug: 'test-stream', name: 'Test Stream' }),
    UpdateStreamDescription: async () => ({ slug: 'test-stream', name: 'Test Stream' }),
    UpdateStreamIcon: async () => ({ slug: 'test-stream', name: 'Test Stream' }),
    DeleteStream: async () => {},

    CreateProject: async () => ({ slug: 'test-project', name: 'Test Project' }),
    ListProjects: async () => [],
    RenameProject: async () => ({ slug: 'test-project', name: 'Test Project' }),
    UpdateProjectDescription: async () => ({ slug: 'test-project', name: 'Test Project' }),
    UpdateProjectIcon: async () => ({ slug: 'test-project', name: 'Test Project' }),
    DeleteProject: async () => {},

    CreateCategory: async () => ({ slug: 'test-category', name: 'Test Category' }),
    ListCategories: async () => [],
    RenameCategory: async () => ({ slug: 'test-category', name: 'Test Category' }),
    DeleteCategory: async () => {},
    MoveCategoryCards: async () => {},
    CopyCategory: async () => ({ slug: 'copy-category', name: 'Copy Category' }),
    UpdateCategoryAcceptedTypes: async () => ({ slug: 'test-category', name: 'Test Category' }),
    UpdateCategoryDescription: async () => ({ slug: 'test-category', name: 'Test Category' }),
    UpdateCategoryIcon: async () => ({ slug: 'test-category', name: 'Test Category' }),

    CreateCard: async () => ({ id: 'card-1', title: '', type: '', tags: [], due_date: null, created_at: '', fields: {}, blocks: [] }),
    GetCard: async () => ({ id: 'card-1', title: '', type: '', tags: [], due_date: null, created_at: '', fields: {}, blocks: [] }),
    ListCards: async () => [],
    DeleteCard: async () => {},
    DuplicateCard: async () => ({ id: 'card-2', title: '', type: '', tags: [], due_date: null, created_at: '', fields: {}, blocks: [] }),

    UpdateCardTitle: async () => ({}),
    UpdateCardType: async () => ({}),
    RefreshTypeBlocks: async () => ({}),
    UpdateCardDescription: async () => ({}),
    UpdateCardBlocks: async () => ({}),
    UpdateCardTags: async () => ({}),
    UpdateCardLabels: async () => ({}),
    UpdateCardDueDate: async () => ({}),

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
    CopyBrand: async () => ({}),
    CopyStream: async () => ({}),
    CopyProject: async () => ({}),

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

    ListCardTemplates: async (): Promise<CardTemplate[]> => [],
    CreateCardTemplate: async (): Promise<CardTemplate> => ({ id: 'tpl-1', name: '', blocks: [] }),
    UpdateCardTemplate: async (): Promise<CardTemplate> => ({ id: 'tpl-1', name: '', blocks: [] }),
    DeleteCardTemplate: async () => {},

    SearchCards: async () => [],
    SearchOrphanedCards: async () => [],
    GetCardProjectContext: async () => '',
    RebuildIndex: async () => ({}),
    RefreshIndex: async () => ({}),
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

    LoadChatHistory: async () => ({ card_id: '', messages: [] }),
    SendChatMessage: async () => ({ card_id: '', messages: [] }),
    LoadProjectChatHistory: async () => ({ card_id: '', messages: [] }),
    SendProjectChatMessage: async () => ({ card_id: '', messages: [] }),
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

    AcceptPendingEdit: async () => ({}),
    RejectPendingEdit: async () => ({}),
    AcceptAllPendingEdits: async () => ({}),
    RejectAllPendingEdits: async () => ({}),
    ApplyPendingEdits: async () => ({}),
    ApplyProjectPendingEdits: async () => ({}),

    AddCardAttachment: async () => ({} as any),
    RemoveCardAttachment: async () => ({} as any),

    GetTokenPricing: async () => ({}),
    SaveTokenPricing: async () => {},

    GetDueDateSettings: async () => ({ enabled: true, thresholds: ['24h', '1h', '0'], channels: 'in-app,system' }),
    SaveDueDateSettings: async () => {},

    GetPreferences: async () => ({}),
    SetPreferences: async () => {},

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
