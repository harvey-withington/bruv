package supervisor

// The explicit JSON-RPC surfaces of *Runtime (/repos/<id>/rpc) and
// *MachineService (/server/rpc). The transport's reflection dispatcher
// registers ONLY these names (transporthttp.RPCSurface): every other
// exported method — lifecycle (Close), wiring (WithPush) and internal
// accessors (Bus, Repo, Index, MCPRegistry, ResolveAttachment, …) — is
// unreachable over the network, where any device token could otherwise
// call it.
//
// Derived from what the clients actually call: the BackendAdapter
// interface in shared/types.ts (desktop, via shared/adapters/cloud.ts;
// SERVER_METHODS there route to the machine surface), repoRPC /
// machineRPC calls in mobile/src, and repoRPC calls in clipper/src.
// rpc_surface_test.go pins every exported method of both types as
// either an RPC or internal, so a new exported helper can't silently
// become callable — add a new RPC here AND to the clients' adapter.

// repoRPCMethods is the per-repo RPC surface of *Runtime.
var repoRPCMethods = []string{
	// Agents
	"CancelAgent", "ClearAgentRuns", "DeleteAgent", "DescribeAgent",
	"GetAgentAnalytics", "GetAgentConfig", "GetAgentRuns",
	"GetAgentSchedulerStatus", "GetAllAgentRuns", "GetAllAgents",
	"ListAgentCardStates", "PauseAllAgents", "ResumeAllAgents",
	"SaveAgentConfig", "TriggerAgent", "ValidateSchedulePreview",

	// Board structure: brands, streams, projects, categories
	"CopyBrand", "CopyCategory", "CopyProject", "CopyStream",
	"CreateBrand", "CreateCategory", "CreateProject", "CreateStream",
	"DeleteBrand", "DeleteCategory", "DeleteProject", "DeleteStream",
	"GetBrand", "GetCategoryAcceptedTypes", "GetProjectLocation",
	"GetProjectMembers", "ListAllCategories", "ListBrands", "ListCategories",
	"ListProjectTemplates", "ListProjects", "ListStreams",
	"MoveCategoryCards", "MoveProject", "MoveStream",
	"RenameBrand", "RenameCategory", "RenameProject", "RenameStream",
	"ReorderBrands", "ReorderCategories", "ReorderProjects", "ReorderStreams",
	"UpdateBrandDescription", "UpdateBrandIcon", "UpdateCategoryAcceptedTypes",
	"UpdateCategoryDescription", "UpdateCategoryIcon", "UpdateProjectDescription",
	"UpdateProjectIcon", "UpdateStreamDescription", "UpdateStreamIcon",
	"PromoteCardToProject",

	// Project labels + tags
	"AddProjectLabel", "AssignTagColor", "GetProjectLabels", "GetTagColors",
	"RemoveProjectLabel", "SetProjectLabelIcon", "SetTagColor",
	"UpdateProjectLabel",

	// Cards
	"AcceptPinSuggestion", "CreateCard", "DeleteCard", "DuplicateCard",
	"GetCard", "GetCardLocation", "GetCardPinBreadcrumbs", "GetCardPins",
	"GetCardProjectContext", "ListCardIDsByTag", "ListCardIDsInCategory",
	"ListCards", "ListOrphanedCardIDs", "ListRecentlyUpdatedCards",
	"MoveCardInCategory", "MoveCardToCategory", "PinCard",
	"PopulateCardWithAI", "RecentCards", "RefreshTypeBlocks",
	"RejectPinSuggestion", "SearchCards", "SearchOrphanedCards", "UnpinCard",
	"UpdateCardBlocks", "UpdateCardDescription", "UpdateCardDueDate",
	"UpdateCardLabels", "UpdateCardTags", "UpdateCardTitle", "UpdateCardType",
	"ValidateCardFields",

	// Card comments, attachments, live state, presenting
	"AddCardComment", "DeleteCardComment", "ListCardComments",
	"UpdateCardComment",
	"AddCardAttachment", "OpenCardAttachmentText", "RemoveCardAttachment",
	"SaveCardAttachmentText", "SignAttachmentURL", "StatCardAttachmentText",
	"GetBlockLiveState", "SetBlockLiveState",
	"ListPresentingCards", "SetPresenting", "SignPresentURL",
	"AppendDeckSlide",

	// Card types + templates
	"CreateCardTemplate", "CreateCardTypeFromCard", "CreateUserCardType",
	"DeleteCardTemplate", "DeleteUserCardType", "ExportCardTypesToFile",
	"GetTemplatePrefs", "ImportCardTypesFromFile", "ImportCardTypesFromRepo",
	"ListCardTemplates", "ListCardTypes", "SetTemplatePrefs",
	"UpdateBuiltinCardType", "UpdateCardTemplate", "UpdateUserCardType",
	"UpdateUserCardTypeIcon",

	// Chat + pending edits
	"AcceptAllPendingEdits", "AcceptPendingEdit", "ApplyPendingEdits",
	"ApplyProjectPendingEdits", "ClearCardChatHistory",
	"ClearProjectChatHistory", "GetCardChatModel", "GetProjectChatModel",
	"LoadChatHistory", "LoadProjectChatHistory", "RejectAllPendingEdits",
	"RejectPendingEdit", "SendChatMessage", "SendProjectChatMessage",
	"SetCardChatModel", "SetProjectChatModel", "StopChatMessage",
	"StopProjectChatMessage", "ToggleChatBookmark",
	"ToggleProjectChatBookmark",

	// Capture
	"CaptureFromURL", "CompleteCapture", "GetCapturePrefs", "MatchCaptureURL",
	"PreviewCapture", "RetryCapture", "SetCapturePrefs",

	// MCP servers
	"AddMCPServer", "ApproveMCPServer", "DeleteMCPServer", "GetMCPServerSecretStatus",
	"ListMCPServers", "RestartMCPServer", "SetMCPServerSecret",
	"UpdateMCPServer",

	// Notifications + activity
	"ClearAllNotifications", "DeleteNotification", "ListActivityLog",
	"MarkAllNotificationsRead", "MarkNotificationRead", "TestSystemNotification",

	// Repo, index, import/export
	"ExportProjectToFile", "GetCurrentRepo", "GetRepoDescription",
	"ImportTrelloBoard", "ImportTrelloBoardFromJSON", "RebuildIndex",
	"RefreshIndex", "UpdateRepoDescription",

	// Workspaces
	"AttachWorkspace", "CreateWorkspaceDir", "CreateWorkspaceFile",
	"DeleteWorkspaceTemplate", "DetachWorkspace", "DisableWorkspaceGitServe",
	"EnableWorkspaceGitServe", "GenerateWorkspaceFromTemplate",
	"GenerateWorkspaceTemplate", "GetWorkspaceState",
	"GetWorkspaceTemplateParams", "ImportWorkspaceTemplate",
	"InspectWorkspaceGitServe", "InspectWorkspaceTemplateFolder",
	"ListWorkspaceDir", "ListWorkspaceTemplates", "OpenWorkspaceFile",
	"PreviewWorkspaceTemplate", "ReadWorkspaceFile", "RefreshWorkspaceIndex",
	"ResolveWorkspace", "SaveWorkspaceFile", "SaveWorkspaceTemplate",
	"SetWorkspaceCommitOnSave", "SetWorkspaceLaunchCommand",
	"StatWorkspaceFile",

	// Machine-level settings still answered per-repo for older clients
	// (current clients route these to /server/rpc).
	"GetAuthInfo", "GetDueDateSettings", "GetLLMAccounts", "GetLLMConfig",
	"GetNotifications", "GetNotifyConfig", "GetPreferences", "GetProfile",
	"IsLLMConfigured", "SaveDueDateSettings", "SaveLLMAccounts",
	"SetLLMConfig", "SetNotifyConfig", "SetPreferences", "SetProfile",
}

// machineRPCMethods is the per-machine RPC surface of *MachineService
// (everything but the WithPush wiring hook).
var machineRPCMethods = []string{
	"DiscoverLLMModels", "GetAuthInfo", "GetDueDateSettings",
	"GetLLMAccounts", "GetLLMConfig", "GetLLMRegistry", "GetLLMRouting",
	"GetNotifications", "GetNotifyConfig", "GetPreferences", "GetProfile",
	"GetVapidPublicKey", "IsLLMConfigured", "NewLLMRouter", "PreviewLLMRoute",
	"RegisterPushSubscription", "SaveDueDateSettings", "SaveLLMAccounts",
	"SaveLLMRouting", "SetLLMConfig", "SetNotifyConfig", "SetPreferences",
	"SetProfile", "TestLLMModel", "UnregisterPushSubscription",
}

// RPCMethods implements transporthttp.RPCSurface: the only *Runtime
// methods reachable over /repos/<id>/rpc.
func (r *Runtime) RPCMethods() []string { return repoRPCMethods }

// RPCMethods implements transporthttp.RPCSurface: the only
// *MachineService methods reachable over /server/rpc.
func (m *MachineService) RPCMethods() []string { return machineRPCMethods }
