// Backend-agnostic API — delegates to the active adapter (wails, cloud, etc.)

import { getBackend } from './adapters'

export type { UserProfile, AuthInfo, LLMConfig, LLMAccount, BackendCapabilities, BackendEvent, CardTypeInfo, UserCardType, CardTemplate, Attachment, ActivityEntry, RecentCard, AgentConfig, AgentRun, AgentFile, AgentStatus, AgentSummary, AgentRunEntry, AgentAnalytics, AppNotification, NotifyConfig, ModelPricing, BuildInfo, UpdateCheckResult } from './types'

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
export const GetBuildInfo = () => getBackend().GetBuildInfo()
export const OpenConfigFolder = () => getBackend().OpenConfigFolder()
export const OpenBugReportURL = () => getBackend().OpenBugReportURL()
export const MarkLLMNudgeShown = () => getBackend().MarkLLMNudgeShown()
export const CheckForUpdates = () => getBackend().CheckForUpdates()
export const HasRepository = () => getBackend().HasRepository()
export const InitRepository = (...args: Parameters<ReturnType<typeof getBackend>['InitRepository']>) => getBackend().InitRepository(...args)
export const OpenRepository = (...args: Parameters<ReturnType<typeof getBackend>['OpenRepository']>) => getBackend().OpenRepository(...args)
export const CloseRepository = () => getBackend().CloseRepository()
export const PickFolder = (...args: Parameters<ReturnType<typeof getBackend>['PickFolder']>) => getBackend().PickFolder(...args)
export const ListRecentRepos = () => getBackend().ListRecentRepos()
export const RemoveRecentRepo = (...args: Parameters<ReturnType<typeof getBackend>['RemoveRecentRepo']>) => getBackend().RemoveRecentRepo(...args)
export const GetRepoDescription = () => getBackend().GetRepoDescription()
export const UpdateRepoDescription = (...args: Parameters<ReturnType<typeof getBackend>['UpdateRepoDescription']>) => getBackend().UpdateRepoDescription(...args)

// Brand CRUD
export const CreateBrand = (...args: Parameters<ReturnType<typeof getBackend>['CreateBrand']>) => getBackend().CreateBrand(...args)
export const GetBrand = (...args: Parameters<ReturnType<typeof getBackend>['GetBrand']>) => getBackend().GetBrand(...args)
export const ListBrands = () => getBackend().ListBrands()
export const RenameBrand = (...args: Parameters<ReturnType<typeof getBackend>['RenameBrand']>) => getBackend().RenameBrand(...args)
export const UpdateBrandDescription = (...args: Parameters<ReturnType<typeof getBackend>['UpdateBrandDescription']>) => getBackend().UpdateBrandDescription(...args)
export const UpdateBrandIcon = (...args: Parameters<ReturnType<typeof getBackend>['UpdateBrandIcon']>) => getBackend().UpdateBrandIcon(...args)
export const DeleteBrand = (...args: Parameters<ReturnType<typeof getBackend>['DeleteBrand']>) => getBackend().DeleteBrand(...args)

// Stream CRUD
export const CreateStream = (...args: Parameters<ReturnType<typeof getBackend>['CreateStream']>) => getBackend().CreateStream(...args)
export const ListStreams = (...args: Parameters<ReturnType<typeof getBackend>['ListStreams']>) => getBackend().ListStreams(...args)
export const RenameStream = (...args: Parameters<ReturnType<typeof getBackend>['RenameStream']>) => getBackend().RenameStream(...args)
export const UpdateStreamDescription = (...args: Parameters<ReturnType<typeof getBackend>['UpdateStreamDescription']>) => getBackend().UpdateStreamDescription(...args)
export const UpdateStreamIcon = (...args: Parameters<ReturnType<typeof getBackend>['UpdateStreamIcon']>) => getBackend().UpdateStreamIcon(...args)
export const DeleteStream = (...args: Parameters<ReturnType<typeof getBackend>['DeleteStream']>) => getBackend().DeleteStream(...args)

