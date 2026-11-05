package llm

// The workspace tools (core/boardtools, read-only) by what a chat must
// have to be offered them. Project chat offers the project ones only when
// a workspace is attached, by its context level: `all` gets everything,
// `metadata` no file contents, `none` nothing. Card chat offers only
// CardFileTool, and only when the card names files.
var ProjectWorkspaceTools = []string{"workspace_index", "workspace_status", "workspace_read_file", "template_list", "template_params"}

const (
	// WorkspaceFileReadTool reads a project workspace file's content.
	WorkspaceFileReadTool = "workspace_read_file"
	// CardFileTool reads the files a card's Workspace Files block names.
	CardFileTool = "read_card_file"
)
