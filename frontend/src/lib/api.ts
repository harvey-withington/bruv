// Backend-agnostic API — delegates to the active adapter (wails, cloud, etc.)

import { getBackend } from './adapters'

export type { UserProfile, AuthInfo, LLMConfig, BackendCapabilities, BackendEvent } from './types'

// Capabilities
export const getCapabilities = () => getBackend().getCapabilities()

// Auth / identity
export const GetAuthInfo = () => getBackend().GetAuthInfo()

// User profile
export const GetProfile = () => getBackend().GetProfile()
export const SetProfile = (...args: Parameters<ReturnType<typeof getBackend>['SetProfile']>) => getBackend().SetProfile(...args)

// LLM config
export const GetLLMConfig = () => getBackend().GetLLMConfig()
export const SetLLMConfig = (...args: Parameters<ReturnType<typeof getBackend>['SetLLMConfig']>) => getBackend().SetLLMConfig(...args)

// Real-time events
export const subscribe = (...args: Parameters<ReturnType<typeof getBackend>['subscribe']>) => getBackend().subscribe(...args)
export const unsubscribe = (...args: Parameters<ReturnType<typeof getBackend>['unsubscribe']>) => getBackend().unsubscribe(...args)

// Repository / workspace management
export const Version = () => getBackend().Version()
export const HasRepository = () => getBackend().HasRepository()
export const InitRepository = (...args: Parameters<ReturnType<typeof getBackend>['InitRepository']>) => getBackend().InitRepository(...args)
export const OpenRepository = (...args: Parameters<ReturnType<typeof getBackend>['OpenRepository']>) => getBackend().OpenRepository(...args)
export const CloseRepository = () => getBackend().CloseRepository()
export const PickFolder = (...args: Parameters<ReturnType<typeof getBackend>['PickFolder']>) => getBackend().PickFolder(...args)
export const ListRecentRepos = () => getBackend().ListRecentRepos()
export const RemoveRecentRepo = (...args: Parameters<ReturnType<typeof getBackend>['RemoveRecentRepo']>) => getBackend().RemoveRecentRepo(...args)

// Brand CRUD
export const CreateBrand = (...args: Parameters<ReturnType<typeof getBackend>['CreateBrand']>) => getBackend().CreateBrand(...args)
export const GetBrand = (...args: Parameters<ReturnType<typeof getBackend>['GetBrand']>) => getBackend().GetBrand(...args)
export const ListBrands = () => getBackend().ListBrands()
export const RenameBrand = (...args: Parameters<ReturnType<typeof getBackend>['RenameBrand']>) => getBackend().RenameBrand(...args)
export const DeleteBrand = (...args: Parameters<ReturnType<typeof getBackend>['DeleteBrand']>) => getBackend().DeleteBrand(...args)

// Stream CRUD
export const CreateStream = (...args: Parameters<ReturnType<typeof getBackend>['CreateStream']>) => getBackend().CreateStream(...args)
export const ListStreams = (...args: Parameters<ReturnType<typeof getBackend>['ListStreams']>) => getBackend().ListStreams(...args)
export const RenameStream = (...args: Parameters<ReturnType<typeof getBackend>['RenameStream']>) => getBackend().RenameStream(...args)
export const DeleteStream = (...args: Parameters<ReturnType<typeof getBackend>['DeleteStream']>) => getBackend().DeleteStream(...args)

// Project CRUD
export const CreateProject = (...args: Parameters<ReturnType<typeof getBackend>['CreateProject']>) => getBackend().CreateProject(...args)
export const ListProjects = (...args: Parameters<ReturnType<typeof getBackend>['ListProjects']>) => getBackend().ListProjects(...args)
export const RenameProject = (...args: Parameters<ReturnType<typeof getBackend>['RenameProject']>) => getBackend().RenameProject(...args)
export const DeleteProject = (...args: Parameters<ReturnType<typeof getBackend>['DeleteProject']>) => getBackend().DeleteProject(...args)

// Category CRUD
export const CreateCategory = (...args: Parameters<ReturnType<typeof getBackend>['CreateCategory']>) => getBackend().CreateCategory(...args)
export const ListCategories = (...args: Parameters<ReturnType<typeof getBackend>['ListCategories']>) => getBackend().ListCategories(...args)
export const RenameCategory = (...args: Parameters<ReturnType<typeof getBackend>['RenameCategory']>) => getBackend().RenameCategory(...args)
export const DeleteCategory = (...args: Parameters<ReturnType<typeof getBackend>['DeleteCategory']>) => getBackend().DeleteCategory(...args)
export const MoveCategoryCards = (...args: Parameters<ReturnType<typeof getBackend>['MoveCategoryCards']>) => getBackend().MoveCategoryCards(...args)
export const CopyCategory = (...args: Parameters<ReturnType<typeof getBackend>['CopyCategory']>) => getBackend().CopyCategory(...args)

// Card CRUD
export const CreateCard = (...args: Parameters<ReturnType<typeof getBackend>['CreateCard']>) => getBackend().CreateCard(...args)
export const GetCard = (...args: Parameters<ReturnType<typeof getBackend>['GetCard']>) => getBackend().GetCard(...args)
export const ListCards = () => getBackend().ListCards()
export const DeleteCard = (...args: Parameters<ReturnType<typeof getBackend>['DeleteCard']>) => getBackend().DeleteCard(...args)
export const DuplicateCard = (...args: Parameters<ReturnType<typeof getBackend>['DuplicateCard']>) => getBackend().DuplicateCard(...args)

