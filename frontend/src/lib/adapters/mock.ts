import type { BackendAdapter, EventCallback, CardTypeInfo, CardTemplate, UserCardType } from '../types'

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
      auto_pin: 'off',
      ai_mode: 'edit',
    }),
    SetLLMConfig: async () => {},

    subscribe: (_cb: EventCallback) => {},
    unsubscribe: (_cb: EventCallback) => {},

    Version: async () => '0.1.0-test',
    HasRepository: async () => true,
    InitRepository: async () => '/tmp/test-repo',
    OpenRepository: async () => {},
    CloseRepository: async () => {},
    PickFolder: async () => '/tmp/picked',
    ListRecentRepos: async () => [],
    RemoveRecentRepo: async () => {},

    CreateBrand: async () => ({ slug: 'test-brand', name: 'Test Brand' }),
    GetBrand: async () => ({ slug: 'test-brand', name: 'Test Brand' }),
    ListBrands: async () => [],
    RenameBrand: async () => ({ slug: 'test-brand', name: 'Test Brand' }),
    DeleteBrand: async () => {},

    CreateStream: async () => ({ slug: 'test-stream', name: 'Test Stream' }),
    ListStreams: async () => [],
    RenameStream: async () => ({ slug: 'test-stream', name: 'Test Stream' }),
    DeleteStream: async () => {},

    CreateProject: async () => ({ slug: 'test-project', name: 'Test Project' }),
    ListProjects: async () => [],
    RenameProject: async () => ({ slug: 'test-project', name: 'Test Project' }),
    DeleteProject: async () => {},

    CreateCategory: async () => ({ slug: 'test-category', name: 'Test Category' }),
    ListCategories: async () => [],
    RenameCategory: async () => ({ slug: 'test-category', name: 'Test Category' }),
    DeleteCategory: async () => {},
    MoveCategoryCards: async () => {},
    CopyCategory: async () => ({ slug: 'copy-category', name: 'Copy Category' }),
    UpdateCategoryAcceptedTypes: async () => ({ slug: 'test-category', name: 'Test Category' }),

    CreateCard: async () => ({ id: 'card-1', title: '', type: '', tags: [], due_date: null, created_at: '', fields: {}, blocks: [] }),
    GetCard: async () => ({ id: 'card-1', title: '', type: '', tags: [], due_date: null, created_at: '', fields: {}, blocks: [] }),
    ListCards: async () => [],
    DeleteCard: async () => {},
    DuplicateCard: async () => ({ id: 'card-2', title: '', type: '', tags: [], due_date: null, created_at: '', fields: {}, blocks: [] }),

    UpdateCardTitle: async () => ({}),
    UpdateCardType: async () => ({}),
    UpdateCardFields: async () => ({}),
    UpdateCardBlocks: async () => ({}),
    UpdateCardTags: async () => ({}),
    UpdateCardLabels: async () => ({}),
    UpdateCardDueDate: async () => ({}),

    AddChecklistItem: async () => ({}),
    ToggleChecklistItem: async () => ({}),
    RemoveChecklistItem: async () => ({}),

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

    ListCardTypes: async (): Promise<CardTypeInfo[]> => [],
    ValidateCardFields: async () => [],

    CreateUserCardType: async (): Promise<UserCardType> => ({ id: 'ut-1', label: '', color: '', description: '' }),
    UpdateUserCardType: async (): Promise<UserCardType> => ({ id: 'ut-1', label: '', color: '', description: '' }),
    DeleteUserCardType: async () => {},
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

    LoadChatHistory: async () => ({ card_id: '', messages: [] }),
    SendChatMessage: async () => ({ card_id: '', messages: [] }),
    IsLLMConfigured: async () => false,
    TestLLMConnection: async () => 'OK',

    AcceptPinSuggestion: async () => {},
    RejectPinSuggestion: async () => {},

    AcceptPendingEdit: async () => ({}),
    RejectPendingEdit: async () => ({}),
    AcceptAllPendingEdits: async () => ({}),
    RejectAllPendingEdits: async () => ({}),
    ApplyPendingEdits: async () => ({}),

    GetPreferences: async () => ({}),
    SetPreferences: async () => {},
  }

  return { ...base, ...overrides }
}
