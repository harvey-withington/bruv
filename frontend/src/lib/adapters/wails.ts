import type { BackendAdapter, EventCallback } from '../types'

import {
  Version, HasRepository, InitRepository, OpenRepository, CloseRepository,
  PickFolder, ListRecentRepos, RemoveRecentRepo,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetBuildInfo,
  // @ts-ignore — generated after `wails generate` with updated Go code
  OpenConfigFolder,
  // @ts-ignore — generated after `wails generate` with updated Go code
  OpenBugReportURL,
  // @ts-ignore — generated after `wails generate` with updated Go code
  MarkLLMNudgeShown,
  // @ts-ignore — generated after `wails generate` with updated Go code
  CheckForUpdates,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ExportCardTypesToFile,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ImportCardTypesFromFile,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ImportCardTypesFromRepo,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ListMCPServers,
  // @ts-ignore — generated after `wails generate` with updated Go code
  AddMCPServer,
  // @ts-ignore — generated after `wails generate` with updated Go code
  UpdateMCPServer,
  // @ts-ignore — generated after `wails generate` with updated Go code
  DeleteMCPServer,
  // @ts-ignore — generated after `wails generate` with updated Go code
  SetMCPServerSecret,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetMCPServerSecretStatus,
  // @ts-ignore — generated after `wails generate` with updated Go code
  RestartMCPServer,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetRepoDescription,
  // @ts-ignore — generated after `wails generate` with updated Go code
  UpdateRepoDescription,
  CreateBrand, GetBrand, ListBrands, RenameBrand, UpdateBrandDescription, DeleteBrand,
  // @ts-ignore — generated after `wails generate` with updated Go code
  UpdateBrandIcon,
  CreateStream, ListStreams, RenameStream, UpdateStreamDescription, DeleteStream,
  // @ts-ignore — generated after `wails generate` with updated Go code
  UpdateStreamIcon,
  CreateProject, ListProjects, RenameProject, UpdateProjectDescription, DeleteProject,
  // @ts-ignore — generated after `wails generate` with updated Go code
  UpdateProjectIcon,
  CreateCategory, ListCategories, RenameCategory, DeleteCategory, MoveCategoryCards, CopyCategory, UpdateCategoryAcceptedTypes,
  // @ts-ignore — generated after `wails generate` with updated Go code
  UpdateCategoryDescription,
  // @ts-ignore — generated after `wails generate` with updated Go code
  UpdateCategoryIcon,
  CreateCard, GetCard, ListCards, DeleteCard, DuplicateCard,
  UpdateCardTitle, UpdateCardType, UpdateCardFields, UpdateCardBlocks, UpdateCardTags, UpdateCardLabels, UpdateCardDueDate,
  // @ts-ignore — generated after `wails generate` with updated Go code
  RefreshTypeBlocks,
  AddChecklistItem, ToggleChecklistItem, RemoveChecklistItem,
  PinCard, UnpinCard, GetCardPins, GetCardLocation, GetProjectLocation,
  ListAllCategories, GetCardPinBreadcrumbs,
  MoveCardInCategory, MoveCardToCategory, ReorderBrands, ReorderStreams, ReorderProjects, ReorderCategories,
  MoveProject, MoveStream, CopyBrand, CopyStream, CopyProject,
  GetTagColors, SetTagColor, AssignTagColor,
  GetProjectLabels, AddProjectLabel, RemoveProjectLabel, UpdateProjectLabel, SetProjectLabelIcon,
  ListCardTypes, ValidateCardFields,
  CreateUserCardType, UpdateUserCardType, DeleteUserCardType, UpdateBuiltinCardType,
  // @ts-ignore — generated after `wails generate` with updated Go code
  UpdateUserCardTypeIcon,
  ListCardTemplates, CreateCardTemplate, UpdateCardTemplate, DeleteCardTemplate,
  SearchCards, SearchOrphanedCards, GetCardProjectContext, RebuildIndex, RefreshIndex,
  ListCardIDsInCategory, ListOrphanedCardIDs, ListCardIDsByTag,
  GetPreferences, SetPreferences,
  GetProfile, SetProfile,
  GetAuthInfo, GetLLMConfig, SetLLMConfig,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetNotifyConfig,
  // @ts-ignore — generated after `wails generate` with updated Go code
  SetNotifyConfig,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetNotifications,
  // @ts-ignore — generated after `wails generate` with updated Go code
  MarkNotificationRead,
  // @ts-ignore — generated after `wails generate` with updated Go code
  MarkAllNotificationsRead,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ClearAllNotifications,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ListAgentCardIDs,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetCategoryAcceptedTypes,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ValidateSchedulePreview,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetAgentConfig,
  // @ts-ignore — generated after `wails generate` with updated Go code
  SaveAgentConfig,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetAgentRuns,
  // @ts-ignore — generated after `wails generate` with updated Go code
  TriggerAgent,
  // @ts-ignore — generated after `wails generate` with updated Go code
  CancelAgent,
  // @ts-ignore — generated after `wails generate`
  ClearAgentRuns,
  // @ts-ignore — generated after `wails generate` with updated Go code
  PauseAllAgents,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ResumeAllAgents,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetAgentSchedulerStatus,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetAllAgents,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetAllAgentRuns,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetAgentAnalytics,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ForceQuit,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetTokenPricing,
  // @ts-ignore — generated after `wails generate` with updated Go code
  SaveTokenPricing,
  LoadChatHistory, SendChatMessage,
  // @ts-ignore — generated after `wails generate` with updated Go code
  LoadProjectChatHistory,
  // @ts-ignore — generated after `wails generate` with updated Go code
  SendProjectChatMessage,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ClearProjectChatHistory,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ClearCardChatHistory,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetLLMAccounts,
  // @ts-ignore — generated after `wails generate` with updated Go code
  SaveLLMAccounts,
  // @ts-ignore — generated after `wails generate` with updated Go code
  TestLLMAccountConnection,
  // @ts-ignore — generated after `wails generate` with updated Go code
  TestSystemNotification,
  IsLLMConfigured, TestLLMConnection,
  AcceptPinSuggestion, RejectPinSuggestion,
  AcceptPendingEdit, RejectPendingEdit, AcceptAllPendingEdits, RejectAllPendingEdits, ApplyPendingEdits,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ApplyProjectPendingEdits,
  // @ts-ignore — generated after `wails generate` with updated Go code
  AddCardAttachment,
  // @ts-ignore — generated after `wails generate` with updated Go code
  RemoveCardAttachment,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetDueDateSettings,
  // @ts-ignore — generated after `wails generate` with updated Go code
  SaveDueDateSettings,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ListActivityLog,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ListRecentlyUpdatedCards,
  // @ts-ignore — generated after `wails generate` with updated Go code
  PickFile,
  // @ts-ignore — generated after `wails generate` with updated Go code
  PickSaveFile,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ImportTrelloBoard,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ImportTrelloBoardFromJSON,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ExportProjectToFile,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ListCardComments,
  // @ts-ignore — generated after `wails generate` with updated Go code
  AddCardComment,
  // @ts-ignore — generated after `wails generate` with updated Go code
  UpdateCardComment,
  // @ts-ignore — generated after `wails generate` with updated Go code
  DeleteCardComment,
} from '../../../wailsjs/go/main/App.js'