// Project CRUD
export const CreateProject = (...args: Parameters<ReturnType<typeof getBackend>['CreateProject']>) => getBackend().CreateProject(...args)
export const ListProjects = (...args: Parameters<ReturnType<typeof getBackend>['ListProjects']>) => getBackend().ListProjects(...args)
export const RenameProject = (...args: Parameters<ReturnType<typeof getBackend>['RenameProject']>) => getBackend().RenameProject(...args)
export const UpdateProjectDescription = (...args: Parameters<ReturnType<typeof getBackend>['UpdateProjectDescription']>) => getBackend().UpdateProjectDescription(...args)
export const UpdateProjectIcon = (...args: Parameters<ReturnType<typeof getBackend>['UpdateProjectIcon']>) => getBackend().UpdateProjectIcon(...args)
export const DeleteProject = (...args: Parameters<ReturnType<typeof getBackend>['DeleteProject']>) => getBackend().DeleteProject(...args)

// Category CRUD
export const CreateCategory = (...args: Parameters<ReturnType<typeof getBackend>['CreateCategory']>) => getBackend().CreateCategory(...args)
export const ListCategories = (...args: Parameters<ReturnType<typeof getBackend>['ListCategories']>) => getBackend().ListCategories(...args)
export const RenameCategory = (...args: Parameters<ReturnType<typeof getBackend>['RenameCategory']>) => getBackend().RenameCategory(...args)
export const DeleteCategory = (...args: Parameters<ReturnType<typeof getBackend>['DeleteCategory']>) => getBackend().DeleteCategory(...args)
export const MoveCategoryCards = (...args: Parameters<ReturnType<typeof getBackend>['MoveCategoryCards']>) => getBackend().MoveCategoryCards(...args)
export const CopyCategory = (...args: Parameters<ReturnType<typeof getBackend>['CopyCategory']>) => getBackend().CopyCategory(...args)
export const UpdateCategoryAcceptedTypes = (...args: Parameters<ReturnType<typeof getBackend>['UpdateCategoryAcceptedTypes']>) => getBackend().UpdateCategoryAcceptedTypes(...args)
export const UpdateCategoryDescription = (...args: Parameters<ReturnType<typeof getBackend>['UpdateCategoryDescription']>) => getBackend().UpdateCategoryDescription(...args)
export const UpdateCategoryIcon = (...args: Parameters<ReturnType<typeof getBackend>['UpdateCategoryIcon']>) => getBackend().UpdateCategoryIcon(...args)

// Card CRUD
export const CreateCard = (...args: Parameters<ReturnType<typeof getBackend>['CreateCard']>) => getBackend().CreateCard(...args)
export const GetCard = (...args: Parameters<ReturnType<typeof getBackend>['GetCard']>) => getBackend().GetCard(...args)
export const ListCards = () => getBackend().ListCards()
export const DeleteCard = (...args: Parameters<ReturnType<typeof getBackend>['DeleteCard']>) => getBackend().DeleteCard(...args)
export const DuplicateCard = (...args: Parameters<ReturnType<typeof getBackend>['DuplicateCard']>) => getBackend().DuplicateCard(...args)

// Card updates
export const UpdateCardTitle = (...args: Parameters<ReturnType<typeof getBackend>['UpdateCardTitle']>) => getBackend().UpdateCardTitle(...args)
export const UpdateCardType = (...args: Parameters<ReturnType<typeof getBackend>['UpdateCardType']>) => getBackend().UpdateCardType(...args)
export const RefreshTypeBlocks = (...args: Parameters<ReturnType<typeof getBackend>['RefreshTypeBlocks']>) => getBackend().RefreshTypeBlocks(...args)
export const UpdateCardFields = (...args: Parameters<ReturnType<typeof getBackend>['UpdateCardFields']>) => getBackend().UpdateCardFields(...args)
export const UpdateCardBlocks = (...args: Parameters<ReturnType<typeof getBackend>['UpdateCardBlocks']>) => getBackend().UpdateCardBlocks(...args)
export const UpdateCardTags = (...args: Parameters<ReturnType<typeof getBackend>['UpdateCardTags']>) => getBackend().UpdateCardTags(...args)
export const UpdateCardLabels = (...args: Parameters<ReturnType<typeof getBackend>['UpdateCardLabels']>) => getBackend().UpdateCardLabels(...args)
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
export const GetProjectLocation = (...args: Parameters<ReturnType<typeof getBackend>['GetProjectLocation']>) => getBackend().GetProjectLocation(...args)
export const ListAllCategories = () => getBackend().ListAllCategories()
export const GetCardPinBreadcrumbs = (...args: Parameters<ReturnType<typeof getBackend>['GetCardPinBreadcrumbs']>) => getBackend().GetCardPinBreadcrumbs(...args)

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