// Card updates
export const UpdateCardTitle = (...args: Parameters<ReturnType<typeof getBackend>['UpdateCardTitle']>) => getBackend().UpdateCardTitle(...args)
export const UpdateCardFields = (...args: Parameters<ReturnType<typeof getBackend>['UpdateCardFields']>) => getBackend().UpdateCardFields(...args)
export const UpdateCardTags = (...args: Parameters<ReturnType<typeof getBackend>['UpdateCardTags']>) => getBackend().UpdateCardTags(...args)
export const UpdateCardDueDate = (...args: Parameters<ReturnType<typeof getBackend>['UpdateCardDueDate']>) => getBackend().UpdateCardDueDate(...args)

// Checklist
export const AddChecklistItem = (...args: Parameters<ReturnType<typeof getBackend>['AddChecklistItem']>) => getBackend().AddChecklistItem(...args)
export const ToggleChecklistItem = (...args: Parameters<ReturnType<typeof getBackend>['ToggleChecklistItem']>) => getBackend().ToggleChecklistItem(...args)
export const RemoveChecklistItem = (...args: Parameters<ReturnType<typeof getBackend>['RemoveChecklistItem']>) => getBackend().RemoveChecklistItem(...args)

// Pins
export const PinCard = (...args: Parameters<ReturnType<typeof getBackend>['PinCard']>) => getBackend().PinCard(...args)
export const UnpinCard = (...args: Parameters<ReturnType<typeof getBackend>['UnpinCard']>) => getBackend().UnpinCard(...args)
export const GetCardPins = (...args: Parameters<ReturnType<typeof getBackend>['GetCardPins']>) => getBackend().GetCardPins(...args)
export const GetCardLocation = (...args: Parameters<ReturnType<typeof getBackend>['GetCardLocation']>) => getBackend().GetCardLocation(...args)

// Move & reorder
export const MoveCardInCategory = (...args: Parameters<ReturnType<typeof getBackend>['MoveCardInCategory']>) => getBackend().MoveCardInCategory(...args)
export const MoveCardToCategory = (...args: Parameters<ReturnType<typeof getBackend>['MoveCardToCategory']>) => getBackend().MoveCardToCategory(...args)
export const ReorderBrands = (...args: Parameters<ReturnType<typeof getBackend>['ReorderBrands']>) => getBackend().ReorderBrands(...args)
export const ReorderStreams = (...args: Parameters<ReturnType<typeof getBackend>['ReorderStreams']>) => getBackend().ReorderStreams(...args)
export const ReorderProjects = (...args: Parameters<ReturnType<typeof getBackend>['ReorderProjects']>) => getBackend().ReorderProjects(...args)
export const ReorderCategories = (...args: Parameters<ReturnType<typeof getBackend>['ReorderCategories']>) => getBackend().ReorderCategories(...args)

// Move & copy (cross-hierarchy)
export const MoveProject = (...args: Parameters<ReturnType<typeof getBackend>['MoveProject']>) => getBackend().MoveProject(...args)
export const MoveStream = (...args: Parameters<ReturnType<typeof getBackend>['MoveStream']>) => getBackend().MoveStream(...args)
export const CopyBrand = (...args: Parameters<ReturnType<typeof getBackend>['CopyBrand']>) => getBackend().CopyBrand(...args)
export const CopyStream = (...args: Parameters<ReturnType<typeof getBackend>['CopyStream']>) => getBackend().CopyStream(...args)
export const CopyProject = (...args: Parameters<ReturnType<typeof getBackend>['CopyProject']>) => getBackend().CopyProject(...args)

// Tag colors
export const GetTagColors = () => getBackend().GetTagColors()
export const SetTagColor = (...args: Parameters<ReturnType<typeof getBackend>['SetTagColor']>) => getBackend().SetTagColor(...args)
export const AssignTagColor = (...args: Parameters<ReturnType<typeof getBackend>['AssignTagColor']>) => getBackend().AssignTagColor(...args)

// Schema
export const ListCardTypes = () => getBackend().ListCardTypes()
export const ValidateCardFields = (...args: Parameters<ReturnType<typeof getBackend>['ValidateCardFields']>) => getBackend().ValidateCardFields(...args)

// Index / search
export const SearchCards = (...args: Parameters<ReturnType<typeof getBackend>['SearchCards']>) => getBackend().SearchCards(...args)
export const RebuildIndex = () => getBackend().RebuildIndex()
export const RefreshIndex = () => getBackend().RefreshIndex()
export const ListCardIDsInCategory = (...args: Parameters<ReturnType<typeof getBackend>['ListCardIDsInCategory']>) => getBackend().ListCardIDsInCategory(...args)
export const ListOrphanedCardIDs = () => getBackend().ListOrphanedCardIDs()
export const ListCardIDsByTag = (...args: Parameters<ReturnType<typeof getBackend>['ListCardIDsByTag']>) => getBackend().ListCardIDsByTag(...args)

// User preferences
export const GetPreferences = () => getBackend().GetPreferences()
export const SetPreferences = (...args: Parameters<ReturnType<typeof getBackend>['SetPreferences']>) => getBackend().SetPreferences(...args)