export const wailsAdapter: BackendAdapter = {
  getCapabilities: () => ({
    hasLocalFilesystem: true,
    hasAuth: false,
    hasRealtime: false,
  }),

  GetAuthInfo,
  GetProfile,
  SetProfile,
  GetLLMConfig: GetLLMConfig as unknown as BackendAdapter['GetLLMConfig'],
  SetLLMConfig,

  subscribe(_cb: EventCallback) {},
  unsubscribe(_cb: EventCallback) {},

  Version,
  GetBuildInfo: GetBuildInfo as unknown as BackendAdapter['GetBuildInfo'],
  OpenConfigFolder: OpenConfigFolder as unknown as BackendAdapter['OpenConfigFolder'],
  OpenBugReportURL: OpenBugReportURL as unknown as BackendAdapter['OpenBugReportURL'],
  MarkLLMNudgeShown: MarkLLMNudgeShown as unknown as BackendAdapter['MarkLLMNudgeShown'],
  CheckForUpdates: CheckForUpdates as unknown as BackendAdapter['CheckForUpdates'],
  ExportCardTypesToFile: ExportCardTypesToFile as unknown as BackendAdapter['ExportCardTypesToFile'],
  ImportCardTypesFromFile: ImportCardTypesFromFile as unknown as BackendAdapter['ImportCardTypesFromFile'],
  ImportCardTypesFromRepo: ImportCardTypesFromRepo as unknown as BackendAdapter['ImportCardTypesFromRepo'],
  ListMCPServers: ListMCPServers as unknown as BackendAdapter['ListMCPServers'],
  AddMCPServer: AddMCPServer as unknown as BackendAdapter['AddMCPServer'],
  UpdateMCPServer: UpdateMCPServer as unknown as BackendAdapter['UpdateMCPServer'],
  DeleteMCPServer: DeleteMCPServer as unknown as BackendAdapter['DeleteMCPServer'],
  SetMCPServerSecret: SetMCPServerSecret as unknown as BackendAdapter['SetMCPServerSecret'],
  GetMCPServerSecretStatus: GetMCPServerSecretStatus as unknown as BackendAdapter['GetMCPServerSecretStatus'],
  RestartMCPServer: RestartMCPServer as unknown as BackendAdapter['RestartMCPServer'],
  HasRepository,
  InitRepository,
  OpenRepository,
  CloseRepository,
  PickFolder,
  PickFile: PickFile as unknown as BackendAdapter['PickFile'],
  PickSaveFile: PickSaveFile as unknown as BackendAdapter['PickSaveFile'],
  ListRecentRepos,
  RemoveRecentRepo,
  GetRepoDescription,
  UpdateRepoDescription,

  CreateBrand,
  GetBrand,
  ListBrands,
  RenameBrand,
  UpdateBrandDescription,
  UpdateBrandIcon,
  DeleteBrand,

  CreateStream,
  ListStreams,
  RenameStream,
  UpdateStreamDescription,
  UpdateStreamIcon,
  DeleteStream,

  CreateProject,
  ListProjects,
  RenameProject,
  UpdateProjectDescription,
  UpdateProjectIcon,
  DeleteProject,

  CreateCategory,
  ListCategories,
  RenameCategory,
  DeleteCategory,
  MoveCategoryCards,
  CopyCategory,
  UpdateCategoryAcceptedTypes,
  UpdateCategoryDescription,
  UpdateCategoryIcon,

  CreateCard,
  GetCard,
  ListCards,
  DeleteCard,
  DuplicateCard,

  UpdateCardTitle,
  UpdateCardType,
  RefreshTypeBlocks,
  UpdateCardFields,
  UpdateCardBlocks,
  UpdateCardTags,
  UpdateCardLabels,
  UpdateCardDueDate,

  AddChecklistItem,
  ToggleChecklistItem,
  RemoveChecklistItem,

  PinCard,
  UnpinCard,
  GetCardPins,
  GetCardLocation,
  GetProjectLocation,
  ListAllCategories,
  GetCardPinBreadcrumbs,

  MoveCardInCategory,
  MoveCardToCategory,
  ReorderBrands,
  ReorderStreams,
  ReorderProjects,
  ReorderCategories,

  MoveProject,
  MoveStream,
  CopyBrand,
  CopyStream,
  CopyProject,

  GetTagColors,
  SetTagColor,
  AssignTagColor,

  GetProjectLabels,
  AddProjectLabel,
  RemoveProjectLabel,
  UpdateProjectLabel,
  SetProjectLabelIcon,

  ListCardTypes: ListCardTypes as BackendAdapter['ListCardTypes'],
  ValidateCardFields,
  CreateUserCardType,
  UpdateUserCardType,
  DeleteUserCardType,
  UpdateUserCardTypeIcon,
  UpdateBuiltinCardType,
  ListCardTemplates: ListCardTemplates as unknown as BackendAdapter['ListCardTemplates'],
  CreateCardTemplate: CreateCardTemplate as unknown as BackendAdapter['CreateCardTemplate'],
  UpdateCardTemplate: UpdateCardTemplate as unknown as BackendAdapter['UpdateCardTemplate'],
  DeleteCardTemplate,

  SearchCards,
  SearchOrphanedCards,
  GetCardProjectContext,
  RebuildIndex,
  RefreshIndex,
  ListCardIDsInCategory,
  ListOrphanedCardIDs,
  ListCardIDsByTag,

  ListAgentCardIDs: ListAgentCardIDs as unknown as BackendAdapter['ListAgentCardIDs'],
  GetNotifyConfig: GetNotifyConfig as unknown as BackendAdapter['GetNotifyConfig'],
  SetNotifyConfig,
  GetNotifications: GetNotifications as unknown as BackendAdapter['GetNotifications'],
  MarkNotificationRead,
  MarkAllNotificationsRead,
  ClearAllNotifications,
  GetCategoryAcceptedTypes,
  ValidateSchedulePreview: ValidateSchedulePreview as unknown as BackendAdapter['ValidateSchedulePreview'],
  GetAgentConfig: GetAgentConfig as unknown as BackendAdapter['GetAgentConfig'],
  SaveAgentConfig: SaveAgentConfig as unknown as BackendAdapter['SaveAgentConfig'],
  GetAgentRuns: GetAgentRuns as unknown as BackendAdapter['GetAgentRuns'],
  TriggerAgent,
  CancelAgent,
  ClearAgentRuns,
  PauseAllAgents,
  ResumeAllAgents,
  GetAgentSchedulerStatus: GetAgentSchedulerStatus as unknown as BackendAdapter['GetAgentSchedulerStatus'],
  GetAllAgents: GetAllAgents as unknown as BackendAdapter['GetAllAgents'],
  GetAllAgentRuns: GetAllAgentRuns as unknown as BackendAdapter['GetAllAgentRuns'],
  GetAgentAnalytics: GetAgentAnalytics as unknown as BackendAdapter['GetAgentAnalytics'],
  ForceQuit,
  GetTokenPricing: GetTokenPricing as unknown as BackendAdapter['GetTokenPricing'],
  SaveTokenPricing: SaveTokenPricing as unknown as BackendAdapter['SaveTokenPricing'],

  LoadChatHistory,
  SendChatMessage,
  LoadProjectChatHistory: LoadProjectChatHistory as unknown as BackendAdapter['LoadProjectChatHistory'],
  SendProjectChatMessage: SendProjectChatMessage as unknown as BackendAdapter['SendProjectChatMessage'],
  ClearProjectChatHistory,
  ClearCardChatHistory,
  GetLLMAccounts: GetLLMAccounts as unknown as BackendAdapter['GetLLMAccounts'],
  SaveLLMAccounts,
  TestLLMAccountConnection,
  TestSystemNotification,
  IsLLMConfigured,
  TestLLMConnection,
  AcceptPinSuggestion,
  RejectPinSuggestion,
  AcceptPendingEdit,
  RejectPendingEdit,
  AcceptAllPendingEdits,
  RejectAllPendingEdits,
  ApplyPendingEdits,
  ApplyProjectPendingEdits,

  AddCardAttachment: AddCardAttachment as unknown as BackendAdapter['AddCardAttachment'],
  RemoveCardAttachment: RemoveCardAttachment as unknown as BackendAdapter['RemoveCardAttachment'],

  GetDueDateSettings: GetDueDateSettings as unknown as BackendAdapter['GetDueDateSettings'],
  SaveDueDateSettings,

  GetPreferences,
  SetPreferences,

  ListActivityLog: ListActivityLog as unknown as BackendAdapter['ListActivityLog'],
  ListRecentlyUpdatedCards: ListRecentlyUpdatedCards as unknown as BackendAdapter['ListRecentlyUpdatedCards'],

  ListCardComments: ListCardComments as unknown as BackendAdapter['ListCardComments'],
  AddCardComment: AddCardComment as unknown as BackendAdapter['AddCardComment'],
  UpdateCardComment: UpdateCardComment as unknown as BackendAdapter['UpdateCardComment'],
  DeleteCardComment,

  ImportTrelloBoard: ImportTrelloBoard as unknown as BackendAdapter['ImportTrelloBoard'],
  ImportTrelloBoardFromJSON: ImportTrelloBoardFromJSON as unknown as BackendAdapter['ImportTrelloBoardFromJSON'],
  ExportProjectToFile: ExportProjectToFile as unknown as BackendAdapter['ExportProjectToFile'],
}