// Labels (per-project)
export const GetProjectLabels = (...args: Parameters<ReturnType<typeof getBackend>['GetProjectLabels']>) => getBackend().GetProjectLabels(...args)
export const AddProjectLabel = (...args: Parameters<ReturnType<typeof getBackend>['AddProjectLabel']>) => getBackend().AddProjectLabel(...args)
export const RemoveProjectLabel = (...args: Parameters<ReturnType<typeof getBackend>['RemoveProjectLabel']>) => getBackend().RemoveProjectLabel(...args)
export const UpdateProjectLabel = (...args: Parameters<ReturnType<typeof getBackend>['UpdateProjectLabel']>) => getBackend().UpdateProjectLabel(...args)
export const SetProjectLabelIcon = (...args: Parameters<ReturnType<typeof getBackend>['SetProjectLabelIcon']>) => getBackend().SetProjectLabelIcon(...args)

// Schema
export const ListCardTypes = () => getBackend().ListCardTypes()
export const ValidateCardFields = (...args: Parameters<ReturnType<typeof getBackend>['ValidateCardFields']>) => getBackend().ValidateCardFields(...args)

// User card types
export const CreateUserCardType = (...args: Parameters<ReturnType<typeof getBackend>['CreateUserCardType']>) => getBackend().CreateUserCardType(...args)
export const UpdateUserCardType = (...args: Parameters<ReturnType<typeof getBackend>['UpdateUserCardType']>) => getBackend().UpdateUserCardType(...args)
export const DeleteUserCardType = (...args: Parameters<ReturnType<typeof getBackend>['DeleteUserCardType']>) => getBackend().DeleteUserCardType(...args)
export const UpdateUserCardTypeIcon = (...args: Parameters<ReturnType<typeof getBackend>['UpdateUserCardTypeIcon']>) => getBackend().UpdateUserCardTypeIcon(...args)
export const UpdateBuiltinCardType = (...args: Parameters<ReturnType<typeof getBackend>['UpdateBuiltinCardType']>) => getBackend().UpdateBuiltinCardType(...args)

// Card templates
export const ListCardTemplates = () => getBackend().ListCardTemplates()
export const CreateCardTemplate = (...args: Parameters<ReturnType<typeof getBackend>['CreateCardTemplate']>) => getBackend().CreateCardTemplate(...args)
export const UpdateCardTemplate = (...args: Parameters<ReturnType<typeof getBackend>['UpdateCardTemplate']>) => getBackend().UpdateCardTemplate(...args)
export const DeleteCardTemplate = (...args: Parameters<ReturnType<typeof getBackend>['DeleteCardTemplate']>) => getBackend().DeleteCardTemplate(...args)

// Index / search
export const SearchCards = (...args: Parameters<ReturnType<typeof getBackend>['SearchCards']>) => getBackend().SearchCards(...args)
export const SearchOrphanedCards = (...args: Parameters<ReturnType<typeof getBackend>['SearchOrphanedCards']>) => getBackend().SearchOrphanedCards(...args)
export const GetCardProjectContext = (...args: Parameters<ReturnType<typeof getBackend>['GetCardProjectContext']>) => getBackend().GetCardProjectContext(...args)
export const RebuildIndex = () => getBackend().RebuildIndex()
export const RefreshIndex = () => getBackend().RefreshIndex()
export const ListCardIDsInCategory = (...args: Parameters<ReturnType<typeof getBackend>['ListCardIDsInCategory']>) => getBackend().ListCardIDsInCategory(...args)
export const ListOrphanedCardIDs = () => getBackend().ListOrphanedCardIDs()
export const ListCardIDsByTag = (...args: Parameters<ReturnType<typeof getBackend>['ListCardIDsByTag']>) => getBackend().ListCardIDsByTag(...args)

// Agent card IDs
export const ListAgentCardIDs = () => getBackend().ListAgentCardIDs()

// Notifications
export const GetNotifyConfig = () => getBackend().GetNotifyConfig()
export const SetNotifyConfig = (...args: Parameters<ReturnType<typeof getBackend>['SetNotifyConfig']>) => getBackend().SetNotifyConfig(...args)
export const GetNotifications = () => getBackend().GetNotifications()
export const MarkNotificationRead = (...args: Parameters<ReturnType<typeof getBackend>['MarkNotificationRead']>) => getBackend().MarkNotificationRead(...args)
export const MarkAllNotificationsRead = () => getBackend().MarkAllNotificationsRead()
export const ClearAllNotifications = () => getBackend().ClearAllNotifications()

