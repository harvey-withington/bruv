import type { BackendAdapter, EventCallback } from '../types'

import {
  Version, HasRepository, InitRepository, OpenRepository, CloseRepository,
  PickFolder, ListRecentRepos, RemoveRecentRepo,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetRepoDescription,
  // @ts-ignore — generated after `wails generate` with updated Go code
  UpdateRepoDescription,
  CreateBrand, GetBrand, ListBrands, RenameBrand, UpdateBrandDescription, DeleteBrand,
  CreateStream, ListStreams, RenameStream, UpdateStreamDescription, DeleteStream,
  CreateProject, ListProjects, RenameProject, UpdateProjectDescription, DeleteProject,
  CreateCategory, ListCategories, RenameCategory, DeleteCategory, MoveCategoryCards, CopyCategory, UpdateCategoryAcceptedTypes,
  // @ts-ignore — generated after `wails generate` with updated Go code
  UpdateCategoryDescription,
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
  GetProjectLabels, AddProjectLabel, RemoveProjectLabel, UpdateProjectLabel,
  ListCardTypes, ValidateCardFields,
  CreateUserCardType, UpdateUserCardType, DeleteUserCardType, UpdateBuiltinCardType,
  ListCardTemplates, CreateCardTemplate, UpdateCardTemplate, DeleteCardTemplate,
  SearchCards, SearchOrphanedCards, GetCardProjectContext, RebuildIndex, RefreshIndex,
  ListCardIDsInCategory, ListOrphanedCardIDs, ListCardIDsByTag,
  GetPreferences, SetPreferences,
  GetProfile, SetProfile,
  GetAuthInfo, GetLLMConfig, SetLLMConfig,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetCategoryAcceptedTypes,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetAgentConfig,
  // @ts-ignore — generated after `wails generate` with updated Go code
  SaveAgentConfig,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetAgentRuns,
  // @ts-ignore — generated after `wails generate` with updated Go code
  TriggerAgent,
  // @ts-ignore — generated after `wails generate` with updated Go code
  PauseAllAgents,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ResumeAllAgents,
  // @ts-ignore — generated after `wails generate` with updated Go code
  GetAgentSchedulerStatus,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ForceQuit,
  LoadChatHistory, SendChatMessage,
  // @ts-ignore — generated after `wails generate` with updated Go code
  LoadProjectChatHistory,
  // @ts-ignore — generated after `wails generate` with updated Go code
  SendProjectChatMessage,
  IsLLMConfigured, TestLLMConnection,
  AcceptPinSuggestion, RejectPinSuggestion,
  AcceptPendingEdit, RejectPendingEdit, AcceptAllPendingEdits, RejectAllPendingEdits, ApplyPendingEdits,
  // @ts-ignore — generated after `wails generate` with updated Go code
  AddCardAttachment,
  // @ts-ignore — generated after `wails generate` with updated Go code
  RemoveCardAttachment,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ListActivityLog,
  // @ts-ignore — generated after `wails generate` with updated Go code
  ListRecentlyUpdatedCards,
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
  HasRepository,
  InitRepository,
  OpenRepository,
  CloseRepository,
  PickFolder,
  ListRecentRepos,
  RemoveRecentRepo,
  GetRepoDescription,
  UpdateRepoDescription,

  CreateBrand,
  GetBrand,
  ListBrands,
  RenameBrand,
  UpdateBrandDescription,
  DeleteBrand,

  CreateStream,
  ListStreams,
  RenameStream,
  UpdateStreamDescription,
  DeleteStream,

  CreateProject,
  ListProjects,
  RenameProject,
  UpdateProjectDescription,
  DeleteProject,

  CreateCategory,
  ListCategories,
  RenameCategory,
  DeleteCategory,
  MoveCategoryCards,
  CopyCategory,
  UpdateCategoryAcceptedTypes,
  UpdateCategoryDescription,

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

  ListCardTypes: ListCardTypes as BackendAdapter['ListCardTypes'],
  ValidateCardFields,
  CreateUserCardType,
  UpdateUserCardType,
  DeleteUserCardType,
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

  GetCategoryAcceptedTypes,
  GetAgentConfig: GetAgentConfig as unknown as BackendAdapter['GetAgentConfig'],
  SaveAgentConfig,
  GetAgentRuns: GetAgentRuns as unknown as BackendAdapter['GetAgentRuns'],
  TriggerAgent,
  PauseAllAgents,
  ResumeAllAgents,
  GetAgentSchedulerStatus,
  ForceQuit,

  LoadChatHistory,
  SendChatMessage,
  LoadProjectChatHistory: LoadProjectChatHistory as unknown as BackendAdapter['LoadProjectChatHistory'],
  SendProjectChatMessage: SendProjectChatMessage as unknown as BackendAdapter['SendProjectChatMessage'],
  IsLLMConfigured,
  TestLLMConnection,
  AcceptPinSuggestion,
  RejectPinSuggestion,
  AcceptPendingEdit,
  RejectPendingEdit,
  AcceptAllPendingEdits,
  RejectAllPendingEdits,
  ApplyPendingEdits,

  AddCardAttachment: AddCardAttachment as unknown as BackendAdapter['AddCardAttachment'],
  RemoveCardAttachment: RemoveCardAttachment as unknown as BackendAdapter['RemoveCardAttachment'],

  GetPreferences,
  SetPreferences,

  ListActivityLog: ListActivityLog as unknown as BackendAdapter['ListActivityLog'],
  ListRecentlyUpdatedCards: ListRecentlyUpdatedCards as unknown as BackendAdapter['ListRecentlyUpdatedCards'],
}
