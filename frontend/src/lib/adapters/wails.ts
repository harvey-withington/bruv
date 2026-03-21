import type { BackendAdapter, EventCallback } from '../types'

import {
  Version, HasRepository, InitRepository, OpenRepository, CloseRepository,
  PickFolder, ListRecentRepos, RemoveRecentRepo,
  CreateBrand, GetBrand, ListBrands, RenameBrand, DeleteBrand,
  CreateStream, ListStreams, RenameStream, DeleteStream,
  CreateProject, ListProjects, RenameProject, DeleteProject,
  CreateCategory, ListCategories, RenameCategory, DeleteCategory, MoveCategoryCards, CopyCategory,
  CreateCard, GetCard, ListCards, DeleteCard, DuplicateCard,
  UpdateCardTitle, UpdateCardFields, UpdateCardBlocks, UpdateCardTags, UpdateCardDueDate,
  AddChecklistItem, ToggleChecklistItem, RemoveChecklistItem,
  PinCard, UnpinCard, GetCardPins, GetCardLocation, GetProjectLocation,
  MoveCardInCategory, MoveCardToCategory, ReorderBrands, ReorderStreams, ReorderProjects, ReorderCategories,
  MoveProject, MoveStream, CopyBrand, CopyStream, CopyProject,
  GetTagColors, SetTagColor, AssignTagColor,
  ListCardTypes, ValidateCardFields,
  SearchCards, GetCardProjectContext, RebuildIndex, RefreshIndex,
  ListCardIDsInCategory, ListOrphanedCardIDs, ListCardIDsByTag,
  GetPreferences, SetPreferences,
  GetProfile, SetProfile,
  GetAuthInfo, GetLLMConfig, SetLLMConfig,
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
  GetLLMConfig,
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

  CreateBrand,
  GetBrand,
  ListBrands,
  RenameBrand,
  DeleteBrand,

  CreateStream,
  ListStreams,
  RenameStream,
  DeleteStream,

  CreateProject,
  ListProjects,
  RenameProject,
  DeleteProject,

  CreateCategory,
  ListCategories,
  RenameCategory,
  DeleteCategory,
  MoveCategoryCards,
  CopyCategory,

  CreateCard,
  GetCard,
  ListCards,
  DeleteCard,
  DuplicateCard,

  UpdateCardTitle,
  UpdateCardFields,
  UpdateCardBlocks,
  UpdateCardTags,
  UpdateCardDueDate,

  AddChecklistItem,
  ToggleChecklistItem,
  RemoveChecklistItem,

  PinCard,
  UnpinCard,
  GetCardPins,
  GetCardLocation,
  GetProjectLocation,

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

  ListCardTypes,
  ValidateCardFields,

  SearchCards,
  GetCardProjectContext,
  RebuildIndex,
  RefreshIndex,
  ListCardIDsInCategory,
  ListOrphanedCardIDs,
  ListCardIDsByTag,

  GetPreferences,
  SetPreferences,
}