// Category details
export const GetCategoryAcceptedTypes = (...args: Parameters<ReturnType<typeof getBackend>['GetCategoryAcceptedTypes']>) => getBackend().GetCategoryAcceptedTypes(...args)

// Schedule preview
export const ValidateSchedulePreview = (...args: Parameters<ReturnType<typeof getBackend>['ValidateSchedulePreview']>) => getBackend().ValidateSchedulePreview(...args)

// Agent
export const GetAgentConfig = (...args: Parameters<ReturnType<typeof getBackend>['GetAgentConfig']>) => getBackend().GetAgentConfig(...args)
export const SaveAgentConfig = (...args: Parameters<ReturnType<typeof getBackend>['SaveAgentConfig']>) => getBackend().SaveAgentConfig(...args)
export const GetAgentRuns = (...args: Parameters<ReturnType<typeof getBackend>['GetAgentRuns']>) => getBackend().GetAgentRuns(...args)
export const TriggerAgent = (...args: Parameters<ReturnType<typeof getBackend>['TriggerAgent']>) => getBackend().TriggerAgent(...args)
export const CancelAgent = (...args: Parameters<ReturnType<typeof getBackend>['CancelAgent']>) => getBackend().CancelAgent(...args)
export const ClearAgentRuns = (...args: Parameters<ReturnType<typeof getBackend>['ClearAgentRuns']>) => getBackend().ClearAgentRuns(...args)
export const PauseAllAgents = () => getBackend().PauseAllAgents()
export const ResumeAllAgents = () => getBackend().ResumeAllAgents()
export const GetAgentSchedulerStatus = () => getBackend().GetAgentSchedulerStatus()
export const GetAllAgents = () => getBackend().GetAllAgents()
export const GetAllAgentRuns = (...args: Parameters<ReturnType<typeof getBackend>['GetAllAgentRuns']>) => getBackend().GetAllAgentRuns(...args)
export const GetAgentAnalytics = () => getBackend().GetAgentAnalytics()
export const ForceQuit = () => getBackend().ForceQuit()

// Token pricing
export const GetTokenPricing = () => getBackend().GetTokenPricing()
export const SaveTokenPricing = (...args: Parameters<ReturnType<typeof getBackend>['SaveTokenPricing']>) => getBackend().SaveTokenPricing(...args)

// Chat
export const LoadChatHistory = (...args: Parameters<ReturnType<typeof getBackend>['LoadChatHistory']>) => getBackend().LoadChatHistory(...args)
export const SendChatMessage = (...args: Parameters<ReturnType<typeof getBackend>['SendChatMessage']>) => getBackend().SendChatMessage(...args)

// Project chat
export const LoadProjectChatHistory = (...args: Parameters<ReturnType<typeof getBackend>['LoadProjectChatHistory']>) => getBackend().LoadProjectChatHistory(...args)
export const SendProjectChatMessage = (...args: Parameters<ReturnType<typeof getBackend>['SendProjectChatMessage']>) => getBackend().SendProjectChatMessage(...args)
export const ClearProjectChatHistory = (...args: Parameters<ReturnType<typeof getBackend>['ClearProjectChatHistory']>) => getBackend().ClearProjectChatHistory(...args)
export const ClearCardChatHistory = (...args: Parameters<ReturnType<typeof getBackend>['ClearCardChatHistory']>) => getBackend().ClearCardChatHistory(...args)

// LLM accounts
export const GetLLMAccounts = () => getBackend().GetLLMAccounts()
export const SaveLLMAccounts = (...args: Parameters<ReturnType<typeof getBackend>['SaveLLMAccounts']>) => getBackend().SaveLLMAccounts(...args)
export const TestLLMAccountConnection = (...args: Parameters<ReturnType<typeof getBackend>['TestLLMAccountConnection']>) => getBackend().TestLLMAccountConnection(...args)

// LLM utilities
export const IsLLMConfigured = () => getBackend().IsLLMConfigured()
export const TestLLMConnection = () => getBackend().TestLLMConnection()
export const TestSystemNotification = () => getBackend().TestSystemNotification()

