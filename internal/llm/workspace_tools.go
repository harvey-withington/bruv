package llm

// Workspace tool definitions (read-only — AI write access to workspace files
// is out of scope by spec). Offered to project chat, gated by the session's
// context level: `all` gets everything, `metadata` gets structure/status but
// NOT file contents, `none` gets nothing. Generation from templates is
// deliberately not a tool — it is always user-confirmed in the UI.

// WorkspaceTools returns the workspace tool definitions. includeFileRead
// controls whether workspace_read_file is offered (context level `all` only).
func WorkspaceTools(includeFileRead bool) []ToolDef {
	tools := []ToolDef{
		{
			Name:        "workspace_index",
			Description: "Read the project workspace's indexed file tree and adapter summary (file names, sizes, git branch/commits or vault stats). CALL THIS first when the user asks about the workspace, its files, or its structure.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "workspace_status",
			Description: "Get the project workspace's status: origin (local folder / git / rclone), adapter, whether the files are on this machine, and when the index was last refreshed.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "template_list",
			Description: "List the folder templates available in this vault (global and brand-scoped). Templates generate new workspace folders; generation itself is done by the user through the UI.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "template_params",
			Description: "Get a folder template's parameter list (names, prompts, defaults) so you can suggest values for the user to confirm in the creation form.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"template_id": map[string]any{
						"type":        "string",
						"description": "The template's id from template_list",
					},
				},
				"required": []string{"template_id"},
			},
		},
	}
	if includeFileRead {
		tools = append(tools, ToolDef{
			Name:        "workspace_read_file",
			Description: "Read one text file from the project workspace by its relative path (as listed by workspace_index). Returns the file content (truncated if large). Only text files — binaries are refused.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path within the workspace, e.g. \"chapters/01.md\"",
					},
				},
				"required": []string{"path"},
			},
		})
	}
	return tools
}