// Pin suggestions (from AI)
export const AcceptPinSuggestion = (...args: Parameters<ReturnType<typeof getBackend>['AcceptPinSuggestion']>) => getBackend().AcceptPinSuggestion(...args)
export const RejectPinSuggestion = (...args: Parameters<ReturnType<typeof getBackend>['RejectPinSuggestion']>) => getBackend().RejectPinSuggestion(...args)

// Pending edits (Suggest mode)
export const AcceptPendingEdit = (...args: Parameters<ReturnType<typeof getBackend>['AcceptPendingEdit']>) => getBackend().AcceptPendingEdit(...args)
export const RejectPendingEdit = (...args: Parameters<ReturnType<typeof getBackend>['RejectPendingEdit']>) => getBackend().RejectPendingEdit(...args)
export const AcceptAllPendingEdits = (...args: Parameters<ReturnType<typeof getBackend>['AcceptAllPendingEdits']>) => getBackend().AcceptAllPendingEdits(...args)
export const RejectAllPendingEdits = (...args: Parameters<ReturnType<typeof getBackend>['RejectAllPendingEdits']>) => getBackend().RejectAllPendingEdits(...args)
export const ApplyPendingEdits = (...args: Parameters<ReturnType<typeof getBackend>['ApplyPendingEdits']>) => getBackend().ApplyPendingEdits(...args)
export const ApplyProjectPendingEdits = (...args: Parameters<ReturnType<typeof getBackend>['ApplyProjectPendingEdits']>) => getBackend().ApplyProjectPendingEdits(...args)

// Attachments
export const AddCardAttachment = (...args: Parameters<ReturnType<typeof getBackend>['AddCardAttachment']>) => getBackend().AddCardAttachment(...args)
export const RemoveCardAttachment = (...args: Parameters<ReturnType<typeof getBackend>['RemoveCardAttachment']>) => getBackend().RemoveCardAttachment(...args)

// Due-date notifications
export const GetDueDateSettings = () => getBackend().GetDueDateSettings()
export const SaveDueDateSettings = (...args: Parameters<ReturnType<typeof getBackend>['SaveDueDateSettings']>) => getBackend().SaveDueDateSettings(...args)

// User preferences
export const GetPreferences = () => getBackend().GetPreferences()
export const SetPreferences = (...args: Parameters<ReturnType<typeof getBackend>['SetPreferences']>) => getBackend().SetPreferences(...args)

// Activity & recently updated
export const ListActivityLog = (...args: Parameters<ReturnType<typeof getBackend>['ListActivityLog']>) => getBackend().ListActivityLog(...args)
export const ListRecentlyUpdatedCards = (...args: Parameters<ReturnType<typeof getBackend>['ListRecentlyUpdatedCards']>) => getBackend().ListRecentlyUpdatedCards(...args)

// Native dialogs
export const PickFile = (...args: Parameters<ReturnType<typeof getBackend>['PickFile']>) => getBackend().PickFile(...args)
export const PickSaveFile = (...args: Parameters<ReturnType<typeof getBackend>['PickSaveFile']>) => getBackend().PickSaveFile(...args)

// Card comments
export const ListCardComments = (...args: Parameters<ReturnType<typeof getBackend>['ListCardComments']>) => getBackend().ListCardComments(...args)
export const AddCardComment = (...args: Parameters<ReturnType<typeof getBackend>['AddCardComment']>) => getBackend().AddCardComment(...args)
export const UpdateCardComment = (...args: Parameters<ReturnType<typeof getBackend>['UpdateCardComment']>) => getBackend().UpdateCardComment(...args)
export const DeleteCardComment = (...args: Parameters<ReturnType<typeof getBackend>['DeleteCardComment']>) => getBackend().DeleteCardComment(...args)

// Import / Export
export const ImportTrelloBoard = (...args: Parameters<ReturnType<typeof getBackend>['ImportTrelloBoard']>) => getBackend().ImportTrelloBoard(...args)
export const ImportTrelloBoardFromJSON = (...args: Parameters<ReturnType<typeof getBackend>['ImportTrelloBoardFromJSON']>) => getBackend().ImportTrelloBoardFromJSON(...args)
export const ExportProjectToFile = (...args: Parameters<ReturnType<typeof getBackend>['ExportProjectToFile']>) => getBackend().ExportProjectToFile(...args)
